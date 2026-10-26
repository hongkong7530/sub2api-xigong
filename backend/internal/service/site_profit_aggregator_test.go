package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type profitStopRepo struct {
	profitRepoStub
	once    sync.Once
	started chan struct{}
}

func (r *profitStopRepo) Aggregate(ctx context.Context, _ int) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return 0, ctx.Err()
}
func TestSiteProfitAggregatorCancelsInflightAndStopsIdempotently(t *testing.T) {
	r := &profitStopRepo{started: make(chan struct{})}
	a := NewSiteProfitAggregator(r, &config.Config{})
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("aggregator never started")
	}
	done := make(chan struct{})
	go func() { a.Stop(); a.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("aggregation did not cancel")
	}
}
func TestSiteProfitAggregatorWorkerRoleHasNoBackgroundJob(t *testing.T) {
	r := &profitStopRepo{started: make(chan struct{})}
	a := NewSiteProfitAggregator(r, &config.Config{Cluster: config.ClusterConfig{Enabled: true, Roles: []string{config.RoleWorker}}})
	a.Stop()
	select {
	case <-r.started:
		t.Fatal("worker role started reporting task")
	default:
	}
}
