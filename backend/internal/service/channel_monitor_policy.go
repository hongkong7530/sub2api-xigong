package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// applyAutomaticProbePolicy keeps automatic group probes aligned across both runners.
// Manual vendor monitors retain their endpoint and scheduling configuration.
func (s *ChannelMonitorService) applyAutomaticProbePolicy(ctx context.Context, m *ChannelMonitor) {
	if m == nil || m.GroupID == nil || m.KeyProvisionMode != "auto" || defaultCheckMode(m.CheckMode) == MonitorCheckModeQuota {
		return
	}
	m.IntervalSeconds = autoMonitorIntervalSeconds
	m.JitterSeconds = autoMonitorJitterSeconds
	m.ExtraModels = []string{}
	if endpoint := s.probeRuntime(ctx).ProbeEndpoint; endpoint != "" {
		m.Endpoint = endpoint
	}
}

func applyAutoMonitorModels(m *ChannelMonitor, models []string) {
	if m == nil || len(models) == 0 { return }
	available := make(map[string]string, len(models))
	for _, model := range models { model = strings.TrimSpace(model); if model != "" { available[strings.ToLower(model)] = model } }
	primary := strings.ToLower(strings.TrimSpace(m.PrimaryModel))
	if _, ok := available[primary]; !ok { for _, model := range models { if probeableMonitorModel(model) { m.PrimaryModel = strings.TrimSpace(model); break } } }
	seen := map[string]struct{}{strings.ToLower(strings.TrimSpace(m.PrimaryModel)): {}}
	extras := make([]string, 0)
	for _, model := range models { model = strings.TrimSpace(model); key:=strings.ToLower(model); if model=="" || !probeableMonitorModel(model) {continue}; if _,ok:=seen[key];ok{continue}; seen[key]=struct{}{}; extras=append(extras,model) }
	m.ExtraModels=extras
}

func probeableMonitorModel(model string) bool {
	value := strings.ToLower(strings.TrimSpace(model))
	if value == "" {
		return false
	}
	for _, part := range []string{"image", "embedding", "rerank", "whisper", "tts", "sora", "auto-review"} {
		if strings.Contains(value, part) {
			return false
		}
	}
	return true
}

// ScheduledCheckDue is evaluated while holding the distributed monitor lock.
// 自动监控在最近60秒有真实请求时不探测；空闲探测之间保留滑动冷却窗口。
func (s *ChannelMonitorService) ScheduledCheckDue(ctx context.Context, id int64) (bool, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	if !m.Enabled {
		return false, nil
	}
	now := time.Now().UTC()
	interval := time.Duration(m.IntervalSeconds) * time.Second
	if isHybridMonitor(m) {
		observations, err := s.latestGroupObservations(ctx, []int64{*m.GroupID}, now)
		if err != nil {
			return false, err
		}
		if observations[*m.GroupID] != nil {
			return false, nil
		}
		interval = monitorRealRequestWindow
	}
	if interval <= 0 {
		return false, nil
	}
	recent, err := s.repo.ListHistory(ctx, id, "", 1)
	if err != nil {
		return false, err
	}
	if len(recent) == 0 {
		return true, nil
	}
	return !recent[0].CheckedAt.After(now.Add(-interval)), nil
}

// selectPopularMonitorModel ranks bounded recent real requests, never probe traffic.
// Ties are resolved by the repository; no history preserves a supported primary.
func (s *ChannelMonitorService) selectPopularMonitorModel(ctx context.Context, m *ChannelMonitor, models []string) {
	if m == nil {
		return
	}
	supported := make(map[string]string, len(models))
	candidates := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if probeableMonitorModel(model) {
			supported[strings.ToLower(model)] = model
			candidates = append(candidates, model)
		}
	}
	if len(candidates) == 0 {
		m.ExtraModels = []string{}
		return
	}
	applyAutoMonitorModels(m, candidates)
	if s.repo == nil || m.GroupID == nil {
		return
	}
	ranked, err := s.repo.ListPopularGroupModels(ctx, *m.GroupID)
	if err != nil {
		slog.Warn("channel_monitor: popularity query failed", "group_id", *m.GroupID, "error", err)
		return
	}
	for _, model := range ranked {
		if actual, ok := supported[strings.ToLower(strings.TrimSpace(model))]; ok {
			m.PrimaryModel = actual
			return
		}
	}
}
