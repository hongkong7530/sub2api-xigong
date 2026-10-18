package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type OpsClusterController struct {
	repo    OpsRepository
	runtime OpsHAProxyRuntime
	cfg     *config.Config

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewOpsClusterController(repo OpsRepository, runtime OpsHAProxyRuntime, cfg *config.Config) *OpsClusterController {
	return &OpsClusterController{repo: repo, runtime: runtime, cfg: cfg, stopCh: make(chan struct{})}
}

func opsClusterControllerEnabled(enabled bool, nodeID string) bool {
	// Deployment selects the local HAProxy owner; node names must not pin the primary.
	return enabled && strings.TrimSpace(nodeID) != ""
}

func (c *OpsClusterController) Start() {
	if c == nil || c.cfg == nil || !opsClusterControllerEnabled(c.cfg.Ops.ClusterControllerEnabled, LoadOpsNodeIdentity().ID) {
		return
	}
	c.startOnce.Do(func() { go c.run() })
}

func (c *OpsClusterController) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() { close(c.stopCh) })
}

func (c *OpsClusterController) run() {
	c.reconcile()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.reconcile()
		case <-c.stopCh:
			return
		}
	}
}

func (c *OpsClusterController) reconcile() {
	if c == nil || c.repo == nil || c.runtime == nil || c.cfg == nil || !c.cfg.Ops.ClusterControllerEnabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	configs, err := c.repo.ListClusterNodeConfigs(ctx)
	if err != nil {
		return
	}
	statuses, err := c.repo.ListClusterNodeStatuses(ctx)
	if err != nil {
		return
	}
	statusByNode := make(map[string]*OpsClusterNodeStatus, len(statuses))
	for _, status := range statuses {
		if status != nil {
			statusByNode[status.NodeID] = status
		}
	}
	for _, nodeConfig := range configs {
		if nodeConfig == nil {
			continue
		}
		c.reconcileNode(ctx, nodeConfig, statusByNode[nodeConfig.NodeID])
	}
}

func (c *OpsClusterController) reconcileNode(ctx context.Context, nodeConfig *OpsClusterNodeConfig, status *OpsClusterNodeStatus) {
	desiredState, desiredWeight := desiredHAProxyState(nodeConfig, status, time.Now().UTC())
	backend := strings.TrimSpace(c.cfg.Ops.ClusterBackendName)
	if backend == "" {
		backend = "sub2api_nodes"
	}
	server := nodeConfig.NodeID
	// Drain first for dependency/health failures so no new request enters while
	// the runtime state is being changed. Resource pressure does not change admission.
	if desiredState == "drain" || desiredState == "maint" {
		if err := c.runtime.SetServerState(backend, server, desiredState); err != nil {
			c.recordApplyError(ctx, nodeConfig, status, err)
			return
		}
	}
	if err := c.runtime.SetServerWeight(backend, server, desiredWeight); err != nil {
		c.recordApplyError(ctx, nodeConfig, status, err)
		return
	}
	if desiredState == "ready" {
		if err := c.runtime.SetServerState(backend, server, desiredState); err != nil {
			c.recordApplyError(ctx, nodeConfig, status, err)
			return
		}
	}
	actual, err := c.runtime.GetServerState(backend, server)
	if err != nil {
		c.recordApplyError(ctx, nodeConfig, status, err)
		return
	}
	if actual.Weight != desiredWeight || !haProxyStateMatches(actual.State, desiredState) {
		c.recordApplyError(ctx, nodeConfig, status, fmt.Errorf("HAProxy readback mismatch: state=%s weight=%d", actual.State, actual.Weight))
		return
	}
	now := time.Now().UTC()
	_ = c.repo.UpdateClusterNodeAppliedState(ctx, OpsClusterAppliedStateUpdate{
		NodeID: nodeConfig.NodeID, ObservedRevision: nodeConfig.DesiredRevision,
		ApplyStatus: OpsClusterApplyApplied, AppliedAt: &now,
		ActualWeight: actual.Weight, HAProxyState: actual.State,
	})
}

func (c *OpsClusterController) recordApplyError(ctx context.Context, nodeConfig *OpsClusterNodeConfig, status *OpsClusterNodeStatus, applyErr error) {
	message := applyErr.Error()
	actualWeight := 0
	haproxyState := "unknown"
	observedRevision := int64(0)
	if status != nil {
		actualWeight = status.ActualWeight
		haproxyState = status.HAProxyState
		observedRevision = status.ObservedRevision
	}
	_ = c.repo.UpdateClusterNodeAppliedState(ctx, OpsClusterAppliedStateUpdate{
		NodeID: nodeConfig.NodeID, ObservedRevision: observedRevision,
		ApplyStatus: OpsClusterApplyError, ApplyError: &message,
		ActualWeight: actualWeight, HAProxyState: haproxyState,
	})
}

func desiredHAProxyState(cfg *OpsClusterNodeConfig, status *OpsClusterNodeStatus, now time.Time) (string, int) {
	if cfg == nil || !cfg.Enabled || cfg.DesiredWeight == 0 {
		return "maint", 0
	}
	if status == nil || now.Sub(status.HeartbeatAt) > opsClusterHeartbeatStaleAfter {
		return "drain", 0
	}
	switch status.AdmissionState {
	case OpsAdmissionReady:
		return "ready", cfg.DesiredWeight
	case OpsAdmissionThrottledResource, OpsAdmissionDrainResource:
		// Ignore legacy resource pauses after upgrade without terminating existing streams.
		return "ready", cfg.DesiredWeight
	case OpsAdmissionDrainDependency, OpsAdmissionWaitingRecovery, OpsAdmissionStale, OpsAdmissionDownHealth:
		return "drain", 0
	case OpsAdmissionDisabled:
		return "maint", 0
	default:
		return "drain", 0
	}
}

func haProxyStateMatches(actual, desired string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	desired = strings.ToLower(strings.TrimSpace(desired))
	switch desired {
	case "ready":
		return strings.Contains(actual, "up") && !strings.Contains(actual, "drain") && !strings.Contains(actual, "maint")
	case "drain":
		return strings.Contains(actual, "drain")
	case "maint":
		return strings.Contains(actual, "maint")
	default:
		return false
	}
}
