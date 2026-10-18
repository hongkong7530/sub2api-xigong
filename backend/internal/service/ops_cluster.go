package service

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const opsClusterHeartbeatStaleAfter = 15 * time.Second

func (s *OpsService) GetPublicClusterHealth(ctx context.Context, now time.Time) (string, int, error) {
	if s == nil || s.opsRepo == nil {
		return "unknown", 0, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	configs, err := s.opsRepo.ListClusterNodeConfigs(ctx)
	if err != nil {
		return "unknown", 0, err
	}
	statuses, err := s.opsRepo.ListClusterNodeStatuses(ctx)
	if err != nil {
		return "unknown", 0, err
	}
	statusByNode := make(map[string]*OpsClusterNodeStatus, len(statuses))
	for _, status := range statuses {
		if status != nil {
			statusByNode[status.NodeID] = status
		}
	}
	enabled := 0
	healthy := 0
	for _, cfg := range configs {
		if cfg == nil || !cfg.Enabled {
			continue
		}
		enabled++
		status := statusByNode[cfg.NodeID]
		if status != nil && !status.HeartbeatAt.IsZero() && now.Sub(status.HeartbeatAt) <= opsClusterHeartbeatStaleAfter {
			healthy++
		}
	}
	if enabled == 0 || healthy == 0 {
		return "unknown", 0, nil
	}
	if healthy == enabled {
		return "operational", healthy, nil
	}
	return "degraded", healthy, nil
}

func (s *OpsService) GetClusterSnapshot(ctx context.Context, start, end time.Time) (*OpsClusterSnapshot, error) {
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	configs, err := s.opsRepo.ListClusterNodeConfigs(ctx)
	if err != nil {
		return nil, err
	}
	statuses, err := s.opsRepo.ListClusterNodeStatuses(ctx)
	if err != nil {
		return nil, err
	}
	metrics, err := s.opsRepo.ListLatestSystemMetrics(ctx, 1)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	counts, err := s.opsRepo.GetClusterRequestCounts(ctx, start, end)
	if err != nil {
		return nil, err
	}

	statusByNode := make(map[string]*OpsClusterNodeStatus, len(statuses))
	for _, item := range statuses {
		if item != nil {
			statusByNode[item.NodeID] = item
		}
	}
	metricsByNode := make(map[string]*OpsSystemMetricsSnapshot, len(metrics))
	for _, item := range metrics {
		if item != nil {
			metricsByNode[item.NodeID] = item
		}
	}
	totalRequests := int64(0)
	for _, count := range counts {
		totalRequests += count
	}

	now := time.Now().UTC()
	snapshot := &OpsClusterSnapshot{GeneratedAt: now, Nodes: make([]*OpsClusterNode, 0, len(configs))}
	snapshot.Summary.Status = "healthy"
	if s.cfg != nil && s.cfg.Ops.StrictSyncRequired {
		snapshot.Summary.PostgresSyncState = "missing"
	} else {
		snapshot.Summary.PostgresSyncState = "not_required"
	}
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		status := statusByNode[cfg.NodeID]
		metrics := metricsByNode[cfg.NodeID]
		if status != nil && status.LiveMetrics != nil && !status.LiveMetrics.CreatedAt.IsZero() {
			metrics = status.LiveMetrics
		}
		attachOpsMetricsLimits(s, metrics)
		node := &OpsClusterNode{Config: cfg, Status: status, Metrics: metrics, RequestCount: counts[cfg.NodeID]}
		if totalRequests > 0 {
			node.TrafficPercent = math.Round(float64(node.RequestCount)*1000/float64(totalRequests)) / 10
		}
		snapshot.Nodes = append(snapshot.Nodes, node)
		snapshot.Summary.TotalDesiredWeight += cfg.DesiredWeight
		if node.Status != nil {
			snapshot.Summary.TotalActualWeight += node.Status.ActualWeight
		}
		if node.Status == nil || now.Sub(node.Status.HeartbeatAt) > opsClusterHeartbeatStaleAfter {
			snapshot.Summary.Status = "degraded"
			snapshot.Summary.StatusReason = "node heartbeat is stale"
			continue
		}
		snapshot.Summary.OnlineNodes++
		if node.Status.AdmissionState == OpsAdmissionReady {
			snapshot.Summary.AdmittedNodes++
		}
		if s.cfg != nil && s.cfg.Ops.StrictSyncRequired && node.Status.PostgresSyncState != "sync" {
			snapshot.Summary.PostgresSyncState = node.Status.PostgresSyncState
			snapshot.Summary.Status = "write_paused"
			snapshot.Summary.StatusReason = "strict synchronous standby is unavailable"
		}
		if s.cfg != nil && s.cfg.Ops.StrictSyncRequired && node.Status.PostgresSyncState == "sync" {
			snapshot.Summary.PostgresSyncState = "sync"
		}
		if node.Status.AdmissionState != OpsAdmissionReady && node.Status.AdmissionState != OpsAdmissionThrottledResource && node.Status.AdmissionState != OpsAdmissionDrainResource && snapshot.Summary.Status == "healthy" {
			snapshot.Summary.Status = "degraded"
			snapshot.Summary.StatusReason = strings.TrimSpace(pointerString(node.Status.AdmissionReason))
		}
		if node.Metrics != nil {
			if node.Metrics.CPUCores != nil && node.Metrics.CPUUsagePercent != nil {
				snapshot.Summary.TotalCPUCores += *node.Metrics.CPUCores
				snapshot.Summary.CPUUsagePercent += float64(*node.Metrics.CPUCores) * *node.Metrics.CPUUsagePercent
			}
			if node.Metrics.MemoryUsedMB != nil {
				snapshot.Summary.MemoryUsedMB += *node.Metrics.MemoryUsedMB
			}
			if node.Metrics.MemoryTotalMB != nil {
				snapshot.Summary.MemoryTotalMB += *node.Metrics.MemoryTotalMB
			}
		}
	}
	if snapshot.Summary.TotalCPUCores > 0 {
		snapshot.Summary.CPUUsagePercent = math.Round(snapshot.Summary.CPUUsagePercent/float64(snapshot.Summary.TotalCPUCores)*10) / 10
	}
	if snapshot.Summary.MemoryTotalMB > 0 {
		snapshot.Summary.MemoryUsagePercent = math.Round(float64(snapshot.Summary.MemoryUsedMB)/float64(snapshot.Summary.MemoryTotalMB)*1000) / 10
	}
	sort.Slice(snapshot.Nodes, func(i, j int) bool { return snapshot.Nodes[i].Config.NodeID < snapshot.Nodes[j].Config.NodeID })
	return snapshot, nil
}

func (s *OpsService) GetClusterLiveSnapshot(ctx context.Context) (*OpsClusterSnapshot, error) {
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	configs, err := s.opsRepo.ListClusterNodeConfigs(ctx)
	if err != nil {
		return nil, err
	}
	statuses, err := s.opsRepo.ListClusterNodeStatuses(ctx)
	if err != nil {
		return nil, err
	}
	statusByNode := make(map[string]*OpsClusterNodeStatus, len(statuses))
	for _, status := range statuses {
		if status != nil {
			statusByNode[status.NodeID] = status
		}
	}
	now := time.Now().UTC()
	snapshot := &OpsClusterSnapshot{GeneratedAt: now, Nodes: make([]*OpsClusterNode, 0, len(configs))}
	snapshot.Summary.Status = "healthy"
	if s.cfg != nil && s.cfg.Ops.StrictSyncRequired {
		snapshot.Summary.PostgresSyncState = "missing"
	} else {
		snapshot.Summary.PostgresSyncState = "not_required"
	}
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		status := statusByNode[cfg.NodeID]
		var metrics *OpsSystemMetricsSnapshot
		if status != nil {
			metrics = status.LiveMetrics
		}
		attachOpsMetricsLimits(s, metrics)
		node := &OpsClusterNode{Config: cfg, Status: status, Metrics: metrics}
		snapshot.Nodes = append(snapshot.Nodes, node)
		snapshot.Summary.TotalDesiredWeight += cfg.DesiredWeight
		if status == nil || now.Sub(status.HeartbeatAt) > opsClusterHeartbeatStaleAfter {
			snapshot.Summary.Status = "degraded"
			snapshot.Summary.StatusReason = "node heartbeat is stale"
			continue
		}
		snapshot.Summary.OnlineNodes++
		if status.AdmissionState == OpsAdmissionReady {
			snapshot.Summary.AdmittedNodes++
		}
		snapshot.Summary.TotalActualWeight += status.ActualWeight
		if s.cfg != nil && s.cfg.Ops.StrictSyncRequired {
			if status.PostgresSyncState == "sync" {
				snapshot.Summary.PostgresSyncState = "sync"
			} else {
				snapshot.Summary.PostgresSyncState = status.PostgresSyncState
				snapshot.Summary.Status = "write_paused"
				snapshot.Summary.StatusReason = "strict synchronous standby is unavailable"
			}
		}
		if status.AdmissionState != OpsAdmissionReady && status.AdmissionState != OpsAdmissionThrottledResource && status.AdmissionState != OpsAdmissionDrainResource && snapshot.Summary.Status == "healthy" {
			snapshot.Summary.Status = "degraded"
			snapshot.Summary.StatusReason = strings.TrimSpace(pointerString(status.AdmissionReason))
		}
		if metrics != nil {
			if metrics.CPUCores != nil && metrics.CPUUsagePercent != nil {
				snapshot.Summary.TotalCPUCores += *metrics.CPUCores
				snapshot.Summary.CPUUsagePercent += float64(*metrics.CPUCores) * *metrics.CPUUsagePercent
			}
			if metrics.MemoryUsedMB != nil {
				snapshot.Summary.MemoryUsedMB += *metrics.MemoryUsedMB
			}
			if metrics.MemoryTotalMB != nil {
				snapshot.Summary.MemoryTotalMB += *metrics.MemoryTotalMB
			}
		}
	}
	if snapshot.Summary.TotalCPUCores > 0 {
		snapshot.Summary.CPUUsagePercent = math.Round(snapshot.Summary.CPUUsagePercent/float64(snapshot.Summary.TotalCPUCores)*10) / 10
	}
	if snapshot.Summary.MemoryTotalMB > 0 {
		snapshot.Summary.MemoryUsagePercent = math.Round(float64(snapshot.Summary.MemoryUsedMB)/float64(snapshot.Summary.MemoryTotalMB)*1000) / 10
	}
	sort.Slice(snapshot.Nodes, func(i, j int) bool { return snapshot.Nodes[i].Config.NodeID < snapshot.Nodes[j].Config.NodeID })
	return snapshot, nil
}

func (s *OpsService) GetClusterNode(ctx context.Context, nodeID string, start, end time.Time) (*OpsClusterNode, error) {
	snapshot, err := s.GetClusterSnapshot(ctx, start, end)
	if err != nil {
		return nil, err
	}
	for _, node := range snapshot.Nodes {
		if node != nil && node.Config != nil && node.Config.NodeID == nodeID {
			return node, nil
		}
	}
	return nil, infraerrors.NotFound("OPS_CLUSTER_NODE_NOT_FOUND", "Cluster node not found")
}

func (s *OpsService) UpdateClusterNodeConfig(ctx context.Context, nodeID string, input OpsClusterNodeConfigUpdate) (*OpsClusterNodeConfig, error) {
	currentNode, err := s.GetClusterNode(ctx, nodeID, time.Now().Add(-time.Minute), time.Now())
	if err != nil {
		return nil, err
	}
	memoryTotal := int64(0)
	if currentNode.Metrics != nil && currentNode.Metrics.MemoryTotalMB != nil {
		memoryTotal = *currentNode.Metrics.MemoryTotalMB
	}
	if memoryTotal == 0 {
		memoryTotal = input.MinAvailableMemoryMB * 2
	}
	if input.NodeName == "" {
		input.NodeName = currentNode.Config.NodeName
	}
	if err := ValidateOpsClusterConfigUpdate(input, memoryTotal); err != nil {
		return nil, infraerrors.BadRequest("OPS_CLUSTER_CONFIG_INVALID", err.Error())
	}
	updated, err := s.opsRepo.UpdateClusterNodeConfig(ctx, nodeID, input)
	if errors.Is(err, ErrOpsClusterRevisionConflict) {
		return nil, infraerrors.Conflict("OPS_CLUSTER_REVISION_CONFLICT", "Cluster node configuration was changed by another administrator")
	}
	return updated, err
}

func (s *OpsService) ReapplyClusterNodeConfig(ctx context.Context, nodeID string, expectedRevision int64, operatorUserID *int64) (*OpsClusterNodeConfig, error) {
	cfg, err := s.opsRepo.GetClusterNodeConfig(ctx, nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_CLUSTER_NODE_NOT_FOUND", "Cluster node not found")
		}
		return nil, err
	}
	return s.UpdateClusterNodeConfig(ctx, nodeID, OpsClusterNodeConfigUpdate{
		NodeName: cfg.NodeName, Enabled: cfg.Enabled, CPULimitPercent: cfg.CPULimitPercent,
		MinAvailableMemoryMB: cfg.MinAvailableMemoryMB, DesiredWeight: cfg.DesiredWeight,
		RecoveryStableSeconds: cfg.RecoveryStableSeconds, ExpectedRevision: expectedRevision,
		OperatorUserID: operatorUserID,
	})
}

func EvaluateOpsAdmission(cfg *OpsClusterNodeConfig, metrics *OpsSystemMetricsSnapshot, dependenciesOK bool, previous *OpsClusterNodeStatus, now time.Time) (state, reason string) {
	if cfg == nil || !cfg.Enabled || cfg.DesiredWeight == 0 {
		return OpsAdmissionDisabled, "node is disabled by configuration"
	}
	if metrics == nil || metrics.CreatedAt.IsZero() || now.Sub(metrics.CreatedAt) > opsClusterHeartbeatStaleAfter {
		return OpsAdmissionStale, "node metrics are stale"
	}
	if metrics.DBOK != nil && !*metrics.DBOK || metrics.RedisOK != nil && !*metrics.RedisOK || !dependenciesOK {
		return OpsAdmissionDrainDependency, "shared state dependency is unavailable"
	}
	// Resource measurements are advisory; only dependency failures suspend admission.
	if previous != nil && previous.AdmissionState != OpsAdmissionReady &&
		previous.AdmissionState != OpsAdmissionThrottledResource &&
		previous.AdmissionState != OpsAdmissionDrainResource {
		if previous.RecoveryStartedAt == nil || now.Sub(*previous.RecoveryStartedAt) < time.Duration(cfg.RecoveryStableSeconds)*time.Second {
			return OpsAdmissionWaitingRecovery, "waiting for the configured dependency recovery window"
		}
	}
	return OpsAdmissionReady, ""
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
