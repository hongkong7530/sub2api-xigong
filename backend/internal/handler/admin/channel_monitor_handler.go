package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	// monitorMaxPageSize 列表分页上限。
	monitorMaxPageSize = 100
	// monitorAPIKeyMaskPrefix 脱敏时保留的明文前缀长度。
	monitorAPIKeyMaskPrefix = 4
	// monitorAPIKeyMaskSuffix 脱敏后追加的占位字符串。
	monitorAPIKeyMaskSuffix = "***"
)

// ChannelMonitorHandler 渠道监控管理后台 handler。
type ChannelMonitorHandler struct {
	monitorService    *service.ChannelMonitorService
	automationService *service.GroupAutomationService
	reportService     *service.ChannelGroupReportService
}

// NewChannelMonitorHandler 创建 handler。
func NewChannelMonitorHandler(monitorService *service.ChannelMonitorService) *ChannelMonitorHandler {
	return &ChannelMonitorHandler{monitorService: monitorService}
}

func (h *ChannelMonitorHandler) SetGroupServices(automation *service.GroupAutomationService, reports *service.ChannelGroupReportService) {
	h.automationService = automation
	h.reportService = reports
}

// --- Request / Response ---

type channelMonitorCreateRequest struct {
	Name             string            `json:"name" binding:"required,max=100"`
	Provider         string            `json:"provider" binding:"required,oneof=openai anthropic gemini grok antigravity kimi zhipu deepseek minimax opencode_go"`
	APIMode          string            `json:"api_mode" binding:"omitempty,oneof=chat_completions responses"`
	Endpoint         string            `json:"endpoint" binding:"omitempty,max=500"`
	APIKey           string            `json:"api_key" binding:"omitempty,max=2000"`
	PrimaryModel     string            `json:"primary_model" binding:"max=200"`
	ExtraModels      []string          `json:"extra_models"`
	GroupName        string            `json:"group_name" binding:"max=100"`
	Enabled          *bool             `json:"enabled"`
	ProbeMode        string            `json:"probe_mode" binding:"omitempty,oneof=hybrid passive"`
	GroupID          *int64            `json:"group_id"`
	KeyProvisionMode string            `json:"key_provision_mode" binding:"omitempty,oneof=manual auto"`
	IntervalSeconds  int               `json:"interval_seconds" binding:"required,min=15,max=3600"`
	JitterSeconds    int               `json:"jitter_seconds" binding:"omitempty,min=0,max=3585"`
	TemplateID       *int64            `json:"template_id"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	BodyOverrideMode string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     map[string]any    `json:"body_override"`

	// CheckMode: probe（默认）/ quota / quota_probe。quota 模式 endpoint/api_key
	// 可空（条件必填校验在 service 层按模式分支）。
	CheckMode string `json:"check_mode" binding:"omitempty,oneof=probe quota quota_probe"`
	// AccountID: 配额模式关联的账号 ID。
	AccountID *int64 `json:"account_id"`
}

type channelMonitorUpdateRequest struct {
	Name             *string            `json:"name" binding:"omitempty,max=100"`
	Provider         *string            `json:"provider" binding:"omitempty,oneof=openai anthropic gemini grok antigravity kimi zhipu deepseek minimax opencode_go"`
	APIMode          *string            `json:"api_mode" binding:"omitempty,oneof=chat_completions responses"`
	Endpoint         *string            `json:"endpoint" binding:"omitempty,max=500"`
	APIKey           *string            `json:"api_key" binding:"omitempty,max=2000"`
	PrimaryModel     *string            `json:"primary_model" binding:"omitempty,max=200"`
	ExtraModels      *[]string          `json:"extra_models"`
	GroupName        *string            `json:"group_name" binding:"omitempty,max=100"`
	Enabled          *bool              `json:"enabled"`
	ProbeMode        *string            `json:"probe_mode" binding:"omitempty,oneof=hybrid passive"`
	IntervalSeconds  *int               `json:"interval_seconds" binding:"omitempty,min=15,max=3600"`
	JitterSeconds    *int               `json:"jitter_seconds" binding:"omitempty,min=0,max=3585"`
	TemplateID       *int64             `json:"template_id"`
	ClearTemplate    bool               `json:"clear_template"` // true 时把 template_id 置空，忽略 TemplateID
	ExtraHeaders     *map[string]string `json:"extra_headers"`
	BodyOverrideMode *string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     *map[string]any    `json:"body_override"`

	// CheckMode/AccountID：nil = 不更新；AccountID 指向 0 = 清空关联。
	CheckMode *string `json:"check_mode" binding:"omitempty,oneof=probe quota quota_probe"`
	AccountID *int64  `json:"account_id"`
}

type channelMonitorResponse struct {
	ID                  int64                                `json:"id"`
	Name                string                               `json:"name"`
	Provider            string                               `json:"provider"`
	APIMode             string                               `json:"api_mode"`
	Endpoint            string                               `json:"endpoint"`
	APIKeyMasked        string                               `json:"api_key_masked"`
	APIKeyDecryptFailed bool                                 `json:"api_key_decrypt_failed"`
	PrimaryModel        string                               `json:"primary_model"`
	ExtraModels         []string                             `json:"extra_models"`
	GroupName           string                               `json:"group_name"`
	GroupID             *int64                               `json:"group_id"`
	KeyProvisionMode    string                               `json:"key_provision_mode"`
	Description         string                               `json:"description"`
	DescriptionColor    string                               `json:"description_color"`
	StatusModel         string                               `json:"status_model"`
	StatusSource        string                               `json:"status_source"`
	StatusObservedAt    *time.Time                           `json:"status_observed_at"`
	PrimaryFirstTokenMs *int                                 `json:"primary_first_token_ms"`
	OverallStatus       string                               `json:"overall_status"`
	Enabled             bool                                 `json:"enabled"`
	ProbeMode           string                               `json:"probe_mode"`
	IntervalSeconds     int                                  `json:"interval_seconds"`
	JitterSeconds       int                                  `json:"jitter_seconds"`
	LastCheckedAt       *string                              `json:"last_checked_at"`
	CreatedBy           int64                                `json:"created_by"`
	CreatedAt           string                               `json:"created_at"`
	UpdatedAt           string                               `json:"updated_at"`
	PrimaryStatus       string                               `json:"primary_status"`
	PrimaryLatencyMs    *int                                 `json:"primary_latency_ms"`
	Availability7d      *float64                             `json:"availability_7d"`
	ExtraModelsStatus   []dto.ChannelMonitorExtraModelStatus `json:"extra_models_status"`
	// 请求自定义快照：前端编辑 / 展示「高级设置」用
	TemplateID       *int64            `json:"template_id"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	BodyOverrideMode string            `json:"body_override_mode"`
	BodyOverride     map[string]any    `json:"body_override"`

	// 配额模式：check_mode + 关联账号 + 主模型最近配额快照
	// （LatestQuota 由 List handler 批量聚合后填充；管理端不受 channel_monitor_show_quota 影响）。
	CheckMode   string                       `json:"check_mode"`
	AccountID   *int64                       `json:"account_id"`
	LatestQuota *domain.MonitorQuotaSnapshot `json:"latest_quota,omitempty"`
}

type channelMonitorCheckResultResponse struct {
	Model                 string                       `json:"model"`
	Status                string                       `json:"status"`
	LatencyMs             *int                         `json:"latency_ms"`
	OutputTokensPerSecond *float64                     `json:"output_tokens_per_second"`
	PingLatencyMs         *int                         `json:"ping_latency_ms"`
	Message               string                       `json:"message"`
	CheckedAt             string                       `json:"checked_at"`
	Quota                 *domain.MonitorQuotaSnapshot `json:"quota,omitempty"`
}

type channelMonitorHistoryItemResponse struct {
	ID                    int64                        `json:"id"`
	Model                 string                       `json:"model"`
	Status                string                       `json:"status"`
	LatencyMs             *int                         `json:"latency_ms"`
	OutputTokensPerSecond *float64                     `json:"output_tokens_per_second"`
	PingLatencyMs         *int                         `json:"ping_latency_ms"`
	Message               string                       `json:"message"`
	CheckedAt             string                       `json:"checked_at"`
	Quota                 *domain.MonitorQuotaSnapshot `json:"quota,omitempty"`
}

// maskAPIKey 对 API Key 明文做脱敏：前 4 字符 + "***"，长度 ≤ 4 时只显示 "***"。
func maskAPIKey(plain string) string {
	if len(plain) <= monitorAPIKeyMaskPrefix {
		return monitorAPIKeyMaskSuffix
	}
	return plain[:monitorAPIKeyMaskPrefix] + monitorAPIKeyMaskSuffix
}

func channelMonitorToResponse(m *service.ChannelMonitor) *channelMonitorResponse {
	if m == nil {
		return nil
	}
	extras := m.ExtraModels
	if extras == nil {
		extras = []string{}
	}
	headers := m.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	resp := &channelMonitorResponse{
		ID:                  m.ID,
		Name:                m.Name,
		Provider:            m.Provider,
		APIMode:             m.APIMode,
		Endpoint:            m.Endpoint,
		APIKeyMasked:        maskAPIKey(m.APIKey),
		APIKeyDecryptFailed: m.APIKeyDecryptFailed,
		PrimaryModel:        m.PrimaryModel,
		ExtraModels:         extras,
		GroupName:           m.GroupName,
		GroupID:             m.GroupID,
		KeyProvisionMode:    m.KeyProvisionMode,
		Description:         m.Description,
		DescriptionColor:    m.DescriptionColor,
		Enabled:             m.Enabled,
		ProbeMode:           service.DefaultMonitorProbeMode(m.ProbeMode),
		IntervalSeconds:     m.IntervalSeconds,
		JitterSeconds:       m.JitterSeconds,
		CreatedBy:           m.CreatedBy,
		CreatedAt:           m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:           m.UpdatedAt.UTC().Format(time.RFC3339),
		TemplateID:          m.TemplateID,
		ExtraHeaders:        headers,
		BodyOverrideMode:    m.BodyOverrideMode,
		BodyOverride:        m.BodyOverride,
		CheckMode:           m.CheckMode,
		AccountID:           m.AccountID,
		// PrimaryStatus / PrimaryLatencyMs / Availability7d / LatestQuota
		// 由 List handler 在批量聚合后填充。
	}
	if m.LastCheckedAt != nil {
		s := m.LastCheckedAt.UTC().Format(time.RFC3339)
		resp.LastCheckedAt = &s
	}
	return resp
}

func checkResultToResponse(r *service.CheckResult) channelMonitorCheckResultResponse {
	return channelMonitorCheckResultResponse{
		Model:                 r.Model,
		Status:                r.Status,
		LatencyMs:             r.LatencyMs,
		OutputTokensPerSecond: r.OutputTokensPerSecond,
		Message:               r.Message,
		CheckedAt:             r.CheckedAt.UTC().Format(time.RFC3339),
		Quota:                 r.Quota,
	}
}

func historyEntryToResponse(e *service.ChannelMonitorHistoryEntry) channelMonitorHistoryItemResponse {
	return channelMonitorHistoryItemResponse{
		ID:                    e.ID,
		Model:                 e.Model,
		Status:                e.Status,
		LatencyMs:             e.LatencyMs,
		OutputTokensPerSecond: e.OutputTokensPerSecond,
		Message:               e.Message,
		CheckedAt:             e.CheckedAt.UTC().Format(time.RFC3339),
		Quota:                 e.Quota,
	}
}

// ParseChannelMonitorID 提取并校验路径参数 :id（admin 与 user handler 共享）。
// 校验失败时已写入 4xx 响应，调用方只需 return。
func ParseChannelMonitorID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_MONITOR_ID", "invalid monitor id"))
		return 0, false
	}
	return id, true
}

// parseListEnabled 解析 enabled query 参数：true/false 转为 *bool，空或非法则返回 nil。
func parseListEnabled(raw string) *bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes":
		v := true
		return &v
	case "false", "0", "no":
		v := false
		return &v
	default:
		return nil
	}
}

// --- Handlers ---

// List GET /api/v1/admin/channel-monitors
func (h *ChannelMonitorHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > monitorMaxPageSize {
		pageSize = monitorMaxPageSize
	}

	params := service.ChannelMonitorListParams{
		Page:     page,
		PageSize: pageSize,
		Provider: strings.TrimSpace(c.Query("provider")),
		Enabled:  parseListEnabled(c.Query("enabled")),
		Search:   strings.TrimSpace(c.Query("search")),
	}

	items, total, err := h.monitorService.List(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	summaries := h.batchSummaryFor(c, items)
	out := make([]*channelMonitorResponse, 0, len(items))
	for _, m := range items {
		out = append(out, buildListItemResponse(m, summaries[m.ID]))
	}
	response.Paginated(c, out, total, page, pageSize)
}

// batchSummaryFor 批量聚合 latest + 7d 可用率，避免每行 2 次 SQL（消除 N+1）。
func (h *ChannelMonitorHandler) batchSummaryFor(c *gin.Context, items []*service.ChannelMonitor) map[int64]service.MonitorStatusSummary {
	return h.monitorService.MonitorSummaries(c.Request.Context(), items)
}

// buildListItemResponse 把 monitor + summary 装成 admin list 的响应行。
func buildListItemResponse(m *service.ChannelMonitor, summary service.MonitorStatusSummary) *channelMonitorResponse {
	resp := channelMonitorToResponse(m)
	resp.PrimaryStatus = summary.PrimaryStatus
	resp.PrimaryLatencyMs = summary.PrimaryLatencyMs
	resp.PrimaryFirstTokenMs = summary.PrimaryFirstTokenMs
	resp.OverallStatus = summary.OverallStatus
	resp.StatusModel = summary.StatusModel
	resp.StatusSource = summary.StatusSource
	resp.StatusObservedAt = summary.StatusObservedAt
	if summary.AvailabilitySamples7d > 0 {
		value := summary.Availability7d
		resp.Availability7d = &value
	}
	resp.LatestQuota = summary.LatestQuota
	resp.ExtraModelsStatus = make([]dto.ChannelMonitorExtraModelStatus, 0, len(summary.ExtraModels))
	for _, e := range summary.ExtraModels {
		resp.ExtraModelsStatus = append(resp.ExtraModelsStatus, dto.ChannelMonitorExtraModelStatus{
			Model:        e.Model,
			Status:       e.Status,
			LatencyMs:    e.LatencyMs,
			FirstTokenMs: e.FirstTokenMs,
		})
	}
	return resp
}

// Get GET /api/v1/admin/channel-monitors/:id
func (h *ChannelMonitorHandler) Get(c *gin.Context) {
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	m, err := h.monitorService.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, channelMonitorToResponse(m))
}

// Create POST /api/v1/admin/channel-monitors
func (h *ChannelMonitorHandler) Create(c *gin.Context) {
	var req channelMonitorCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	subject, _ := middleware2.GetAuthSubjectFromContext(c)

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	m, err := h.monitorService.Create(c.Request.Context(), service.ChannelMonitorCreateParams{
		Name:             req.Name,
		Provider:         req.Provider,
		APIMode:          req.APIMode,
		Endpoint:         req.Endpoint,
		APIKey:           req.APIKey,
		PrimaryModel:     req.PrimaryModel,
		ExtraModels:      req.ExtraModels,
		GroupName:        req.GroupName,
		GroupID:          req.GroupID,
		KeyProvisionMode: req.KeyProvisionMode,
		Enabled:          enabled,
		ProbeMode:        req.ProbeMode,
		IntervalSeconds:  req.IntervalSeconds,
		JitterSeconds:    req.JitterSeconds,
		CreatedBy:        subject.UserID,
		TemplateID:       req.TemplateID,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
		CheckMode:        req.CheckMode,
		AccountID:        req.AccountID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, channelMonitorToResponse(m))
}

// Duplicate POST /api/v1/admin/channel-monitors/:id/duplicate
func (h *ChannelMonitorHandler) Duplicate(c *gin.Context) {
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	actorScope := adminActorScope(c)

	result, err := executeAdminIdempotent(
		c,
		"admin.channel_monitors.duplicate",
		struct {
			MonitorID int64 `json:"monitor_id"`
		}{MonitorID: id},
		service.DefaultWriteIdempotencyTTL(),
		func(ctx context.Context) (any, error) {
			monitor, err := h.monitorService.Duplicate(
				ctx,
				id,
				subject.UserID,
				actorScope,
				c.GetHeader("Idempotency-Key"),
			)
			if err != nil {
				return nil, err
			}
			return channelMonitorToResponse(monitor), nil
		},
	)
	if err != nil {
		reason := infraerrors.Reason(err)
		if reason == infraerrors.Reason(service.ErrIdempotencyInProgress) || reason == infraerrors.Reason(service.ErrIdempotencyStoreUnavail) {
			recovered, recoverErr := h.monitorService.RecoverDuplicate(
				c.Request.Context(),
				id,
				actorScope,
				c.GetHeader("Idempotency-Key"),
			)
			if recoverErr != nil {
				slog.Warn("channel_monitor_duplicate_recovery_failed", "monitor_id", id, "actor_scope", actorScope, "reason", reason, "error", recoverErr)
			} else if recovered != nil {
				c.Header("X-Idempotency-Recovered", "true")
				response.Success(c, channelMonitorToResponse(recovered))
				return
			}
		}
		response.ErrorFrom(c, err)
		return
	}

	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
}

// Update PUT /api/v1/admin/channel-monitors/:id
func (h *ChannelMonitorHandler) Update(c *gin.Context) {
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	var req channelMonitorUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	m, err := h.monitorService.Update(c.Request.Context(), id, service.ChannelMonitorUpdateParams{
		Name:             req.Name,
		Provider:         req.Provider,
		APIMode:          req.APIMode,
		Endpoint:         req.Endpoint,
		APIKey:           req.APIKey,
		PrimaryModel:     req.PrimaryModel,
		ExtraModels:      req.ExtraModels,
		GroupName:        req.GroupName,
		Enabled:          req.Enabled,
		ProbeMode:        req.ProbeMode,
		IntervalSeconds:  req.IntervalSeconds,
		JitterSeconds:    req.JitterSeconds,
		TemplateID:       req.TemplateID,
		ClearTemplate:    req.ClearTemplate,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
		CheckMode:        req.CheckMode,
		AccountID:        req.AccountID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, channelMonitorToResponse(m))
}

// Delete DELETE /api/v1/admin/channel-monitors/:id
func (h *ChannelMonitorHandler) Delete(c *gin.Context) {
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	if err := h.monitorService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// Run POST /api/v1/admin/channel-monitors/:id/run
func (h *ChannelMonitorHandler) Run(c *gin.Context) {
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	results, err := h.monitorService.RunCheck(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]channelMonitorCheckResultResponse, 0, len(results))
	for _, r := range results {
		out = append(out, checkResultToResponse(r))
	}
	response.Success(c, gin.H{"results": out})
}

// AutoProvision POST /api/v1/admin/channel-monitors/auto-provision
// 扫描全部公开分组（active 且非专属），为缺失项自动创建分组绑定监控与
// 模型广场渠道条目；只创建、不修改既有配置，可安全重复执行。
func (h *ChannelMonitorHandler) AutoProvision(c *gin.Context) {
	if h.automationService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("GROUP_AUTOMATION_UNAVAILABLE", "group automation is not configured"))
		return
	}
	var req struct {
		Groups []service.GroupAutomationSelection `json:"groups" binding:"required,min=1,max=500,dive"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("GROUP_SELECTION_REQUIRED", "Select public groups and monitoring options"))
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	report, err := h.automationService.AutoProvisionPublicGroups(c.Request.Context(), subject.UserID, req.Groups...)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}

// GroupModels GET /api/v1/admin/channel-monitor-groups/:group_id/models
func (h *ChannelMonitorHandler) GroupModels(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	models, err := h.monitorService.GroupModels(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"models": models})
}

// GroupMetrics GET /api/v1/admin/channel-monitor-groups/:group_id/metrics
func (h *ChannelMonitorHandler) GroupMetrics(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	metrics, err := h.monitorService.GroupMetrics(c.Request.Context(), groupID, 24*time.Hour)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"rate_multiplier": metrics.RateMultiplier, "effective_rate_multiplier": metrics.EffectiveRateMultiplier,
		"peak_rate_enabled": metrics.PeakRateEnabled, "peak_rate_multiplier": metrics.PeakRateMultiplier,
		"applied_peak_multiplier": metrics.AppliedPeakMultiplier, "peak_start": metrics.PeakStart, "peak_end": metrics.PeakEnd,
		"cache_read_tokens": metrics.CacheReadTokens, "cache_creation_tokens": metrics.CacheCreationTokens, "input_tokens": metrics.InputTokens,
		"cache_hit_rate": cachePercentWithSamples(metrics), "cache_sample_count": metrics.CacheSampleCount,
		"cache_window_seconds": metrics.CacheWindowSeconds, "observed_at": metrics.ObservedAt,
	})
}

func cachePercentWithSamples(metrics *service.ChannelMonitorGroupMetrics) *float64 {
	if metrics == nil || metrics.InputTokens+metrics.CacheReadTokens+metrics.CacheCreationTokens <= 0 {
		return nil
	}
	value := metrics.CacheHitRate
	return &value
}

// GroupFirstTokens GET /api/v1/admin/channel-monitor-groups/:group_id/first-tokens
func (h *ChannelMonitorHandler) GroupFirstTokens(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	items, err := h.monitorService.GroupFirstTokens(c.Request.Context(), groupID, 30)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		row := gin.H{"id": item.ID, "model": item.Model, "requested_model": item.RequestedModel, "first_token_ms": item.FirstTokenMs, "duration_ms": item.DurationMs, "created_at": item.CreatedAt.UTC().Format(time.RFC3339Nano)}
		out = append(out, row)
	}
	response.Success(c, gin.H{"items": out})
}

// GroupReports GET /api/v1/admin/channel-monitor-groups/:group_id/reports
func (h *ChannelMonitorHandler) GroupReports(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	if h.reportService == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_REPORT_UNAVAILABLE", "group report service is not configured"))
		return
	}
	items, err := h.reportService.ListByGroup(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, gin.H{
			"id":         item.ID,
			"user_id":    item.UserID,
			"group_id":   item.GroupID,
			"category":   item.Category,
			"body":       item.Body,
			"created_at": item.CreatedAt.UTC().Format(time.RFC3339Nano),
			"username":   item.Username,
			"email":      item.Email,
			"deleted":    item.Deleted,
		})
	}
	response.Success(c, gin.H{"items": out, "total": len(out)})
}

func (h *ChannelMonitorHandler) GroupRateHistory(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	limit := parseHistoryLimit(c.Query("limit"))
	points, err := h.monitorService.GroupRateHistory(c.Request.Context(), groupID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": points})
}

// History GET /api/v1/admin/channel-monitors/:id/history
func (h *ChannelMonitorHandler) History(c *gin.Context) {
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	limit := parseHistoryLimit(c.Query("limit"))
	model := strings.TrimSpace(c.Query("model"))

	entries, err := h.monitorService.ListHistory(c.Request.Context(), id, model, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]channelMonitorHistoryItemResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, historyEntryToResponse(e))
	}
	response.Success(c, gin.H{"items": out})
}

// parseHistoryLimit 解析 history 接口的 limit query。
// 使用 service 包的统一上下限常量，避免在 handler 重复定义同名魔法值。
func parseHistoryLimit(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return service.MonitorHistoryDefaultLimit
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return service.MonitorHistoryDefaultLimit
	}
	if v > service.MonitorHistoryMaxLimit {
		return service.MonitorHistoryMaxLimit
	}
	return v
}
