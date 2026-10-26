package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestClusterSettlementPostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	id := uuid.NewString()
	clusterID := "billing-" + id
	user, err := client.User.Create().SetEmail(id + "@example.invalid").SetPasswordHash("fixture").SetBalance(20).Save(ctx)
	require.NoError(t, err)
	account, err := client.Account.Create().SetName(id).SetPlatform("openai").SetType("apikey").SetCredentials(map[string]any{}).Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetName(id).SetKey("fixture-" + id).Save(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO cluster_control(cluster_id,settlement_mode) VALUES($1,'atomic')`, clusterID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO user_platform_quotas(user_id,platform,daily_limit_usd,weekly_limit_usd,monthly_limit_usd) VALUES($1,'openai',100,100,100)`, user.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM cluster_settlement_events WHERE cluster_id=$1`, clusterID)
		_, _ = db.Exec(`DELETE FROM cluster_control WHERE cluster_id=$1`, clusterID)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, user.ID)
		_, _ = db.Exec(`DELETE FROM accounts WHERE id=$1`, account.ID)
	})
	cfg := &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: clusterID, NodeID: "worker-a", StateDir: t.TempDir(), MaxInFlight: 8}}
	repoInterface, err := ProvideUsageBillingRepository(client, db, cfg)
	require.NoError(t, err)
	repo := repoInterface.(*usageBillingRepository)
	makeCommand := func(event string) *service.UsageBillingCommand {
		return &service.UsageBillingCommand{ClusterID: clusterID, NodeID: "worker-a", RequestID: event, APIKeyID: key.ID, UserID: user.ID, AccountID: account.ID, AccountType: "apikey", Model: "fixture", BillingType: service.BillingTypeBalance, BalanceCost: 2, APIKeyQuotaCost: 2, PlatformQuota: &service.PlatformQuotaCharge{Platform: "openai", Amount: 2}, UsageLog: &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: event, Model: "fixture", InputTokens: 10, TotalCost: 2, ActualCost: 2, BillingType: service.BillingTypeBalance, RateMultiplier: 1, CreatedAt: time.Now().UTC()}}
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.Apply(ctx, makeCommand("event-"+id))
			if err == nil && !result.Committed {
				err = ErrClusterSettlementPending
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var balance, daily, quota float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT u.balance,q.daily_usage_usd,k.quota_used FROM users u JOIN user_platform_quotas q ON q.user_id=u.id JOIN api_keys k ON k.user_id=u.id WHERE u.id=$1`, user.ID).Scan(&balance, &daily, &quota))
	require.Equal(t, 18.0, balance)
	require.Equal(t, 2.0, daily)
	require.Equal(t, 2.0, quota)
	_, err = db.ExecContext(ctx, `UPDATE user_platform_quotas SET daily_usage_usd=0 WHERE user_id=$1`, user.ID)
	require.Error(t, err, "legacy absolute overwrite must be fenced")
	var logID int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT usage_log_id FROM billing_usage_entries WHERE user_id=$1`, user.ID).Scan(&logID))
	_, err = db.ExecContext(ctx, `DELETE FROM usage_logs WHERE id=$1`, logID)
	require.NoError(t, err)
	var preserved int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM billing_usage_entries WHERE user_id=$1 AND usage_log_id IS NULL AND settlement_event_id=$2`, user.ID, "event-"+id).Scan(&preserved))
	require.Equal(t, 1, preserved)
	again, err := repo.Apply(ctx, makeCommand("event-"+id))
	require.NoError(t, err)
	require.True(t, again.Committed)
	require.False(t, again.Applied)
	conflict := makeCommand("event-" + id)
	conflict.BalanceCost = 3
	_, err = repo.Apply(ctx, conflict)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	pending := makeCommand("recover-" + id)
	pending.Normalize()
	safe, err := settlementEvidence(pending)
	require.NoError(t, err)
	data, err := json.Marshal(safe)
	require.NoError(t, err)
	require.NoError(t, repo.journal.save(safe, data))
	count, err := repo.RecoverClusterSettlements(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user.ID).Scan(&balance))
	require.Equal(t, 16.0, balance)
	connector, err := pq.NewConnector(dsn)
	require.NoError(t, err)
	uncertainDB := sql.OpenDB(ackConnector{Connector: connector})
	defer uncertainDB.Close()
	uncertain := &usageBillingRepository{db: uncertainDB, journal: repo.journal}
	result, err := uncertain.Apply(ctx, makeCommand("ack-lost-"+id))
	require.NoError(t, err)
	require.True(t, result.Committed)
	again, err = repo.Apply(ctx, makeCommand("ack-lost-"+id))
	require.NoError(t, err)
	require.True(t, again.Committed)
	require.False(t, again.Applied)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user.ID).Scan(&balance))
	require.Equal(t, 14.0, balance)

	executionID := uuid.NewString()
	_, err = db.ExecContext(ctx, `INSERT INTO cluster_executions(execution_id,cluster_id,node_id,boot_id,protocol,state,work_finished) VALUES($1,$2,'worker-a',$3,'fixture','executing',FALSE)`, executionID, clusterID, uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, repo.journal.reserve.Reserve(executionID))
	for _, event := range []string{"physical-a-" + id, "physical-b-" + id} {
		command := makeCommand(event)
		command.ExecutionID = executionID
		command.Normalize()
		evidence, err := settlementEvidence(command)
		require.NoError(t, err)
		encoded, err := json.Marshal(evidence)
		require.NoError(t, err)
		require.NoError(t, repo.journal.reserve.SaveEvent(executionID, event, encoded))
	}
	require.NoError(t, repo.journal.reserve.SaveCompletion(executionID, "awaiting_settlement"))
	count, err = repo.RecoverClusterSettlements(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	count, err = repo.RecoverClusterSettlements(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user.ID).Scan(&balance))
	require.Equal(t, 10.0, balance)
	var state string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT state FROM cluster_executions WHERE execution_id=$1`, executionID).Scan(&state))
	require.Equal(t, "settled", state)
	reserved, err := repo.journal.reserve.ReadAll()
	require.NoError(t, err)
	require.NotContains(t, reserved, executionID)
	_, err = db.ExecContext(ctx, `DELETE FROM cluster_executions WHERE execution_id=$1`, executionID)
	require.NoError(t, err)

	binary, err := os.Executable()
	require.NoError(t, err)
	children := make([]*exec.Cmd, 2)
	for i := range children {
		child := exec.CommandContext(ctx, binary, "-test.run=^TestClusterSettlementChildProcess$", "-test.timeout=20s")
		child.Env = append(os.Environ(), "SUB2API_CLUSTER_CHILD=1", "SUB2API_CLUSTER_CHILD_CLUSTER="+clusterID, "SUB2API_CLUSTER_CHILD_USER="+strconv.FormatInt(user.ID, 10), "SUB2API_CLUSTER_CHILD_KEY="+strconv.FormatInt(key.ID, 10), "SUB2API_CLUSTER_CHILD_ACCOUNT="+strconv.FormatInt(account.ID, 10), "SUB2API_CLUSTER_CHILD_EVENT=cross-process-"+id)
		require.NoError(t, child.Start())
		children[i] = child
	}
	for _, child := range children {
		require.NoError(t, child.Wait())
	}
	require.NoError(t, db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user.ID).Scan(&balance))
	require.Equal(t, 8.0, balance)
}

func TestClusterSettlementChildProcess(t *testing.T) {
	if os.Getenv("SUB2API_CLUSTER_CHILD") != "1" {
		t.Skip("only spawned by isolated multi-process settlement test")
	}
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	parse := func(name string) int64 {
		value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
		require.NoError(t, err)
		return value
	}
	user, key, account := parse("SUB2API_CLUSTER_CHILD_USER"), parse("SUB2API_CLUSTER_CHILD_KEY"), parse("SUB2API_CLUSTER_CHILD_ACCOUNT")
	clusterID, event := os.Getenv("SUB2API_CLUSTER_CHILD_CLUSTER"), os.Getenv("SUB2API_CLUSTER_CHILD_EVENT")
	cfg := &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: clusterID, NodeID: "worker-a", StateDir: t.TempDir(), MaxInFlight: 8}}
	repo, err := ProvideUsageBillingRepository(nil, db, cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := repo.Apply(ctx, &service.UsageBillingCommand{ClusterID: clusterID, NodeID: "worker-a", RequestID: event, UserID: user, APIKeyID: key, AccountID: account, AccountType: "apikey", Model: "fixture", BalanceCost: 2, APIKeyQuotaCost: 2, PlatformQuota: &service.PlatformQuotaCharge{Platform: "openai", Amount: 2}, UsageLog: &service.UsageLog{UserID: user, APIKeyID: key, AccountID: account, RequestID: event, Model: "fixture", BillingType: service.BillingTypeBalance, TotalCost: 2, ActualCost: 2, CreatedAt: time.Now().UTC()}})
	require.NoError(t, err)
	require.True(t, result.Committed)
}

func TestClusterSettlementEvidenceHasNoRelatedSecrets(t *testing.T) {
	cmd := &service.UsageBillingCommand{RequestID: "event", UsageLog: &service.UsageLog{Model: "fixture", User: &service.User{Email: "sensitive"}, UserAgent: ptrClusterString("private-agent"), IPAddress: ptrClusterString("private-address")}}
	copy, err := settlementEvidence(cmd)
	require.NoError(t, err)
	data, err := json.Marshal(copy)
	require.NoError(t, err)
	for _, secret := range []string{"sensitive", "private-agent", "private-address"} {
		require.False(t, strings.Contains(string(data), secret))
	}
}

// The admission gate counts unsettled executions, so a finished request whose money
// was already settled by another path must stop blocking admission. The verdict has to
// come from the ledger, and replayable evidence must be left untouched.
func TestClusterSettlementReconcilesStaleExecutionsFromLedger(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	id := uuid.NewString()
	clusterID := "stale-" + id
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	user, err := client.User.Create().SetEmail(id + "@example.invalid").SetPasswordHash("fixture").SetBalance(10).Save(ctx)
	require.NoError(t, err)
	account, err := client.Account.Create().SetName(id).SetPlatform("openai").SetType("apikey").SetCredentials(map[string]any{}).Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetName(id).SetKey("fixture-" + id).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM cluster_settlement_events WHERE cluster_id=$1`, clusterID)
		_, _ = db.Exec(`DELETE FROM cluster_executions WHERE cluster_id=$1`, clusterID)
		_, _ = db.Exec(`DELETE FROM billing_usage_entries WHERE user_id=$1`, user.ID)
		_, _ = db.Exec(`DELETE FROM usage_logs WHERE user_id=$1`, user.ID)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, user.ID)
		_, _ = db.Exec(`DELETE FROM accounts WHERE id=$1`, account.ID)
		_, _ = db.Exec(`DELETE FROM api_keys WHERE id=$1`, key.ID)
	})
	cfg := &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: clusterID, NodeID: "worker-a", StateDir: t.TempDir(), MaxInFlight: 8}}
	repoInterface, err := ProvideUsageBillingRepository(client, db, cfg)
	require.NoError(t, err)
	repo := repoInterface.(*usageBillingRepository)
	// Age the rows past the grace window so the live-request guard is exercised too.
	makeStale := func(state string) string {
		executionID := uuid.NewString()
		_, err := db.ExecContext(ctx, `INSERT INTO cluster_executions(execution_id,cluster_id,node_id,boot_id,protocol,state,work_finished,created_at) VALUES($1,$2,'worker-a',$3,'fixture',$4,TRUE,NOW()-INTERVAL '30 minutes')`, executionID, clusterID, uuid.NewString(), state)
		require.NoError(t, err)
		return executionID
	}
	// Booked: a usage log with a positive ledger row exists for this execution.
	booked := makeStale("needs_review")
	bookedCommand := &service.UsageBillingCommand{ClusterID: clusterID, NodeID: "worker-a", RequestID: "execution:" + booked, APIKeyID: key.ID, UserID: user.ID, AccountID: account.ID, AccountType: "apikey", Model: "fixture", BillingType: service.BillingTypeBalance, BalanceCost: 2, UsageLog: &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: "execution:" + booked, Model: "fixture", TotalCost: 2, ActualCost: 2, BillingType: service.BillingTypeBalance, RateMultiplier: 1, CreatedAt: time.Now().UTC()}}
	bookedCommand.Normalize()
	_, err = repo.Apply(ctx, bookedCommand)
	require.NoError(t, err)
	// Unbooked: finished with an error and no monetary obligation at all.
	unbooked := makeStale("needs_review")
	// Evidence-bearing: a replayable slot must be skipped by reconciliation.
	withEvidence := makeStale("awaiting_settlement")
	require.NoError(t, repo.journal.reserve.Reserve(withEvidence))
	command := &service.UsageBillingCommand{ClusterID: clusterID, NodeID: "worker-a", RequestID: "evidence-" + withEvidence, APIKeyID: key.ID, UserID: user.ID, AccountID: account.ID, Model: "fixture", BillingType: service.BillingTypeBalance, BalanceCost: 2, UsageLog: &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: "evidence-" + withEvidence, Model: "fixture", TotalCost: 2, ActualCost: 2, BillingType: service.BillingTypeBalance, RateMultiplier: 1, CreatedAt: time.Now().UTC()}, ExecutionID: withEvidence}
	command.Normalize()
	evidence, err := settlementEvidence(command)
	require.NoError(t, err)
	encoded, err := json.Marshal(evidence)
	require.NoError(t, err)
	require.NoError(t, repo.journal.reserve.SaveEvent(withEvidence, command.RequestID, encoded))
	// A live row inside the grace window must never be touched.
	live := uuid.NewString()
	_, err = db.ExecContext(ctx, `INSERT INTO cluster_executions(execution_id,cluster_id,node_id,boot_id,protocol,state,work_finished) VALUES($1,$2,'worker-a',$3,'fixture','needs_review',TRUE)`, live, clusterID, uuid.NewString())
	require.NoError(t, err)
	// Completion-only evidence must not be confused with unreplayed monetary commands.
	completionOnly := makeStale("needs_review")
	require.NoError(t, repo.journal.reserve.Reserve(completionOnly))
	require.NoError(t, repo.journal.reserve.SaveCompletion(completionOnly, "needs_review"))
	pendingEvidence, err := repo.journal.reserve.ReadAll()
	require.NoError(t, err)
	resolved, err := repo.reconcileStaleExecutions(ctx, pendingEvidence, clusterStaleExecutionGrace, clusterStaleExecutionBatch)
	require.NoError(t, err)
	require.Equal(t, 3, resolved, "booked, unbooked and completion-only finished rows are reconciled")
	stateOf := func(executionID string) string {
		var state string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT state FROM cluster_executions WHERE execution_id=$1`, executionID).Scan(&state))
		return state
	}
	require.Equal(t, "settled", stateOf(booked), "ledger shows the money was taken")
	require.Equal(t, "rejected", stateOf(unbooked), "no ledger row proves no money was taken")
	require.Equal(t, "awaiting_settlement", stateOf(withEvidence), "replayable evidence must be left for the replay loop")
	require.Equal(t, "needs_review", stateOf(live), "a request inside the grace window must not be raced")
	require.Equal(t, "rejected", stateOf(completionOnly))
	remaining, err := repo.journal.reserve.ReadAll()
	require.NoError(t, err)
	require.NotContains(t, remaining, completionOnly)
	require.NotEmpty(t, remaining[withEvidence], "monetary evidence is retained")
	var balance float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user.ID).Scan(&balance))
	require.Equal(t, 8.0, balance, "reconciliation never moves money")
}

func ptrClusterString(value string) *string { return &value }
