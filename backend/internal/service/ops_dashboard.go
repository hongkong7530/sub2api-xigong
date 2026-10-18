package service

import (
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// opsDashboardMetricsWindowMinutes selects the metric cadence for the dashboard header's
// per-node system snapshots. The repository collapses this window to the newest row of
// each node, so the value picks 1-minute rows rather than limiting how many nodes return.
const opsDashboardMetricsWindowMinutes = 1

func (s *OpsService) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}

	// Resolve query mode (requested via query param, or DB default).
	filter.QueryMode = s.resolveOpsQueryMode(ctx, filter.QueryMode)

	overview, err := s.opsRepo.GetDashboardOverview(ctx, filter)
	if err != nil && shouldFallbackOpsPreagg(filter, err) {
		rawFilter := cloneOpsFilterWithMode(filter, OpsQueryModeRaw)
		overview, err = s.opsRepo.GetDashboardOverview(ctx, rawFilter)
	}
	if err != nil {
		if errors.Is(err, ErrOpsPreaggregatedNotPopulated) {
			return nil, infraerrors.Conflict("OPS_PREAGG_NOT_READY", "Pre-aggregated ops metrics are not populated yet")
		}
		return nil, err
	}

	overview.ResponseNodeID = LoadOpsNodeIdentity().ID
	overview.MetricsStaleAfterSeconds = 180
	if s.settingRepo != nil {
		if raw, e := s.settingRepo.GetValue(ctx, SettingKeyOpsMetricsIntervalSeconds); e == nil {
			if seconds, e := strconv.Atoi(raw); e == nil {
				overview.MetricsStaleAfterSeconds = max(180, min(3600, seconds)*2+60)
			}
		}
	}
	// Machine telemetry belongs to its node, never to the legacy routing registry.
	if metrics, metricsErr := s.opsRepo.ListLatestSystemMetrics(ctx, opsDashboardMetricsWindowMinutes); metricsErr == nil {
		sort.Slice(metrics, func(i, j int) bool { return metrics[i].NodeID < metrics[j].NodeID })
		overview.SystemMetricsNodes = metrics
		for _, item := range metrics {
			attachOpsMetricsLimits(s, item)
			if item.NodeID == overview.ResponseNodeID {
				overview.SystemMetrics = item
			}
		}
	} else {
		log.Printf("[Ops] ListLatestSystemMetrics failed: %v", metricsErr)
	}

	if heartbeats, err := s.opsRepo.ListJobHeartbeats(ctx); err == nil {
		overview.JobHeartbeats = heartbeats
	} else {
		log.Printf("[Ops] ListJobHeartbeats failed: %v", err)
	}

	if cluster, clusterErr := s.GetClusterSnapshot(ctx, filter.StartTime, filter.EndTime); clusterErr == nil {
		overview.Cluster = cluster
	} else {
		log.Printf("[Ops] GetClusterSnapshot failed: %v", clusterErr)
	}

	overview.HealthScore = computeDashboardHealthScore(time.Now().UTC(), overview)

	return overview, nil
}

func attachOpsMetricsLimits(s *OpsService, metrics *OpsSystemMetricsSnapshot) {
	if s == nil || s.cfg == nil || metrics == nil || metrics.NodeID != LoadOpsNodeIdentity().ID {
		return
	}
	if s.cfg.Database.MaxOpenConns > 0 {
		metrics.DBMaxOpenConns = intPtr(s.cfg.Database.MaxOpenConns)
	}
	if s.cfg.Redis.PoolSize > 0 {
		metrics.RedisPoolSize = intPtr(s.cfg.Redis.PoolSize)
	}
}

func (s *OpsService) resolveOpsQueryMode(ctx context.Context, requested OpsQueryMode) OpsQueryMode {
	if requested.IsValid() {
		// Allow "auto" to be disabled via config until preagg is proven stable in production.
		// Forced `preagg` via query param still works.
		if requested == OpsQueryModeAuto && s != nil && s.cfg != nil && !s.cfg.Ops.UsePreaggregatedTables {
			return OpsQueryModeRaw
		}
		return requested
	}

	mode := OpsQueryModeAuto
	if s != nil && s.settingRepo != nil {
		if raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsQueryModeDefault); err == nil {
			mode = ParseOpsQueryMode(raw)
		}
	}

	if mode == OpsQueryModeAuto && s != nil && s.cfg != nil && !s.cfg.Ops.UsePreaggregatedTables {
		return OpsQueryModeRaw
	}
	return mode
}
