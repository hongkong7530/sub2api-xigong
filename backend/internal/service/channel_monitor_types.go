package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// MonitorBodyOverrideMode 自定义请求体处理模式。
//
//   - off     使用 adapter 默认 body（忽略 BodyOverride）
//   - merge   adapter 默认 body 与 BodyOverride 浅合并（用户优先；
//     model/messages/contents 等关键字段在 checker 黑名单内会被静默丢弃）
//   - replace 完全用 BodyOverride 作为 body；跳过 challenge 校验，
//     改成 HTTP 2xx + 响应非空即视为可用（用户负责构造 body）
const (
	MonitorBodyOverrideModeOff     = "off"
	MonitorBodyOverrideModeMerge   = "merge"
	MonitorBodyOverrideModeReplace = "replace"
)

// MonitorAPIMode 描述 OpenAI provider 的请求协议。
//
//   - chat_completions  OpenAI-compatible Chat Completions: /v1/chat/completions + messages
//   - responses         OpenAI Responses API: /v1/responses + instructions/input
//
// 非 OpenAI provider 固定使用 chat_completions 作为占位默认值，避免为每个 provider 单独扩表。
const (
	MonitorAPIModeChatCompletions = "chat_completions"
	MonitorAPIModeResponses       = "responses"
)

// ChannelMonitor 渠道监控配置（service 层模型，不直接暴露 ent 类型）。
type ChannelMonitor struct {
	ID                  int64
	Name                string
	Provider            string
	APIMode             string
	Endpoint            string
	APIKey              string // 解密后的明文 API Key（仅在 service 内部使用，handler 层不应直接序列化返回）
	PrimaryModel        string
	ExtraModels         []string
	GroupName           string
	GroupID             *int64
	KeyProvisionMode    string
	ProvisionedAPIKeyID *int64
	Description         string
	DescriptionColor    string
	Enabled             bool
	ProbeMode           string // hybrid（主动+被动）或 passive（仅分组真实请求）
	IntervalSeconds     int
	JitterSeconds       int // 每次调度 ± [0, jitter] 的随机偏移（秒），0 = 固定间隔
	LastCheckedAt       *time.Time
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time

	// 配额模式（check_mode = quota / quota_probe）：
	// 关联已有账号复用账号侧用量服务，Endpoint/APIKey 可为空（quota 模式）。
	CheckMode string // probe（默认）/ quota / quota_probe；空串按 probe 处理
	AccountID *int64 // 关联账号 ID；账号删除后被 DB 置空（监控保留并报「账号未关联」）

	// 请求自定义快照（来自模板拷贝 or 用户手填，运行时直接读取）
	TemplateID       *int64            // 仅用于 UI 分组 + 一键应用，运行时不用
	ExtraHeaders     map[string]string // 与 adapter 默认 headers 合并，用户优先
	BodyOverrideMode string            // off / merge / replace
	BodyOverride     map[string]any    // 仅 mode != off 时使用

	// DuplicateOperationID is internal persistence metadata used to recover an
	// already committed duplicate after an ambiguous idempotency-store failure.
	// Repository implementations must keep it out of ExtraHeaders so it can
	// never be serialized to clients or forwarded to an upstream provider.
	DuplicateOperationID string

	// APIKeyDecryptFailed 表示 APIKey 字段无法解密（密钥不一致或损坏）。
	// 此时 APIKey 为空字符串，runner / RunCheck 必须跳过该监控并提示重填。
	APIKeyDecryptFailed bool
}

// ChannelMonitorListParams 列表查询过滤参数。
type ChannelMonitorListParams struct {
	Page     int
	PageSize int
	Provider string
	GroupID  *int64
	Enabled  *bool
	Search   string
}

// ChannelMonitorCreateParams 创建参数。
type ChannelMonitorCreateParams struct {
	Name             string
	Provider         string
	APIMode          string
	Endpoint         string
	APIKey           string
	PrimaryModel     string
	ExtraModels      []string
	GroupName        string
	GroupID          *int64
	KeyProvisionMode string
	Description      string
	DescriptionColor string
	Enabled          bool
	ProbeMode        string
	IntervalSeconds  int
	JitterSeconds    int
	CreatedBy        int64
	TemplateID       *int64
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any

	// 配额模式：CheckMode 空串默认 probe；quota/quota_probe 必须关联账号。
	CheckMode string
	AccountID *int64
}

// ChannelMonitorUpdateParams 更新参数（指针字段表示"未提供则不更新"）。
type ChannelMonitorUpdateParams struct {
	Name             *string
	Provider         *string
	APIMode          *string
	Endpoint         *string
	APIKey           *string // 空字符串表示不修改；非空字符串覆盖
	PrimaryModel     *string
	ExtraModels      *[]string
	GroupName        *string
	GroupID          *int64
	KeyProvisionMode *string
	Description      *string
	DescriptionColor *string
	Enabled          *bool
	ProbeMode        *string
	IntervalSeconds  *int
	JitterSeconds    *int
	// 自定义快照字段：指针为 nil 表示不更新，非 nil 覆盖
	// TemplateID *(*int64)：用 ** 表达三态：nil=不更新；&nil=清空；&&id=设为 id。
	// 简化处理：用 ClearTemplate 显式标志 + TemplateID（普通指针）
	TemplateID       *int64
	ClearTemplate    bool // true 时无视 TemplateID，把监控的 template_id 置空
	ExtraHeaders     *map[string]string
	BodyOverrideMode *string
	BodyOverride     *map[string]any

	// 配额模式：CheckMode nil = 不更新；AccountID nil = 不更新，
	// 指向 0 = 清空关联（退回 probe 模式时由 CheckMode 分支兜底）。
	CheckMode *string
	AccountID *int64
}

// CheckResult 单个模型一次检测的结果。
type CheckResult struct {
	Model                 string
	Status                string // operational / degraded / failed / error
	LatencyMs             *int
	FirstTokenMs          *int
	OutputTokensPerSecond *float64
	PingLatencyMs         *int
	Message               string
	CheckedAt             time.Time
	// Quota 配额模式附带快照（quota 模式唯一数据；quota_probe 挂在主模型行）。
	Quota *domain.MonitorQuotaSnapshot
}

type ChannelMonitorGroupRatePoint struct {
	IsInitial               bool      `json:"is_initial"`
	RateMultiplier          float64   `json:"rate_multiplier"`
	EffectiveRateMultiplier float64   `json:"effective_rate_multiplier"`
	PeakRateEnabled         bool      `json:"peak_rate_enabled"`
	PeakRateMultiplier      float64   `json:"peak_rate_multiplier"`
	PeakStart               string    `json:"peak_start"`
	PeakEnd                 string    `json:"peak_end"`
	RecordedAt              time.Time `json:"recorded_at"`
}

type ChannelMonitorGroupMetrics struct {
	RateMultiplier          float64 `json:"rate_multiplier"`
	SubscriptionType        string  `json:"-"`
	PeakRateEnabled         bool    `json:"peak_rate_enabled"`
	PeakRateMultiplier      float64 `json:"peak_rate_multiplier"`
	PeakStart               string  `json:"peak_start"`
	PeakEnd                 string  `json:"peak_end"`
	AppliedPeakMultiplier   float64 `json:"applied_peak_multiplier"`
	EffectiveRateMultiplier float64 `json:"effective_rate_multiplier"`
	CacheReadTokens         int64   `json:"cache_read_tokens"`
	CacheCreationTokens     int64   `json:"cache_creation_tokens"`
	InputTokens             int64   `json:"input_tokens"`
	CacheInputTokens        int64
	CacheSampleCount        int64
	CacheWindowSeconds      int64
	CacheHitRate            float64   `json:"cache_hit_rate"`
	ObservedAt              time.Time `json:"observed_at"`
}

type ChannelMonitorGroupFirstToken struct {
	ID             int64
	Model          string
	RequestedModel string
	FirstTokenMs   *int
	DurationMs     *int
	CreatedAt      time.Time
}

// UserMonitorView 用户只读视图：监控概览（含主模型最近状态 + 7d 可用率 + 附加模型最近状态）。
type UserMonitorView struct {
	ID        int64
	Name      string
	Provider  string
	GroupName string
	// GroupID 绑定分组 ID；用户端订阅通知以此为键，未绑定为 nil。
	GroupID       *int64
	PrimaryModel  string
	PrimaryStatus string
	// StatusSource identifies the actual observation, not a synthetic probe result.
	StatusSource     string
	StatusObservedAt *time.Time
	// OverallStatus 全部被监控模型最优状态（整卡颜色用，宽松口径）。
	OverallStatus    string
	PrimaryLatencyMs *int
	// PrimaryFirstTokenMs 主模型最近一次首字延迟（TTFT）毫秒。
	PrimaryFirstTokenMs          *int
	PrimaryOutputTokensPerSecond *float64
	PrimaryPingLatencyMs         *int    // 主模型最近一次 ping 延迟
	Availability7d               float64 // 0-100
	AvailabilitySamples7d        int
	ExtraModels                  []ExtraModelStatus
	Timeline                     []UserMonitorTimelinePoint // 主模型最近 N 个历史点（按 checked_at DESC，最新在前）
	// LatestQuota 主模型最近一次配额快照；channel_monitor_show_quota=false
	// 时由 handler 服务端剥离。
	LatestQuota *domain.MonitorQuotaSnapshot
	// GroupInsight 分组绑定监控的分组实时指标（倍率/缓存率）；未绑定分组为 nil。
	GroupInsight *UserGroupInsight
}

// UserGroupInsight 分组绑定监控的分组实时指标。
type UserGroupInsight struct {
	// RateMultiplier 是配置的基础倍率；EffectiveRateMultiplier 是当前计费有效倍率
	// （基础倍率 × 当前应用的高峰因子）。
	RateMultiplier          float64
	EffectiveRateMultiplier float64
	PeakRateEnabled         bool
	PeakRateMultiplier      float64
	AppliedPeakMultiplier   float64
	CacheInputTokens        int64
	CacheSampleCount        int64
	CacheWindowSeconds      int64
	CacheHitRate            float64
	RateHistory             []UserGroupRatePoint
	RecentFirstTokens       []*ChannelMonitorGroupFirstToken
}

// UserGroupRatePoint 倍率变更历史单点。
type UserGroupRatePoint struct {
	IsInitial               bool
	RateMultiplier          float64
	EffectiveRateMultiplier float64
	RecordedAt              time.Time
}

// UserMonitorTimelinePoint 用户视图 timeline 单点数据（去除 message 以减小响应体）。
type UserMonitorTimelinePoint struct {
	OutputTokensPerSecond *float64  `json:"output_tokens_per_second"`
	Model                 string    `json:"model"`
	Source                string    `json:"source"`
	Status                string    `json:"status"`
	LatencyMs             *int      `json:"latency_ms"`
	FirstTokenMs          *int      `json:"first_token_ms"`
	PingLatencyMs         *int      `json:"ping_latency_ms"`
	CheckedAt             time.Time `json:"checked_at"`
}

// ExtraModelStatus 附加模型最近一次状态。
type ExtraModelStatus struct {
	Model        string
	Status       string
	LatencyMs    *int
	FirstTokenMs *int
}

// UserMonitorDetail 用户只读视图：监控详情（含全部模型 7d/15d/30d 可用率与平均延迟）。
type UserMonitorDetail struct {
	GroupAvailability map[int]*ChannelMonitorAvailability
	ID                int64
	Name              string
	Provider          string
	GroupName         string
	Models            []ModelDetail
	// GroupInsight 分组实时指标 + 倍率变更历史（前端画曲线用）。
	GroupInsight *UserGroupInsight
}

// ModelDetail 单个模型的可用率/延迟统计。
type ModelDetail struct {
	Model           string
	LatestStatus    string
	LatestLatencyMs *int
	// LatestFirstTokenMs 最近一次首字延迟（TTFT）毫秒。
	LatestOutputTokensPerSecond *float64
	LatestFirstTokenMs          *int
	Availability7d              float64 // 0-100
	Availability15d             float64
	Availability30d             float64
	Samples7d                   int
	Samples15d                  int
	Samples30d                  int
	AvgLatency7dMs              *int
}

// ChannelMonitorHistoryRow 历史记录入库行（service 层向 repository 提交的数据）。
type ChannelMonitorHistoryRow struct {
	MonitorID             int64
	Model                 string
	Status                string
	LatencyMs             *int
	FirstTokenMs          *int
	OutputTokensPerSecond *float64
	PingLatencyMs         *int
	Message               string
	CheckedAt             time.Time
	Quota                 *domain.MonitorQuotaSnapshot
}

// ChannelMonitorHistoryEntry 历史记录查询返回行（含 ent 主键 ID）。
type ChannelMonitorHistoryEntry struct {
	Source                string
	ID                    int64
	Model                 string
	Status                string
	LatencyMs             *int
	FirstTokenMs          *int
	OutputTokensPerSecond *float64
	PingLatencyMs         *int
	Message               string
	CheckedAt             time.Time
	Quota                 *domain.MonitorQuotaSnapshot
}

// ChannelMonitorLatest 最近一次检测的简明信息（用于 UserMonitorView 聚合）。
type ChannelMonitorLatest struct {
	Model                 string
	Status                string
	LatencyMs             *int
	FirstTokenMs          *int
	OutputTokensPerSecond *float64
	PingLatencyMs         *int
	CheckedAt             time.Time
	Quota                 *domain.MonitorQuotaSnapshot
}

// ChannelMonitorAvailability 单个模型在某窗口内的可用率与平均延迟（用于 UserMonitorDetail 聚合）。
type ChannelMonitorAvailability struct {
	Model             string
	WindowDays        int
	TotalChecks       int
	OperationalChecks int // operational + degraded 视为可用
	AvailabilityPct   float64
	AvgLatencyMs      *int
}

// MonitorStatusSummary 监控状态聚合（admin list 用，单次 repo 查询消除前端 N+1）。
// PrimaryStatus / PrimaryLatencyMs 描述主模型最近状态；Availability7d 是主模型 7 天可用率；
// ExtraModels 描述附加模型最近状态（用于 hover 展示）。
type MonitorStatusSummary struct {
	PrimaryStatus    string // 空字符串表示无历史
	PrimaryLatencyMs *int
	// PrimaryFirstTokenMs 主模型最近一次首字延迟（TTFT）毫秒。
	PrimaryFirstTokenMs *int
	// 状态事实与配置的探测模型分离，混合监控可来自分组其他模型的真实请求。
	StatusModel      string
	StatusSource     string
	StatusObservedAt *time.Time
	// OverallStatus 全部被监控模型的最优状态（一绿即绿，宽松口径）。
	OverallStatus         string
	Availability7d        float64 // 0-100；样本数0表示未知
	AvailabilitySamples7d int
	ExtraModels           []ExtraModelStatus
	LatestQuota           *domain.MonitorQuotaSnapshot // 主模型最近配额快照（配额模式）
}
