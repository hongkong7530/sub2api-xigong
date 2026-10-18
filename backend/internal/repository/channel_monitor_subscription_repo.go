package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// channelMonitorSubscriptionRepository 分组监控订阅与通知状态机仓储。
// 全部走原生 SQL：两张表由迁移 237 管理，不进 ent schema；
// 状态转移用 SELECT ... FOR UPDATE 行锁事务，保证双节点并发下只有一次转移成功。
type channelMonitorSubscriptionRepository struct {
	db *sql.DB
}

// NewChannelMonitorSubscriptionRepository 创建仓储实例。
func NewChannelMonitorSubscriptionRepository(db *sql.DB) service.ChannelMonitorSubscriptionRepository {
	return &channelMonitorSubscriptionRepository{db: db}
}

func (r *channelMonitorSubscriptionRepository) ListByUser(ctx context.Context, userID int64) ([]*service.ChannelMonitorSubscription, error) {
	return r.list(ctx, `SELECT user_id, group_id, notify_first_token, notify_rate_change, updated_at
FROM channel_monitor_subscriptions WHERE user_id = $1 ORDER BY group_id`, userID)
}

func (r *channelMonitorSubscriptionRepository) ListByGroup(ctx context.Context, groupID int64) ([]*service.ChannelMonitorSubscription, error) {
	return r.list(ctx, `SELECT user_id, group_id, notify_first_token, notify_rate_change, updated_at
FROM channel_monitor_subscriptions WHERE group_id = $1 ORDER BY user_id`, groupID)
}

func (r *channelMonitorSubscriptionRepository) list(ctx context.Context, q string, arg int64) ([]*service.ChannelMonitorSubscription, error) {
	rows, err := r.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("list monitor subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*service.ChannelMonitorSubscription, 0)
	for rows.Next() {
		sub := &service.ChannelMonitorSubscription{}
		if err := rows.Scan(&sub.UserID, &sub.GroupID, &sub.NotifyFirstToken, &sub.NotifyRateChange, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan monitor subscription: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (r *channelMonitorSubscriptionRepository) Upsert(ctx context.Context, sub *service.ChannelMonitorSubscription) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO channel_monitor_subscriptions (user_id, group_id, notify_first_token, notify_rate_change, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())
ON CONFLICT (user_id, group_id) DO UPDATE
   SET notify_first_token = EXCLUDED.notify_first_token,
       notify_rate_change = EXCLUDED.notify_rate_change,
       updated_at = NOW()`, sub.UserID, sub.GroupID, sub.NotifyFirstToken, sub.NotifyRateChange)
	if err != nil {
		return fmt.Errorf("upsert monitor subscription: %w", err)
	}
	return nil
}

func (r *channelMonitorSubscriptionRepository) Delete(ctx context.Context, userID, groupID int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM channel_monitor_subscriptions WHERE user_id = $1 AND group_id = $2`, userID, groupID); err != nil {
		return fmt.Errorf("delete monitor subscription: %w", err)
	}
	return nil
}

// ApplyFirstTokenObservation 见接口注释。状态机：
//
//	red    -> state=pending_recovery, green=0
//	yellow -> green=0（state 不变）
//	green  -> state==pending 时 green+1；达到 streak 则 state=ok, green=0, 返回 true
func (r *channelMonitorSubscriptionRepository) ApplyFirstTokenObservation(ctx context.Context, groupID int64, status string, firstTokenMs *int, streak int) (bool, error) {
	if streak <= 0 {
		streak = 1
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin notify state tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_monitor_group_notify_state (group_id) VALUES ($1)
ON CONFLICT (group_id) DO NOTHING`, groupID); err != nil {
		return false, fmt.Errorf("ensure notify state: %w", err)
	}
	var state string
	var green int
	if err := tx.QueryRowContext(ctx, `
SELECT first_token_state, consecutive_green FROM channel_monitor_group_notify_state
WHERE group_id = $1 FOR UPDATE`, groupID).Scan(&state, &green); err != nil {
		return false, fmt.Errorf("lock notify state: %w", err)
	}

	recovered := false
	switch status {
	case service.MonitorStatusOperational:
		if state == service.MonitorNotifyStatePendingRecovery {
			green++
			if green >= streak {
				state = service.MonitorNotifyStateOK
				green = 0
				recovered = true
			}
		} else {
			green = 0
		}
	case service.MonitorStatusDegraded:
		green = 0
	default:
		state = service.MonitorNotifyStatePendingRecovery
		green = 0
	}

	var ms sql.NullInt64
	if firstTokenMs != nil {
		ms = sql.NullInt64{Int64: int64(*firstTokenMs), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE channel_monitor_group_notify_state
   SET first_token_state = $2, consecutive_green = $3, last_first_token_ms = $4, updated_at = NOW()
 WHERE group_id = $1`, groupID, state, green, ms); err != nil {
		return false, fmt.Errorf("update notify state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit notify state: %w", err)
	}
	return recovered, nil
}

// MarkEffectiveRate 见接口注释。倍率比较到 1e-6，避免浮点噪声触发通知。
func (r *channelMonitorSubscriptionRepository) MarkEffectiveRate(ctx context.Context, groupID int64, rate float64) (*float64, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin rate state tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_monitor_group_notify_state (group_id) VALUES ($1)
ON CONFLICT (group_id) DO NOTHING`, groupID); err != nil {
		return nil, false, fmt.Errorf("ensure rate state: %w", err)
	}
	var prev sql.NullFloat64
	if err := tx.QueryRowContext(ctx, `
SELECT last_effective_rate FROM channel_monitor_group_notify_state WHERE group_id = $1 FOR UPDATE`, groupID).Scan(&prev); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lock rate state: %w", err)
	}
	if prev.Valid && math.Abs(prev.Float64-rate) < 1e-6 {
		return &prev.Float64, false, nil
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE channel_monitor_group_notify_state SET last_effective_rate = $2, updated_at = NOW() WHERE group_id = $1`, groupID, rate); err != nil {
		return nil, false, fmt.Errorf("update rate state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit rate state: %w", err)
	}
	if !prev.Valid {
		// 首次记录只建立基线，不视为变化。
		return nil, false, nil
	}
	previous := prev.Float64
	return &previous, true, nil
}
