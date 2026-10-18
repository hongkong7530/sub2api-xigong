package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsOutputTPSExcludesFirstTokenWait(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	mock.ExpectQuery(`(?s)ul\.output_tokens \* 1000\.0 / NULLIF\(ul\.output_duration_ms, 0\).*ul\.output_duration_ms >= 100`).WillReturnRows(sqlmock.NewRows([]string{"p5", "p10", "p50", "avg", "count"}).AddRow(201, 201, 201, 201, 1))
	start := time.Now().UTC()
	result, err := repo.queryOutputTPS(context.Background(), &service.OpsDashboardFilter{StartTime: start, EndTime: start.Add(time.Hour)})
	require.NoError(t, err)
	require.Equal(t, 201.0, *result.P50)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsOpenAITokenStatsExcludesFirstTokenWait(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	pattern := `(?s)` + regexp.QuoteMeta("ul.output_duration_ms >= 100") + `.*` + regexp.QuoteMeta("ul.output_tokens * 1000.0 / ul.output_duration_ms")
	mock.ExpectQuery(pattern).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(pattern).WillReturnRows(sqlmock.NewRows([]string{"model", "count", "rate", "first", "output", "duration", "ttft_count"}).AddRow("model", 1, 201.0, 1000.0, 201, 2000, 1))
	start := time.Now().UTC()
	result, err := repo.GetOpenAITokenStats(context.Background(), &service.OpsOpenAITokenStatsFilter{StartTime: start, EndTime: start.Add(time.Hour), Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}
