package cluster

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClusterExecutionLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id := "execution-" + uuid.NewString()
	reserve, err := NewRecoveryReserve(t.TempDir(), 8)
	require.NoError(t, err)
	r := &Runtime{cfg: &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: id, NodeID: "worker"}}, db: db, Gate: NewGate(2, 1), reserve: reserve, bootID: uuid.NewString()}
	r.Gate.Configure(true, 2, 1)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM cluster_executions WHERE cluster_id=$1`, id) })
	accepted, err := r.BeginExecution(ctx, "GET /v1/responses")
	require.NoError(t, err)
	require.NoError(t, BeginTurn(accepted, 1))
	first := ExecutionFrom(accepted).ID
	a, _, _ := r.Gate.Counts()
	require.Equal(t, 1, a)
	require.NoError(t, MarkDispatched(accepted))
	EndTurn(accepted)
	a, _, _ = r.Gate.Counts()
	require.Zero(t, a)
	var state string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT state FROM cluster_executions WHERE execution_id=$1`, first).Scan(&state))
	require.Equal(t, "needs_review", state)
	require.NoError(t, BeginTurn(accepted, 2))
	second := ExecutionFrom(accepted).ID
	require.NotEqual(t, first, second)
	EndTurn(accepted)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT state FROM cluster_executions WHERE execution_id=$1`, second).Scan(&state))
	require.Equal(t, "rejected", state)
	r.Pause()
	require.ErrorIs(t, BeginTurn(accepted, 3), ErrNotReady)
	// A detached accepted image keeps one logical slot after its HTTP owner leaves.
	r.Gate.Configure(true, 2, 1)
	httpRelease, err := r.Gate.Acquire(ctx, time.Second)
	require.NoError(t, err)
	image, err := r.BeginExecution(ctx, "POST /v1/images/generations/async")
	require.NoError(t, err)
	finish, err := DetachAccepted(image)
	require.NoError(t, err)
	httpRelease()
	r.FinishExecution(image, false)
	a, _, _ = r.Gate.Counts()
	require.Equal(t, 1, a)
	require.NoError(t, MarkDispatched(image))
	finish()
	a, _, _ = r.Gate.Counts()
	require.Zero(t, a)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT state FROM cluster_executions WHERE execution_id=$1`, ExecutionFrom(image).ID).Scan(&state))
	require.Equal(t, "needs_review", state)
}
