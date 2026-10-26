package openai_ws_v2

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ratemetric"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// A terminal may carry billable usage even when its downstream write fails.
func TestRelayObservationPublishedAfterTerminalDelivery(t *testing.T) {
	for _, mode := range []string{"delivered", "write_failure", "drain"} {
		t.Run(mode, func(t *testing.T) {
			base := time.Unix(100, 0)
			tick := base
			frames := []passthroughTestFrame{}
			for _, payload := range []string{
				`{"type":"response.created","response":{"id":"resp"}}`,
				`{"type":"response.output_text.delta","response_id":"resp","delta":"a"}`,
				`{"type":"response.output_text.delta","response_id":"resp","delta":"b"}`,
				`{"type":"response.output_text.delta","response_id":"resp","delta":"c"}`,
				`{"type":"response.completed","response":{"id":"resp","status":"completed","usage":{"input_tokens":2,"output_tokens":1000}}}`,
			} {
				frames = append(frames, passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(payload)})
			}
			upstream := newPassthroughTestFrameConn(frames, true)
			state := &relayState{}
			state.setPendingTurnStartedAt(base.Add(-time.Second))
			drop := &atomic.Bool{}
			drop.Store(mode == "drain")
			var delivered bool
			var turns []RelayTurnResult
			exit := make(chan relayExitSignal, 1)
			runUpstreamToClient(context.Background(), upstream, func(_ coderws.MessageType, p []byte) error {
				if strings.Contains(string(p), `"type":"response.completed"`) {
					tick = tick.Add(250 * time.Millisecond)
					if mode == "write_failure" {
						return errors.New("downstream disconnected")
					}
					delivered = true
				}
				return nil
			}, base, func() time.Time { tick = tick.Add(500 * time.Millisecond); return tick }, state, nil, func(turn RelayTurnResult) {
				require.Equal(t, mode == "delivered", delivered, "callback must follow terminal delivery or its failure")
				turns = append(turns, turn)
			}, nil, nil, nil, nil, drop, &atomic.Int64{}, &atomic.Int64{}, func() {}, nil, exit)
			require.Len(t, turns, 1)
			require.Equal(t, 1000, turns[0].Usage.OutputTokens)
			require.NotNil(t, turns[0].RateObservation)
			if mode == "delivered" {
				require.Equal(t, "timing_uncertain", turns[0].RateObservation.Quality)
			} else {
				require.Equal(t, "incomplete", turns[0].RateObservation.Quality)
			}
			require.False(t, ratemetric.IsEligible(turns[0].RateObservation))
			require.Greater(t, *turns[0].RateObservation.RequestDurationMs, int(turns[0].Duration.Milliseconds()))
			var final RelayResult
			enrichResult(&final, state, time.Second)
			require.Equal(t, turns[0].RateObservation, final.RateObservation)
			require.Equal(t, turns[0].OutputDurationMs, final.OutputDurationMs)
		})
	}
}
