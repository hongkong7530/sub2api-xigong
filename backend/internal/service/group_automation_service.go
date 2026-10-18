package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

// GroupAutomationService 面向「公开分组」（status=active 且非专属）的一键自动化：
//   - 缺少分组绑定监控的公开分组，自动创建 auto-key 探活监控（自动配给分组 Key）；
//   - 未绑定广场渠道的公开分组，自动创建 name-only 渠道条目让分组出现在模型广场。
//
// INVARIANT: 监控只做「无则创建」，广场只同步自动化拥有的模型列表，
// 绝不修改管理员手工配置；重复执行不得创建重复资源。
type GroupAutomationService struct {
	groupRepo      GroupRepository
	channelService *ChannelService
	monitorService *ChannelMonitorService
	adminService   AdminService

	// 周期重同步：把自动广场渠道持续对齐到分组真实模型集（含无监控的分组）。
	lockCache   LeaderLockCache
	db          *sql.DB
	instanceID  string
	provisionMu sync.Mutex
	stopOnce    sync.Once
	stopCh      chan struct{}
}

// NewGroupAutomationService 创建公开分组自动化服务。
func NewGroupAutomationService(
	groupRepo GroupRepository,
	channelService *ChannelService,
	monitorService *ChannelMonitorService,
	adminService AdminService,
) *GroupAutomationService {
	s := &GroupAutomationService{
		groupRepo:      groupRepo,
		channelService: channelService,
		monitorService: monitorService,
		adminService:   adminService,
		instanceID:     uuid.NewString(),
		stopCh:         make(chan struct{}),
	}
	if monitorService != nil {
		monitorService.SetPlazaResyncer(s)
	}
	return s
}

// autoPlazaReconcileInterval 自动广场渠道周期重同步间隔；首轮在启动后 autoPlazaReconcileInitialDelay 执行。
const (
	autoPlazaReconcileInterval     = 15 * time.Minute
	autoPlazaReconcileInitialDelay = 45 * time.Second
	autoPlazaReconcileLockKey      = "group-automation:plaza-reconcile"
)

// SetLeaderLock 注入跨节点互斥依赖（双 nil 时单机模式）。
func (s *GroupAutomationService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// StartPlazaReconcile 启动周期重同步循环。只修改带自动化标记的广场渠道，人工渠道不动。
func (s *GroupAutomationService) StartPlazaReconcile() {
	if s == nil {
		return
	}
	go func() {
		timer := time.NewTimer(autoPlazaReconcileInitialDelay)
		defer timer.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-timer.C:
				s.reconcileOnce()
				timer.Reset(autoPlazaReconcileInterval)
			}
		}
	}()
}

// StopPlazaReconcile 停止周期循环。
func (s *GroupAutomationService) StopPlazaReconcile() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *GroupAutomationService) reconcileOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := s.ReconcileAutoPlazaChannels(ctx); err != nil {
		slog.Warn("group_automation: plaza reconcile failed", "error", err)
	}
}

// ReconcileAutoPlazaChannels 遍历公开分组，把带自动化标记的广场渠道重同步到分组真实模型集。
// 模型来源：分组 Key 网关实取（已按账号显式映射过滤）> 账号显式映射 > 模型候选。
func (s *GroupAutomationService) ReconcileAutoPlazaChannels(ctx context.Context) error {
	if s.channelService == nil || s.channelService.repo == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	release, ok := s.acquireMutation(ctx)
	if !ok {
		return nil
	}
	defer release()
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active groups: %w", err)
	}
	if s.channelService != nil {
		s.channelService.beginBatchMutation()
		defer s.channelService.endBatchMutation()
	}
	report := &GroupAutomationReport{}
	for i := range groups {
		g := &groups[i]
		if g.IsExclusive {
			continue
		}
		channelID, err := s.channelService.repo.GetChannelIDByGroupID(ctx, g.ID)
		if err != nil || channelID <= 0 {
			continue
		}
		// 人工渠道无需模型发现；否则一次无改动扫描也会逐组访问网关。
		if !s.isAutoPlazaChannel(ctx, g, channelID, report) {
			continue
		}
		models, source := s.resolveGroupModels(ctx, g, s.groupModelLoader(ctx, g))
		if source == "unavailable" {
			report.Items = append(report.Items, GroupAutomationItem{GroupID: g.ID, GroupName: g.Name, Action: "plaza_error", Detail: "模型发现失败，保留已有广场目录"})
			continue
		}
		s.resyncAutoPlazaChannel(ctx, g, channelID, models, source, report)
	}
	for _, item := range report.Items {
		if item.Action == "plaza_channel_updated" || item.Action == "plaza_error" {
			slog.Info("group_automation: plaza reconcile", "group_id", item.GroupID, "action", item.Action, "detail", item.Detail)
		}
	}
	return nil
}

// ResyncAutoPlazaForGroup 接收成功发现的网关模型集；空列表表示已确认无模型。
// 调用方只可在发现成功后调用，失败时保留旧目录。
func (s *GroupAutomationService) ResyncAutoPlazaForGroup(ctx context.Context, groupID int64, models []string) {
	if s == nil || s.channelService == nil || s.channelService.repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	release, ok := s.acquireMutation(ctx)
	if !ok {
		return
	}
	defer release()
	channelID, err := s.channelService.repo.GetChannelIDByGroupID(ctx, groupID)
	if err != nil || channelID <= 0 {
		return
	}
	g, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil || g == nil || g.IsExclusive || g.Status != StatusActive {
		return
	}
	report := &GroupAutomationReport{}
	s.resyncAutoPlazaChannel(ctx, g, channelID, models, "gateway", report)
	for _, item := range report.Items {
		if item.Action == "plaza_channel_updated" || item.Action == "plaza_error" {
			slog.Info("group_automation: plaza resynced from model discovery", "group_id", groupID, "detail", item.Detail)
		}
	}
}

// groupModelLoader 返回分组模型候选加载器：账号显式 model_mapping 优先（真实支持集），
// 无显式映射时才回落平台默认候选——避免国产分组被 Claude 默认表污染。
func (s *GroupAutomationService) groupModelLoader(ctx context.Context, g *Group) func() ([]string, error) {
	var models []string
	var loadErr error
	loaded := false
	return func() ([]string, error) {
		if loaded {
			return models, loadErr
		}
		loaded = true
		if s.monitorService != nil {
			if explicit, err := s.monitorService.GroupExplicitModels(ctx, g.ID); err == nil && len(explicit) > 0 {
				models = automationGroupModels(g, explicit)
				return models, nil
			}
		}
		models, loadErr = s.adminService.GetGroupModelsListCandidates(ctx, g.ID, "")
		models = automationGroupModels(g, models)
		return models, loadErr
	}
}

// GroupAutomationItem 单个分组单个动作的处理结果。
type GroupAutomationItem struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	// Action: monitor_created / monitor_repaired / monitor_skipped / monitor_error /
	//         plaza_channel_created / plaza_channel_updated / plaza_skipped / plaza_error
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// GroupAutomationReport 一次自动化执行的汇总报告。
type GroupAutomationReport struct {
	GroupsScanned        int                   `json:"groups_scanned"`
	MonitorsCreated      int                   `json:"monitors_created"`
	PlazaChannelsCreated int                   `json:"plaza_channels_created"`
	PlazaModelsAdded     int                   `json:"plaza_models_added"`
	Items                []GroupAutomationItem `json:"items"`
}

// 分组自动监控每分钟只探测一个热门模型，不附加抖动或额外模型。
const (
	autoMonitorIntervalSeconds = 60
	autoMonitorJitterSeconds   = 0
	autoMonitorMaxExtraModels  = 0
)

// autoPlazaChannelDescription 自动化创建的广场渠道标记。带此描述的渠道视为
// 自动化专有，后续执行会按分组 Key 实取的模型列表重同步；人工渠道绝不改动。
const autoPlazaChannelDescription = "自动化创建：公开分组模型广场条目"
const autoGroupMonitorDescription = "自动化创建：公开分组监控"

// 所有自动化入口覆盖同一个发现→写入区间，防止慢发现覆盖较新的结果。
func (s *GroupAutomationService) acquireMutation(ctx context.Context) (func(), bool) {
	if !s.provisionMu.TryLock() {
		return nil, false
	}
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, autoPlazaReconcileLockKey, s.instanceID, 6*time.Minute)
	if !ok {
		s.provisionMu.Unlock()
		return nil, false
	}
	return func() { release(); s.provisionMu.Unlock() }, true
}

// monitorProviderForPlatform 把分组平台映射为监控探测协议。
// composite 等无法唯一确定协议的平台返回空串，调用方跳过并提示人工创建。
func monitorProviderForPlatform(platform string) string {
	switch platform {
	case PlatformAnthropic:
		return "anthropic"
	case PlatformOpenAI:
		return "openai"
	case PlatformGemini:
		return "gemini"
	case PlatformAntigravity:
		return "antigravity"
	// 国产三家与 Grok 经本机网关走 Chat Completions 同构探测；此前返回空串导致
	// 这些公开分组从不建监控，广场只能回落默认候选。
	case PlatformKimi:
		return MonitorProviderKimi
	case PlatformZhipu:
		return MonitorProviderZhipu
	case PlatformDeepseek:
		return MonitorProviderDeepseek
	case PlatformGrok:
		return MonitorProviderGrok
	default:
		return ""
	}
}

// autoMonitorName 生成不超过实体上限（100 字符）的监控名。
func autoMonitorName(groupName string) string {
	name := strings.TrimSpace(groupName)
	if runes := []rune(name); len(runes) > 80 {
		name = string(runes[:80])
	}
	return name + " · 自动监控"
}

// AutoProvisionPublicGroups 扫描全部公开分组并补齐监控与广场条目。
func (s *GroupAutomationService) AutoProvisionPublicGroups(ctx context.Context, operatorID int64) (*GroupAutomationReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	release, ok := s.acquireMutation(ctx)
	if !ok {
		return nil, infraerrors.Conflict("GROUP_AUTOMATION_IN_PROGRESS", "public group automation is already running")
	}
	defer release()
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	boundIDs, err := s.monitorService.BoundGroupIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list monitor-bound groups: %w", err)
	}
	bound := make(map[int64]struct{}, len(boundIDs))
	for _, id := range boundIDs {
		bound[id] = struct{}{}
	}

	// 整批进入批量变更作用域：逐分组建渠道会各自触发一次渠道缓存全量重建，
	// 7 个公开分组就是 7 次全表读取 + 7 次广播，表现为点一次按钮整站变卡。
	// 作用域内保留旧快照对外服务，退出时只重建一次。
	if s.channelService != nil {
		s.channelService.beginBatchMutation()
		defer s.channelService.endBatchMutation()
	}

	report := &GroupAutomationReport{Items: make([]GroupAutomationItem, 0, len(groups)*2)}
	for i := range groups {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("public group automation interrupted: %w", err)
		}
		g := &groups[i]
		if g.IsExclusive {
			// 专属分组不公开，不纳入自动化范围。
			continue
		}
		report.GroupsScanned++

		// 模型候选按需拉取一次，监控与广场两个动作共用（显式映射优先）。
		loadModels := s.groupModelLoader(ctx, g)

		s.ensureGroupMonitor(ctx, g, bound, loadModels, operatorID, report)
		s.ensurePlazaChannel(ctx, g, loadModels, report)
	}
	return report, nil
}

func (s *GroupAutomationService) ensureGroupMonitor(ctx context.Context, g *Group, bound map[int64]struct{}, loadModels func() ([]string, error), operatorID int64, report *GroupAutomationReport) {
	item := GroupAutomationItem{GroupID: g.ID, GroupName: g.Name}
	if _, ok := bound[g.ID]; ok {
		repaired, err := s.monitorService.repairIncompleteAutoMonitor(ctx, g.ID)
		switch {
		case err != nil:
			item.Action, item.Detail = "monitor_error", err.Error()
		case repaired:
			item.Action, item.Detail = "monitor_repaired", "上次配给中断，已补齐自动监控 Key"
		default:
			item.Action, item.Detail = "monitor_skipped", "已有分组绑定监控，保留现有配置"
		}
		report.Items = append(report.Items, item)
		return
	}
	provider := monitorProviderForPlatform(g.Platform)
	if provider == "" {
		item.Action = "monitor_skipped"
		item.Detail = fmt.Sprintf("平台 %s 无法自动确定探测协议，请手动创建监控", g.Platform)
		report.Items = append(report.Items, item)
		return
	}
	models, err := loadModels()
	if err != nil {
		item.Action = "monitor_error"
		item.Detail = err.Error()
		report.Items = append(report.Items, item)
		return
	}
	if len(models) == 0 {
		item.Action = "monitor_skipped"
		item.Detail = "分组暂无可用模型候选"
		report.Items = append(report.Items, item)
		return
	}
	extras := models[1:]
	if len(extras) > autoMonitorMaxExtraModels {
		extras = extras[:autoMonitorMaxExtraModels]
	}
	groupID := g.ID
	primary := &ChannelMonitor{PrimaryModel: models[0], GroupID: &groupID}
	s.monitorService.selectPopularMonitorModel(ctx, primary, models)
	// OpenAI 系优先 Responses 协议探测；国产 provider（kimi/zhipu/deepseek）保持 Chat Completions。
	apiMode := ""
	if provider == MonitorProviderOpenAI {
		apiMode = MonitorAPIModeResponses
	}
	_, err = s.monitorService.Create(ctx, ChannelMonitorCreateParams{
		Name:             autoMonitorName(g.Name),
		Provider:         provider,
		APIMode:          apiMode,
		PrimaryModel:     primary.PrimaryModel,
		Endpoint:         s.monitorService.probeRuntime(ctx).ProbeEndpoint,
		ExtraModels:      append([]string(nil), extras...),
		GroupName:        g.Name,
		GroupID:          &groupID,
		KeyProvisionMode: "auto",
		Description:      autoGroupMonitorDescription,
		Enabled:          true,
		IntervalSeconds:  autoMonitorIntervalSeconds,
		JitterSeconds:    autoMonitorJitterSeconds,
		CreatedBy:        operatorID,
	})
	if err != nil {
		item.Action = "monitor_error"
		item.Detail = err.Error()
	} else {
		item.Action = "monitor_created"
		item.Detail = fmt.Sprintf("主模型 %s，附加 %d 个模型，自动配给分组 Key", primary.PrimaryModel, len(extras))
		bound[g.ID] = struct{}{}
		report.MonitorsCreated++
	}
	report.Items = append(report.Items, item)
}

func (s *GroupAutomationService) ensurePlazaChannel(ctx context.Context, g *Group, loadModels func() ([]string, error), report *GroupAutomationReport) {
	item := GroupAutomationItem{GroupID: g.ID, GroupName: g.Name}
	if s.channelService == nil || s.channelService.repo == nil {
		item.Action = "plaza_skipped"
		item.Detail = "渠道服务不可用"
		report.Items = append(report.Items, item)
		return
	}
	channelID, err := s.channelService.repo.GetChannelIDByGroupID(ctx, g.ID)
	if err != nil {
		item.Action = "plaza_error"
		item.Detail = err.Error()
		report.Items = append(report.Items, item)
		return
	}

	if channelID > 0 && !s.isAutoPlazaChannel(ctx, g, channelID, report) {
		return
	}
	// 模型来源优先级：分组 auto Key 实取本机网关 /v1/models（与用户拿 Key 看到的
	// 列表完全同源）；网关/Key 不可用时回退模型候选，报告注明来源。
	models, source := s.resolveGroupModels(ctx, g, loadModels)
	if source == "unavailable" {
		item.Action, item.Detail = "plaza_error", "模型发现失败，保留已有广场目录"
		report.Items = append(report.Items, item)
		return
	}
	if channelID > 0 {
		s.resyncAutoPlazaChannel(ctx, g, channelID, models, source, report)
		return
	}
	if len(models) == 0 {
		item.Action = "plaza_skipped"
		item.Detail = "分组暂无可用模型，不新建空渠道"
		report.Items = append(report.Items, item)
		return
	}

	if g.Platform == PlatformComposite {
		item.Action = "plaza_skipped"
		item.Detail = "composite 分组需按具体平台手动配置广场条目"
		report.Items = append(report.Items, item)
		return
	}
	name, err := s.pickChannelName(ctx, g.Name)
	if err != nil {
		item.Action = "plaza_error"
		item.Detail = err.Error()
		report.Items = append(report.Items, item)
		return
	}
	// SAFETY: name-only 定价条目（价格字段全部留空）。广场展示按官方目录回落
	// （pricingNeedsFallback），计费解析对 token 模式只用非空字段做覆盖，
	// 空价条目不会改变任何计费结果。
	_, err = s.channelService.Create(ctx, &CreateChannelInput{
		Name:        name,
		Description: autoPlazaChannelDescription,
		GroupIDs:    []int64{g.ID},
		ModelPricing: []ChannelModelPricing{{
			Platform:    g.Platform,
			Models:      append([]string(nil), models...),
			BillingMode: BillingModeToken,
		}},
	})
	if err != nil {
		item.Action = "plaza_error"
		item.Detail = err.Error()
	} else {
		item.Action = "plaza_channel_created"
		item.Detail = fmt.Sprintf("渠道 %s，按%s登记 %d 个模型", name, modelSourceLabel(source), len(models))
		report.PlazaChannelsCreated++
		report.PlazaModelsAdded += len(models)
	}
	report.Items = append(report.Items, item)
}

// resolveGroupModels 返回广场登记用的模型列表与来源（gateway / candidates）。
func (s *GroupAutomationService) resolveGroupModels(ctx context.Context, g *Group, loadModels func() ([]string, error)) ([]string, string) {
	if s.monitorService != nil {
		if ids, err := s.monitorService.GroupGatewayModelIDs(ctx, g.ID); err == nil {
			// 成功的空列表也是权威结果，不能回退成用户实际不可调用的模型。
			return automationGroupModels(g, ids), "gateway"
		}
	}
	models, err := loadModels()
	if err != nil {
		return nil, "unavailable"
	}
	return automationGroupModels(g, models), "candidates"
}

// 网关不可用时也必须遵守分组白名单，并按同一规则比较模型集合。
func automationGroupModels(g *Group, models []string) []string {
	models = dedupeAndSortModelIDs(models)
	seen := make(map[string]struct{}, len(models))
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		key := strings.ToLower(model)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, model)
	}
	if g != nil {
		filtered = g.ModelAllowlist.FilterForListing(filtered)
	}
	return filtered
}

func (s *GroupAutomationService) isAutoPlazaChannel(ctx context.Context, g *Group, channelID int64, report *GroupAutomationReport) bool {
	channel, err := s.channelService.repo.GetByID(ctx, channelID)
	if err != nil {
		report.Items = append(report.Items, GroupAutomationItem{GroupID: g.ID, GroupName: g.Name, Action: "plaza_error", Detail: err.Error()})
		return false
	}
	if !automationOwnsPlazaModels(channel) {
		report.Items = append(report.Items, GroupAutomationItem{GroupID: g.ID, GroupName: g.Name, Action: "plaza_skipped", Detail: "渠道含人工配置或定价，保留不改动"})
		return false
	}
	return true
}

func modelSourceLabel(source string) string {
	if source == "gateway" {
		return "分组 Key 网关实取"
	}
	return "模型候选回退"
}

// 描述标记不是覆盖定价的授权；任何后续人工价格、映射或规则均移交人工维护。
func automationOwnsPlazaModels(channel *Channel) bool {
	if channel.Description != autoPlazaChannelDescription || len(channel.ModelMapping) > 0 ||
		(channel.BillingModelSource != "" && channel.BillingModelSource != BillingModelSourceChannelMapped) ||
		channel.RestrictModels || channel.ApplyPricingToAccountStats || len(channel.AccountStatsPricingRules) > 0 ||
		len(channel.FeaturesConfig) > 0 || len(channel.GroupIDs) > 1 {
		return false
	}
	for _, p := range channel.ModelPricing {
		if (p.BillingMode != "" && p.BillingMode != BillingModeToken) ||
			p.InputPrice != nil || p.OutputPrice != nil || p.CacheWritePrice != nil || p.CacheWrite1hPrice != nil || p.CacheReadPrice != nil ||
			p.LongContextInputThreshold != nil || p.FastMultiplier != nil || p.FlexMultiplier != nil || len(p.ReasoningEffortMultipliers) > 0 ||
			p.ImageInputPrice != nil || p.ImageOutputPrice != nil || p.PerRequestPrice != nil || len(p.Intervals) > 0 || p.TimePricing != nil {
			return false
		}
	}
	return true
}

// resyncAutoPlazaChannel 仅重同步仍为 name-only 的自动渠道；人工配置保留不动。
func (s *GroupAutomationService) resyncAutoPlazaChannel(ctx context.Context, g *Group, channelID int64, models []string, source string, report *GroupAutomationReport) {
	item := GroupAutomationItem{GroupID: g.ID, GroupName: g.Name}
	channel, err := s.channelService.repo.GetByID(ctx, channelID)
	if err != nil {
		item.Action = "plaza_error"
		item.Detail = err.Error()
		report.Items = append(report.Items, item)
		return
	}
	if !automationOwnsPlazaModels(channel) {
		item.Action = "plaza_skipped"
		item.Detail = "渠道含人工配置或定价，保留不改动"
		report.Items = append(report.Items, item)
		return
	}
	models = automationGroupModels(g, models)
	if sameModelSet(channel.ModelPricing, g.Platform, models) {
		item.Action = "plaza_skipped"
		item.Detail = fmt.Sprintf("广场模型已与%s一致（%d 个）", modelSourceLabel(source), len(models))
		report.Items = append(report.Items, item)
		return
	}
	pricing := []ChannelModelPricing{{
		Platform:    g.Platform,
		Models:      append([]string(nil), models...),
		BillingMode: BillingModeToken,
	}}
	if err := s.channelService.repo.ReplaceModelPricing(ctx, channelID, pricing); err != nil {
		item.Action = "plaza_error"
		item.Detail = err.Error()
		report.Items = append(report.Items, item)
		return
	}
	s.channelService.invalidateCache()
	item.Action = "plaza_channel_updated"
	item.Detail = fmt.Sprintf("按%s重同步 %d 个模型", modelSourceLabel(source), len(models))
	report.PlazaModelsAdded += len(models)
	report.Items = append(report.Items, item)
}

// sameModelSet 判断渠道现有定价与目标 name-only 列表是否同集合（大小写不敏感）。
func sameModelSet(pricing []ChannelModelPricing, platform string, models []string) bool {
	models = automationGroupModels(nil, models)
	existing := make(map[string]struct{})
	for i := range pricing {
		if pricing[i].Platform != platform {
			return false
		}
		for _, m := range pricing[i].Models {
			existing[strings.ToLower(strings.TrimSpace(m))] = struct{}{}
		}
	}
	if len(existing) != len(models) {
		return false
	}
	for _, m := range models {
		if _, ok := existing[strings.ToLower(strings.TrimSpace(m))]; !ok {
			return false
		}
	}
	return true
}

// pickChannelName 选取未被占用的渠道名：优先分组名，冲突时加「 · 广场」后缀。
func (s *GroupAutomationService) pickChannelName(ctx context.Context, groupName string) (string, error) {
	base := strings.TrimSpace(groupName)
	for _, candidate := range []string{base, base + " · 广场"} {
		exists, err := s.channelService.repo.ExistsByName(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("check channel name: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("渠道名 %s 已被占用，请手动创建广场渠道", base)
}
