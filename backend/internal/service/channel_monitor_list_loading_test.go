package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type monitorListLoadingRepo struct {
	ChannelMonitorRepository
	monitors     []*ChannelMonitor
	latestCalls  atomic.Int32
	mu           sync.Mutex
	metricsCalls  map[int64]int
	metricsWindow map[int64]time.Duration
	firstCalls    map[int64]int
	active       int
	maxActive    int
	entered      chan struct{}
	release      chan struct{}
}

func (r *monitorListLoadingRepo) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	return r.monitors, nil
}
func (r *monitorListLoadingRepo) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*ChannelMonitorLatest, error) {
	r.latestCalls.Add(1)
	return map[int64][]*ChannelMonitorLatest{1: {{Model: "probe", Status: MonitorStatusError}}}, nil
}
func (r *monitorListLoadingRepo) ComputeAvailabilityForMonitors(context.Context, []int64, int) (map[int64][]*ChannelMonitorAvailability, error) {
	return map[int64][]*ChannelMonitorAvailability{1: {{Model: "probe", TotalChecks: 2, AvailabilityPct: 50}}}, nil
}
func (r *monitorListLoadingRepo) ListRecentHistoryForMonitors(context.Context, []int64, map[int64]string, int) (map[int64][]*ChannelMonitorHistoryEntry, error) {
	return map[int64][]*ChannelMonitorHistoryEntry{1: {{Model: "probe", Status: MonitorStatusError}}}, nil
}
func (r *monitorListLoadingRepo) GetGroupMetrics(ctx context.Context, id int64, window time.Duration) (*ChannelMonitorGroupMetrics, error) {
	r.mu.Lock()
	r.metricsCalls[id]++
	r.metricsWindow[id] = window
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.active--; r.mu.Unlock() }()
	if r.entered != nil {
		r.entered <- struct{}{}
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if id == 9 {
		return nil, errors.New("metrics unavailable")
	}
	return &ChannelMonitorGroupMetrics{RateMultiplier: float64(id), SubscriptionType: "standard", ObservedAt: time.Now()}, nil
}
func (r *monitorListLoadingRepo) ListRecentGroupFirstTokens(_ context.Context, id int64, _ int) ([]*ChannelMonitorGroupFirstToken, error) {
	r.mu.Lock()
	r.firstCalls[id]++
	r.mu.Unlock()
	return []*ChannelMonitorGroupFirstToken{{Model: "actual", CreatedAt: time.Now()}}, nil
}
func newMonitorListLoadingRepo() *monitorListLoadingRepo {
	return &monitorListLoadingRepo{metricsCalls: map[int64]int{}, metricsWindow: map[int64]time.Duration{}, firstCalls: map[int64]int{}}
}
func TestMonitorListLoadsLatestOnceAndKeepsFailedFacts(t *testing.T) {
	r := newMonitorListLoadingRepo()
	r.monitors = []*ChannelMonitor{{ID: 1, PrimaryModel: "probe", Name: "failed"}}
	s := NewChannelMonitorService(r, nil)
	got, err := s.ListUserView(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, r.latestCalls.Load())
	require.Len(t, got, 1)
	require.Equal(t, MonitorStatusError, got[0].PrimaryStatus)
	require.Equal(t, MonitorStatusError, got[0].Timeline[0].Status)
	require.Equal(t, 2, got[0].AvailabilitySamples7d)
}
func TestMonitorGroupInsightsBoundedConcurrentUniqueAndFresh(t *testing.T) {
	r := newMonitorListLoadingRepo()
	r.entered = make(chan struct{}, 12)
	r.release = make(chan struct{})
	s := NewChannelMonitorService(r, nil)
	monitors := []*ChannelMonitor{}
	for id := int64(1); id <= 9; id++ {
		monitors = append(monitors, &ChannelMonitor{GroupID: &id})
	}
	monitors = append(monitors, monitors[0], &ChannelMonitor{})
	done := make(chan map[int64]*UserGroupInsight, 1)
	go func() { done <- s.groupInsightsFor(context.Background(), monitors, DefaultCacheRateWindow) }()
	t.Cleanup(func() {
		select {
		case <-r.release:
		default:
			close(r.release)
		}
	})
	for i := 0; i < 4; i++ {
		select {
		case <-r.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("independent groups were serialized")
		}
	}
	select {
	case <-r.entered:
		t.Fatal("more than four concurrent group reads")
	default:
	}
	close(r.release)
	var got map[int64]*UserGroupInsight
	select {
	case got = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("group reads did not finish")
	}
	require.Len(t, got, 9)
	require.Nil(t, got[9], "failure remains unknown")
	require.Equal(t, 4, r.maxActive)
	for id := int64(1); id <= 9; id++ {
		require.Equal(t, 1, r.metricsCalls[id])
	}
	s.groupInsightsFor(context.Background(), monitors, DefaultCacheRateWindow)
	for id := int64(1); id <= 8; id++ {
		require.Equal(t, 1, r.metricsCalls[id], "cached metrics are reused")
		require.Equal(t, 2, r.firstCalls[id], "recent first-token facts remain fresh")
	}
}
func TestMonitorGroupInsightsCancelledDuringReadDoesNotCacheFailure(t *testing.T) {
	r := newMonitorListLoadingRepo()
	r.entered = make(chan struct{}, 2)
	r.release = make(chan struct{})
	s := NewChannelMonitorService(r, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *UserGroupInsight, 1)
	go func() { done <- s.cachedGroupInsight(ctx, 1, DefaultCacheRateWindow) }()
	select {
	case <-r.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("group read did not start")
	}
	cancel()
	select {
	case got := <-done:
		require.Nil(t, got)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled read did not stop")
	}
	_, cached := s.groupInsightCache.Load(groupInsightCacheKey{groupID: 1, windowSeconds: int64(DefaultCacheRateWindow.Seconds())})
	require.False(t, cached, "cancelled work cannot publish a negative cache entry")
	close(r.release)
	got := s.cachedGroupInsight(context.Background(), 1, DefaultCacheRateWindow)
	require.NotNil(t, got)
	require.Equal(t, 2, r.metricsCalls[1])
	require.Len(t, got.RecentFirstTokens, 1)
}

func TestMonitorGroupInsightCacheIsIsolatedByWindow(t *testing.T) {
	r := newMonitorListLoadingRepo()
	s := NewChannelMonitorService(r, nil)
	short := s.cachedGroupInsight(context.Background(), 7, 30*time.Minute)
	require.NotNil(t, short)
	require.Equal(t, 30*time.Minute, r.metricsWindow[7])
	shortAgain := s.cachedGroupInsight(context.Background(), 7, 30*time.Minute)
	require.NotNil(t, shortAgain)
	require.Equal(t, 1, r.metricsCalls[7], "same-window requests reuse the short cache")

	long := s.cachedGroupInsight(context.Background(), 7, 24*time.Hour)
	require.NotNil(t, long)
	require.Equal(t, 24*time.Hour, r.metricsWindow[7])
	require.Equal(t, 2, r.metricsCalls[7], "another window must run its own aggregation")

	short = s.cachedGroupInsight(context.Background(), 7, 30*time.Minute)
	require.NotNil(t, short)
	require.Equal(t, 2, r.metricsCalls[7], "returning to a window reuses only that window's cache")
}

func TestMonitorGroupInsightsCancelledBeforeStartDoesNotQuery(t *testing.T) {
	r := newMonitorListLoadingRepo()
	s := NewChannelMonitorService(r, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	id := int64(1)
	got := s.groupInsightsFor(ctx, []*ChannelMonitor{{GroupID: &id}}, DefaultCacheRateWindow)
	require.Nil(t, got[id])
	require.Empty(t, r.metricsCalls)
}
