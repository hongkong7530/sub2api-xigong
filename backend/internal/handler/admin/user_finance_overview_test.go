package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type financeOverviewReaderStub struct { err error }
func (s financeOverviewReaderStub) GetOverview(context.Context) (*service.UserFinanceOverview,error) {
	if s.err != nil { return nil,s.err }
	return &service.UserFinanceOverview{HistoricalRecharged:150,CurrentRecharged:125,HistoricalSpent:82.25,TodaySpent:1.75},nil
}

func TestUserFinanceOverviewHandlerReturnsActualTotals(t *testing.T) {
	h:=&UserHandler{financeOverview:financeOverviewReaderStub{}}
	r:=gin.New();r.GET("/finance-overview",h.GetFinanceOverview)
	w:=httptest.NewRecorder();r.ServeHTTP(w,httptest.NewRequest("GET","/finance-overview",nil))
	require.Equal(t,200,w.Code)
	var payload struct { Data service.UserFinanceOverview `json:"data"` }
	require.NoError(t,json.Unmarshal(w.Body.Bytes(),&payload))
	require.Equal(t,150.0,payload.Data.HistoricalRecharged)
	require.Equal(t,82.25,payload.Data.HistoricalSpent)
}

func TestUserFinanceOverviewHandlerDoesNotReturnZeroWhenUnavailable(t *testing.T) {
	for _,reader:=range []service.UserFinanceOverviewReader{nil,financeOverviewReaderStub{err:errors.New("database unavailable")}} {
		h:=&UserHandler{financeOverview:reader}
		r:=gin.New();r.GET("/finance-overview",h.GetFinanceOverview)
		w:=httptest.NewRecorder();r.ServeHTTP(w,httptest.NewRequest("GET","/finance-overview",nil))
		require.GreaterOrEqual(t,w.Code,500)
		require.NotContains(t,w.Body.String(),"historical_recharged")
	}
}
