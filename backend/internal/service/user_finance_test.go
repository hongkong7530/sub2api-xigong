//go:build unit

package service

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
 "github.com/stretchr/testify/require"
)

type financeUsersStub struct { UserRepository; err error }
func (s *financeUsersStub) SumTotalRecharged(_ context.Context, deleted bool) (float64,error) {
 if s.err != nil { return 0,s.err };if deleted {return 150,nil};return 125,nil
}
type financeUsageStub struct { UsageLogRepository; err error; start,end time.Time }
func (s *financeUsageStub) GetDashboardStats(context.Context) (*usagestats.DashboardStats,error) {
 return &usagestats.DashboardStats{TotalActualCost:82.25,TodayActualCost:1.75},s.err
}
func (s *financeUsageStub) GetUsageTrendWithFilters(_ context.Context,start,end time.Time,_ string,_ int64,_ int64,_ int64,_ int64,_ string,_ *int16,_ *bool,_ *int8) ([]usagestats.TrendDataPoint,error) {
 s.start,s.end=start,end;return []usagestats.TrendDataPoint{{Date:start.Format("2006-01-02"),ActualCost:2.5}},s.err
}
func TestUserFinanceOverviewUsesIndependentRechargeAndActualLifetimeSpending(t *testing.T) {
 usage:=&financeUsageStub{}
 svc:=NewUserFinanceService(&financeUsersStub{},usage)
 got,err:=svc.GetOverview(context.Background())
 require.NoError(t,err)
 require.Equal(t,150.0,got.HistoricalRecharged)
 require.Equal(t,125.0,got.CurrentRecharged)
 require.Equal(t,82.25,got.HistoricalSpent)
 require.Equal(t,1.75,got.TodaySpent)
 require.Len(t,got.SpendTrend,30)
 require.Equal(t,2.5,got.SpendTrend[0].ActualCost)
 require.Zero(t,got.SpendTrend[29].ActualCost)
 require.True(t,usage.end.After(usage.start))
}
func TestUserFinanceOverviewFailsInsteadOfFabricatingZero(t *testing.T) {
 for _,svc:=range []*UserFinanceService{NewUserFinanceService(&financeUsersStub{err:errors.New("db unavailable")},&financeUsageStub{}),NewUserFinanceService(&financeUsersStub{},&financeUsageStub{err:errors.New("db unavailable")})} {
  got,err:=svc.GetOverview(context.Background());require.Error(t,err);require.Nil(t,got)
 }
}
