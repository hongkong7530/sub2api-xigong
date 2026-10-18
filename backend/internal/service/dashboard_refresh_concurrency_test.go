package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type blockedDashboardRepo struct {
	UsageLogRepository
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (r *blockedDashboardRepo) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	r.calls.Add(1)
	select {
	case r.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return &usagestats.DashboardStats{TotalUsers: 42}, nil
	}
}
func TestDashboardColdRefreshSharesLoadWithoutSharingCancellation(t *testing.T) {
	repo := &blockedDashboardRepo{started: make(chan struct{}, 2), release: make(chan struct{})}
	svc := NewDashboardService(repo, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := svc.GetDashboardStats(ctx); first <- err }()
	<-repo.started
	second := make(chan *usagestats.DashboardStats, 1)
	go func() { value, _ := svc.GetDashboardStats(context.Background()); second <- value }()
	cancel()
	require.ErrorIs(t, <-first, context.Canceled)
	close(repo.release)
	select {
	case value := <-second:
		require.NotNil(t, value)
		require.EqualValues(t, 42, value.TotalUsers)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter interrupted shared load")
	}
}
