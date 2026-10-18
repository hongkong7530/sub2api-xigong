package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMonitorLatestQueriesPreserveFirstToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &channelMonitorRepository{db: db}
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT DISTINCT ON \(model\)`).WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"model", "status", "latency_ms", "first_token_ms", "output_tokens_per_second", "checked_at"}).
			AddRow("gpt-test", "operational", 1400, 1320, 10, now).
			AddRow("gpt-other", "error", 0, nil, nil, now),
	)
	single, err := repo.ListLatestPerModel(context.Background(), 3)
	require.NoError(t, err)
	require.Len(t, single, 2)
	require.Equal(t, 1320, *single[0].FirstTokenMs)
	require.Equal(t, 10.0, *single[0].OutputTokensPerSecond)
	require.Nil(t,single[1].OutputTokensPerSecond)
	require.Nil(t, single[1].FirstTokenMs)
	mock.ExpectQuery(`SELECT DISTINCT ON \(monitor_id, model\)`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"monitor_id", "model", "status", "latency_ms", "first_token_ms", "output_tokens_per_second", "checked_at", "quota"}).
			AddRow(3, "gpt-test", "operational", 1400, 1320, 10, now, nil),
	)
	batch, err := repo.ListLatestForMonitorIDs(context.Background(), []int64{3})
	require.NoError(t, err)
	require.Len(t, batch[3], 1)
	require.Equal(t, 1320, *batch[3][0].FirstTokenMs)
	require.Equal(t, 10.0, *batch[3][0].OutputTokensPerSecond)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMonitorRecentHistoryPreservesFirstToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &channelMonitorRepository{db: db}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT monitor_id, status, latency_ms, first_token_ms, output_tokens_per_second, checked_at")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 10).WillReturnRows(
		sqlmock.NewRows([]string{"monitor_id", "status", "latency_ms", "first_token_ms", "output_tokens_per_second", "checked_at"}).
			AddRow(3, "operational", 1400, 1320, 10, time.Now().UTC()),
	)
	rows, err := repo.ListRecentHistoryForMonitors(context.Background(), []int64{3}, map[int64]string{3: "gpt-test"}, 10)
	require.NoError(t, err)
	require.Len(t, rows[3], 1)
	require.Equal(t, 1320, *rows[3][0].FirstTokenMs)
	require.Equal(t, 10.0, *rows[3][0].OutputTokensPerSecond)
	require.NoError(t, mock.ExpectationsWereMet())
}
