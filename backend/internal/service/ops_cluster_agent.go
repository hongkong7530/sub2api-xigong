package service

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type OpsClusterAgent struct {
	repo  OpsRepository
	db    *sql.DB
	redis *redis.Client
	cfg   *config.Config

	lastRequestTotal    uint64
	lastRequestSampleAt time.Time

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewOpsClusterAgent(repo OpsRepository, db *sql.DB, redisClient *redis.Client, cfg *config.Config) *OpsClusterAgent {
	return &OpsClusterAgent{repo: repo, db: db, redis: redisClient, cfg: cfg, stopCh: make(chan struct{})}
}

func (a *OpsClusterAgent) Start() {
	if a == nil {
		return
	}
	a.startOnce.Do(func() { go a.run() })
}

func (a *OpsClusterAgent) Stop() {
	if a == nil {
		return
	}
	a.stopOnce.Do(func() { close(a.stopCh) })
}

func (a *OpsClusterAgent) run() {
	a.sample()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.sample()
		case <-a.stopCh:
			return
		}
	}
}

func (a *OpsClusterAgent) sample() {
	if a == nil || a.repo == nil {
		SetClusterWritesAllowed(false)
		return
	}
	identity := LoadOpsNodeIdentity()
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cfg, err := a.repo.GetClusterNodeConfig(ctx, identity.ID)
	if err != nil {
		SetClusterWritesAllowed(false)
		return
	}
	previous, err := a.repo.GetClusterNodeStatus(ctx, identity.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		SetClusterWritesAllowed(false)
		return
	}
	metrics := a.liveMetrics(ctx, identity)
	postgresState, lag := a.postgresSyncState(ctx, identity.ID)
	dependenciesOK := metrics.DBOK != nil && *metrics.DBOK && metrics.RedisOK != nil && *metrics.RedisOK
	if a.cfg != nil && a.cfg.Ops.StrictSyncRequired {
		dependenciesOK = dependenciesOK && postgresState == "sync"
	}
	now := time.Now().UTC()
	state, reason := EvaluateOpsAdmission(cfg, metrics, dependenciesOK, previous, now)

	var recoveryStartedAt *time.Time
	if state == OpsAdmissionWaitingRecovery {
		if previous != nil && previous.RecoveryStartedAt != nil {
			recoveryStartedAt = previous.RecoveryStartedAt
		} else {
			started := now
			recoveryStartedAt = &started
		}
	}
	if state != OpsAdmissionReady && state != OpsAdmissionWaitingRecovery {
		recoveryStartedAt = nil
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	activeRequests, queueDepth := a.liveRequestLoad()
	metrics.QPS = a.liveQPS(now)
	if a.cfg != nil && a.cfg.Ops.StrictSyncRequired {
		SetClusterWritesAllowed(dependenciesOK)
	} else {
		SetClusterWritesAllowed(true)
	}
	_ = a.repo.UpsertClusterNodeStatus(ctx, OpsClusterStatusUpdate{
		NodeID: identity.ID, HeartbeatAt: now, ObservedRevision: observedRevision(previous),
		ApplyStatus: applyStatus(previous), AdmissionState: state, AdmissionReason: reasonPtr,
		RecoveryStartedAt: recoveryStartedAt, ActualWeight: actualWeight(previous),
		HAProxyState: haproxyState(previous), ActiveRequests: activeRequests, QueueDepth: queueDepth,
		PostgresSyncState: postgresState, WALReplayLagBytes: lag,
		LiveMetrics: liveClusterMetrics(metrics, activeRequests, queueDepth, a.db, a.redis),
	})
}

func (a *OpsClusterAgent) liveRequestLoad() (active, waiting int) {
	return ClusterActiveRequests(), 0
}

func (a *OpsClusterAgent) liveQPS(now time.Time) *float64 {
	total := ClusterTotalRequests()
	if a.lastRequestSampleAt.IsZero() || total < a.lastRequestTotal {
		a.lastRequestTotal = total
		a.lastRequestSampleAt = now
		return nil
	}
	elapsed := now.Sub(a.lastRequestSampleAt).Seconds()
	if elapsed <= 0 {
		return nil
	}
	qps := float64(total-a.lastRequestTotal) / elapsed
	a.lastRequestTotal = total
	a.lastRequestSampleAt = now
	return &qps
}

func liveClusterMetrics(metrics *OpsSystemMetricsSnapshot, activeRequests, queueDepth int, db *sql.DB, redisClient *redis.Client) *OpsClusterLiveMetrics {
	if metrics == nil {
		return nil
	}
	live := &OpsClusterLiveMetrics{
		CreatedAt: metrics.CreatedAt, NodeID: metrics.NodeID, NodeName: metrics.NodeName,
		QPS: metrics.QPS, CPUUsagePercent: metrics.CPUUsagePercent, CPUCores: metrics.CPUCores,
		MemoryUsedMB: metrics.MemoryUsedMB, MemoryAvailableMB: metrics.MemoryAvailableMB,
		MemoryTotalMB: metrics.MemoryTotalMB, MemoryUsagePercent: metrics.MemoryUsagePercent,
		DBOK: metrics.DBOK, RedisOK: metrics.RedisOK,
		GoroutineCount: intPtr(runtime.NumGoroutine()), ConcurrencyQueueDepth: intPtr(queueDepth),
	}
	if db != nil {
		stats := db.Stats()
		live.DBConnActive = intPtr(stats.InUse)
		live.DBConnIdle = intPtr(stats.Idle)
	}
	if redisClient != nil {
		stats := redisClient.PoolStats()
		total := int(stats.TotalConns)
		idle := int(stats.IdleConns)
		live.RedisConnTotal = &total
		live.RedisConnIdle = &idle
	}
	return live
}

func (a *OpsClusterAgent) liveMetrics(ctx context.Context, identity OpsNodeIdentity) *OpsSystemMetricsSnapshot {
	now := time.Now().UTC()
	metrics := &OpsSystemMetricsSnapshot{CreatedAt: now, NodeID: identity.ID, NodeName: identity.Name}
	cores := runtime.NumCPU()
	metrics.CPUCores = &cores
	if values, err := cpu.PercentWithContext(ctx, 200*time.Millisecond, false); err == nil && len(values) > 0 {
		value := clampHostCPUPercent(values[0])
		metrics.CPUUsagePercent = &value
	}
	if memory, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		used := int64(memory.Used / bytesPerMB)
		available := int64(memory.Available / bytesPerMB)
		total := int64(memory.Total / bytesPerMB)
		percent := memory.UsedPercent
		metrics.MemoryUsedMB = &used
		metrics.MemoryAvailableMB = &available
		metrics.MemoryTotalMB = &total
		metrics.MemoryUsagePercent = &percent
	}
	dbOK := false
	if a.db != nil {
		dbOK = a.db.PingContext(ctx) == nil
	}
	metrics.DBOK = &dbOK
	redisOK := false
	if a.redis != nil {
		redisOK = a.redis.Ping(ctx).Err() == nil
	}
	metrics.RedisOK = &redisOK
	return metrics
}

func (a *OpsClusterAgent) postgresSyncState(ctx context.Context, _ string) (string, *int64) {
	if a == nil || a.db == nil || a.cfg == nil || !a.cfg.Ops.StrictSyncRequired {
		return "not_required", nil
	}
	var state string
	var lag sql.NullInt64
	err := a.db.QueryRowContext(ctx, `
SELECT COALESCE(sync_state, 'missing'),
       pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn)::bigint
FROM pg_stat_replication
WHERE application_name = 'xigong_sync'
LIMIT 1`).Scan(&state, &lag)
	if err != nil {
		return "missing", nil
	}
	if lag.Valid {
		value := lag.Int64
		return state, &value
	}
	return state, nil
}

func observedRevision(status *OpsClusterNodeStatus) int64 {
	if status == nil {
		return 0
	}
	return status.ObservedRevision
}

func applyStatus(status *OpsClusterNodeStatus) string {
	if status == nil || status.ApplyStatus == "" {
		return OpsClusterApplyPending
	}
	return status.ApplyStatus
}

func actualWeight(status *OpsClusterNodeStatus) int {
	if status == nil {
		return 0
	}
	return status.ActualWeight
}

func haproxyState(status *OpsClusterNodeStatus) string {
	if status == nil || status.HAProxyState == "" {
		return "unknown"
	}
	return status.HAProxyState
}
