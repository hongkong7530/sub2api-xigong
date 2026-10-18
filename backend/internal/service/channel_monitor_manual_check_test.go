//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type manualCheckRepoStub struct {
	quotaModeRepoStub
	observation *ChannelMonitorObservation
	saveErr error
}
func (r *manualCheckRepoStub) ListLatestGroupObservations(context.Context, []int64, time.Time) (map[int64]*ChannelMonitorObservation, error) {
	return map[int64]*ChannelMonitorObservation{9:r.observation},nil
}
func (r *manualCheckRepoStub) InsertHistoryBatch(ctx context.Context, rows []*ChannelMonitorHistoryRow) error {
	if r.saveErr != nil { return r.saveErr }
	return r.quotaModeRepoStub.InsertHistoryBatch(ctx, rows)
}

func TestManualMonitorCheckMakesNewHTTPRequestDespiteRecentRealTraffic(t *testing.T) {
	var calls atomic.Int64
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.Method==http.MethodHead { return }
		calls.Add(1)
		w.Header().Set("Content-Type","application/json")
		_,_=w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer server.Close()
	groupID:=int64(9)
	repo:=&manualCheckRepoStub{quotaModeRepoStub:quotaModeRepoStub{monitor:&ChannelMonitor{ID:1,GroupID:&groupID,Enabled:true,KeyProvisionMode:"auto",Provider:MonitorProviderOpenAI,APIMode:MonitorAPIModeResponses,PrimaryModel:"new-model",APIKey:"OLD:key",BodyOverrideMode:MonitorBodyOverrideModeReplace,BodyOverride:map[string]any{"model":"new-model","input":"hi","instructions":"reply ok"}}},observation:&ChannelMonitorObservation{Model:"old-model",ObservedAt:time.Now()}}
	svc:=NewChannelMonitorService(repo,&duplicateChannelMonitorEncryptor{})
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt:ChannelMonitorRuntime{Enabled:true,Mode:ChannelMonitorModeV1}})
	svc.SetLocalGatewayBase(server.URL)
	started:=time.Now()
	result,err:=svc.RunCheck(context.Background(),1)
	require.NoError(t,err)
	require.Len(t,result,1)
	require.Equal(t,MonitorStatusOperational,result[0].Status,result[0].Message)
	require.Equal(t,int64(1),calls.Load())
	require.Equal(t,"new-model",result[0].Model)
	require.False(t,result[0].CheckedAt.Before(started))
	require.Len(t,repo.history,1)
	require.Equal(t,[]int64{1},repo.markedIDs)
}
func TestMonitorPersistenceFailureIsNotReportedAsSaved(t *testing.T) {
	repo:=&manualCheckRepoStub{saveErr:errors.New("history unavailable")}
	svc:=NewChannelMonitorService(repo,nil)
	err:=svc.persistCheckResults(context.Background(),&ChannelMonitor{ID:1},[]*CheckResult{{Model:"m",Status:MonitorStatusOperational,CheckedAt:time.Now()}})
	require.ErrorContains(t,err,"history unavailable")
	require.Empty(t,repo.markedIDs)
}
