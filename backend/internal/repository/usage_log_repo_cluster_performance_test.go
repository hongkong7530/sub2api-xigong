package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestClusterPerformanceSameWindowAndZeroTrafficNodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	end := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)WITH recent AS.*FROM usage_logs WHERE created_at >= \$1 AND created_at < \$2.*FROM usage_retained_hourly.*FROM cluster_nodes WHERE state <> 'retired'.*ORDER BY m.node_id`).
		WithArgs(end.Add(-5*time.Minute), end).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "requests", "tokens"}).
			AddRow("unassigned", 1, 5).AddRow("xigong", 0, 0).AddRow("xigong2", 2, 20).AddRow("xigong3", 3, 30))
	r := &usageLogRepository{sql: db}
	stats := &DashboardStats{}
	require.NoError(t, r.fillClusterPerformance(context.Background(), stats, end))
	require.Len(t, stats.RPMNodes, 4)
	require.Equal(t, "cluster_completed_usage", stats.RPMScope)
	require.Equal(t, 5, stats.RPMWindowMinutes)
	require.Equal(t, end, stats.PerformanceSampledAt)
	require.Equal(t, 1.2, stats.ClusterRPM)
	require.EqualValues(t, 1, stats.Rpm)
	var rpm float64
	for _, n := range stats.RPMNodes {
		rpm += n.RPM
	}
	require.InDelta(t, stats.ClusterRPM, rpm, 0.000001)
	require.Zero(t, stats.RPMNodes[1].RPM)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClusterPerformanceQueryFailureIsNotZeroTraffic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`WITH recent AS`).WillReturnError(context.DeadlineExceeded)
	err = (&usageLogRepository{sql: db}).fillClusterPerformance(context.Background(), &DashboardStats{}, time.Now())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, mock.ExpectationsWereMet())
}
