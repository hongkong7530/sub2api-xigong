package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const opsClusterNodeConfigSelect = `
SELECT node_id, node_name, enabled, cpu_limit_percent,
       min_available_memory_mb, desired_weight, recovery_stable_seconds,
       desired_revision, updated_by_user_id, updated_at
FROM ops_cluster_node_configs`

func scanOpsClusterNodeConfig(scanner interface{ Scan(...any) error }) (*service.OpsClusterNodeConfig, error) {
	var item service.OpsClusterNodeConfig
	var updatedBy sql.NullInt64
	if err := scanner.Scan(
		&item.NodeID, &item.NodeName, &item.Enabled, &item.CPULimitPercent,
		&item.MinAvailableMemoryMB, &item.DesiredWeight, &item.RecoveryStableSeconds,
		&item.DesiredRevision, &updatedBy, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if updatedBy.Valid {
		value := updatedBy.Int64
		item.UpdatedByUserID = &value
	}
	return &item, nil
}

func (r *opsRepository) GetClusterNodeConfig(ctx context.Context, nodeID string) (*service.OpsClusterNodeConfig, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	return scanOpsClusterNodeConfig(r.db.QueryRowContext(ctx, opsClusterNodeConfigSelect+" WHERE node_id = $1", strings.TrimSpace(nodeID)))
}

func (r *opsRepository) ListClusterNodeConfigs(ctx context.Context) ([]*service.OpsClusterNodeConfig, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	rows, err := r.db.QueryContext(ctx, opsClusterNodeConfigSelect+" ORDER BY node_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*service.OpsClusterNodeConfig, 0, 2)
	for rows.Next() {
		item, err := scanOpsClusterNodeConfig(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *opsRepository) UpdateClusterNodeConfig(ctx context.Context, nodeID string, input service.OpsClusterNodeConfigUpdate) (_ *service.OpsClusterNodeConfig, err error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := scanOpsClusterNodeConfig(tx.QueryRowContext(ctx, opsClusterNodeConfigSelect+" WHERE node_id = $1 FOR UPDATE", nodeID))
	if err != nil {
		return nil, err
	}
	if current.DesiredRevision != input.ExpectedRevision {
		return nil, service.ErrOpsClusterRevisionConflict
	}
	oldJSON, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}

	updated, err := scanOpsClusterNodeConfig(tx.QueryRowContext(ctx, `
UPDATE ops_cluster_node_configs
SET node_name = $2,
    enabled = $3,
    cpu_limit_percent = $4,
    min_available_memory_mb = $5,
    desired_weight = $6,
    recovery_stable_seconds = $7,
    desired_revision = desired_revision + 1,
    updated_by_user_id = $8,
    updated_at = NOW()
WHERE node_id = $1
RETURNING node_id, node_name, enabled, cpu_limit_percent,
          min_available_memory_mb, desired_weight, recovery_stable_seconds,
          desired_revision, updated_by_user_id, updated_at
`, nodeID, input.NodeName, input.Enabled, input.CPULimitPercent,
		input.MinAvailableMemoryMB, input.DesiredWeight, input.RecoveryStableSeconds,
		opsNullInt64(input.OperatorUserID)))
	if err != nil {
		return nil, err
	}
	newJSON, err := json.Marshal(updated)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO ops_cluster_config_audit (
  node_id, previous_revision, desired_revision, old_config, new_config, operator_user_id
) VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6)
`, nodeID, current.DesiredRevision, updated.DesiredRevision, string(oldJSON), string(newJSON), opsNullInt64(input.OperatorUserID)); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE ops_cluster_node_status
SET apply_status = 'pending', apply_error = NULL, updated_at = NOW()
WHERE node_id = $1
`, nodeID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

const opsClusterNodeStatusSelect = `
SELECT node_id, heartbeat_at, observed_revision, apply_status, apply_error,
       applied_at, admission_state, admission_reason, recovery_started_at, actual_weight,
       haproxy_state, active_requests, queue_depth, postgres_sync_state,
       wal_replay_lag_bytes, live_metrics, updated_at
FROM ops_cluster_node_status`

func scanOpsClusterNodeStatus(scanner interface{ Scan(...any) error }) (*service.OpsClusterNodeStatus, error) {
	var item service.OpsClusterNodeStatus
	var applyError, admissionReason sql.NullString
	var appliedAt, recoveryStartedAt sql.NullTime
	var replayLag sql.NullInt64
	var liveMetricsJSON []byte
	if err := scanner.Scan(
		&item.NodeID, &item.HeartbeatAt, &item.ObservedRevision, &item.ApplyStatus,
		&applyError, &appliedAt, &item.AdmissionState, &admissionReason, &recoveryStartedAt,
		&item.ActualWeight, &item.HAProxyState, &item.ActiveRequests,
		&item.QueueDepth, &item.PostgresSyncState, &replayLag, &liveMetricsJSON, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if applyError.Valid {
		item.ApplyError = &applyError.String
	}
	if admissionReason.Valid {
		item.AdmissionReason = &admissionReason.String
	}
	if recoveryStartedAt.Valid {
		value := recoveryStartedAt.Time
		item.RecoveryStartedAt = &value
	}
	if appliedAt.Valid {
		value := appliedAt.Time
		item.AppliedAt = &value
	}
	if replayLag.Valid {
		value := replayLag.Int64
		item.WALReplayLagBytes = &value
	}
	if len(liveMetricsJSON) > 0 {
		var live service.OpsClusterLiveMetrics
		if err := json.Unmarshal(liveMetricsJSON, &live); err != nil {
			return nil, fmt.Errorf("decode cluster live metrics: %w", err)
		}
		item.LiveMetrics = &live
	}
	return &item, nil
}

func (r *opsRepository) GetClusterNodeStatus(ctx context.Context, nodeID string) (*service.OpsClusterNodeStatus, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	return scanOpsClusterNodeStatus(r.db.QueryRowContext(ctx, opsClusterNodeStatusSelect+" WHERE node_id = $1", strings.TrimSpace(nodeID)))
}

func (r *opsRepository) ListClusterNodeStatuses(ctx context.Context) ([]*service.OpsClusterNodeStatus, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	rows, err := r.db.QueryContext(ctx, opsClusterNodeStatusSelect+" ORDER BY node_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*service.OpsClusterNodeStatus, 0, 2)
	for rows.Next() {
		item, err := scanOpsClusterNodeStatus(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *opsRepository) UpsertClusterNodeStatus(ctx context.Context, input service.OpsClusterStatusUpdate) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input.HeartbeatAt.IsZero() {
		input.HeartbeatAt = time.Now().UTC()
	}
	liveMetricsJSON, err := json.Marshal(input.LiveMetrics)
	if err != nil {
		return fmt.Errorf("encode cluster live metrics: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO ops_cluster_node_status (
  node_id, heartbeat_at, observed_revision, apply_status, apply_error,
  applied_at, admission_state, admission_reason, recovery_started_at, actual_weight,
  haproxy_state, active_requests, queue_depth, postgres_sync_state,
  wal_replay_lag_bytes, live_metrics, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  heartbeat_at = EXCLUDED.heartbeat_at,
  observed_revision = GREATEST(ops_cluster_node_status.observed_revision, EXCLUDED.observed_revision),
  admission_state = EXCLUDED.admission_state,
  admission_reason = EXCLUDED.admission_reason,
  recovery_started_at = EXCLUDED.recovery_started_at,
  active_requests = EXCLUDED.active_requests,
  queue_depth = EXCLUDED.queue_depth,
  postgres_sync_state = EXCLUDED.postgres_sync_state,
  wal_replay_lag_bytes = EXCLUDED.wal_replay_lag_bytes,
  live_metrics = EXCLUDED.live_metrics,
  updated_at = NOW()
`, input.NodeID, input.HeartbeatAt, input.ObservedRevision,
		defaultString(input.ApplyStatus, service.OpsClusterApplyPending), opsNullString(input.ApplyError),
		opsNullTime(input.AppliedAt), defaultString(input.AdmissionState, service.OpsAdmissionStale),
		opsNullString(input.AdmissionReason), opsNullTime(input.RecoveryStartedAt), input.ActualWeight,
		defaultString(input.HAProxyState, "unknown"), input.ActiveRequests,
		input.QueueDepth, defaultString(input.PostgresSyncState, "unknown"),
		opsNullInt64(input.WALReplayLagBytes), string(liveMetricsJSON))
	return err
}

func (r *opsRepository) UpdateClusterNodeAppliedState(ctx context.Context, input service.OpsClusterAppliedStateUpdate) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE ops_cluster_node_status
SET observed_revision = $2,
    apply_status = $3,
    apply_error = $4,
    applied_at = $5,
    actual_weight = $6,
    haproxy_state = $7,
    updated_at = NOW()
WHERE node_id = $1
`, input.NodeID, input.ObservedRevision, input.ApplyStatus, opsNullString(input.ApplyError),
		opsNullTime(input.AppliedAt), input.ActualWeight, input.HAProxyState)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *opsRepository) GetClusterRequestCounts(ctx context.Context, start, end time.Time) (map[string]int64, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT node_id, SUM(request_count)
FROM (
  SELECT COALESCE(NULLIF(node_id, ''), 'legacy') AS node_id, COUNT(*)::bigint AS request_count
  FROM usage_logs WHERE created_at >= $1 AND created_at < $2 GROUP BY 1
  UNION ALL
  SELECT COALESCE(NULLIF(node_id, ''), 'legacy') AS node_id, COUNT(*)::bigint AS request_count
  FROM ops_error_logs
  WHERE created_at >= $1 AND created_at < $2 AND COALESCE(status_code, 0) >= 400 AND is_count_tokens = FALSE
  GROUP BY 1
) counts
GROUP BY node_id
`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int64, 3)
	for rows.Next() {
		var nodeID string
		var count int64
		if err := rows.Scan(&nodeID, &count); err != nil {
			return nil, err
		}
		result[nodeID] = count
	}
	return result, rows.Err()
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
