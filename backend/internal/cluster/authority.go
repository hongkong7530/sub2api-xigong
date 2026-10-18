package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CheckRedisGeneration persists the hold in PostgreSQL so restarting all app processes cannot clear it.
// The initial generation also requires an explicit, quiescent operator acknowledgement.
func (s *Store) CheckRedisGeneration(ctx context.Context, generation string) (bool, error) {
	if generation == "" {
		return false, ErrNotReady
	}
	var allowed bool
	err := s.DB.QueryRowContext(ctx, `SELECT cluster_observe_redis_generation($1,$2)`, s.ClusterID, generation).Scan(&allowed)
	return allowed, err
}

func (r *Runtime) AcknowledgeRedisRecovery(ctx context.Context, generation string, actor int64) error {
	if !r.IsController() || actor <= 0 {
		return ErrForbidden
	}
	nodes, err := r.Store.Nodes(ctx)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if node.State == "retired" {
			continue
		}
		snapshot, err := r.Snapshot(ctx, node.ID)
		if err != nil {
			return err
		}
		if snapshot == nil || !snapshot.Fresh(time.Now()) || snapshot.Active != 0 || snapshot.Waiting != 0 || snapshot.PendingSettlement != 0 {
			return ErrUnsafeExit
		}
	}
	result, err := r.db.ExecContext(ctx, `UPDATE cluster_control SET redis_recovery_required=FALSE,updated_at=NOW() WHERE cluster_id=$1 AND redis_generation=$2 AND NOT EXISTS(SELECT 1 FROM cluster_executions WHERE cluster_id=$1 AND state NOT IN ('settled','rejected'))`, r.cfg.Cluster.ID, generation)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrUnsafeExit
	}
	return nil
}

func (r *Runtime) BeginExecution(ctx context.Context, protocol string) (context.Context, error) {
	if !r.Enabled() {
		return ctx, nil
	}
	id := uuid.NewString()
	if err := r.reserve.Reserve(id); err != nil {
		return nil, err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO cluster_executions(execution_id,cluster_id,node_id,boot_id,protocol) VALUES($1,$2,$3,$4,$5)`, id, r.cfg.Cluster.ID, r.cfg.Cluster.NodeID, r.bootID, protocol)
	if err != nil {
		return nil, fmt.Errorf("persist execution intent: %w", err)
	}
	return WithExecution(ctx, &Execution{ID: id, Gate: r.Gate, DB: r.db, Reserve: r.reserve, ClusterID: r.cfg.Cluster.ID, NodeID: r.cfg.Cluster.NodeID, BootID: r.bootID, Protocol: protocol, WebSocket: strings.HasPrefix(protocol, "GET ")}), nil
}

// Request completion without a confirmed monetary result stays visible for reconciliation.
func (r *Runtime) FinishExecution(ctx context.Context, rejected bool) {
	execution := ExecutionFrom(ctx)
	if execution == nil || !r.Enabled() || IsDetached(ctx) {
		return
	}
	state := "awaiting_settlement"
	execution.mu.Lock()
	id, dispatched := execution.ID, execution.started
	execution.mu.Unlock()
	if rejected && dispatched {
		state = "needs_review"
	}
	if !dispatched {
		state = "rejected"
	}
	execution.finish(id, state)
}
func (s *Store) Unsettled(ctx context.Context, node string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM cluster_settlement_events WHERE cluster_id=$1 AND node_id=$2 AND state<>'committed') + (SELECT count(*) FROM cluster_executions WHERE cluster_id=$1 AND node_id=$2 AND state IN ('needs_review','awaiting_settlement') AND NOT EXISTS(SELECT 1 FROM cluster_settlement_events e WHERE e.execution_id=cluster_executions.execution_id AND e.state<>'committed'))`, s.ClusterID, node).Scan(&n)
	return n, err
}
func (s *Store) Generation(ctx context.Context) (string, bool, error) {
	var id string
	var held bool
	err := s.DB.QueryRowContext(ctx, `SELECT redis_generation,redis_recovery_required FROM cluster_control WHERE cluster_id=$1`, s.ClusterID).Scan(&id, &held)
	if errors.Is(err, sql.ErrNoRows) {
		return "", true, nil
	}
	return id, held, err
}
