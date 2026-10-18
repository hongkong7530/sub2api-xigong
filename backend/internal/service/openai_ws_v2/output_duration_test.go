package openai_ws_v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRelayOutputDurationKeepsTTFTAndTurnsIndependent(t *testing.T) {
	base := time.Unix(100, 0)
	state := &relayState{}
	observe := func(payload string, offset time.Duration) observedUpstreamEvent {
		return observeUpstreamMessage(state, []byte(payload), base, func() time.Time { return base.Add(offset) }, nil)
	}
	observe(`{"type":"response.created","response":{"id":"resp_1"}}`, time.Second)
	observe(`{"type":"response.output_text.delta","response_id":"resp_1","delta":"answer"}`, 4*time.Second)
	success := observe(`{"type":"response.completed","response":{"id":"resp_1","status":"completed","error":null,"usage":{"input_tokens":5,"output_tokens":300,"output_tokens_details":{"reasoning_tokens":250}}}}`, 6*time.Second)
	require.NotNil(t, success.outputDuration)
	require.Equal(t, 5000, *success.outputDuration)
	require.Equal(t, 300, success.usage.OutputTokens)
	require.NotNil(t, success.firstToken)
	require.Greater(t, *success.firstToken, 0)
	require.Equal(t, 5000, *state.outputDurationMs)
	observe(`{"type":"response.created","response":{"id":"resp_2"}}`, 10*time.Second)
	failed := observe(`{"type":"response.failed","response":{"id":"resp_2","status":"failed"}}`, 12*time.Second)
	require.Nil(t, failed.outputDuration)
	require.Nil(t, state.outputDurationMs)
	observe(`{"type":"response.created","response":{"id":"resp_3"}}`, 20*time.Second)
	next := observe(`{"type":"response.completed","response":{"id":"resp_3","status":"completed"}}`, 23*time.Second)
	require.Equal(t, 3000, *next.outputDuration)
}

func TestRelayOutputDurationRejectsNestedFailureAndTerminalOnly(t *testing.T) {
	base := time.Unix(100, 0)
	for _, status := range []string{"incomplete", "failed"} {
		state := &relayState{}
		observeUpstreamMessage(state, []byte(`{"type":"response.created","response":{"id":"resp"}}`), base, func() time.Time { return base }, nil)
		result := observeUpstreamMessage(state, []byte(`{"type":"response.completed","response":{"id":"resp","status":"`+status+`"}}`), base, func() time.Time { return base.Add(time.Second) }, nil)
		require.Nil(t, result.outputDuration)
	}
	state := &relayState{}
	result := observeUpstreamMessage(state, []byte(`{"type":"response.completed","response":{"id":"alone"}}`), base, func() time.Time { return base.Add(time.Second) }, nil)
	require.Nil(t, result.outputDuration)
}
