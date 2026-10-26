// Package ratemetric records arrival observations, never provider-internal token generation times.
package ratemetric

import (
	"context"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const Version = 1
const Eligible = "observed"

// Observation contains only bounded counts/times, not prompts, output text or credentials.
// RequestDurationMs ends at forwarding completion, before asynchronous settlement.
// app_request starts at this application's HTTP ingress; ws_turn starts at frame receipt.
type Observation struct {
	Version           int    `json:"version"`
	RequestDurationMs *int   `json:"request_duration_ms,omitempty"`
	TimingScope       string `json:"timing_scope,omitempty"`
	WindowMs          *int   `json:"window_ms,omitempty"`
	OutputEvents      int    `json:"output_events"`
	EventSpanMs       int    `json:"event_span_ms"`
	OutputBytes       int64  `json:"output_bytes"`
	MaxBurstBytes     int64  `json:"max_burst_bytes"`
	Quality           string `json:"quality"`
}

type clockKey struct{}
type Clock struct {
	Start     time.Time
	WebSocket bool
}

func WithClock(ctx context.Context, start time.Time, websocket bool) context.Context {
	return context.WithValue(ctx, clockKey{}, Clock{start, websocket})
}
func RequestClock(ctx context.Context) (Clock, bool) {
	if ctx == nil {
		return Clock{}, false
	}
	v, ok := ctx.Value(clockKey{}).(Clock)
	return v, ok
}

// Collector keeps bounded arrival statistics. Adjacent 50ms buckets are combined
// conservatively so a burst crossing a bucket edge cannot evade classification.
type Collector struct {
	first, last, burstStart               time.Time
	count                                 int
	bytes, burst, previousBurst, maxBurst int64
	timingUncertain                       bool
}

func (c *Collector) Add(now time.Time, size int) {
	if size <= 0 {
		return
	}
	if c.first.IsZero() {
		c.first = now
	}
	if c.burstStart.IsZero() {
		c.burstStart = now
	}
	if now.Sub(c.burstStart) >= 50*time.Millisecond {
		buckets := now.Sub(c.burstStart) / (50 * time.Millisecond)
		c.previousBurst = 0
		if buckets == 1 {
			c.previousBurst = c.burst
		}
		c.burstStart = c.burstStart.Add(time.Duration(buckets) * 50 * time.Millisecond)
		c.burst = 0
	}
	c.last = now
	c.count++
	c.bytes += int64(size)
	c.burst += int64(size)
	if combined := c.previousBurst + c.burst; combined > c.maxBurst {
		c.maxBurst = combined
	}
}

// MarkTimingUncertain excludes consumer-timed/queued observations from rewards.
func (c *Collector) MarkTimingUncertain() { c.timingUncertain = true }

func (c *Collector) Observe(payload []byte, eventType string, now time.Time) {
	if !gjson.ValidBytes(payload) {
		return
	}
	v := gjson.ParseBytes(payload)
	kind := strings.TrimSpace(eventType)
	if kind == "" {
		kind = v.Get("type").String()
	}
	size := 0
	if strings.HasPrefix(kind, "response.") && strings.HasSuffix(kind, ".delta") {
		size += len(v.Get("delta").String())
	}
	if kind == "content_block_delta" {
		for _, p := range []string{"delta.text", "delta.thinking", "delta.partial_json"} {
			size += len(v.Get(p).String())
		}
	}
	for depth := 0; depth < 2 && !v.Get("candidates").Exists() && v.Get("response").IsObject(); depth++ {
		v = v.Get("response")
	}
	for _, choice := range v.Get("choices").Array() {
		d := choice.Get("delta")
		for _, p := range []string{"content", "reasoning_content", "reasoning"} {
			size += len(d.Get(p).String())
		}
		for _, tool := range d.Get("tool_calls").Array() {
			size += len(tool.Get("function.arguments").String())
		}
	}
	for _, candidate := range v.Get("candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			size += len(part.Get("text").String())
			if args := part.Get("functionCall.args"); args.Exists() {
				size += len(args.Raw)
			}
		}
	}
	c.Add(now, size)
}

// Snapshot never redefines the original output window. Conservative rules only
// determine suitability for highlights/percentiles: >=1s, >=3 output events,
// >=250ms event span and no adjacent 50ms buckets carrying >=80% of bytes.
func (c *Collector) Snapshot(windowMs *int, complete bool) *Observation {
	o := &Observation{Version: Version, WindowMs: windowMs, OutputEvents: c.count, OutputBytes: c.bytes, MaxBurstBytes: c.maxBurst, Quality: "incomplete"}
	if !c.first.IsZero() && !c.last.Before(c.first) {
		o.EventSpanMs = int(c.last.Sub(c.first).Milliseconds())
	}
	if !complete {
		return o
	}
	switch {
	case windowMs == nil:
		o.Quality = "unknown"
	case *windowMs < 1000:
		o.Quality = "short_window"
	case c.count < 3 || o.EventSpanMs < 250:
		o.Quality = "single_batch"
	case c.bytes > 0 && c.maxBurst*5 >= c.bytes*4:
		o.Quality = "concentrated"
	case c.timingUncertain:
		o.Quality = "timing_uncertain"
	default:
		o.Quality = Eligible
	}
	return o
}
func WithDuration(o *Observation, start, end time.Time, scope string) *Observation {
	if o == nil {
		o = &Observation{Version: Version, Quality: "unknown"}
	}
	copy := *o
	if !start.IsZero() && !end.Before(start) {
		ms := int(end.Sub(start).Milliseconds())
		copy.RequestDurationMs = &ms
		copy.TimingScope = scope
	}
	return &copy
}
func ForRequest(ctx context.Context, o *Observation, now time.Time) *Observation {
	clock, ok := RequestClock(ctx)
	if !ok || clock.WebSocket {
		return o
	}
	return WithDuration(o, clock.Start, now, "app_request")
}
func IsEligible(o *Observation) bool {
	return o != nil && o.Version == Version && o.Quality == Eligible && o.WindowMs != nil && *o.WindowMs >= 1000 && o.OutputEvents >= 3 && o.EventSpanMs >= 250 && o.OutputBytes > 0 && o.MaxBurstBytes >= 0 && o.MaxBurstBytes*5 < o.OutputBytes*4
}
