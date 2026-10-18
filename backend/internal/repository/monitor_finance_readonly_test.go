//go:build unit

package repository_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Explicit opt-in reads a deployed database with its existing application identity.
// No schema initialization, user impersonation, writes, or paid probes are performed.
func TestMonitorFinanceReadOnlyDatabaseContracts(t *testing.T) {
	path:=os.Getenv("SUB2API_READONLY_CONTRACT_CONFIG")
	if path=="" { t.Skip("read-only database contract not requested") }
	t.Setenv("CONFIG_FILE",path)
	cfg,err:=config.Load();require.NoError(t,err)
	db,err:=sql.Open("postgres",cfg.Database.DSNWithTimezone(cfg.Timezone));require.NoError(t,err)
	defer db.Close()
	db.SetMaxOpenConns(3)
	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
	require.NoError(t,db.PingContext(ctx))
	client:=dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres,db)))
	defer client.Close()
	users:=repository.NewUserRepository(client,db)
	usage:=repository.NewUsageLogRepository(client,db)
	finance:=service.NewUserFinanceService(users,usage)
	// Natural traffic may commit between reads. Check the actual value against
	// database totals on both sides instead of weakening precision or freezing production.
	var spendBefore float64
	require.NoError(t,db.QueryRowContext(ctx,"SELECT COALESCE(SUM(actual_cost),0) FROM usage_statistics").Scan(&spendBefore))
	overview,err:=finance.GetOverview(ctx);require.NoError(t,err)
	var recharged,spent float64
	require.NoError(t,db.QueryRowContext(ctx,"SELECT COALESCE(SUM(total_recharged),0) FROM users").Scan(&recharged))
	require.NoError(t,db.QueryRowContext(ctx,"SELECT COALESCE(SUM(actual_cost),0) FROM usage_statistics").Scan(&spent))
	require.InDelta(t,recharged,overview.HistoricalRecharged,1e-7)
	require.GreaterOrEqual(t, overview.HistoricalSpent, spendBefore)
	require.LessOrEqual(t, overview.HistoricalSpent, spent)
	require.Len(t,overview.SpendTrend,30)
	h:=admin.NewUserHandler(nil,nil,nil,nil,nil,nil,nil);h.SetFinanceOverview(finance)
	r:=gin.New();r.GET("/finance",h.GetFinanceOverview)
	response:=httptest.NewRecorder();r.ServeHTTP(response,httptest.NewRequest("GET","/finance",nil).WithContext(ctx))
	require.Equal(t,200,response.Code)
	var payload struct { Data service.UserFinanceOverview `json:"data"` }
	require.NoError(t,json.Unmarshal(response.Body.Bytes(),&payload))
	require.InDelta(t,recharged,payload.Data.HistoricalRecharged,1e-7)

	monitors:=repository.NewChannelMonitorRepository(client,db)
	ids:=[]int64{}
	rows,err:=db.QueryContext(ctx,"SELECT id FROM channel_monitors WHERE enabled ORDER BY id");require.NoError(t,err)
	for rows.Next() { var id int64;require.NoError(t,rows.Scan(&id));ids=append(ids,id) }
	require.NoError(t,rows.Err());rows.Close()
	health,err:=monitors.(service.ChannelMonitorHealthRepository).ReadMonitorHealth(ctx,ids,7,60)
	require.NoError(t,err)
	realPoints:=0
	measuredRates:=0
	for _,id:=range ids {
		h:=health[id];require.NotNil(t,h)
		require.LessOrEqual(t,len(h.Recent),60)
		require.LessOrEqual(t,h.Availability.OperationalChecks,h.Availability.TotalChecks)
		for i,e:=range h.Recent {
			require.Contains(t,[]string{"user_request","probe"},e.Source)
			if e.Source=="user_request" { realPoints++ }
			if e.OutputTokensPerSecond != nil { require.Greater(t,*e.OutputTokensPerSecond,0.0); measuredRates++ }
			if i>0 { require.False(t,e.CheckedAt.After(h.Recent[i-1].CheckedAt)) }
		}
	}
	require.Greater(t,realPoints,0,"real requests must contribute to recent monitor history")
	// Exercise the actual user-list and detail serializers, without impersonating a logged-in user.
	monitorService := service.NewChannelMonitorService(monitors,nil)
	monitorHandler := handler.NewChannelMonitorUserHandler(monitorService,nil)
	viewsRouter := gin.New()
	viewsRouter.GET("/monitors",monitorHandler.List)
	viewsRouter.GET("/monitors/:id/status",monitorHandler.GetStatus)
	listResponse := httptest.NewRecorder()
	viewsRouter.ServeHTTP(listResponse,httptest.NewRequest("GET","/monitors",nil).WithContext(ctx))
	require.Equal(t,200,listResponse.Code,listResponse.Body.String())
	var listPayload struct { Data struct { Items []struct {
		ID int64 `json:"id"`
		GroupID *int64 `json:"group_id"`
		Availability *float64 `json:"availability_7d"`
		Samples int `json:"availability_samples_7d"`
		OutputRate *float64 `json:"primary_output_tokens_per_second"`
		Timeline []struct { Source string `json:"source"`; Model string `json:"model"`; OutputRate *float64 `json:"output_tokens_per_second"` } `json:"timeline"`
		Insight *struct { Rate *float64 `json:"cache_hit_rate"`; Samples int64 `json:"cache_sample_count"`; Window int64 `json:"cache_window_seconds"` } `json:"group_insight"`
	} `json:"items"` } `json:"data"` }
	require.NoError(t,json.Unmarshal(listResponse.Body.Bytes(),&listPayload))
	require.Len(t,listPayload.Data.Items,len(ids))
	var detailID int64
	for _,view := range listPayload.Data.Items {
		require.LessOrEqual(t,len(view.Timeline),60)
		if view.Samples > 0 { require.NotNil(t,view.Availability);require.GreaterOrEqual(t,*view.Availability,0.0);require.LessOrEqual(t,*view.Availability,100.0) } else { require.Nil(t,view.Availability) }
		if view.OutputRate != nil { require.Greater(t,*view.OutputRate,0.0) }
		if view.GroupID != nil {
			require.NotNil(t,view.Insight)
			require.Equal(t,int64(86400),view.Insight.Window)
			if view.Insight.Samples > 0 { require.NotNil(t,view.Insight.Rate) } else { require.Nil(t,view.Insight.Rate) }
			for _,point := range view.Timeline { require.Contains(t,[]string{"probe","user_request"},point.Source);require.NotEmpty(t,point.Model); if point.OutputRate != nil { require.Greater(t,*point.OutputRate,0.0) } }
			detailID = view.ID
		}
	}
	require.Positive(t,detailID)
	detailResponse := httptest.NewRecorder()
	viewsRouter.ServeHTTP(detailResponse,httptest.NewRequest("GET",fmt.Sprintf("/monitors/%d/status",detailID),nil).WithContext(ctx))
	require.Equal(t,200,detailResponse.Code,detailResponse.Body.String())
	var detailPayload struct { Data struct { Availability map[string]*float64 `json:"group_availability"` } `json:"data"` }
	require.NoError(t,json.Unmarshal(detailResponse.Body.Bytes(),&detailPayload))
	for _,window := range []string{"7","15","30"} { require.Contains(t,detailPayload.Data.Availability,window) }
	require.Greater(t,measuredRates,0,"valid natural stream samples must expose measured output speed")
	t.Logf("read-only list/detail contracts passed: monitors=%d real_points=%d output_rates=%d recharge=%.8f spend=%.8f",len(ids),realPoints,measuredRates,recharged,spent)
}
