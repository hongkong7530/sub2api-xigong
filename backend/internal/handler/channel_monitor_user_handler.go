package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ChannelMonitorUserHandler 渠道监控用户只读 handler。
type ChannelMonitorUserHandler struct {
	monitorService *service.ChannelMonitorService
	settingService *service.SettingService
	publicCache    publicMonitorCache
}

// NewChannelMonitorUserHandler 创建 handler。
// settingService 用于每次请求前读取功能开关；关闭时 List/GetStatus 直接返回空/404。
func NewChannelMonitorUserHandler(
	monitorService *service.ChannelMonitorService,
	settingService *service.SettingService,
) *ChannelMonitorUserHandler {
	return &ChannelMonitorUserHandler{
		monitorService: monitorService,
		settingService: settingService,
	}
}

// featureEnabled 返回当前渠道监控功能是否开启。
// settingService 为 nil（测试场景）视为启用。
func (h *ChannelMonitorUserHandler) featureEnabled(c *gin.Context) bool {
	if h.settingService == nil {
		return true
	}
	runtime := h.settingService.GetChannelMonitorRuntime(c.Request.Context())
	return runtime.Enabled && runtime.Mode == service.ChannelMonitorModeV1
}

// quotaVisible 返回用户端是否展示配额/余额快照（channel_monitor_show_quota，
// fail-closed：未配置/非 "true" 一律视为关闭）。settingService 为 nil 时 fail-closed。
func (h *ChannelMonitorUserHandler) quotaVisible(c *gin.Context) bool {
	if h.settingService == nil {
		return false
	}
	return h.settingService.GetChannelMonitorRuntime(c.Request.Context()).ShowQuota
}

// --- Response ---

type channelMonitorUserListItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	GroupName string `json:"group_name"`
	// GroupID 绑定分组 ID；订阅/反馈入口以它为键，未绑定为 null。
	GroupID          *int64     `json:"group_id"`
	PrimaryModel     string     `json:"primary_model"`
	PrimaryStatus    string     `json:"primary_status"`
	StatusSource     string     `json:"status_source"`
	StatusObservedAt *time.Time `json:"status_observed_at"`
	// OverallStatus 全部被监控模型的最优状态（一绿即绿）；整卡颜色用。
	OverallStatus    string `json:"overall_status"`
	PrimaryLatencyMs *int   `json:"primary_latency_ms"`
	// PrimaryFirstTokenMs 主模型最近一次首字延迟（TTFT）毫秒。
	PrimaryFirstTokenMs          *int                                 `json:"primary_first_token_ms"`
	PrimaryOutputTokensPerSecond *float64                             `json:"primary_output_tokens_per_second"`
	PrimaryPingLatencyMs         *int                                 `json:"primary_ping_latency_ms"`
	Availability7d               *float64                             `json:"availability_7d"`
	AvailabilitySamples7d        int                                  `json:"availability_samples_7d"`
	ExtraModels                  []dto.ChannelMonitorExtraModelStatus `json:"extra_models"`
	Timeline                     []channelMonitorUserTimelinePoint    `json:"timeline"`
	// LatestQuota 主模型最近配额快照；channel_monitor_show_quota=false 时
	// 由 userMonitorViewToItem 的调用方传入 false 剥离（服务端脱敏，非仅前端隐藏）。
	LatestQuota *domain.MonitorQuotaSnapshot `json:"latest_quota,omitempty"`
	// GroupInsight 分组实时指标；未绑定分组为 null。
	GroupInsight *channelMonitorGroupInsightPayload `json:"group_insight,omitempty"`
}

// channelMonitorGroupInsightPayload 用户端分组实时指标（倍率/缓存率/首字样本）。
type channelMonitorGroupInsightPayload struct {
	RateMultiplier          float64                          `json:"rate_multiplier"`
	EffectiveRateMultiplier float64                          `json:"effective_rate_multiplier"`
	PeakRateEnabled         bool                             `json:"peak_rate_enabled"`
	PeakRateMultiplier      float64                          `json:"peak_rate_multiplier"`
	AppliedPeakMultiplier   float64                          `json:"applied_peak_multiplier"`
	CacheHitRate            *float64                         `json:"cache_hit_rate"`
	CacheSampleCount        int64                            `json:"cache_sample_count"`
	CacheWindowSeconds      int64                            `json:"cache_window_seconds"`
	RateHistory             []channelMonitorGroupRatePoint   `json:"rate_history,omitempty"`
	RecentFirstTokens       []channelMonitorPublicFirstToken `json:"recent_first_tokens"`
}

// 公开样本不含用户、账号、Key 与用量记录标识。
type channelMonitorPublicFirstToken struct {
	Model        string `json:"model"`
	FirstTokenMs *int   `json:"first_token_ms"`
	CreatedAt    string `json:"created_at"`
}

type channelMonitorGroupRatePoint struct {
	IsInitial               bool    `json:"is_initial"`
	RateMultiplier          float64 `json:"rate_multiplier"`
	EffectiveRateMultiplier float64 `json:"effective_rate_multiplier"`
	RecordedAt              string  `json:"recorded_at"`
}

func observedPercent(value float64, samples int64) *float64 {
	if samples <= 0 {
		return nil
	}
	return &value
}

func groupInsightToPayload(v *service.UserGroupInsight) *channelMonitorGroupInsightPayload {
	if v == nil {
		return nil
	}
	out := &channelMonitorGroupInsightPayload{
		RateMultiplier:          v.RateMultiplier,
		EffectiveRateMultiplier: v.EffectiveRateMultiplier,
		PeakRateEnabled:         v.PeakRateEnabled,
		PeakRateMultiplier:      v.PeakRateMultiplier,
		AppliedPeakMultiplier:   v.AppliedPeakMultiplier,
		CacheHitRate:            observedPercent(v.CacheHitRate, v.CacheInputTokens),
		CacheSampleCount:        v.CacheSampleCount,
		CacheWindowSeconds:      v.CacheWindowSeconds,
	}
	out.RecentFirstTokens = make([]channelMonitorPublicFirstToken, 0, len(v.RecentFirstTokens))
	for _, sample := range v.RecentFirstTokens {
		if sample == nil {
			continue
		}
		model := sample.RequestedModel
		if model == "" {
			model = sample.Model
		}
		out.RecentFirstTokens = append(out.RecentFirstTokens, channelMonitorPublicFirstToken{
			Model: model, FirstTokenMs: sample.FirstTokenMs, CreatedAt: sample.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	for _, p := range v.RateHistory {
		out.RateHistory = append(out.RateHistory, channelMonitorGroupRatePoint{
			IsInitial:               p.IsInitial,
			RateMultiplier:          p.RateMultiplier,
			EffectiveRateMultiplier: p.EffectiveRateMultiplier,
			RecordedAt:              p.RecordedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

// channelMonitorUserTimelinePoint 主模型最近一次检测的 timeline 点。
// 仅用于用户视图 list 响应，admin 视图不使用。
type channelMonitorUserTimelinePoint struct {
	OutputTokensPerSecond *float64 `json:"output_tokens_per_second"`
	Model                 string   `json:"model"`
	Source                string   `json:"source"`
	Status                string   `json:"status"`
	LatencyMs             *int     `json:"latency_ms"`
	FirstTokenMs          *int     `json:"first_token_ms"`
	PingLatencyMs         *int     `json:"ping_latency_ms"`
	CheckedAt             string   `json:"checked_at"`
}

type channelMonitorUserDetailResponse struct {
	ID        int64                         `json:"id"`
	Name      string                        `json:"name"`
	Provider  string                        `json:"provider"`
	GroupName string                        `json:"group_name"`
	Models    []channelMonitorUserModelStat `json:"models"`
	// GroupInsight 分组实时指标 + 倍率变更历史（画曲线用）。
	GroupInsight      *channelMonitorGroupInsightPayload `json:"group_insight,omitempty"`
	GroupAvailability map[int]*float64                   `json:"group_availability,omitempty"`
}

type channelMonitorUserModelStat struct {
	Model           string `json:"model"`
	LatestStatus    string `json:"latest_status"`
	LatestLatencyMs *int   `json:"latest_latency_ms"`
	// LatestFirstTokenMs 最近一次首字延迟（TTFT）毫秒。
	LatestFirstTokenMs          *int     `json:"latest_first_token_ms"`
	LatestOutputTokensPerSecond *float64 `json:"latest_output_tokens_per_second"`
	Availability7d              *float64 `json:"availability_7d"`
	Availability15d             *float64 `json:"availability_15d"`
	Availability30d             *float64 `json:"availability_30d"`
	AvgLatency7dMs              *int     `json:"avg_latency_7d_ms"`
}

func userMonitorViewToItem(v *service.UserMonitorView, includeQuota bool) channelMonitorUserListItem {
	extras := make([]dto.ChannelMonitorExtraModelStatus, 0, len(v.ExtraModels))
	for _, e := range v.ExtraModels {
		extras = append(extras, dto.ChannelMonitorExtraModelStatus{
			Model:        e.Model,
			Status:       e.Status,
			LatencyMs:    e.LatencyMs,
			FirstTokenMs: e.FirstTokenMs,
		})
	}
	timeline := make([]channelMonitorUserTimelinePoint, 0, len(v.Timeline))
	for _, p := range v.Timeline {
		timeline = append(timeline, channelMonitorUserTimelinePoint{
			Model: p.Model, Source: p.Source,
			Status:                p.Status,
			LatencyMs:             p.LatencyMs,
			FirstTokenMs:          p.FirstTokenMs,
			OutputTokensPerSecond: p.OutputTokensPerSecond,
			CheckedAt:             p.CheckedAt.UTC().Format(time.RFC3339),
		})
	}
	item := channelMonitorUserListItem{
		ID:                           v.ID,
		Name:                         v.Name,
		Provider:                     v.Provider,
		GroupName:                    v.GroupName,
		PrimaryModel:                 v.PrimaryModel,
		PrimaryStatus:                v.PrimaryStatus,
		StatusSource:                 v.StatusSource,
		StatusObservedAt:             v.StatusObservedAt,
		PrimaryLatencyMs:             v.PrimaryLatencyMs,
		PrimaryOutputTokensPerSecond: v.PrimaryOutputTokensPerSecond,
		Availability7d:               observedPercent(v.Availability7d, int64(v.AvailabilitySamples7d)),
		AvailabilitySamples7d:        v.AvailabilitySamples7d,
		ExtraModels:                  extras,
		Timeline:                     timeline,
	}
	if includeQuota {
		item.LatestQuota = v.LatestQuota
	}
	item.GroupID = v.GroupID
	item.OverallStatus = v.OverallStatus
	item.PrimaryFirstTokenMs = v.PrimaryFirstTokenMs
	item.GroupInsight = groupInsightToPayload(v.GroupInsight)
	return item
}

func userMonitorDetailToResponse(d *service.UserMonitorDetail) *channelMonitorUserDetailResponse {
	models := make([]channelMonitorUserModelStat, 0, len(d.Models))
	for _, m := range d.Models {
		models = append(models, channelMonitorUserModelStat{
			Model:                       m.Model,
			LatestStatus:                m.LatestStatus,
			LatestLatencyMs:             m.LatestLatencyMs,
			LatestFirstTokenMs:          m.LatestFirstTokenMs,
			LatestOutputTokensPerSecond: m.LatestOutputTokensPerSecond,
			Availability7d:              observedPercent(m.Availability7d, int64(m.Samples7d)),
			Availability15d:             observedPercent(m.Availability15d, int64(m.Samples15d)),
			Availability30d:             observedPercent(m.Availability30d, int64(m.Samples30d)),
			AvgLatency7dMs:              m.AvgLatency7dMs,
		})
	}
	availability := make(map[int]*float64, len(d.GroupAvailability))
	for days, a := range d.GroupAvailability {
		availability[days] = observedPercent(a.AvailabilityPct, int64(a.TotalChecks))
	}
	return &channelMonitorUserDetailResponse{
		GroupAvailability: availability,
		ID:                d.ID,
		Name:              d.Name,
		Provider:          d.Provider,
		GroupName:         d.GroupName,
		Models:            models,
		GroupInsight:      groupInsightToPayload(d.GroupInsight),
	}
}

// --- Handlers ---

// cacheRateWindowFromQuery accepts only the UI's supported cache-rate windows.
func cacheRateWindowFromQuery(raw string) (time.Duration, bool) {
	if raw == "" {
		return service.DefaultCacheRateWindow, true
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	switch seconds {
	case 1800:
		return 30 * time.Minute, true
	case 3600:
		return time.Hour, true
	case 21600:
		return 6 * time.Hour, true
	case 86400:
		return 24 * time.Hour, true
	default:
		return 0, false
	}
}

// List GET /api/v1/channel-monitors
func (h *ChannelMonitorUserHandler) List(c *gin.Context) {
	cacheWindow, ok := cacheRateWindowFromQuery(c.Query("cache_window_seconds"))
	if !ok {
		response.BadRequest(c, "Invalid cache rate window")
		return
	}
	if !h.featureEnabled(c) {
		response.Success(c, gin.H{"items": []channelMonitorUserListItem{}})
		return
	}
	views, err := h.monitorService.ListUserViewWithCacheRateWindow(c.Request.Context(), cacheWindow)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	includeQuota := h.quotaVisible(c)
	items := make([]channelMonitorUserListItem, 0, len(views))
	for _, v := range views {
		items = append(items, userMonitorViewToItem(v, includeQuota))
	}
	response.Success(c, gin.H{"items": items})
}

type channelMonitorSubscriptionItem struct {
	GroupID          int64 `json:"group_id"`
	NotifyFirstToken bool  `json:"notify_first_token"`
	NotifyRateChange bool  `json:"notify_rate_change"`
}

type channelMonitorSubscriptionRequest struct {
	NotifyFirstToken bool `json:"notify_first_token"`
	NotifyRateChange bool `json:"notify_rate_change"`
}

func subscriptionToItem(sub *service.ChannelMonitorSubscription) channelMonitorSubscriptionItem {
	if sub == nil {
		return channelMonitorSubscriptionItem{}
	}
	return channelMonitorSubscriptionItem{
		GroupID:          sub.GroupID,
		NotifyFirstToken: sub.NotifyFirstToken,
		NotifyRateChange: sub.NotifyRateChange,
	}
}

// ListSubscriptions GET /api/v1/channel-monitor-subscriptions
func (h *ChannelMonitorUserHandler) ListSubscriptions(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	subs, err := h.monitorService.ListSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]channelMonitorSubscriptionItem, 0, len(subs))
	for _, sub := range subs {
		items = append(items, subscriptionToItem(sub))
	}
	response.Success(c, gin.H{"items": items})
}

// SetSubscription PUT /api/v1/channel-monitor-subscriptions/:groupId
// 两开关全关等价于退订。
func (h *ChannelMonitorUserHandler) SetSubscription(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	groupID, err := strconv.ParseInt(c.Param("groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, service.ErrChannelMonitorSubscriptionGroupInvalid)
		return
	}
	var req channelMonitorSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	sub, err := h.monitorService.SetSubscription(c.Request.Context(), subject.UserID, groupID, req.NotifyFirstToken, req.NotifyRateChange)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, subscriptionToItem(sub))
}

// GetStatus GET /api/v1/channel-monitors/:id/status
func (h *ChannelMonitorUserHandler) GetStatus(c *gin.Context) {
	if !h.featureEnabled(c) {
		response.ErrorFrom(c, service.ErrChannelMonitorNotFound)
		return
	}
	// 复用 admin.ParseChannelMonitorID 保持错误码与日志一致。
	id, ok := admin.ParseChannelMonitorID(c)
	if !ok {
		return
	}
	detail, err := h.monitorService.GetUserDetail(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, userMonitorDetailToResponse(detail))
}
