package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ratemetric"
	"github.com/stretchr/testify/require"
)

func TestRateObservationIncludesPreForwardWaitWithoutChangingLegacyTiming(t *testing.T) {
	start := time.Now().Add(-3 * time.Second)
	ctx := ratemetric.WithClock(context.Background(), start, false)
	duration, first := 1000, 500
	result := &OpenAIForwardResult{Duration: time.Second, FirstTokenMs: &first, OutputDurationMs: &duration}
	finishOpenAIRateObservation(ctx, result)
	require.NotNil(t, result.RateObservation)
	require.Equal(t, "app_request", result.RateObservation.TimingScope)
	require.GreaterOrEqual(t, *result.RateObservation.RequestDurationMs, 3000)
	require.Equal(t, time.Second, result.Duration)
	require.Equal(t, 500, *result.FirstTokenMs)
	require.Equal(t, 1000, *result.OutputDurationMs)
}
func TestRateObservationDoesNotUseWebSocketConnectionLifetime(t *testing.T) {
	ctx := ratemetric.WithClock(context.Background(), time.Now().Add(-time.Hour), true)
	r := &OpenAIForwardResult{}
	finishOpenAIRateObservation(ctx, r)
	require.Nil(t, r.RateObservation)
	finishWSTurnRateObservation(r, time.Now().Add(-2*time.Second))
	require.Equal(t, "ws_turn", r.RateObservation.TimingScope)
	require.Less(t, *r.RateObservation.RequestDurationMs, 3000)
}
func TestRateObservationFailedForwardKeepsUsageButDropsQualification(t *testing.T) {
	window := 2000
	o := &RateObservation{Version: 1, WindowMs: &window, Quality: "observed", OutputEvents: 3, EventSpanMs: 1500, OutputBytes: 30, MaxBurstBytes: 10}
	r := &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 1000}, RateObservation: o}
	finishOpenAIRateObservation(context.Background(), r, context.Canceled)
	require.Equal(t, 1000, r.Usage.OutputTokens)
	require.Equal(t, "incomplete", r.RateObservation.Quality)
	require.Equal(t, "observed", o.Quality)
}
func TestStreamObservationShortAndBufferedWindowStayRaw(t *testing.T) {
	base := time.Unix(100, 0)
	var w streamOutputWindow
	w.observe([]byte(`{"type":"response.created"}`), "", base)
	w.observe([]byte(`{"type":"response.output_text.delta","delta":"answer"}`), "", base.Add(200*time.Millisecond))
	w.observe([]byte(`{"type":"response.completed"}`), "", base.Add(204*time.Millisecond))
	o := w.rateObservation()
	require.Equal(t, "short_window", o.Quality)
	require.Equal(t, 204, *o.WindowMs)
	require.Equal(t, 204, *w.durationMs())
	require.False(t, ratemetric.IsEligible(o))
	require.Nil(t, ratemetric.ForRequest(context.Background(), nil, time.Now()))
}
