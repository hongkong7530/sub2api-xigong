package ratemetric

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestArrivalQualityDoesNotInventGenerationSpeed(t *testing.T) {
	base := time.Unix(100, 0)
	for _, tc := range []struct {
		name    string
		window  int
		offsets []int
		sizes   []int
		want    string
	}{
		{"short", 204, []int{0, 100, 200}, []int{10, 10, 10}, "short_window"},
		{"single batch", 2000, []int{0, 1, 2}, []int{10, 10, 10}, "single_batch"},
		{"concentrated", 2000, []int{0, 500, 1500}, []int{1, 1, 100}, "concentrated"},
		{"distributed", 2000, []int{0, 500, 1500}, []int{10, 10, 10}, Eligible},
		{"cross bucket burst", 1200, []int{0, 49, 51, 300}, []int{1, 499, 499, 1}, "concentrated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c Collector
			for i, n := range tc.offsets {
				c.Add(base.Add(time.Duration(n)*time.Millisecond), tc.sizes[i])
			}
			o := c.Snapshot(&tc.window, true)
			require.Equal(t, tc.want, o.Quality)
			require.Equal(t, tc.want == Eligible, IsEligible(o))
			require.Equal(t, tc.window, *o.WindowMs)
			require.Equal(t, "incomplete", c.Snapshot(&tc.window, false).Quality)
		})
	}
}
func TestArrivalConsumerClockCannotEarnQualification(t *testing.T) {
	var c Collector
	base := time.Unix(100, 0)
	for _, ms := range []int{0, 500, 1500} {
		c.Add(base.Add(time.Duration(ms)*time.Millisecond), 10)
	}
	c.MarkTimingUncertain()
	window := 2000
	o := c.Snapshot(&window, true)
	require.Equal(t, "timing_uncertain", o.Quality)
	require.False(t, IsEligible(o))
	require.Equal(t, 3, o.OutputEvents)
}
func TestIngressClockAndWebSocketScope(t *testing.T) {
	base := time.Now()
	ctx := WithClock(context.Background(), base, false)
	o := ForRequest(ctx, nil, base.Add(4*time.Second))
	require.Equal(t, 4000, *o.RequestDurationMs)
	require.Equal(t, "app_request", o.TimingScope)
	require.Nil(t, ForRequest(WithClock(context.Background(), base, true), nil, base.Add(time.Hour)))
	turn := WithDuration(nil, base.Add(time.Second), base.Add(3*time.Second), "ws_turn")
	require.Equal(t, 2000, *turn.RequestDurationMs)
}
func TestPayloadStatisticsNeverPersistText(t *testing.T) {
	var c Collector
	base := time.Now()
	c.Observe([]byte(`{"type":"response.created"}`), "", base)
	c.Observe([]byte(`{"type":"response.output_text.delta","delta":"secret-content"}`), "", base.Add(time.Second))
	ms := 2000
	o := c.Snapshot(&ms, true)
	raw, err := json.Marshal(o)
	require.NoError(t, err)
	require.Equal(t, 1, o.OutputEvents)
	require.NotContains(t, string(raw), "secret-content")
}
