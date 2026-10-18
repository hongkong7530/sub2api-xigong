package repository

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSettlementEvidencePreservesRoutingWithoutClientMetadata(t *testing.T) {
	inbound, upstream := "/v1/chat/completions", "/v1/responses"
	ua, ip := "private-client-agent", "192.0.2.1"
	cmd := &service.UsageBillingCommand{UsageLog: &service.UsageLog{
		Model: "gpt-test", InboundEndpoint: &inbound, UpstreamEndpoint: &upstream,
		InputTokens: 100, OutputTokens: 10, CacheReadTokens: 50,
		TotalCost: 0.01, ActualCost: 0.005, UserAgent: &ua, IPAddress: &ip,
	}}
	evidence, err := settlementEvidence(cmd)
	require.NoError(t, err)
	require.Equal(t, cmd.UsageLog.InboundEndpoint, evidence.UsageLog.InboundEndpoint)
	require.Equal(t, cmd.UsageLog.UpstreamEndpoint, evidence.UsageLog.UpstreamEndpoint)
	require.Equal(t, cmd.UsageLog.InputTokens, evidence.UsageLog.InputTokens)
	require.Equal(t, cmd.UsageLog.CacheReadTokens, evidence.UsageLog.CacheReadTokens)
	require.Equal(t, cmd.UsageLog.ActualCost, evidence.UsageLog.ActualCost)
	require.Nil(t, evidence.UsageLog.UserAgent)
	require.Nil(t, evidence.UsageLog.IPAddress)

	data, err := json.Marshal(evidence)
	require.NoError(t, err)
	var recovered service.UsageBillingCommand
	require.NoError(t, json.Unmarshal(data, &recovered))
	require.Equal(t, &inbound, recovered.UsageLog.InboundEndpoint)
	require.Equal(t, &upstream, recovered.UsageLog.UpstreamEndpoint)
	require.Equal(t, evidence.UsageLog.ActualCost, recovered.UsageLog.ActualCost)
}
