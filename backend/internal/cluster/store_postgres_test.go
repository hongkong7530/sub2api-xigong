package cluster

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Uses a dedicated disposable database; never infer a production DSN.
func TestClusterStorePostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN required")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	id := "test-" + uuid.NewString()
	s := &Store{DB: db, ClusterID: id, ConnectionBudget: 16}
	t.Cleanup(func() {
		for _, table := range []string{"cluster_operations", "cluster_join_tokens", "cluster_traffic_counters", "cluster_nodes", "cluster_control"} {
			_, _ = db.Exec(`DELETE FROM `+table+` WHERE cluster_id=$1`, id)
		}
	})
	n := fixtureNode()
	n.ClusterID = id
	token, err := s.Register(ctx, n, 1)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	_, err = s.Authenticate(ctx, n.ID, n.CertificateSHA256)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Enroll(ctx, n.ID, n.CertificateSHA256, "wrong")
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Enroll(ctx, n.ID, n.CertificateSHA256, token)
	require.NoError(t, err)
	_, err = s.Enroll(ctx, n.ID, n.CertificateSHA256, token)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Authenticate(ctx, n.ID, n.CertificateSHA256)
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, n.ID, strings.Repeat("b", 64))
	require.ErrorIs(t, err, ErrForbidden)
	snapshot := &Snapshot{At: time.Now(), DBOK: true, RedisOK: true}
	change := Change{RequestKey: uuid.NewString(), ExpectedRevision: 1, Action: "activate"}
	op, err := s.Change(ctx, n.ID, change, 1, snapshot)
	require.NoError(t, err)
	require.Equal(t, int64(2), op.TargetRevision)
	duplicate, err := s.Change(ctx, n.ID, change, 1, snapshot)
	require.NoError(t, err)
	require.Equal(t, op.ID, duplicate.ID)
	queried, err := s.OperationByRequestKey(ctx, change.RequestKey)
	require.NoError(t, err)
	require.Equal(t, op.ID, queried.ID)
	change.Action = "pause"
	_, err = s.Change(ctx, n.ID, change, 1, snapshot)
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.Change(ctx, n.ID, Change{RequestKey: uuid.NewString(), ExpectedRevision: 2, Action: "drain"}, 1, snapshot)
	require.NoError(t, err)
	snapshot.ObservedRevision = 3
	snapshot.Active = 1
	_, err = s.Change(ctx, n.ID, Change{RequestKey: uuid.NewString(), ExpectedRevision: 3, Action: "retire"}, 1, snapshot)
	require.ErrorIs(t, err, ErrUnsafeExit)
	snapshot.Active = 0
	_, err = db.ExecContext(ctx, `UPDATE cluster_nodes SET applied_revision=revision WHERE cluster_id=$1 AND node_id=$2`, id, n.ID)
	require.NoError(t, err)
	_, err = s.Change(ctx, n.ID, Change{RequestKey: uuid.NewString(), ExpectedRevision: 3, Action: "retire"}, 1, snapshot)
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, n.ID, n.CertificateSHA256)
	require.ErrorIs(t, err, ErrForbidden)
	oldCertificate := n.CertificateSHA256
	n.CertificateSHA256 = strings.Repeat("c", 64)
	newToken, err := s.Register(ctx, n, 1)
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, n.ID, oldCertificate)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Authenticate(ctx, n.ID, n.CertificateSHA256)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Enroll(ctx, n.ID, n.CertificateSHA256, token)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Enroll(ctx, n.ID, n.CertificateSHA256, newToken)
	require.NoError(t, err)
}
func TestTrafficCycleUsesConfiguredTimezone(t *testing.T) {
	start, err := TrafficCycleStart(time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC), 3, "UTC")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), start)
}
