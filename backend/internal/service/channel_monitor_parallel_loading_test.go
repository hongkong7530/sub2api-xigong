package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type parallelMonitorListRepo struct {
	*monitorListLoadingRepo
	entered chan string
	release chan struct{}
	healthErr error
}

func (r *parallelMonitorListRepo) wait(ctx context.Context, name string) {
	r.entered <- name
	select {
	case <-r.release:
	case <-ctx.Done():
	}
}
func (r *parallelMonitorListRepo) ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*ChannelMonitorLatest, error) {
	r.wait(ctx, "latest")
	return r.monitorListLoadingRepo.ListLatestForMonitorIDs(ctx, ids)
}
func (r *parallelMonitorListRepo) ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, days int) (map[int64][]*ChannelMonitorAvailability, error) {
	r.wait(ctx, "availability")
	return r.monitorListLoadingRepo.ComputeAvailabilityForMonitors(ctx, ids, days)
}
func (r *parallelMonitorListRepo) ListRecentHistoryForMonitors(ctx context.Context, ids []int64, primary map[int64]string, limit int) (map[int64][]*ChannelMonitorHistoryEntry, error) {
	r.wait(ctx, "timeline")
	return r.monitorListLoadingRepo.ListRecentHistoryForMonitors(ctx, ids, primary, limit)
}
func (r *parallelMonitorListRepo) GetGroupMetrics(ctx context.Context, id int64, window time.Duration) (*ChannelMonitorGroupMetrics, error) {
	r.wait(ctx, "insight")
	return r.monitorListLoadingRepo.GetGroupMetrics(ctx, id, window)
}
func (r *parallelMonitorListRepo) ReadMonitorHealth(ctx context.Context, _ []int64, _, _ int) (map[int64]*ChannelMonitorHealth, error) {
	r.wait(ctx, "health")
	return map[int64]*ChannelMonitorHealth{1: {
		Availability: &ChannelMonitorAvailability{TotalChecks: 3, AvailabilityPct: 100.0 / 3},
		Recent: []*ChannelMonitorHistoryEntry{{Model: "actual", Status: MonitorStatusError, Source: "user_request"}},
	}}, r.healthErr
}

func TestMonitorListReadsIndependentBranchesInParallel(t *testing.T) {
	for _, failHealth := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserves facts", true: "preserves health failure"}[failHealth], func(t *testing.T) {
			group := int64(1)
			base := newMonitorListLoadingRepo()
			base.monitors = []*ChannelMonitor{{ID: 1, GroupID: &group, KeyProvisionMode: "auto", PrimaryModel: "probe"}}
			repo := &parallelMonitorListRepo{monitorListLoadingRepo: base, entered: make(chan string, 5), release: make(chan struct{})}
			if failHealth {
				repo.healthErr = errors.New("health unavailable")
			}
			done := make(chan struct{})
			var got []*UserMonitorView
			var err error
			go func() {
				got, err = NewChannelMonitorService(repo, nil).ListUserView(context.Background())
				close(done)
			}()
			t.Cleanup(func() {
				select {
				case <-repo.release:
				default:
					close(repo.release)
				}
			})
			seen := map[string]bool{}
			for i := 0; i < 5; i++ {
				select {
				case name := <-repo.entered:
					seen[name] = true
				case <-time.After(3 * time.Second):
					t.Fatal("independent monitor reads were serialized")
				}
			}
			require.Len(t, seen, 5)
			close(repo.release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("monitor reads did not finish")
			}
			if failHealth {
				require.ErrorIs(t, err, repo.healthErr)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, "actual", got[0].PrimaryModel)
			require.Equal(t, MonitorStatusError, got[0].PrimaryStatus)
			require.Equal(t, "user_request", got[0].StatusSource)
			require.Equal(t, 3, got[0].AvailabilitySamples7d)
			require.Len(t, got[0].Timeline, 1)
			require.NotNil(t, got[0].GroupInsight)
			require.EqualValues(t, 1, base.latestCalls.Load())
		})
	}
}
