//go:build unit

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type parallelDashboardRepo struct {
	service.UsageLogRepository
	started chan string
	release chan struct{}
}

func (r *parallelDashboardRepo) GetUsageTrendWithUsageFilters(ctx context.Context, _, _ time.Time, _ string, _ usagestats.UsageLogFilters) ([]usagestats.TrendDataPoint, error) {
	r.started <- "trend"
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return []usagestats.TrendDataPoint{{Date: "parallel"}}, nil
	}
}
func (r *parallelDashboardRepo) GetModelStatsWithUsageFiltersBySource(ctx context.Context, _, _ time.Time, _ usagestats.UsageLogFilters, _ string) ([]usagestats.ModelStat, error) {
	r.started <- "models"
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return []usagestats.ModelStat{}, nil
	}
}

func TestDashboardSnapshotLoadsIndependentSectionsConcurrently(t *testing.T) {
	repo := &parallelDashboardRepo{started: make(chan string, 2), release: make(chan struct{})}
	svc := service.NewDashboardService(repo, nil, nil, nil)
	h := NewDashboardHandler(svc, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	done := make(chan error, 1)
	go func() {
		_, err := h.buildSnapshotV2Response(ctx, start, start.Add(time.Hour), "hour", &dashboardSnapshotV2Filters{}, false, true, true, false, false, 12)
		done <- err
	}()
	for range 2 {
		select {
		case <-repo.started:
		case <-ctx.Done():
			t.Fatal("independent queries were serialized")
		}
	}
	close(repo.release)
	require.NoError(t, <-done)
}
