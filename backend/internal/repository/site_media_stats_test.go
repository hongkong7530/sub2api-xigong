package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSiteAssetQuotaAndDedup(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &siteContentRepository{db: db}
	m.ExpectBegin()
	m.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(27716001)")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("SELECT EXISTS").WithArgs("asset").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	m.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count", "bytes"}).AddRow(0, 0))
	m.ExpectExec("INSERT INTO site_assets").WithArgs("asset", "image/jpeg", []byte{1, 2}).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	require.NoError(t, r.SaveSiteAsset(context.Background(), "asset", "image/jpeg", []byte{1, 2}))
	require.NoError(t, m.ExpectationsWereMet())
}
func TestSiteUsageUsesLifetimeRetainedCountsAndWeightedTokenPrice(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &siteContentRepository{db: db}
	start := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	m.ExpectBegin()
	m.ExpectQuery(`SELECT COALESCE\(SUM\(request_count\).*FROM usage_statistics WHERE created_at < \$2`).WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"requests", "tokens", "cost", "price_tokens", "samples"}).AddRow(1200, 4000000, 4.5, 3000000, 12))
	m.ExpectQuery(`SELECT COALESCE.*FROM usage_statistics WHERE created_at >=`).WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"model", "tokens", "requests"}).AddRow("gpt-6.1-sol", 3000000, 10).AddRow("private-client-name", 1000000, 2))
	m.ExpectCommit()
	got, err := r.PublicSiteUsage(context.Background(), start, end)
	require.NoError(t, err)
	require.Equal(t, int64(1200), got.Requests)
	require.Equal(t, int64(4000000), got.TodayTokens)
	require.InDelta(t, 1.5, *got.AveragePerMillion, 1e-9)
	require.Len(t, got.Models, 1)
	require.NoError(t, m.ExpectationsWereMet())
}
