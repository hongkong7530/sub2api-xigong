package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMonitorRateInitialRecordIsNotAWindowAssumption(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &channelMonitorRepository{db: db}
	at := time.Now().UTC()
	mock.ExpectQuery(`(?s)SELECT rate_multiplier.*NOT EXISTS.*older.recorded_at.*ORDER BY recorded_at DESC,id DESC LIMIT`).WithArgs(int64(9), 120).
		WillReturnRows(sqlmock.NewRows([]string{"rate", "peak", "peak_rate", "start", "end", "recorded_at", "is_initial"}).AddRow(.25, false, 1, "", "", at, false).AddRow(.3, false, 1, "", "", at.Add(-time.Hour), true))
	points, err := repo.ListGroupRateHistory(context.Background(), 9, 120)
	require.NoError(t, err)
	require.Len(t, points, 2)
	require.False(t, points[0].IsInitial)
	require.True(t, points[1].IsInitial)
	require.Equal(t, at, points[0].RecordedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
