package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) InsertSystemMetrics(ctx context.Context, input *service.OpsInsertSystemMetricsInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return fmt.Errorf("nil input")
	}

	window := input.WindowMinutes
	if window <= 0 {
		window = 1
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	q := `
INSERT INTO ops_system_metrics (
  created_at,
  window_minutes,
  node_id,
  node_name,
  platform,
  group_id,

  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,

  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,

  token_consumed,
  account_switch_count,
  qps,
  tps,

  duration_p50_ms,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,

  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms,

  cpu_usage_percent,
  cpu_cores,
  load_1,
  load_5,
  load_15,
  memory_used_mb,
  memory_available_mb,
  memory_total_mb,
  memory_usage_percent,
  swap_used_mb,
  swap_total_mb,

  db_ok,
  redis_ok,

  redis_conn_total,
  redis_conn_idle,

  db_conn_active,
  db_conn_idle,
  db_conn_waiting,

  goroutine_count,
  concurrency_queue_depth
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,
  $11,$12,$13,
  $14,$15,$16,$17,
  $18,$19,$20,$21,$22,$23,
  $24,$25,$26,$27,$28,$29,
  $30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,
  $41,$42,
  $43,$44,
  $45,$46,$47,
  $48,$49
)`

	_, err := r.db.ExecContext(
		ctx,
		q,
		createdAt,
		window,
		input.NodeID,
		input.NodeName,
		opsNullString(input.Platform),
		opsNullInt64(input.GroupID),

		input.SuccessCount,
		input.ErrorCountTotal,
		input.BusinessLimitedCount,
		input.ErrorCountSLA,

		input.UpstreamErrorCountExcl429529,
		input.Upstream429Count,
		input.Upstream529Count,

		input.TokenConsumed,
		input.AccountSwitchCount,
		opsNullFloat64(input.QPS),
		opsNullFloat64(input.TPS),

		opsNullMetricInt(input.DurationP50Ms),
		opsNullMetricInt(input.DurationP90Ms),
		opsNullMetricInt(input.DurationP95Ms),
		opsNullMetricInt(input.DurationP99Ms),
		opsNullFloat64(input.DurationAvgMs),
		opsNullMetricInt(input.DurationMaxMs),

		opsNullMetricInt(input.TTFTP50Ms),
		opsNullMetricInt(input.TTFTP90Ms),
		opsNullMetricInt(input.TTFTP95Ms),
		opsNullMetricInt(input.TTFTP99Ms),
		opsNullFloat64(input.TTFTAvgMs),
		opsNullMetricInt(input.TTFTMaxMs),

		opsNullFloat64(input.CPUUsagePercent),
		opsNullInt(input.CPUCores),
		opsNullFloat64(input.Load1),
		opsNullFloat64(input.Load5),
		opsNullFloat64(input.Load15),
		opsNullMetricInt64(input.MemoryUsedMB),
		opsNullInt(input.MemoryAvailableMB),
		opsNullMetricInt64(input.MemoryTotalMB),
		opsNullFloat64(input.MemoryUsagePercent),
		opsNullInt(input.SwapUsedMB),
		opsNullInt(input.SwapTotalMB),

		opsNullBool(input.DBOK),
		opsNullBool(input.RedisOK),

		opsNullMetricInt(input.RedisConnTotal),
		opsNullMetricInt(input.RedisConnIdle),

		opsNullMetricInt(input.DBConnActive),
		opsNullMetricInt(input.DBConnIdle),
		opsNullMetricInt(input.DBConnWaiting),

		opsNullMetricInt(input.GoroutineCount),
		opsNullMetricInt(input.ConcurrencyQueueDepth),
	)
	return err
}

func (r *opsRepository) GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*service.OpsSystemMetricsSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}
	q := `SELECT ` + opsSystemMetricsSelectColumns + `
FROM ops_system_metrics
WHERE window_minutes = $1 AND platform IS NULL AND group_id IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1`
	return scanSystemMetrics(func(dest ...any) error {
		return r.db.QueryRowContext(ctx, q, windowMinutes).Scan(dest...)
	})
}

func (r *opsRepository) GetLatestSystemMetricsForNode(ctx context.Context, windowMinutes int, nodeID string) (*service.OpsSystemMetricsSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}
	q := `SELECT ` + opsSystemMetricsSelectColumns + `
FROM ops_system_metrics
WHERE window_minutes = $1 AND node_id = $2 AND platform IS NULL AND group_id IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1`
	return scanSystemMetrics(func(dest ...any) error {
		return r.db.QueryRowContext(ctx, q, windowMinutes, nodeID).Scan(dest...)
	})
}

const opsSystemMetricsSelectColumns = `
  id, created_at, window_minutes, node_id, node_name,
  success_count, error_count_total, business_limited_count, error_count_sla,
  token_consumed, qps, tps,
  duration_p50_ms, duration_p95_ms, duration_p99_ms,
  ttft_p50_ms, ttft_p95_ms, ttft_p99_ms,
  cpu_usage_percent, cpu_cores, load_1, load_5, load_15,
  memory_used_mb, memory_available_mb, memory_total_mb, memory_usage_percent,
  swap_used_mb, swap_total_mb,
  db_ok, redis_ok, redis_conn_total, redis_conn_idle,
  db_conn_active, db_conn_idle, db_conn_waiting,
  goroutine_count, concurrency_queue_depth, account_switch_count`

func (r *opsRepository) ListLatestSystemMetrics(ctx context.Context, windowMinutes int) ([]*service.OpsSystemMetricsSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}

	q := `
WITH latest AS (
  SELECT DISTINCT ON (node_id) ` + opsSystemMetricsSelectColumns + `
  FROM ops_system_metrics
  WHERE window_minutes = $1
    AND platform IS NULL
    AND group_id IS NULL
    AND node_id <> 'legacy'
    AND created_at >= NOW() - INTERVAL '2 hours'
  ORDER BY node_id ASC, created_at DESC, id DESC
)
SELECT ` + opsSystemMetricsSelectColumns + `
FROM latest
ORDER BY node_id ASC`

	rows, err := r.db.QueryContext(ctx, q, windowMinutes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsSystemMetricsSnapshot, 0, 4)
	for rows.Next() {
		item, err := scanSystemMetrics(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type opsSystemMetricsScanner func(dest ...any) error

func scanSystemMetrics(scan opsSystemMetricsScanner) (*service.OpsSystemMetricsSnapshot, error) {
	var out service.OpsSystemMetricsSnapshot
	var qps sql.NullFloat64
	var tps sql.NullFloat64
	var durationP50 sql.NullInt64
	var durationP95 sql.NullInt64
	var durationP99 sql.NullInt64
	var ttftP50 sql.NullInt64
	var ttftP95 sql.NullInt64
	var ttftP99 sql.NullInt64
	var cpu sql.NullFloat64
	var cpuCores sql.NullInt64
	var load1 sql.NullFloat64
	var load5 sql.NullFloat64
	var load15 sql.NullFloat64
	var memUsed sql.NullInt64
	var memAvailable sql.NullInt64
	var memTotal sql.NullInt64
	var memPct sql.NullFloat64
	var swapUsed sql.NullInt64
	var swapTotal sql.NullInt64
	var dbOK sql.NullBool
	var redisOK sql.NullBool
	var redisTotal sql.NullInt64
	var redisIdle sql.NullInt64
	var dbActive sql.NullInt64
	var dbIdle sql.NullInt64
	var dbWaiting sql.NullInt64
	var goroutines sql.NullInt64
	var queueDepth sql.NullInt64
	var accountSwitchCount sql.NullInt64

	if err := scan(
		&out.ID,
		&out.CreatedAt,
		&out.WindowMinutes,
		&out.NodeID,
		&out.NodeName,
		&out.SuccessCount,
		&out.ErrorCountTotal,
		&out.BusinessLimitedCount,
		&out.ErrorCountSLA,
		&out.TokenConsumed,
		&qps,
		&tps,
		&durationP50,
		&durationP95,
		&durationP99,
		&ttftP50,
		&ttftP95,
		&ttftP99,
		&cpu,
		&cpuCores,
		&load1,
		&load5,
		&load15,
		&memUsed,
		&memAvailable,
		&memTotal,
		&memPct,
		&swapUsed,
		&swapTotal,
		&dbOK,
		&redisOK,
		&redisTotal,
		&redisIdle,
		&dbActive,
		&dbIdle,
		&dbWaiting,
		&goroutines,
		&queueDepth,
		&accountSwitchCount,
	); err != nil {
		return nil, err
	}

	if qps.Valid {
		v := qps.Float64
		out.QPS = &v
	}
	if tps.Valid {
		v := tps.Float64
		out.TPS = &v
	}
	if durationP50.Valid {
		v := int(durationP50.Int64)
		out.DurationP50Ms = &v
	}
	if durationP95.Valid {
		v := int(durationP95.Int64)
		out.DurationP95Ms = &v
	}
	if durationP99.Valid {
		v := int(durationP99.Int64)
		out.DurationP99Ms = &v
	}
	if ttftP50.Valid {
		v := int(ttftP50.Int64)
		out.TTFTP50Ms = &v
	}
	if ttftP95.Valid {
		v := int(ttftP95.Int64)
		out.TTFTP95Ms = &v
	}
	if ttftP99.Valid {
		v := int(ttftP99.Int64)
		out.TTFTP99Ms = &v
	}
	if cpu.Valid {
		v := cpu.Float64
		out.CPUUsagePercent = &v
	}
	if cpuCores.Valid {
		v := int(cpuCores.Int64)
		out.CPUCores = &v
	}
	if load1.Valid {
		v := load1.Float64
		out.Load1 = &v
	}
	if load5.Valid {
		v := load5.Float64
		out.Load5 = &v
	}
	if load15.Valid {
		v := load15.Float64
		out.Load15 = &v
	}
	if memUsed.Valid {
		v := memUsed.Int64
		out.MemoryUsedMB = &v
	}
	if memAvailable.Valid {
		v := memAvailable.Int64
		out.MemoryAvailableMB = &v
	}
	if memTotal.Valid {
		v := memTotal.Int64
		out.MemoryTotalMB = &v
	}
	if memPct.Valid {
		v := memPct.Float64
		out.MemoryUsagePercent = &v
	}
	if swapUsed.Valid {
		v := swapUsed.Int64
		out.SwapUsedMB = &v
	}
	if swapTotal.Valid {
		v := swapTotal.Int64
		out.SwapTotalMB = &v
	}
	if dbOK.Valid {
		v := dbOK.Bool
		out.DBOK = &v
	}
	if redisOK.Valid {
		v := redisOK.Bool
		out.RedisOK = &v
	}
	if redisTotal.Valid {
		v := int(redisTotal.Int64)
		out.RedisConnTotal = &v
	}
	if redisIdle.Valid {
		v := int(redisIdle.Int64)
		out.RedisConnIdle = &v
	}
	if dbActive.Valid {
		v := int(dbActive.Int64)
		out.DBConnActive = &v
	}
	if dbIdle.Valid {
		v := int(dbIdle.Int64)
		out.DBConnIdle = &v
	}
	if dbWaiting.Valid {
		v := int(dbWaiting.Int64)
		out.DBConnWaiting = &v
	}
	if goroutines.Valid {
		v := int(goroutines.Int64)
		out.GoroutineCount = &v
	}
	if queueDepth.Valid {
		v := int(queueDepth.Int64)
		out.ConcurrencyQueueDepth = &v
	}
	if accountSwitchCount.Valid {
		v := accountSwitchCount.Int64
		out.AccountSwitchCount = &v
	}
	return &out, nil
}

func (r *opsRepository) UpsertJobHeartbeat(ctx context.Context, input *service.OpsUpsertJobHeartbeatInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return fmt.Errorf("nil input")
	}
	if input.JobName == "" {
		return fmt.Errorf("job_name required")
	}

	nodeID := input.NodeID
	if nodeID == "" {
		nodeID = service.LoadOpsNodeIdentity().ID
	}

	q := `
INSERT INTO ops_job_heartbeats (
  node_id,
  job_name,
  last_run_at,
  last_success_at,
  last_error_at,
  last_error,
  last_duration_ms,
  last_result,
  updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,NOW()
)
ON CONFLICT (node_id, job_name) DO UPDATE SET
  last_run_at = COALESCE(EXCLUDED.last_run_at, ops_job_heartbeats.last_run_at),
  last_success_at = COALESCE(EXCLUDED.last_success_at, ops_job_heartbeats.last_success_at),
  last_error_at = CASE
    WHEN EXCLUDED.last_success_at IS NOT NULL THEN NULL
    ELSE COALESCE(EXCLUDED.last_error_at, ops_job_heartbeats.last_error_at)
  END,
  last_error = CASE
    WHEN EXCLUDED.last_success_at IS NOT NULL THEN NULL
    ELSE COALESCE(EXCLUDED.last_error, ops_job_heartbeats.last_error)
  END,
  last_duration_ms = COALESCE(EXCLUDED.last_duration_ms, ops_job_heartbeats.last_duration_ms),
  last_result = CASE
    WHEN EXCLUDED.last_success_at IS NOT NULL THEN COALESCE(EXCLUDED.last_result, ops_job_heartbeats.last_result)
    ELSE ops_job_heartbeats.last_result
  END,
  updated_at = NOW()`

	_, err := r.db.ExecContext(
		ctx,
		q,
		nodeID,
		input.JobName,
		opsNullTime(input.LastRunAt),
		opsNullTime(input.LastSuccessAt),
		opsNullTime(input.LastErrorAt),
		opsNullString(input.LastError),
		opsNullInt(input.LastDurationMs),
		opsNullString(input.LastResult),
	)
	return err
}

func (r *opsRepository) ListJobHeartbeats(ctx context.Context) ([]*service.OpsJobHeartbeat, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}

	q := `
SELECT
  node_id,
  job_name,
  last_run_at,
  last_success_at,
  last_error_at,
  last_error,
  last_duration_ms,
  last_result,
  updated_at
FROM ops_job_heartbeats
ORDER BY node_id ASC, job_name ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsJobHeartbeat, 0, 8)
	for rows.Next() {
		var item service.OpsJobHeartbeat
		var lastRun sql.NullTime
		var lastSuccess sql.NullTime
		var lastErrorAt sql.NullTime
		var lastError sql.NullString
		var lastDuration sql.NullInt64

		var lastResult sql.NullString

		if err := rows.Scan(
			&item.NodeID,
			&item.JobName,
			&lastRun,
			&lastSuccess,
			&lastErrorAt,
			&lastError,
			&lastDuration,
			&lastResult,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if lastRun.Valid {
			v := lastRun.Time
			item.LastRunAt = &v
		}
		if lastSuccess.Valid {
			v := lastSuccess.Time
			item.LastSuccessAt = &v
		}
		if lastErrorAt.Valid {
			v := lastErrorAt.Time
			item.LastErrorAt = &v
		}
		if lastError.Valid {
			v := lastError.String
			item.LastError = &v
		}
		if lastDuration.Valid {
			v := lastDuration.Int64
			item.LastDurationMs = &v
		}
		if lastResult.Valid {
			v := lastResult.String
			item.LastResult = &v
		}

		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func opsNullBool(v *bool) any {
	if v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *v, Valid: true}
}

func opsNullFloat64(v *float64) any {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}

func opsNullMetricInt(v *int) any {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func opsNullMetricInt64(v *int64) any {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func opsNullTime(v *time.Time) any {
	if v == nil || v.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *v, Valid: true}
}
