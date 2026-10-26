package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecoverClusterSettlementsReplayBudgetLeavesAuthorityReadable(t *testing.T) {
	for _, source := range []string{"reserved", "journal", "database"} {
		t.Run(source, func(t *testing.T) {
			repo, mock := pendingReplayRepository(t)
			execution := ""
			if source == "reserved" {
				execution = "c9ac981e-c6f6-4773-9c9b-9dce34a1cb39"
			}
			cmd, data := pendingReplayCommand(t, execution)
			if source == "reserved" {
				require.NoError(t, repo.journal.reserve.Reserve(execution))
				envelope, err := json.Marshal(cluster.RecoveryEvents{Version: 1, FinishState: "awaiting_settlement", Events: map[string]json.RawMessage{cmd.RequestID: data}})
				require.NoError(t, err)
				require.NoError(t, repo.journal.reserve.Save(execution, envelope))
			} else if source == "journal" {
				require.NoError(t, repo.journal.save(cmd, data))
			} else {
				mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnRows(sqlmock.NewRows([]string{"command"}).AddRow(data))
			}
			// A blocked replay must leave time for an independent authority read.
			mock.ExpectExec("INSERT INTO cluster_settlement_events").WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
			if source != "database" {
				mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnRows(sqlmock.NewRows([]string{"command"}))
			}
			mock.ExpectQuery("WITH candidates AS").WillReturnRows(sqlmock.NewRows([]string{"execution_id"}))
			mock.ExpectQuery("WITH deferred").WillReturnRows(sqlmock.NewRows([]string{"conflict"}).AddRow(false))
			mock.ExpectQuery("SELECT count\\(\\*\\) FROM outstanding").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
			defer cancel()
			count, err := repo.RecoverClusterSettlements(ctx)
			require.NoError(t, err)
			require.NoError(t, ctx.Err(), "replay must not spend the entire authority-read deadline")
			require.Equal(t, 1, count)
			if source == "reserved" {
				slots, err := repo.journal.reserve.ReadAll()
				require.NoError(t, err)
				require.Contains(t, slots, execution)
			} else {
				kept, err := os.ReadFile(repo.journal.file(cmd))
				require.NoError(t, err)
				require.JSONEq(t, string(data), string(kept))
			}
		})
	}
}

func TestRecoverClusterSettlementsDeferredIdentityStillFailsClosed(t *testing.T) {
	for _, mode := range []string{"conflict", "authority read failure"} {
		t.Run(mode, func(t *testing.T) {
			repo, mock := pendingReplayRepository(t)
			cmd, data := pendingReplayCommand(t, "")
			require.NoError(t, repo.journal.save(cmd, data))
			mock.ExpectExec("INSERT INTO cluster_settlement_events").WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnRows(sqlmock.NewRows([]string{"command"}))
			mock.ExpectQuery("WITH candidates AS").WillReturnRows(sqlmock.NewRows([]string{"execution_id"}))
			query := mock.ExpectQuery("WITH deferred")
			failure := errors.New("authority unavailable after replay budget")
			if mode == "conflict" {
				query.WillReturnRows(sqlmock.NewRows([]string{"conflict"}).AddRow(true))
			} else {
				query.WillReturnError(failure)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
			defer cancel()
			count, err := repo.RecoverClusterSettlements(ctx)
			require.Positive(t, count)
			if mode == "conflict" {
				require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
			} else {
				require.ErrorIs(t, err, failure)
			}
			require.NoError(t, ctx.Err())
			kept, readErr := os.ReadFile(repo.journal.file(cmd))
			require.NoError(t, readErr)
			require.JSONEq(t, string(data), string(kept))
		})
	}
}

func TestRecoverClusterSettlementsBudgetRespectsParentCancellation(t *testing.T) {
	repo, _ := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	require.NoError(t, repo.journal.save(cmd, data))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count, err := repo.RecoverClusterSettlements(ctx)
	require.Positive(t, count)
	require.ErrorIs(t, err, context.Canceled)
	_, err = os.Stat(repo.journal.file(cmd))
	require.NoError(t, err)
}

func TestRecoverClusterSettlementsBudgetWithRealUserRowLockPostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var database string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&database))
	require.Contains(t, database, "cluster_acceptance")
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	id := uuid.NewString()
	user, err := client.User.Create().SetEmail(id + "@example.invalid").SetPasswordHash("fixture").SetBalance(20).Save(ctx)
	require.NoError(t, err)
	account, err := client.Account.Create().SetName(id).SetPlatform("openai").SetType("apikey").SetCredentials(map[string]any{}).Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetName(id).SetKey("fixture-" + id).Save(ctx)
	require.NoError(t, err)
	clusterID := "budget-" + id
	_, err = db.ExecContext(ctx, "INSERT INTO cluster_control(cluster_id,settlement_mode) VALUES($1,'atomic')", clusterID)
	require.NoError(t, err)
	cfg := &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: clusterID, NodeID: "worker-budget", StateDir: t.TempDir(), MaxInFlight: 8}}
	provider, err := ProvideUsageBillingRepository(client, db, cfg)
	require.NoError(t, err)
	repo := provider.(*usageBillingRepository)
	cmd := &service.UsageBillingCommand{ClusterID: clusterID, NodeID: cfg.Cluster.NodeID, RequestID: "event-" + id, APIKeyID: key.ID, UserID: user.ID, AccountID: account.ID, AccountType: "apikey", Model: "fixture", BillingType: service.BillingTypeBalance, BalanceCost: 2, UsageLog: &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: "event-" + id, Model: "fixture", InputTokens: 10, TotalCost: 2, ActualCost: 2, BillingType: service.BillingTypeBalance, RateMultiplier: 1, CreatedAt: time.Now().UTC()}}
	cmd.Normalize()
	evidence, err := settlementEvidence(cmd)
	require.NoError(t, err)
	raw, err := json.Marshal(evidence)
	require.NoError(t, err)
	require.NoError(t, repo.persistClusterEvidence(ctx, evidence, raw))
	require.NoError(t, repo.journal.save(evidence, raw))
	holder, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer holder.Rollback()
	_, err = holder.ExecContext(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", user.ID)
	require.NoError(t, err)
	cycle, cancelCycle := context.WithTimeout(ctx, 2*time.Second)
	defer cancelCycle()
	started := time.Now()
	count, err := repo.RecoverClusterSettlements(cycle)
	require.NoError(t, err)
	require.NoError(t, cycle.Err(), "authority reads must succeed while the money replay waits on a user row")
	require.Equal(t, 1, count)
	require.Less(t, time.Since(started), 1800*time.Millisecond)
	_, err = os.Stat(repo.journal.file(evidence))
	require.NoError(t, err)
	var balance float64
	var ledger int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&balance))
	require.Equal(t, 20.0, balance, "the canceled transaction must not partially charge")
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_usage_entries WHERE settlement_event_id=$1", cmd.RequestID).Scan(&ledger))
	require.Zero(t, ledger)
	require.NoError(t, holder.Rollback())
	for attempt := 0; attempt < 2; attempt++ {
		count, err = repo.RecoverClusterSettlements(ctx)
		require.NoError(t, err)
		require.Zero(t, count)
	}
	require.NoError(t, db.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&balance))
	require.Equal(t, 18.0, balance)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_usage_entries WHERE settlement_event_id=$1", cmd.RequestID).Scan(&ledger))
	require.Equal(t, 1, ledger, "two later recovery cycles must commit the protected command exactly once")
	var state string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT state FROM cluster_settlement_events WHERE event_id=$1 AND api_key_id=$2", cmd.RequestID, cmd.APIKeyID).Scan(&state))
	require.Equal(t, "committed", state)
}

func TestRecoverClusterSettlementsBudgetDoesNotSkipDamagedEvidence(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	require.NoError(t, repo.journal.save(cmd, data))
	require.NoError(t, os.WriteFile(filepath.Join(repo.journal.dir, "zz-invalid.json"), []byte("damaged"), 0600))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	count, err := repo.RecoverClusterSettlements(ctx)
	require.ErrorContains(t, err, "invalid settlement record")
	require.Positive(t, count)
	_, err = os.Stat(repo.journal.file(cmd))
	require.NoError(t, err)
}
