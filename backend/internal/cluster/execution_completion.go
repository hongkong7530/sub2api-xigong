package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

func validCompletionState(state string) bool {
	return state == "rejected" || state == "needs_review" || state == "awaiting_settlement"
}

// ApplyExecutionCompletion replays a known work-end fact, not an upstream request.
// Terminal money state is inferred only from the authority's committed events.
func ApplyExecutionCompletion(ctx context.Context, db *sql.DB, id, clusterID, nodeID, state string) error {
	if !validCompletionState(state) {
		return fmt.Errorf("invalid execution completion state")
	}
	result, err := db.ExecContext(ctx, `UPDATE cluster_executions SET work_finished=TRUE,
state=CASE WHEN state IN ('settled','rejected') THEN state
WHEN $4='rejected' THEN 'rejected'
WHEN EXISTS(SELECT 1 FROM cluster_settlement_events WHERE execution_id=$1)
 AND NOT EXISTS(SELECT 1 FROM cluster_settlement_events WHERE execution_id=$1 AND state<>'committed') THEN 'settled'
ELSE $4 END,updated_at=NOW()
WHERE execution_id=$1 AND cluster_id=$2 AND node_id=$3`, id, clusterID, nodeID, state)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("execution completion identity not found")
	}
	return nil
}

func (e *Execution) finish(id, state string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ApplyExecutionCompletion(ctx, e.DB, id, e.ClusterID, e.NodeID, state); err == nil {
		if state == "rejected" && e.Reserve != nil {
			if err := e.Reserve.Confirm(id); err != nil {
				slog.Error("execution recovery slot removal failed", "execution_id", id)
			}
		}
		return
	}
	// The same preallocated slot carries monetary evidence and the failed end marker.
	// Never erase it merely because the HTTP owner or WebSocket turn has returned.
	if e.Reserve != nil {
		if err := e.Reserve.SaveCompletion(id, state); err == nil {
			return
		}
	}
	slog.Error("execution completion could not be persisted; reconciliation required", "execution_id", id, "node_id", e.NodeID)
}
