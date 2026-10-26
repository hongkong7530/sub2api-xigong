package repository

import (
	"context"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ratemetric"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRateObservationPreservedAcrossPreparedInsertAndClusterEvidence(t *testing.T) {
	ms := 2500
	o := &service.RateObservation{Version: 1, RequestDurationMs: &ms, TimingScope: "app_request", WindowMs: &ms, OutputEvents: 4, EventSpanMs: 1800, OutputBytes: 400, MaxBurstBytes: 100, Quality: "observed"}
	log := &service.UsageLog{RequestID: "observation", UserID: 1, APIKeyID: 2, AccountID: 3, Model: "mock", OutputTokens: 1000, ActualCost: 0.3, RateObservation: o}
	prepared := prepareUsageLogInsert(log)
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	raw, ok := prepared.args[len(prepared.args)-1].(string)
	require.True(t, ok)
	var restored service.RateObservation
	require.NoError(t, json.Unmarshal([]byte(raw), &restored))
	require.Equal(t, *o, restored)
	require.True(t, ratemetric.IsEligible(&restored))
	cmd := &service.UsageBillingCommand{RequestID: "observation", BalanceCost: 0.3, UsageLog: log}
	e, err := settlementEvidence(cmd)
	require.NoError(t, err)
	data, err := json.Marshal(e)
	require.NoError(t, err)
	var roundTrip service.UsageBillingCommand
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	require.Equal(t, o, roundTrip.UsageLog.RateObservation)
	require.Equal(t, 0.3, roundTrip.BalanceCost)
	log.RateObservation = nil
	require.Nil(t, prepareUsageLogInsert(log).args[len(prepared.args)-1])
	require.Contains(t, usageLogSelectColumns, "rate_observation")
}

func TestRateObservationSQLScanRoundTripAndLegacyNull(t *testing.T) {
	ms := 2500
	o := &service.RateObservation{Version: 1, RequestDurationMs: &ms, TimingScope: "app_request", WindowMs: &ms, Quality: "short_window"}
	for _, observation := range []*service.RateObservation{o, nil} {
		log := &service.UsageLog{UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "scan", Model: "mock", OutputTokens: 1000, RateObservation: observation}
		p := prepareUsageLogInsert(log)
		// SELECT prepends id and excludes node_id; retain the exact existing insert/scan order.
		values := append([]any{int64(99)}, p.args[:62]...)
		values = append(values, p.args[63:]...)
		db, mock := newSQLMock(t)
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("SELECT .*rate_observation FROM usage_logs WHERE id").WithArgs(int64(99)).WillReturnRows(sqlmock.NewRows(strings.Split(usageLogSelectColumns, ", ")).AddRow(anySliceToDriverValues(values)...))
		got, err := repo.GetByID(context.Background(), 99)
		require.NoError(t, err)
		require.Equal(t, observation, got.RateObservation)
		require.Equal(t, 1000, got.OutputTokens)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}
