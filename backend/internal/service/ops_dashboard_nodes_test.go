package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type perNodeMetricsRepo struct{ opsRepoMock }

func (r *perNodeMetricsRepo) ListLatestSystemMetrics(context.Context, int) ([]*OpsSystemMetricsSnapshot, error) {
	return []*OpsSystemMetricsSnapshot{{NodeID: "xigong3"}, {NodeID: "xigong"}, {NodeID: "xigong2"}}, nil
}
func TestOpsDashboardPreservesAllMachineSamples(t *testing.T) {
	t.Setenv("OPS_NODE_ID", "xigong3")
	s := &OpsService{opsRepo: &perNodeMetricsRepo{}, cfg: &config.Config{}}
	s.cfg.Ops.Enabled = true
	now := time.Now()
	view, err := s.GetDashboardOverview(context.Background(), &OpsDashboardFilter{StartTime: now.Add(-time.Hour), EndTime: now})
	require.NoError(t, err)
	require.Len(t, view.SystemMetricsNodes, 3)
	require.Equal(t, "xigong", view.SystemMetricsNodes[0].NodeID)
	require.Equal(t, "xigong3", view.ResponseNodeID)
	require.Equal(t, "xigong3", view.SystemMetrics.NodeID)
}
func TestOpsCollectorLocksArePerNode(t *testing.T) {
	a, b := opsMetricsCollectorLockKey("xigong3"), opsMetricsCollectorLockKey("xigong2")
	require.NotEqual(t, a, b)
	require.NotEqual(t, hashAdvisoryLockID(a), hashAdvisoryLockID(b))
	require.Equal(t, a, opsMetricsCollectorLockKey("xigong3"))
}
func TestRemoteMetricsNeverUseResponseNodesPoolLimits(t *testing.T) {
	t.Setenv("OPS_NODE_ID", "xigong3")
	s := &OpsService{cfg: &config.Config{}}
	s.cfg.Database.MaxOpenConns = 96
	remote := &OpsSystemMetricsSnapshot{NodeID: "xigong2"}
	attachOpsMetricsLimits(s, remote)
	require.Nil(t, remote.DBMaxOpenConns)
}
