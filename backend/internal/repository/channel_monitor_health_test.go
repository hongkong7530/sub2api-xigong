package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMonitorHealthCombinesRealAndActiveFactsWithSampleCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.MatchExpectationsInOrder(false)
	repo := &channelMonitorRepository{db: db}
	mock.ExpectQuery(`WITH targets AS`).WithArgs(sqlmock.AnyArg(), 7).WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "model", "total", "ok", "avg"}).AddRow(3, "new-model", 3, 2, 1500))
	now := time.Now().UTC()
	mock.ExpectQuery(`WITH targets AS`).WithArgs(sqlmock.AnyArg(), 30, 60).WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "model", "status", "latency", "first", "output_rate", "at", "source"}).AddRow(3, "new-model", "error", 2000, nil, nil, now, "user_request").AddRow(3, "new-model", "operational", 1000, 800, 2, now.Add(-time.Second), "probe"))
	got, err := repo.ReadMonitorHealth(context.Background(), []int64{3, 4}, 7, 60)
	require.NoError(t, err)
	require.Equal(t, 3, got[3].Availability.TotalChecks)
	require.InDelta(t, 200.0/3, got[3].Availability.AvailabilityPct, 1e-9)
	require.Len(t, got[3].Recent, 2)
	require.Equal(t, "user_request", got[3].Recent[0].Source)
	require.Nil(t, got[3].Recent[0].FirstTokenMs)
	require.Equal(t, 800, *got[3].Recent[1].FirstTokenMs)
	require.Equal(t, 2.0, *got[3].Recent[1].OutputTokensPerSecond)
	require.Nil(t, got[3].Recent[0].OutputTokensPerSecond)
	require.Zero(t, got[4].Availability.TotalChecks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMonitorHealthKeepsErrorsWithoutResolvedModel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.MatchExpectationsInOrder(false)
	repo := &channelMonitorRepository{db: db}
	// Both reads must normalize nullable model names without dropping failures.
	query := `(?s)WITH targets AS.*COALESCE\(NULLIF\(u.requested_model,''\),u.model,''\).*COALESCE\(NULLIF\(e.requested_model,''\),e.model,''\)`
	mock.ExpectQuery(query).WithArgs(sqlmock.AnyArg(), 7).WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "model", "total", "ok", "avg"}).AddRow(3, "", 1, 0, nil))
	now := time.Now().UTC()
	mock.ExpectQuery(`(?s)WITH targets AS.*recent_facts AS.*COALESCE\(NULLIF\(u.requested_model,''\),u.model,''\).*COALESCE\(NULLIF\(e.requested_model,''\),e.model,''\)`).WithArgs(sqlmock.AnyArg(), 30, 60).WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "model", "status", "latency", "first", "output_rate", "at", "source"}).AddRow(3, "", "error", nil, nil, nil, now, "user_request"))
	got, err := repo.ReadMonitorHealth(context.Background(), []int64{3}, 7, 60)
	require.NoError(t, err)
	require.Equal(t, 1, got[3].Availability.TotalChecks)
	require.Zero(t, got[3].Availability.OperationalChecks)
	require.Zero(t, got[3].Availability.AvailabilityPct)
	require.Equal(t, 1, got[3].Models[""].TotalChecks)
	require.Len(t, got[3].Recent, 1)
	require.Empty(t, got[3].Recent[0].Model)
	require.Equal(t, "error", got[3].Recent[0].Status)
	require.Equal(t, "user_request", got[3].Recent[0].Source)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMonitorGroupCacheUsesOneSnapshotForBucketsAndSampleCount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.MatchExpectationsInOrder(false)
	repo := &channelMonitorRepository{db: db}
	mock.ExpectQuery(`COUNT\(ul.id\) FILTER`).WithArgs(int64(9), int64(86400)).WillReturnRows(sqlmock.NewRows([]string{"rate", "type", "peak", "mult", "start", "end", "read", "write", "input", "samples", "at"}).AddRow(0.07, "standard", false, 1.0, "", "", 300, 100, 600, 2, time.Now()))
	got, err := repo.GetGroupMetrics(context.Background(), 9, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.CacheSampleCount)
	require.Equal(t, int64(86400), got.CacheWindowSeconds)
	require.Equal(t, 30.0, got.CacheHitRate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMonitorRecentHealthUsesBoundedSourcesAndDeduplicatedFailureSet(t *testing.T) {
	require.Contains(t, monitorRecentHealthSQL, "ORDER BY u.created_at DESC,u.id DESC LIMIT $3")
	require.Contains(t, monitorRecentHealthSQL, "ORDER BY e.created_at DESC,e.id DESC LIMIT $3")
	require.Contains(t, monitorRecentHealthSQL, "ORDER BY h.checked_at DESC,h.id DESC LIMIT $3")
	require.Contains(t, monitorRecentHealthSQL, "FROM error_keys e WHERE e.api_key_id IS NOT NULL")
	require.Contains(t, monitorRecentHealthSQL, "u.api_key_id IS NULL OR u.request_id IS NULL")
	require.Contains(t, monitorRecentHealthSQL, "(u.api_key_id,u.request_id) NOT IN")
	require.Contains(t, monitorRecentHealthSQL, "PARTITION BY monitor_id ORDER BY checked_at DESC,source,fact_id DESC")
	require.Contains(t, monitorRecentHealthSQL, "p.provisioned_api_key_id=u.api_key_id")
	require.NotContains(t, monitorHealthSourcesSQL, "e.*")
}

func TestMonitorDisplayedRateSeparatesTimingCertificationFromEligibility(t *testing.T) {
	for name, query := range map[string]string{"facts": monitorHealthFactsSQL, "recent": monitorRecentHealthSQL} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, query, "quality' IN ('observed','timing_uncertain')")
			for _, gate := range []string{"u.output_tokens>0", "u.output_duration_ms>=1000", "(u.rate_observation->>'version')::int=1", "(u.rate_observation->>'window_ms')::int=u.output_duration_ms", "(u.rate_observation->>'output_events')::int>=3", "(u.rate_observation->>'event_span_ms')::int>=250", "(u.rate_observation->>'max_burst_bytes')::bigint*5<(u.rate_observation->>'output_bytes')::bigint*4", "COALESCE(u.request_type,0) IN(2,3)", "COALESCE(u.image_count,0)=0", "COALESCE(u.image_output_tokens,0)=0", "COALESCE(u.billing_mode,'token')='token'"} {
				require.Contains(t, query, gate)
			}
		})
	}
}

func TestMonitorAvailabilityAggregatesSourcesWithoutChangingEligibility(t *testing.T) {
	require.Contains(t, monitorHealthAvailabilitySQL, "source_totals AS")
	require.Contains(t, monitorHealthAvailabilitySQL, "FROM targets t CROSS JOIN LATERAL")
	require.Contains(t, monitorHealthAvailabilitySQL, "u.group_id=t.group_id AND u.created_at>=NOW()")
	require.Contains(t, monitorHealthAvailabilitySQL, "COUNT(*) AS total,COUNT(*) AS operational")
	require.Contains(t, monitorHealthAvailabilitySQL, "SUM(latency_sum)/NULLIF(SUM(latency_count),0)")
	require.Contains(t, monitorHealthAvailabilitySQL, "u.api_key_id IS NULL OR u.request_id IS NULL")
	require.Contains(t, monitorHealthAvailabilitySQL, "FROM error_keys e WHERE e.api_key_id IS NOT NULL")
	require.Contains(t, monitorHealthAvailabilitySQL, "t.probe_mode<>'passive'")
	require.Contains(t, monitorHealthAvailabilitySQL, "h.status IN('operational','degraded')")
	require.Contains(t, monitorHealthAvailabilitySQL, "u.created_at>=NOW()-($2::int*INTERVAL '1 day')")
	require.NotContains(t, monitorHealthAvailabilitySQL, "LIMIT")
	require.NotContains(t, monitorHealthAvailabilitySQL, "rate_observation")
}

func TestMonitorHealthQueryExcludesProbesAndFailedStreamDuplicates(t *testing.T) {
	require.Contains(t, monitorHealthFactsSQL, "p.provisioned_api_key_id=u.api_key_id")
	require.Contains(t, monitorHealthFactsSQL, "NOT e.resolved")
	require.Contains(t, monitorHealthFactsSQL, "NOT e.is_business_limited")
	require.Contains(t, monitorHealthFactsSQL, "SELECT DISTINCT ON (e.api_key_id")
	require.Contains(t, monitorHealthFactsSQL, "'local:'||e.request_id")
	require.Contains(t, monitorHealthFactsSQL, "e.request_id=u.request_id")
	require.Contains(t, monitorHealthFactsSQL, "FROM targets t JOIN channel_monitor_histories")
}
