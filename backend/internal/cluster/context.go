package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type executionContextKey struct{}
type runtimeContextKey struct{}
type settlementContextKey struct{}

func WithRuntime(ctx context.Context, r *Runtime) context.Context {
	return context.WithValue(ctx, runtimeContextKey{}, r)
}
func RuntimeFrom(ctx context.Context) *Runtime {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(runtimeContextKey{}).(*Runtime)
	return r
}
func WithSettlementID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, settlementContextKey{}, id)
}
func SettlementIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(settlementContextKey{}).(string)
	return id
}

// Execution contains only stable ownership and accounting identity, never user payloads.
type Execution struct {
	Reserve   *RecoveryReserve
	ID        string
	Gate      *Gate
	DB        *sql.DB
	ClusterID string
	NodeID    string
	BootID    string
	Protocol  string
	WebSocket bool
	mu        sync.Mutex
	started   bool
	turn      int
	release   func()
	detached  bool
}

func WithExecution(ctx context.Context, e *Execution) context.Context {
	return context.WithValue(ctx, executionContextKey{}, e)
}
func ExecutionFrom(ctx context.Context) *Execution {
	if ctx == nil {
		return nil
	}
	e, _ := ctx.Value(executionContextKey{}).(*Execution)
	return e
}
func (e *Execution) Snapshot() *Execution {
	e.mu.Lock()
	defer e.mu.Unlock()
	return &Execution{ID: e.ID, Gate: e.Gate, DB: e.DB, Reserve: e.Reserve, ClusterID: e.ClusterID, NodeID: e.NodeID, BootID: e.BootID, Protocol: e.Protocol, started: e.started, turn: e.turn}
}
func (e *Execution) SettlementID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return "execution:" + e.ID
}
func CopyExecution(parent, base context.Context) context.Context {
	if e := ExecutionFrom(parent); e != nil {
		base = WithExecution(base, e.Snapshot())
	}
	if id := SettlementIDFrom(parent); id != "" {
		base = WithSettlementID(base, id)
	}
	return base
}
func CheckNewTurn(ctx context.Context) error {
	if e := ExecutionFrom(ctx); e != nil && e.Gate != nil && !e.Gate.Ready() {
		return ErrNotReady
	}
	return nil
}

// BeginTurn reserves logical work for upgraded connections; retries of the same turn keep its identity.
func BeginTurn(ctx context.Context, turn int) error {
	e := ExecutionFrom(ctx)
	if e == nil {
		return nil
	}
	if err := CheckNewTurn(ctx); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.release != nil {
		return nil
	}
	if e.WebSocket {
		release, err := e.Gate.Acquire(ctx, time.Second)
		if err != nil {
			return err
		}
		e.release = release
	}
	if turn > e.turn && e.turn > 0 {
		id := uuid.NewString()
		if e.Reserve != nil {
			if err := e.Reserve.Reserve(id); err != nil {
				if e.release != nil {
					e.release()
					e.release = nil
				}
				return err
			}
		}
		if _, err := e.DB.ExecContext(ctx, `INSERT INTO cluster_executions(execution_id,cluster_id,node_id,boot_id,protocol) VALUES($1,$2,$3,$4,$5)`, id, e.ClusterID, e.NodeID, e.BootID, e.Protocol); err != nil {
			if e.release != nil {
				e.release()
				e.release = nil
			}
			return err
		}
		e.ID = id
		e.started = false
	}
	e.turn = turn
	return nil
}
func BeginNextTurn(ctx context.Context) error {
	e := ExecutionFrom(ctx)
	if e == nil {
		return nil
	}
	e.mu.Lock()
	turn := e.turn + 1
	e.mu.Unlock()
	return BeginTurn(ctx, turn)
}
func EndTurn(ctx context.Context) {
	e := ExecutionFrom(ctx)
	if e == nil {
		return
	}
	e.mu.Lock()
	release := e.release
	e.release = nil
	id, started := e.ID, e.started
	e.mu.Unlock()
	if release != nil {
		defer release()
	}
	state := "rejected"
	if started {
		state = "needs_review"
	}
	e.finish(id, state)
}
func MarkDispatched(ctx context.Context) error {
	e := ExecutionFrom(ctx)
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return nil
	}
	if e.DB == nil {
		return ErrNotReady
	}
	write, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := e.DB.ExecContext(write, `UPDATE cluster_executions SET state='executing',updated_at=NOW() WHERE execution_id=$1 AND state='accepted'`, e.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("execution intent is no longer dispatchable")
	}
	e.started = true
	return nil
}

// DetachAccepted transfers an already accepted HTTP task to a background owner.
func DetachAccepted(ctx context.Context) (func(), error) {
	e := ExecutionFrom(ctx)
	if e == nil {
		return func() {}, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.detached {
		return nil, fmt.Errorf("execution already detached")
	}
	e.Gate.mu.Lock()
	e.Gate.active++
	e.Gate.mu.Unlock()
	e.detached = true
	var once sync.Once
	return func() {
		once.Do(func() { EndTurn(ctx); e.Gate.mu.Lock(); e.Gate.active--; e.Gate.signal(); e.Gate.mu.Unlock() })
	}, nil
}
func IsDetached(ctx context.Context) bool {
	e := ExecutionFrom(ctx)
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.detached
}

// HoldPhysical keeps detached provider work counted after its HTTP handler has returned.
func HoldPhysical(ctx context.Context) {
	e := ExecutionFrom(ctx)
	if e == nil || e.WebSocket {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.detached {
		return
	}
	e.detached = true
	e.Gate.mu.Lock()
	e.Gate.active++
	e.Gate.mu.Unlock()
	var once sync.Once
	e.release = func() { once.Do(func() { e.Gate.mu.Lock(); e.Gate.active--; e.Gate.signal(); e.Gate.mu.Unlock() }) }
}
