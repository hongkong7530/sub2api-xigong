package service

import (
	"context"
	"fmt"
	"time"
)

const monitorRealRequestWindow = 60 * time.Second

// ChannelMonitorObservation contains only public health facts, never request text or identities.
type ChannelMonitorObservation struct {
	Model        string
	FirstTokenMs *int
	DurationMs   *int
	ObservedAt   time.Time
	Failed       bool
}

// ChannelMonitorObservationRepository reads terminal real requests across all gateway nodes.
// Monitor-owned keys and business-limit rejections do not represent upstream health.
type ChannelMonitorObservationRepository interface {
	ListLatestGroupObservations(ctx context.Context, groupIDs []int64, since time.Time) (map[int64]*ChannelMonitorObservation, error)
}

func (s *ChannelMonitorService) latestGroupObservations(ctx context.Context, groupIDs []int64, now time.Time) (map[int64]*ChannelMonitorObservation, error) {
	if len(groupIDs) == 0 {
		return map[int64]*ChannelMonitorObservation{}, nil
	}
	repo, ok := s.repo.(ChannelMonitorObservationRepository)
	if !ok {
		return nil, fmt.Errorf("real-request monitor observations are not configured")
	}
	return repo.ListLatestGroupObservations(ctx, groupIDs, now.Add(-monitorRealRequestWindow))
}

func isHybridMonitor(m *ChannelMonitor) bool {
	return m != nil && m.GroupID != nil && (defaultKeyProvisionMode(m.KeyProvisionMode) == "auto" || m.ProbeMode == MonitorProbeModePassive) && defaultCheckMode(m.CheckMode) != MonitorCheckModeQuota
}

func observationCheckResult(o *ChannelMonitorObservation) *CheckResult {
	result := &CheckResult{Model: o.Model, CheckedAt: o.ObservedAt, FirstTokenMs: o.FirstTokenMs, LatencyMs: o.DurationMs, Status: MonitorStatusOperational}
	if o.Failed {
		result.Status = MonitorStatusError
		result.Message = "latest real request failed"
	} else if o.FirstTokenMs != nil {
		finalizeFirstTokenStatus(result, time.Duration(*o.FirstTokenMs)*time.Millisecond, *o.FirstTokenMs)
	} else if o.DurationMs != nil {
		finalizeOperationalOrDegraded(result, time.Duration(*o.DurationMs)*time.Millisecond, *o.DurationMs)
	}
	return result
}

func applyRealObservation(v *UserMonitorView, o *ChannelMonitorObservation) {
	if o == nil {
		return
	}
	// A later probe remains authoritative until another real request arrives.
	if len(v.Timeline) > 0 && v.Timeline[0].CheckedAt.After(o.ObservedAt) {
		return
	}
	result := observationCheckResult(o)
	v.PrimaryModel = o.Model
	v.PrimaryStatus = result.Status
	v.OverallStatus = result.Status
	v.PrimaryFirstTokenMs = o.FirstTokenMs
	v.PrimaryLatencyMs = o.DurationMs
	v.PrimaryPingLatencyMs = nil
	v.StatusSource = "user_request"
	v.StatusObservedAt = &o.ObservedAt
	v.Timeline = append([]UserMonitorTimelinePoint{{Status: result.Status, LatencyMs: o.DurationMs, FirstTokenMs: o.FirstTokenMs, CheckedAt: o.ObservedAt}}, v.Timeline...)
	if len(v.Timeline) > monitorTimelineMaxPoints {
		v.Timeline = v.Timeline[:monitorTimelineMaxPoints]
	}
}
