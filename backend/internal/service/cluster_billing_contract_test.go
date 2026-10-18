package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type clusterQuotaReader struct { UserPlatformQuotaRepository; record *UserPlatformQuotaRecord; err error }
func(r clusterQuotaReader)GetByUserPlatform(context.Context,int64,string)(*UserPlatformQuotaRecord,error){return r.record,r.err}
func TestClusterPlatformQuotaNeverFailsOpen(t *testing.T){
	cfg:=&config.Config{Cluster:config.ClusterConfig{Enabled:true}}
	s:=&BillingCacheService{cfg:cfg,userPlatformQuotaRepo:clusterQuotaReader{err:errors.New("database unavailable")}}
	require.Error(t,s.checkUserPlatformQuotaEligibility(context.Background(),1,"openai"))
	zero:=0.0;s.userPlatformQuotaRepo=clusterQuotaReader{record:&UserPlatformQuotaRecord{DailyLimitUSD:&zero}}
	require.Error(t,s.checkUserPlatformQuotaEligibility(context.Background(),1,"openai"))
	s.userPlatformQuotaRepo=clusterQuotaReader{}
	require.NoError(t,s.checkUserPlatformQuotaEligibility(context.Background(),1,"openai"))
}
func TestClusterBillingIDsDoNotCollapseClientCorrelation(t *testing.T){
	ctx:=cluster.WithExecution(context.Background(),&cluster.Execution{ID:"execution-a"})
	require.Equal(t,"execution:execution-a",resolveUsageBillingRequestID(ctx,"upstream"))
	ctx=cluster.WithSettlementID(ctx,"execution:turn-b")
	require.Equal(t,"execution:turn-b",resolveUsageBillingRequestID(ctx,"upstream"))
	for _,physical:=range []string{"devin:turn-a","grok_audio:call-a","grok-video:job-a","web_search:call-a"}{require.Equal(t,physical,resolveUsageBillingRequestID(ctx,physical))}
}
func TestClusterCommandPlatformFingerprintAndQuantization(t *testing.T){
	legacy:=&UsageBillingCommand{RequestID:"a",UserID:1,APIKeyID:2,BalanceCost:0.000078125}
	legacy.Normalize();old:=legacy.RequestFingerprint
	v3:=&UsageBillingCommand{ClusterID:"test",RequestID:"a",UserID:1,APIKeyID:2,BalanceCost:0.000078125,PlatformQuota:&PlatformQuotaCharge{Platform:"openai",Amount:0.000078125}}
	v3.Normalize();require.NotEqual(t,old,v3.RequestFingerprint);require.Equal(t,v3.BalanceCost,v3.PlatformQuota.Amount)
	fingerprint:=v3.RequestFingerprint;v3.Normalize();require.Equal(t,fingerprint,v3.RequestFingerprint)
}
func TestClusterQuotaFlusherShutdownDoesNotFlush(t *testing.T){
	flusher:=&UserPlatformQuotaUsageFlusher{authoritative:true}
	require.NotPanics(t,func(){flusher.Start();flusher.tick();flusher.Stop()})
}
func TestClusterUsageWorkerRunsDurablyInline(t *testing.T){
	pool:=&UsageRecordWorkerPool{durableInline:true,taskTimeout:1000000000}
	called:=false;mode:=pool.Submit(func(context.Context){called=true})
	require.True(t,called);require.Equal(t,UsageRecordSubmitModeSync,mode)
}

// Every gateway that bills must hand the same configuration into billingDeps.
// When OpenAIGatewayService omitted it, deps.cfg was nil, ClusterEnabled() read
// false, and every OpenAI-gateway request skipped cluster settlement: no
// cluster_settlement_events row was written, the unsettled counter grew without
// bound, and admission eventually closed the whole ingress.
func TestGatewayBillingDepsCarryConfiguration(t *testing.T){
	cfg:=&config.Config{Cluster:config.ClusterConfig{Enabled:true,ID:"production",NodeID:"xigong3"}}
	openai:=&OpenAIGatewayService{cfg:cfg}
	deps:=openai.billingDeps()
	require.NotNil(t,deps.cfg,"OpenAI gateway billing deps must carry cfg")
	require.True(t,deps.cfg.ClusterEnabled(),"cluster mode must reach the billing decision")
	require.Equal(t,"production",deps.cfg.Cluster.ID)
	require.Equal(t,"xigong3",deps.cfg.Cluster.NodeID)
	generic:=&GatewayService{cfg:cfg}
	require.NotNil(t,generic.billingDeps().cfg,"generic gateway billing deps must carry cfg")
}

