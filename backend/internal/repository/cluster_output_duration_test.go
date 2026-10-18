//go:build unit

package repository

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestClusterSettlementPreservesStreamOutputDuration(t *testing.T) {
	outputMs, firstMs, totalMs := 5000, 8000, 10000
	cmd := &service.UsageBillingCommand{UsageLog: &service.UsageLog{
		Model: "gpt-6.1-sol", RequestID: "execution:output-window", OutputTokens: 1200,
		OutputDurationMs: &outputMs, FirstTokenMs: &firstMs, DurationMs: &totalMs,
		TotalCost: 0.01, ActualCost: 0.002,
	}}
	evidence, err := settlementEvidence(cmd)
	require.NoError(t, err)
	raw, err := json.Marshal(evidence)
	require.NoError(t, err)
	var replay service.UsageBillingCommand
	require.NoError(t, json.Unmarshal(raw, &replay))
	require.Equal(t, &outputMs, replay.UsageLog.OutputDurationMs)
	require.Equal(t, &firstMs, replay.UsageLog.FirstTokenMs)
	require.Equal(t, &totalMs, replay.UsageLog.DurationMs)
	require.Equal(t, 1200, replay.UsageLog.OutputTokens)
	require.Equal(t, cmd.UsageLog.TotalCost, replay.UsageLog.TotalCost)
	require.Equal(t, cmd.UsageLog.ActualCost, replay.UsageLog.ActualCost)
	cmd.UsageLog.OutputDurationMs = nil
	legacy, err := settlementEvidence(cmd)
	require.NoError(t, err)
	require.Nil(t, legacy.UsageLog.OutputDurationMs)
}
