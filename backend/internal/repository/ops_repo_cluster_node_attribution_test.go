package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetClusterRequestCountsGroupsByPersistedNodeID(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT node_id, SUM(request_count)")).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "request_count"}).
			AddRow("xigong2", int64(5)).
			AddRow("xigong", int64(3)).
			AddRow("legacy", int64(1)))

	counts, err := repo.GetClusterRequestCounts(context.Background(), start, end)
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"xigong2": 5, "xigong": 3, "legacy": 1}, counts)
	require.NoError(t, mock.ExpectationsWereMet())
}
