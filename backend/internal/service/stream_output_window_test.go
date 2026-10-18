//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStreamOutputWindowIncludesPreambleAndReasoning(t *testing.T) {
	base := time.Unix(100, 0)
	var window streamOutputWindow
	window.observe([]byte(`{"type":"ping"}`), "", base)
	window.observe([]byte(`{"type":"response.created"}`), "", base.Add(time.Second))
	window.observe([]byte(`{"type":"response.reasoning_text.delta","delta":"reasoning"}`), "", base.Add(2*time.Second))
	window.observe([]byte(`{"type":"response.output_text.delta","delta":"answer"}`), "", base.Add(4*time.Second))
	window.observe([]byte(`{"type":"response.completed","response":{"usage":{"output_tokens":1200}}}`), "", base.Add(5*time.Second))
	require.Equal(t, 4000, *window.durationMs())
	window.observe([]byte(`[DONE]`), "", base.Add(10*time.Second))
	require.Equal(t, 4000, *window.durationMs(), "usage/transport tail must not extend output")
}

func TestStreamOutputWindowUnknownAndFailureBoundaries(t *testing.T) {
	base := time.Unix(100, 0)
	for _, terminal := range []string{"response.failed", "response.incomplete", "response.cancelled"} {
		var w streamOutputWindow
		w.observe([]byte(`{"type":"response.created"}`), "", base)
		w.observe([]byte(`{"type":"`+terminal+`"}`), "", base.Add(time.Second))
		require.Nil(t, w.durationMs())
	}
	var missing, alone, short streamOutputWindow
	missing.observe([]byte(`{"type":"response.created"}`), "", base)
	alone.observe([]byte(`{"type":"response.completed"}`), "", base)
	short.observe([]byte(`{"type":"response.created"}`), "", base)
	short.observe([]byte(`{"type":"response.completed"}`), "", base.Add(99*time.Millisecond))
	require.Nil(t, missing.durationMs())
	require.Nil(t, alone.durationMs())
	require.Nil(t, short.durationMs())
}

func TestStreamOutputWindowProtocolEnds(t *testing.T) {
	base := time.Unix(100, 0)
	for _, frames := range [][]string{
		{`{"choices":[{"delta":{"role":"assistant"}}]}`, `{"choices":[{"finish_reason":"stop"}]}`},
		{`{"type":"message_start"}`, `{"type":"message_stop"}`},
		{`{"candidates":[{"content":{"parts":[{"thought":true,"text":"thinking"}]}}]}`, `{"candidates":[{"finishReason":"STOP"}]}`},
	} {
		var w streamOutputWindow
		w.observe([]byte(frames[0]), "", base)
		w.observe([]byte(frames[1]), "", base.Add(100*time.Millisecond))
		require.Equal(t, 100, *w.durationMs())
	}
}
func TestStreamOutputWindowWrappedMultiChoiceAndAuthority(t *testing.T) {
	base := time.Unix(100, 0)
	var wrapped streamOutputWindow
	wrapped.observe([]byte(`{"response":{"candidates":[{"content":{"parts":[{"thought":true,"text":"reason"}]}}]}}`), "", base)
	wrapped.observe([]byte(`{"response":{"candidates":[{"finishReason":"STOP"}]}}`), "", base.Add(time.Second))
	require.Equal(t, 1000, *wrapped.durationMs())
	var authority streamOutputWindow
	authority.observe([]byte(`{"type":"response.created"}`), "", base)
	authority.observe([]byte(`{"type":"error","error":{"message":"provisional"}}`), "", base.Add(time.Second))
	require.Nil(t, authority.durationMs())
	authority.observe([]byte(`{"type":"response.completed","response":{"status":"completed","error":null}}`), "", base.Add(2*time.Second))
	require.Equal(t, 2000, *authority.durationMs())
	var nestedFailure streamOutputWindow
	nestedFailure.observe([]byte(`{"type":"response.created"}`), "", base)
	nestedFailure.observe([]byte(`{"type":"response.completed","response":{"status":"incomplete"}}`), "", base.Add(time.Second))
	require.Nil(t, nestedFailure.durationMs())
	var choices streamOutputWindow
	choices.observe([]byte(`{"choices":[{"index":0,"delta":{"role":"assistant"}},{"index":1,"delta":{"role":"assistant"}}]}`), "", base)
	choices.observe([]byte(`{"choices":[{"index":0,"finish_reason":"stop"}]}`), "", base.Add(time.Second))
	require.Nil(t, choices.durationMs())
	choices.observe([]byte(`{"choices":[{"index":1,"finish_reason":"stop"}]}`), "", base.Add(2*time.Second))
	require.Equal(t, 2000, *choices.durationMs())
}
