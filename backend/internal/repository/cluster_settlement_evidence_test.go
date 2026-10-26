package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestSettlementEvidencePreservesRoutingWithoutClientMetadata(t *testing.T) {
	inbound, upstream := "/v1/chat/completions", "/v1/responses"
	ua, ip := "private-client-agent", "192.0.2.1"
	cmd := &service.UsageBillingCommand{UsageLog: &service.UsageLog{
		Model: "gpt-test", InboundEndpoint: &inbound, UpstreamEndpoint: &upstream,
		InputTokens: 100, OutputTokens: 10, CacheReadTokens: 50,
		TotalCost: 0.01, ActualCost: 0.005, UserAgent: &ua, IPAddress: &ip,
	}}
	evidence, err := settlementEvidence(cmd)
	require.NoError(t, err)
	require.Equal(t, cmd.UsageLog.InboundEndpoint, evidence.UsageLog.InboundEndpoint)
	require.Equal(t, cmd.UsageLog.UpstreamEndpoint, evidence.UsageLog.UpstreamEndpoint)
	require.Equal(t, cmd.UsageLog.InputTokens, evidence.UsageLog.InputTokens)
	require.Equal(t, cmd.UsageLog.CacheReadTokens, evidence.UsageLog.CacheReadTokens)
	require.Equal(t, cmd.UsageLog.ActualCost, evidence.UsageLog.ActualCost)
	require.Nil(t, evidence.UsageLog.UserAgent)
	require.Nil(t, evidence.UsageLog.IPAddress)

	data, err := json.Marshal(evidence)
	require.NoError(t, err)
	var recovered service.UsageBillingCommand
	require.NoError(t, json.Unmarshal(data, &recovered))
	require.Equal(t, &inbound, recovered.UsageLog.InboundEndpoint)
	require.Equal(t, &upstream, recovered.UsageLog.UpstreamEndpoint)
	require.Equal(t, evidence.UsageLog.ActualCost, recovered.UsageLog.ActualCost)
}

func pendingReplayRepository(t *testing.T) (*usageBillingRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = db.Close() })
	state := t.TempDir()
	dir := filepath.Join(state, "settlements")
	require.NoError(t, os.MkdirAll(dir, 0700))
	reserve, err := cluster.NewRecoveryReserve(state, 8)
	require.NoError(t, err)
	return &usageBillingRepository{db: db, journal: &clusterSettlementJournal{
		dir: dir, reserve: reserve, clusterID: "replay-test", nodeID: "node-a", maxRecords: 32,
	}}, mock
}

func pendingReplayCommand(t *testing.T, execution string) (*service.UsageBillingCommand, []byte) {
	t.Helper()
	cmd, err := settlementEvidence(&service.UsageBillingCommand{
		ClusterID: "replay-test", NodeID: "node-a", ExecutionID: execution,
		RequestID: "event-one", APIKeyID: 1, RequestFingerprint: "fingerprint-one",
		UsageLog: &service.UsageLog{RequestID: "event-one", APIKeyID: 1, Model: "fixture"},
	})
	require.NoError(t, err)
	data, err := json.Marshal(cmd)
	require.NoError(t, err)
	return cmd, data
}

func expectPendingReplayTail(mock sqlmock.Sqlmock, count int) {
	mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnRows(sqlmock.NewRows([]string{"command"}))
	mock.ExpectQuery("WITH candidates AS").WillReturnRows(sqlmock.NewRows([]string{"execution_id"}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM outstanding").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func TestRecoverClusterSettlementsDurablePendingRetainsReservedCompletion(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	id := "c9ac981e-c6f6-4773-9c9b-9dce34a1cb39"
	cmd, data := pendingReplayCommand(t, id)
	require.NoError(t, repo.journal.reserve.Reserve(id))
	envelope := cluster.RecoveryEvents{Version: 1, FinishState: "awaiting_settlement", Events: map[string]json.RawMessage{cmd.RequestID: data}}
	raw, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.NoError(t, repo.journal.reserve.Save(id, raw))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	// A pending slot must not reach completion UPDATE or terminal-slot release.
	expectPendingReplayTail(mock, 1)
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, pending)
	slots, err := repo.journal.reserve.ReadAll()
	require.NoError(t, err)
	var kept cluster.RecoveryEvents
	require.NoError(t, json.Unmarshal(slots[id], &kept))
	require.Equal(t, envelope.FinishState, kept.FinishState)
	require.JSONEq(t, string(data), string(kept.Events[cmd.RequestID]))
}

func TestRecoverClusterSettlementsDurablePendingRetainsJournal(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	require.NoError(t, repo.journal.save(cmd, data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	expectPendingReplayTail(mock, 1)
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, pending)
	kept, err := os.ReadFile(repo.journal.file(cmd))
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(kept))
}

func TestRecoverClusterSettlementsDurablePendingDatabaseBatch(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnRows(sqlmock.NewRows([]string{"command"}).AddRow(data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	mock.ExpectQuery("WITH candidates AS").WillReturnRows(sqlmock.NewRows([]string{"execution_id"}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM outstanding").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, pending)
	kept, err := os.ReadFile(repo.journal.file(cmd))
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(kept))
}

func TestRecoverClusterSettlementsPendingStillRejectsFingerprintConflict(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	require.NoError(t, repo.journal.save(cmd, data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnResult(sqlmock.NewResult(0, 0))
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.Positive(t, pending)
	_, err = os.Stat(repo.journal.file(cmd))
	require.NoError(t, err)
}

func TestRecoverClusterSettlementsPendingStillRejectsAuthorityReadFailure(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	require.NoError(t, repo.journal.save(cmd, data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	failure := errors.New("authority read unavailable")
	mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnError(failure)
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.ErrorIs(t, err, failure)
	require.Positive(t, pending)
	_, err = os.Stat(repo.journal.file(cmd))
	require.NoError(t, err)
}

func TestRecoverClusterSettlementsStillRejectsInvalidEvidence(t *testing.T) {
	repo, _ := pendingReplayRepository(t)
	id := "c9ac981e-c6f6-4773-9c9b-9dce34a1cb39"
	require.NoError(t, repo.journal.reserve.Reserve(id))
	require.NoError(t, repo.journal.reserve.Save(id, []byte(`{"version":0,"events":{}}`)))
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.ErrorContains(t, err, "invalid reserved settlement envelope")
	require.Positive(t, pending)
	slots, readErr := repo.journal.reserve.ReadAll()
	require.NoError(t, readErr)
	require.Contains(t, slots, id)
}

func TestRecoverClusterSettlementsDeduplicatesAllPendingCopies(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	id := "c9ac981e-c6f6-4773-9c9b-9dce34a1cb39"
	cmd, data := pendingReplayCommand(t, id)
	require.NoError(t, repo.journal.reserve.Reserve(id))
	envelope, err := json.Marshal(cluster.RecoveryEvents{Version: 1, FinishState: "awaiting_settlement", Events: map[string]json.RawMessage{cmd.RequestID: data}})
	require.NoError(t, err)
	require.NoError(t, repo.journal.reserve.Save(id, envelope))
	require.NoError(t, repo.journal.save(cmd, data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	mock.ExpectQuery("SELECT command FROM cluster_settlement_events").WillReturnRows(sqlmock.NewRows([]string{"command"}).AddRow(data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	mock.ExpectQuery("WITH candidates AS").WillReturnRows(sqlmock.NewRows([]string{"execution_id"}))
	// All three copies must contribute one paired identity to the union query.
	mock.ExpectQuery("WITH local_pending").WithArgs("replay-test", "node-a", pq.Array([]string{cmd.RequestID}), pq.Array([]int64{cmd.APIKeyID})).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, pending)
	slots, err := repo.journal.reserve.ReadAll()
	require.NoError(t, err)
	require.Contains(t, slots, id)
	_, err = os.Stat(repo.journal.file(cmd))
	require.NoError(t, err)
}

func TestRecoverClusterSettlementsRejectsInvalidCompletionBeforeReplay(t *testing.T) {
	for _, finish := range []string{"settled", "unexpected", "awaiting_settlement "} {
		t.Run(finish, func(t *testing.T) {
			repo, _ := pendingReplayRepository(t)
			id := "c9ac981e-c6f6-4773-9c9b-9dce34a1cb39"
			cmd, data := pendingReplayCommand(t, id)
			require.NoError(t, repo.journal.reserve.Reserve(id))
			envelope, err := json.Marshal(cluster.RecoveryEvents{Version: 1, FinishState: finish, Events: map[string]json.RawMessage{cmd.RequestID: data}})
			require.NoError(t, err)
			require.NoError(t, repo.journal.reserve.Save(id, envelope))
			// Corrupt completion state must fail before any monetary replay or SQL.
			pending, err := repo.RecoverClusterSettlements(context.Background())
			require.ErrorContains(t, err, "invalid execution completion state")
			require.Positive(t, pending)
			slots, readErr := repo.journal.reserve.ReadAll()
			require.NoError(t, readErr)
			require.Contains(t, slots, id)
		})
	}
}

func TestRecoverClusterSettlementsAuthorityCommitSupersedesLocalPending(t *testing.T) {
	repo, mock := pendingReplayRepository(t)
	cmd, data := pendingReplayCommand(t, "")
	require.NoError(t, repo.journal.save(cmd, data))
	mock.ExpectExec("INSERT INTO cluster_settlement_events").WillReturnError(errors.New("transient authority write"))
	expectPendingReplayTail(mock, 0)
	pending, err := repo.RecoverClusterSettlements(context.Background())
	require.NoError(t, err)
	require.Zero(t, pending, "an authority-confirmed commit is not a new obligation")
	_, err = os.Stat(repo.journal.file(cmd))
	require.NoError(t, err, "the scan itself does not discard its protected copy")
}

const clusterPendingCountFixtures = `WITH cluster_settlement_events(event_id,api_key_id,cluster_id,node_id,state) AS (
 VALUES('evt-a'::text,1::bigint,'replay-test'::text,'node-a'::text,'pending'::text),
 ('evt-b',2,'replay-test','node-a','committed'),
 ('evt-c',3,'replay-test','node-b','pending'),
 ('evt-d',4,'another-cluster','node-a','pending'),
 ('evt-e',5,'replay-test','node-a','pending'),
 ('evt-z',6,'replay-test','node-b','committed')
)`

const clusterPendingCountCases = `[
 {"name":"authority_only","events":[],"keys":[],"expected":2},
 {"name":"local_authority_duplicates","events":["evt-a","evt-a","evt-a"],"keys":[1,1,1],"expected":2},
 {"name":"local_committed","events":["evt-b"],"keys":[2],"expected":2},
 {"name":"new_local","events":["evt-new"],"keys":[7],"expected":3},
 {"name":"new_local_duplicates","events":["evt-new","evt-new"],"keys":[7,7],"expected":3},
 {"name":"same_event_different_key","events":["evt-a"],"keys":[99],"expected":3},
 {"name":"mixed_copies","events":["evt-a","evt-new","evt-a"],"keys":[1,7,1],"expected":3},
 {"name":"committed_on_other_node","events":["evt-z"],"keys":[6],"expected":2},
 {"name":"pending_on_other_node","events":["evt-c"],"keys":[3],"expected":3},
 {"name":"different_cluster_not_confirmation","events":["evt-d"],"keys":[4],"expected":3}
]`

func TestClusterPendingRecoveryCountPostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	var cases []struct {
		Name     string   `json:"name"`
		Events   []string `json:"events"`
		Keys     []int64  `json:"keys"`
		Expected int      `json:"expected"`
	}
	require.NoError(t, json.Unmarshal([]byte(clusterPendingCountCases), &cases))
	query := strings.Replace(clusterPendingRecoveryCountSQL, "WITH local_pending", clusterPendingCountFixtures+", local_pending", 1)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var count int
			require.NoError(t, db.QueryRowContext(context.Background(), query, "replay-test", "node-a", pq.Array(tc.Events), pq.Array(tc.Keys)).Scan(&count))
			require.Equal(t, tc.Expected, count)
		})
	}
}
