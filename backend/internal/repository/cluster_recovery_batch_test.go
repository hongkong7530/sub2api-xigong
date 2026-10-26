package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecoveredCompletionsUseTwoQueriesForManySlots(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	reserve, err := cluster.NewRecoveryReserve(t.TempDir(), 300)
	require.NoError(t, err)
	repo := &usageBillingRepository{db: db, journal: &clusterSettlementJournal{clusterID: "fixture", nodeID: "worker", reserve: reserve}}
	ids := make([]string, 250)
	completions := make(map[string]string)
	for i := range ids {
		ids[i] = uuid.NewString()
		completions[ids[i]] = "needs_review"
	}
	terminal := ids[0]
	require.NoError(t, reserve.Reserve(terminal))
	require.NoError(t, reserve.SaveCompletion(terminal, "needs_review"))
	mock.ExpectExec("UPDATE cluster_executions x SET work_finished=TRUE").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "fixture", "worker").
		WillReturnResult(sqlmock.NewResult(0, 250))
	mock.ExpectQuery("SELECT execution_id::text FROM cluster_executions").
		WithArgs("fixture", "worker", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"execution_id"}).AddRow(terminal))
	require.NoError(t, repo.finishRecoveredExecutions(context.Background(), completions, ids))
	remaining, err := reserve.ReadAll()
	require.NoError(t, err)
	require.NotContains(t, remaining, terminal)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoveredCompletionFailurePreservesSlot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   string
		missing bool
	}{
		{name: "database failure", state: "needs_review"},
		{name: "missing identity", state: "awaiting_settlement", missing: true},
		{name: "invalid state", state: "settled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			reserve, err := cluster.NewRecoveryReserve(t.TempDir(), 128)
			require.NoError(t, err)
			id := uuid.NewString()
			require.NoError(t, reserve.Reserve(id))
			require.NoError(t, reserve.SaveCompletion(id, "needs_review"))
			repo := &usageBillingRepository{db: db, journal: &clusterSettlementJournal{clusterID: "fixture", nodeID: "worker", reserve: reserve}}
			if tc.state != "settled" {
				e := mock.ExpectExec("UPDATE cluster_executions x SET work_finished=TRUE")
				if tc.missing {
					e.WillReturnResult(sqlmock.NewResult(0, 0))
				} else {
					e.WillReturnError(errors.New("unavailable"))
				}
			}
			require.Error(t, repo.finishRecoveredExecutions(context.Background(), map[string]string{id: tc.state}, []string{id}))
			remaining, err := reserve.ReadAll()
			require.NoError(t, err)
			require.NotEmpty(t, remaining[id])
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestReconcileCompletionOnlyDoesNotExcludeButMoneyDoes(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	reserve, err := cluster.NewRecoveryReserve(t.TempDir(), 128)
	require.NoError(t, err)
	repo := &usageBillingRepository{db: db, journal: &clusterSettlementJournal{clusterID: "fixture", nodeID: "worker", reserve: reserve}}
	completion, monetary := uuid.NewString(), uuid.NewString()
	require.NoError(t, reserve.Reserve(completion))
	require.NoError(t, reserve.SaveCompletion(completion, "needs_review"))
	require.NoError(t, reserve.Reserve(monetary))
	require.NoError(t, reserve.SaveEvent(monetary, "event", []byte(`{"amount":1}`)))
	evidence, err := reserve.ReadAll()
	require.NoError(t, err)
	// Only the monetary ID is passed to the SQL exclusion array; a completion
	// marker must be reconciled using committed ledger facts, never assumed paid.
	mock.ExpectQuery("WITH candidates AS").
		WithArgs("fixture", "worker", 600, 64, "{\""+monetary+"\"}").
		WillReturnRows(sqlmock.NewRows([]string{"execution_id"}).AddRow(completion))
	count, err := repo.reconcileStaleExecutions(context.Background(), evidence, 10*time.Minute, 64)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	remaining, err := reserve.ReadAll()
	require.NoError(t, err)
	require.NotContains(t, remaining, completion)
	var envelope cluster.RecoveryEvents
	require.NoError(t, json.Unmarshal(remaining[monetary], &envelope))
	require.Len(t, envelope.Events, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconcileMalformedEvidenceFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageBillingRepository{db: db, journal: &clusterSettlementJournal{clusterID: "fixture", nodeID: "worker"}}
	_, err = repo.reconcileStaleExecutions(context.Background(), map[string][]byte{uuid.NewString(): []byte(`{"version":2}`)}, time.Minute, 64)
	require.ErrorContains(t, err, "invalid reserved settlement envelope")
	require.NoError(t, mock.ExpectationsWereMet())
}
