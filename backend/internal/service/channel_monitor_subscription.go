package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 分组监控邮件订阅：用户按分组订阅「首字恢复」与「倍率变化」通知。
//
// 双节点防重复发信：状态转移全部落在共享主库 channel_monitor_group_notify_state
// 的行锁事务内（见 repository），只有完成 pending_recovery -> ok 转移或
// last_effective_rate 实际变更的那一次调用会拿到「需要发信」的返回值。

// monitorFirstTokenRecoveryStreak 首字变红后需要连续多少次绿首字才判定恢复。
const monitorFirstTokenRecoveryStreak = 3

// 分组通知状态机取值。
const (
	MonitorNotifyStateOK              = "ok"
	MonitorNotifyStatePendingRecovery = "pending_recovery"
)

// ChannelMonitorSubscription 用户对某分组的订阅开关。
type ChannelMonitorSubscription struct {
	UserID           int64
	GroupID          int64
	NotifyFirstToken bool
	NotifyRateChange bool
	UpdatedAt        time.Time
}

// ChannelMonitorSubscriptionRepository 订阅与分组通知状态的数据访问接口。
type ChannelMonitorSubscriptionRepository interface {
	ListByUser(ctx context.Context, userID int64) ([]*ChannelMonitorSubscription, error)
	ListByGroup(ctx context.Context, groupID int64) ([]*ChannelMonitorSubscription, error)
	Upsert(ctx context.Context, sub *ChannelMonitorSubscription) error
	Delete(ctx context.Context, userID, groupID int64) error

	// ApplyFirstTokenObservation 以行锁事务推进分组首字状态机：
	// red -> pending_recovery 且清零；green 在 pending 下 +1，达到 streak 复位为 ok 并返回 recovered=true；
	// yellow/red 清零。同一恢复事件只有一次调用返回 true。
	ApplyFirstTokenObservation(ctx context.Context, groupID int64, status string, firstTokenMs *int, streak int) (recovered bool, err error)
	// MarkEffectiveRate 以行锁事务记录分组当前有效倍率；首次记录返回 (nil,false)，
	// 与上次相同返回 (prev,false)，变化时更新并返回 (prev,true)。
	MarkEffectiveRate(ctx context.Context, groupID int64, rate float64) (previous *float64, changed bool, err error)
}

var (
	ErrChannelMonitorSubscriptionEmailRequired = infraerrors.BadRequest(
		"CHANNEL_MONITOR_SUBSCRIPTION_EMAIL_REQUIRED", "bind an email address before subscribing to monitor notifications",
	)
	ErrChannelMonitorSubscriptionGroupInvalid = infraerrors.BadRequest(
		"CHANNEL_MONITOR_SUBSCRIPTION_GROUP_INVALID", "group_id must be a positive integer",
	)
)

// userEmailReader 读取订阅者邮箱（复用 UserRepository.GetByID）。
type userEmailReader interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

// channelMonitorNotifier 探测结果落库后的通知钩子（由 ChannelMonitorNotifyService 实现）。
type channelMonitorNotifier interface {
	OnCheckResults(ctx context.Context, m *ChannelMonitor, results []*CheckResult)
}

// groupRateReader 读取分组当前有效倍率（ChannelMonitorService.GroupMetrics）。
type groupRateReader interface {
	GroupMetrics(ctx context.Context, groupID int64, window time.Duration) (*ChannelMonitorGroupMetrics, error)
}

// ---------- ChannelMonitorService 订阅管理 ----------

// SetSubscriptionRepository 注入订阅仓储与用户读取器（handler 侧订阅管理用）。
func (s *ChannelMonitorService) SetSubscriptionRepository(repo ChannelMonitorSubscriptionRepository, users userEmailReader) {
	if s != nil {
		s.subscriptions = repo
		s.users = users
	}
}

// SetNotifier 注入探测结果通知钩子。
func (s *ChannelMonitorService) SetNotifier(n channelMonitorNotifier) {
	if s != nil {
		s.notifier = n
	}
}

// ListSubscriptions 返回用户全部分组订阅。
func (s *ChannelMonitorService) ListSubscriptions(ctx context.Context, userID int64) ([]*ChannelMonitorSubscription, error) {
	if s == nil || s.subscriptions == nil {
		return []*ChannelMonitorSubscription{}, nil
	}
	return s.subscriptions.ListByUser(ctx, userID)
}

// SetSubscription 设置分组订阅开关；两开关全关等价于退订（删除行）。
// 订阅时要求用户已绑定邮箱，否则通知无处投递。
func (s *ChannelMonitorService) SetSubscription(ctx context.Context, userID, groupID int64, notifyFirstToken, notifyRateChange bool) (*ChannelMonitorSubscription, error) {
	if s == nil || s.subscriptions == nil {
		return nil, ErrChannelMonitorNotFound
	}
	if groupID <= 0 {
		return nil, ErrChannelMonitorSubscriptionGroupInvalid
	}
	sub := &ChannelMonitorSubscription{
		UserID:           userID,
		GroupID:          groupID,
		NotifyFirstToken: notifyFirstToken,
		NotifyRateChange: notifyRateChange,
	}
	if !notifyFirstToken && !notifyRateChange {
		if err := s.subscriptions.Delete(ctx, userID, groupID); err != nil {
			return nil, err
		}
		return sub, nil
	}
	if s.users != nil {
		user, err := s.users.GetByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if user == nil || strings.TrimSpace(user.Email) == "" {
			return nil, ErrChannelMonitorSubscriptionEmailRequired
		}
	}
	if err := s.subscriptions.Upsert(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// ---------- 通知服务 ----------

// ChannelMonitorNotifyService 消费探测结果，驱动首字恢复状态机与倍率变化侦测并发送邮件。
type ChannelMonitorNotifyService struct {
	subs   ChannelMonitorSubscriptionRepository
	users  userEmailReader
	emails *NotificationEmailService
	rates  groupRateReader
}

// NewChannelMonitorNotifyService 创建通知服务；任一依赖为 nil 时对应能力静默关闭。
func NewChannelMonitorNotifyService(
	subs ChannelMonitorSubscriptionRepository,
	users userEmailReader,
	emails *NotificationEmailService,
	rates groupRateReader,
) *ChannelMonitorNotifyService {
	return &ChannelMonitorNotifyService{subs: subs, users: users, emails: emails, rates: rates}
}

// OnCheckResults 在一轮探测结果落库后调用（持 per-monitor leader-lock 的节点内）。
// 分组级首字状态按用户要求的宽松口径取全部模型最优状态：一绿即绿、双黄为黄、双红为红。
func (n *ChannelMonitorNotifyService) OnCheckResults(ctx context.Context, m *ChannelMonitor, results []*CheckResult) {
	if n == nil || n.subs == nil || m == nil || m.GroupID == nil || len(results) == 0 {
		return
	}
	groupID := *m.GroupID
	status, firstTokenMs := bestCheckResultStatus(results)
	if status != "" {
		recovered, err := n.subs.ApplyFirstTokenObservation(ctx, groupID, status, firstTokenMs, monitorFirstTokenRecoveryStreak)
		if err != nil {
			slog.Warn("channel_monitor: apply first token observation failed", "group_id", groupID, "error", err)
		} else if recovered {
			n.sendFirstTokenRecovered(ctx, groupID, m.GroupName, firstTokenMs)
		}
	}
	n.checkRateChange(ctx, groupID, m.GroupName)
}

// bestCheckResultStatus 返回本轮全部模型的最优状态与对应首字延迟（绿优先取绿的首字）。
func bestCheckResultStatus(results []*CheckResult) (string, *int) {
	best := ""
	var bestMs *int
	rank := func(s string) int {
		switch s {
		case MonitorStatusOperational:
			return 3
		case MonitorStatusDegraded:
			return 2
		case MonitorStatusError, MonitorStatusFailed:
			return 1
		default:
			return 0
		}
	}
	for _, r := range results {
		if r == nil || rank(r.Status) == 0 {
			continue
		}
		if rank(r.Status) > rank(best) {
			best = r.Status
			bestMs = r.FirstTokenMs
		}
	}
	return best, bestMs
}

func (n *ChannelMonitorNotifyService) checkRateChange(ctx context.Context, groupID int64, groupName string) {
	if n.rates == nil {
		return
	}
	metrics, err := n.rates.GroupMetrics(ctx, groupID, 24*time.Hour)
	if err != nil || metrics == nil {
		return
	}
	previous, changed, err := n.subs.MarkEffectiveRate(ctx, groupID, metrics.EffectiveRateMultiplier)
	if err != nil {
		slog.Warn("channel_monitor: mark effective rate failed", "group_id", groupID, "error", err)
		return
	}
	if !changed || previous == nil {
		return
	}
	n.sendRateChanged(ctx, groupID, groupName, *previous, metrics.EffectiveRateMultiplier)
}

func (n *ChannelMonitorNotifyService) sendFirstTokenRecovered(ctx context.Context, groupID int64, groupName string, firstTokenMs *int) {
	now := time.Now()
	variables := map[string]string{
		"group_name":     groupName,
		"first_token_ms": formatOptionalMs(firstTokenMs),
		"recovered_at":   now.Format("2006-01-02 15:04:05"),
		"effective_rate": "-",
		"peak_rate":      "-",
	}
	if n.rates != nil {
		if metrics, err := n.rates.GroupMetrics(ctx, groupID, 24*time.Hour); err == nil && metrics != nil {
			variables["effective_rate"] = strconv.FormatFloat(metrics.EffectiveRateMultiplier, 'f', 4, 64)
			if metrics.PeakRateEnabled {
				variables["peak_rate"] = strconv.FormatFloat(metrics.PeakRateMultiplier, 'f', 4, 64)
			}
		}
	}
	n.broadcast(ctx, groupID, func(sub *ChannelMonitorSubscription) bool { return sub.NotifyFirstToken },
		NotificationEmailEventMonitorFirstTokenRecovered, "channel_monitor_first_token",
		strconv.FormatInt(now.Unix(), 10), variables)
}

func (n *ChannelMonitorNotifyService) sendRateChanged(ctx context.Context, groupID int64, groupName string, previous, current float64) {
	now := time.Now()
	variables := map[string]string{
		"group_name":    groupName,
		"previous_rate": strconv.FormatFloat(previous, 'f', 4, 64),
		"current_rate":  strconv.FormatFloat(current, 'f', 4, 64),
		"changed_at":    now.Format("2006-01-02 15:04:05"),
	}
	n.broadcast(ctx, groupID, func(sub *ChannelMonitorSubscription) bool { return sub.NotifyRateChange },
		NotificationEmailEventMonitorGroupRateChanged, "channel_monitor_rate",
		strconv.FormatInt(now.Unix(), 10), variables)
}

// broadcast 向满足 filter 的分组订阅者逐个发信；单个失败仅日志，不影响其他订阅者。
func (n *ChannelMonitorNotifyService) broadcast(
	ctx context.Context,
	groupID int64,
	filter func(*ChannelMonitorSubscription) bool,
	event, sourceType, reminderKey string,
	variables map[string]string,
) {
	if n.emails == nil || n.users == nil {
		return
	}
	subs, err := n.subs.ListByGroup(ctx, groupID)
	if err != nil {
		slog.Warn("channel_monitor: list group subscriptions failed", "group_id", groupID, "error", err)
		return
	}
	for _, sub := range subs {
		if sub == nil || !filter(sub) {
			continue
		}
		user, err := n.users.GetByID(ctx, sub.UserID)
		if err != nil || user == nil || strings.TrimSpace(user.Email) == "" {
			continue
		}
		vars := make(map[string]string, len(variables))
		for k, v := range variables {
			vars[k] = v
		}
		sendCtx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		err = n.emails.Send(sendCtx, NotificationEmailSendInput{
			Event:          event,
			RecipientEmail: user.Email,
			RecipientName:  emailRecipientName(firstNonEmpty(user.Username, user.Email)),
			UserID:         user.ID,
			SourceType:     sourceType,
			SourceID:       strconv.FormatInt(groupID, 10),
			ReminderKey:    reminderKey,
			Variables:      vars,
		})
		cancel()
		if err != nil {
			slog.Warn("channel_monitor: notification email failed", "event", event, "group_id", groupID, "user_id", user.ID, "error", err)
		}
	}
}

func formatOptionalMs(ms *int) string {
	if ms == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *ms)
}
