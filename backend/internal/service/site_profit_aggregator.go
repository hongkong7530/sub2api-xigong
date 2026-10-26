package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// Profit reporting is asynchronous; a reporting backlog never starts work in a gateway request.
type SiteProfitAggregator struct {
	repo   SiteProfitRepository
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func NewSiteProfitAggregator(repo SiteProfitRepository, cfg *config.Config) *SiteProfitAggregator {
	a := &SiteProfitAggregator{repo: repo, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	if cfg != nil && cfg.RunsRole(config.RoleMaintenance) {
		go a.run(ctx)
	} else {
		close(a.done)
	}
	return a
}
func (a *SiteProfitAggregator) run(ctx context.Context) {
	defer close(a.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		a.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (a *SiteProfitAggregator) tick(ctx context.Context) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for batch := 0; batch < 4; batch++ {
		n, e := a.repo.Aggregate(c, 500)
		if e != nil {
			if ctx.Err() == nil {
				logger.LegacyPrintf("site.profit", "aggregation unavailable: %v", e)
			}
			return
		}
		if n < 500 {
			return
		}
	}
}
func (a *SiteProfitAggregator) Stop() {
	if a == nil {
		return
	}
	a.once.Do(a.cancel)
	<-a.done
}
