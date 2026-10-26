package service

import (
	"context"
	"sort"
)

// ChannelMonitorHealth is a read-only union of real terminal requests and active checks.
// Public consumers receive health facts only; neither request bodies nor identities are exposed.
type ChannelMonitorHealth struct {
	Recent       []*ChannelMonitorHistoryEntry
	Availability *ChannelMonitorAvailability
	Models       map[string]*ChannelMonitorAvailability
}

type ChannelMonitorHealthRepository interface {
	ReadMonitorHealth(ctx context.Context, monitorIDs []int64, windowDays, recentLimit int) (map[int64]*ChannelMonitorHealth, error)
}

// MonitorSummaries uses group-wide observations for automatic monitors, even when the selected probe model changes.
func (s *ChannelMonitorService) MonitorSummaries(ctx context.Context, monitors []*ChannelMonitor) map[int64]MonitorStatusSummary {
	ids, primary, extras := collectMonitorIndexes(monitors)
	out := s.BatchMonitorStatusSummary(ctx, ids, primary, extras)
	health, err := s.readHybridHealth(ctx, monitors, 7, 1)
	for _, m := range monitors {
		if !isHybridMonitor(m) {
			continue
		}
		summary := out[m.ID]
		if err != nil {
			applyAdminMonitorHealth(&summary, nil)
		} else {
			applyAdminMonitorHealth(&summary, health[m.ID])
		}
		out[m.ID] = summary
	}
	return out
}

// 没有最新事实时保持未知，不能把旧探针状态或旧延迟贴到新的分组状态上。
func applyAdminMonitorHealth(summary *MonitorStatusSummary, health *ChannelMonitorHealth) {
	summary.PrimaryStatus, summary.OverallStatus = "", ""
	summary.PrimaryLatencyMs, summary.PrimaryFirstTokenMs = nil, nil
	summary.StatusModel, summary.StatusSource, summary.StatusObservedAt = "", "", nil
	summary.Availability7d, summary.AvailabilitySamples7d = 0, 0
	if health == nil {
		return
	}
	if health.Availability != nil {
		summary.Availability7d, summary.AvailabilitySamples7d = health.Availability.AvailabilityPct, health.Availability.TotalChecks
	}
	if len(health.Recent) == 0 {
		return
	}
	latest := health.Recent[0]
	summary.PrimaryStatus, summary.OverallStatus = latest.Status, latest.Status
	summary.PrimaryLatencyMs, summary.PrimaryFirstTokenMs = latest.LatencyMs, latest.FirstTokenMs
	summary.StatusModel, summary.StatusSource = latest.Model, latest.Source
	observed := latest.CheckedAt
	summary.StatusObservedAt = &observed
}

func (s *ChannelMonitorService) readHybridHealth(ctx context.Context, monitors []*ChannelMonitor, days, limit int) (map[int64]*ChannelMonitorHealth, error) {
	ids := make([]int64, 0, len(monitors))
	for _, m := range monitors {
		if isHybridMonitor(m) {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return map[int64]*ChannelMonitorHealth{}, nil
	}
	repo, ok := s.repo.(ChannelMonitorHealthRepository)
	if !ok {
		return nil, nil
	}
	return repo.ReadMonitorHealth(ctx, ids, days, limit)
}

func (s *ChannelMonitorService) hybridUserDetail(ctx context.Context, m *ChannelMonitor, detail *UserMonitorDetail) (*UserMonitorDetail, error) {
	repo, ok := s.repo.(ChannelMonitorHealthRepository)
	if !ok {
		return detail, nil
	}
	detail.GroupAvailability = make(map[int]*ChannelMonitorAvailability, 3)
	byModel := make(map[string]*ModelDetail)
	order := make([]string, 0)
	for _, d := range detail.Models {
		// Passive mode must not retain a model's old probe-only status after switching modes.
		if m.ProbeMode == MonitorProbeModePassive {
			continue
		}
		copy := d
		copy.Availability7d, copy.Availability15d, copy.Availability30d = 0, 0, 0
		copy.Samples7d, copy.Samples15d, copy.Samples30d = 0, 0, 0
		byModel[d.Model] = &copy
		order = append(order, d.Model)
	}
	for _, days := range []int{7, 15, 30} {
		limit := 0
		if days == 30 {
			limit = 60
		}
		all, err := repo.ReadMonitorHealth(ctx, []int64{m.ID}, days, limit)
		if err != nil {
			return nil, err
		}
		health := all[m.ID]
		if health == nil {
			continue
		}
		detail.GroupAvailability[days] = health.Availability
		modelNames := make([]string, 0, len(health.Models))
		for model := range health.Models {
			modelNames = append(modelNames, model)
		}
		sort.Strings(modelNames)
		for _, model := range modelNames {
			a := health.Models[model]
			d := byModel[model]
			if d == nil {
				d = &ModelDetail{Model: model}
				byModel[model] = d
				order = append(order, model)
			}
			switch days {
			case 7:
				d.Availability7d, d.Samples7d, d.AvgLatency7dMs = a.AvailabilityPct, a.TotalChecks, a.AvgLatencyMs
			case 15:
				d.Availability15d, d.Samples15d = a.AvailabilityPct, a.TotalChecks
			case 30:
				d.Availability30d, d.Samples30d = a.AvailabilityPct, a.TotalChecks
			}
		}
		seen := make(map[string]bool)
		for _, e := range health.Recent {
			if seen[e.Model] {
				continue
			}
			seen[e.Model] = true
			d := byModel[e.Model]
			d.LatestStatus, d.LatestLatencyMs, d.LatestFirstTokenMs = e.Status, e.LatencyMs, e.FirstTokenMs
			d.LatestOutputTokensPerSecond = e.OutputTokensPerSecond
		}
	}
	detail.Models = make([]ModelDetail, 0, len(order))
	for _, model := range order {
		detail.Models = append(detail.Models, *byModel[model])
	}
	return detail, nil
}

func applyMonitorHealth(view *UserMonitorView, health *ChannelMonitorHealth) {
	if health == nil {
		return
	}
	view.Timeline = buildTimelinePoints(health.Recent)
	view.Availability7d = health.Availability.AvailabilityPct
	view.AvailabilitySamples7d = health.Availability.TotalChecks
	if len(health.Recent) == 0 {
		view.PrimaryOutputTokensPerSecond = nil
		view.PrimaryStatus, view.OverallStatus, view.StatusSource = "", "", ""
		view.PrimaryLatencyMs, view.PrimaryFirstTokenMs, view.PrimaryPingLatencyMs, view.StatusObservedAt = nil, nil, nil, nil
		return
	}
	latest := health.Recent[0]
	view.PrimaryModel = latest.Model
	view.PrimaryStatus = latest.Status
	view.OverallStatus = latest.Status
	view.PrimaryLatencyMs = latest.LatencyMs
	view.PrimaryFirstTokenMs = latest.FirstTokenMs
	view.PrimaryOutputTokensPerSecond = latest.OutputTokensPerSecond
	view.PrimaryPingLatencyMs = nil
	view.StatusSource = latest.Source
	view.StatusObservedAt = &latest.CheckedAt
}
