package service

import (
	"errors"
	"fmt"
	"time"
)

const (
	OpsClusterApplyPending = "pending"
	OpsClusterApplyApplied = "applied"
	OpsClusterApplyError   = "error"

	OpsAdmissionReady             = "ready"
	OpsAdmissionThrottledResource = "throttled_resource"
	// OpsAdmissionDrainResource is retained for statuses written by older binaries.
	OpsAdmissionDrainResource   = "drain_resource"
	OpsAdmissionDrainDependency = "drain_dependency"
	OpsAdmissionDownHealth      = "down_health"
	OpsAdmissionWaitingRecovery = "waiting_recovery"
	OpsAdmissionDisabled        = "disabled"
	OpsAdmissionStale           = "stale"
)

var ErrOpsClusterRevisionConflict = errors.New("ops cluster node config revision conflict")

type OpsClusterNodeConfig struct {
	NodeID                string    `json:"node_id"`
	NodeName              string    `json:"node_name"`
	Enabled               bool      `json:"enabled"`
	CPULimitPercent       float64   `json:"cpu_limit_percent"`
	MinAvailableMemoryMB  int64     `json:"min_available_memory_mb"`
	DesiredWeight         int       `json:"desired_weight"`
	RecoveryStableSeconds int       `json:"recovery_stable_seconds"`
	DesiredRevision       int64     `json:"desired_revision"`
	UpdatedByUserID       *int64    `json:"updated_by_user_id,omitempty"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type OpsClusterNodeConfigUpdate struct {
	NodeName              string
	Enabled               bool
	CPULimitPercent       float64
	MinAvailableMemoryMB  int64
	DesiredWeight         int
	RecoveryStableSeconds int
	ExpectedRevision      int64
	OperatorUserID        *int64
}

type OpsClusterNodeStatus struct {
	NodeID            string                 `json:"node_id"`
	HeartbeatAt       time.Time              `json:"heartbeat_at"`
	ObservedRevision  int64                  `json:"observed_revision"`
	ApplyStatus       string                 `json:"apply_status"`
	ApplyError        *string                `json:"apply_error,omitempty"`
	AppliedAt         *time.Time             `json:"applied_at,omitempty"`
	AdmissionState    string                 `json:"admission_state"`
	AdmissionReason   *string                `json:"admission_reason,omitempty"`
	RecoveryStartedAt *time.Time             `json:"recovery_started_at,omitempty"`
	ActualWeight      int                    `json:"actual_weight"`
	HAProxyState      string                 `json:"haproxy_state"`
	ActiveRequests    int                    `json:"active_requests"`
	QueueDepth        int                    `json:"queue_depth"`
	PostgresSyncState string                 `json:"postgres_sync_state"`
	WALReplayLagBytes *int64                 `json:"wal_replay_lag_bytes,omitempty"`
	LiveMetrics       *OpsClusterLiveMetrics `json:"live_metrics,omitempty"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type OpsClusterNode struct {
	Config         *OpsClusterNodeConfig     `json:"config"`
	Status         *OpsClusterNodeStatus     `json:"status,omitempty"`
	Metrics        *OpsSystemMetricsSnapshot `json:"metrics,omitempty"`
	RequestCount   int64                     `json:"request_count"`
	TrafficPercent float64                   `json:"traffic_percent"`
}

type OpsClusterSummary struct {
	Status             string  `json:"status"`
	StatusReason       string  `json:"status_reason,omitempty"`
	OnlineNodes        int     `json:"online_nodes"`
	AdmittedNodes      int     `json:"admitted_nodes"`
	TotalCPUCores      int     `json:"total_cpu_cores"`
	CPUUsagePercent    float64 `json:"cpu_usage_percent"`
	MemoryUsedMB       int64   `json:"memory_used_mb"`
	MemoryTotalMB      int64   `json:"memory_total_mb"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`
	TotalDesiredWeight int     `json:"total_desired_weight"`
	TotalActualWeight  int     `json:"total_actual_weight"`
	PostgresSyncState  string  `json:"postgres_sync_state"`
}

type OpsClusterSnapshot struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Summary     OpsClusterSummary `json:"summary"`
	Nodes       []*OpsClusterNode `json:"nodes"`
}

type OpsClusterAppliedStateUpdate struct {
	NodeID           string
	ObservedRevision int64
	ApplyStatus      string
	ApplyError       *string
	AppliedAt        *time.Time
	ActualWeight     int
	HAProxyState     string
}

type OpsClusterStatusUpdate struct {
	NodeID            string
	HeartbeatAt       time.Time
	ObservedRevision  int64
	ApplyStatus       string
	ApplyError        *string
	AppliedAt         *time.Time
	AdmissionState    string
	AdmissionReason   *string
	RecoveryStartedAt *time.Time
	ActualWeight      int
	HAProxyState      string
	ActiveRequests    int
	QueueDepth        int
	PostgresSyncState string
	WALReplayLagBytes *int64
	LiveMetrics       *OpsClusterLiveMetrics
}

type OpsClusterLiveMetrics = OpsSystemMetricsSnapshot

type OpsHAProxyServerState struct {
	State  string
	Weight int
}

type OpsHAProxyRuntime interface {
	SetServerState(backend, server, state string) error
	SetServerWeight(backend, server string, weight int) error
	GetServerState(backend, server string) (OpsHAProxyServerState, error)
}

func ValidateOpsClusterConfigUpdate(input OpsClusterNodeConfigUpdate, memoryTotalMB int64) error {
	if input.CPULimitPercent < 50 || input.CPULimitPercent > 100 {
		return fmt.Errorf("cpu limit must be between 50 and 100")
	}
	maxMemory := memoryTotalMB / 2
	if maxMemory < 128 {
		maxMemory = 128
	}
	if input.MinAvailableMemoryMB < 128 || input.MinAvailableMemoryMB > maxMemory {
		return fmt.Errorf("minimum available memory must be between 128 and %d MiB", maxMemory)
	}
	if input.DesiredWeight < 0 || input.DesiredWeight > 20 {
		return fmt.Errorf("weight must be between 0 and 20")
	}
	if input.RecoveryStableSeconds < 10 || input.RecoveryStableSeconds > 600 {
		return fmt.Errorf("recovery stable time must be between 10 and 600 seconds")
	}
	if input.ExpectedRevision < 1 {
		return fmt.Errorf("expected revision must be positive")
	}
	return nil
}
