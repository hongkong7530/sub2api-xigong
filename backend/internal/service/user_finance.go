package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// UserFinanceOverview totals are independent of user-list search and include retained usage.
type UserFinanceOverview struct {
	HistoricalRecharged float64 `json:"historical_recharged"`
	CurrentRecharged float64 `json:"current_recharged"`
	TodaySpent float64 `json:"today_spent"`
	HistoricalSpent float64 `json:"historical_spent"`
	SpendTrend []usagestats.TrendDataPoint `json:"spend_trend"`
	ObservedAt time.Time `json:"observed_at"`
}

type UserRechargeAggregator interface {
	SumTotalRecharged(context.Context, bool) (float64, error)
}

type UserFinanceOverviewReader interface {
	GetOverview(context.Context) (*UserFinanceOverview, error)
}

type UserFinanceService struct {
	recharge UserRechargeAggregator
	usage UsageLogRepository
}

func NewUserFinanceService(users UserRepository, usage UsageLogRepository) *UserFinanceService {
	recharge, _ := users.(UserRechargeAggregator)
	return &UserFinanceService{recharge: recharge, usage: usage}
}

func (s *UserFinanceService) GetOverview(ctx context.Context) (*UserFinanceOverview, error) {
	if s == nil || s.recharge == nil || s.usage == nil { return nil, fmt.Errorf("user finance overview is not configured") }
	historical, err := s.recharge.SumTotalRecharged(ctx, true)
	if err != nil { return nil, fmt.Errorf("historical recharge totals: %w", err) }
	current, err := s.recharge.SumTotalRecharged(ctx, false)
	if err != nil { return nil, fmt.Errorf("current recharge totals: %w", err) }
	stats, err := s.usage.GetDashboardStats(ctx)
	if err != nil { return nil, fmt.Errorf("finance spending totals: %w", err) }
	if stats == nil { return nil, fmt.Errorf("finance spending totals unavailable") }
	now := timezone.Now()
	start := timezone.StartOfDay(now.AddDate(0,0,-29))
	end := timezone.StartOfDay(now).AddDate(0,0,1)
	raw, err := s.usage.GetUsageTrendWithFilters(ctx,start,end,"day",0,0,0,0,"",nil,nil,nil)
	if err != nil { return nil, fmt.Errorf("finance spending trend: %w", err) }
	byDay := make(map[string]usagestats.TrendDataPoint,len(raw))
	for _,point := range raw { byDay[point.Date] = point }
	trend := make([]usagestats.TrendDataPoint,0,30)
	for day := start;day.Before(end);day=day.AddDate(0,0,1) {
		key := day.Format("2006-01-02")
		point := byDay[key];point.Date=key;trend=append(trend,point)
	}
	return &UserFinanceOverview{HistoricalRecharged:historical,CurrentRecharged:current,TodaySpent:stats.TodayActualCost,HistoricalSpent:stats.TotalActualCost,SpendTrend:trend,ObservedAt:now},nil
}
