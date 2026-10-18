//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorOutputRateUsesGenerationTimeAndUnknownBoundaries(t *testing.T) {
	duration, first := 3000, 1000
	require.Equal(t, 50.0, *MonitorOutputRate(100, &duration, &first))
	tooShort, boundary := 1099, 1100
	require.Nil(t, MonitorOutputRate(100, &tooShort, &first))
	require.Equal(t, 1000.0, *MonitorOutputRate(100, &boundary, &first))
	for _, args := range []struct {
		tokens          int64
		duration, first *int
	}{
		{0, &duration, &first}, {-1, &duration, &first}, {100, nil, &first}, {100, &duration, nil}, {100, &first, &duration}, {100, &first, &first},
	} {
		require.Nil(t, MonitorOutputRate(args.tokens, args.duration, args.first))
	}
}

func TestMonitorOutputUsageIsCumulativeAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		payload string
		tokens  int64
		known   bool
	}{
		{`{"usage":{"completion_tokens":12,"completion_tokens_details":{"reasoning_tokens":5}}}`, 12, true},
		{`{"usage":{"prompt_tokens":32,"completion_tokens":9,"completion_tokens_details":{"reasoning_tokens":94},"total_tokens":135}}`, 103, true},
		{`{"usage":{"prompt_tokens":32,"completion_tokens":103,"completion_tokens_details":{"reasoning_tokens":94},"total_tokens":135}}`, 103, true},
		{`{"response":{"usage":{"output_tokens":24}}}`, 24, true},
		{`{"type":"message_delta","usage":{"output_tokens":30}}`, 30, true},
		{`{"usageMetadata":{"candidatesTokenCount":10,"thoughtsTokenCount":4}}`, 14, true},
		{`{"usage":{"completion_tokens":-1}}`, 0, false},
		{`{"usage":{"completion_tokens":1.5}}`, 0, false},
		{`{"choices":[{"delta":{"content":"long text is not a token counter"}}]}`, 0, false},
	} {
		tokens, known := monitorOutputTokens([]byte(tc.payload))
		require.Equal(t, tc.tokens, tokens)
		require.Equal(t, tc.known, known)
	}
}

func TestMonitorOutputStreamProtocolTerminalAndUsage(t *testing.T) {
	for _, tc := range []struct {
		name, stream string
		adapter      streamAdapter
		tokens       int64
	}{
		{"chat", "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"completion_tokens\":12}}\n\ndata: [DONE]\n\n", newOpenAICompatibleChatStreamAdapter(providerOpenAIPath), 12},
		{"responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"output_tokens\":24}}}\n\n", openAIResponsesStreamAdapter, 24},
		{"anthropic", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"output_tokens\":1}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":12}}\n\ndata: {\"type\":\"message_stop\"}\n\n", anthropicStreamAdapter, 12},
		{"gemini", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"candidatesTokenCount\":10,\"thoughtsTokenCount\":4}}\n\n", geminiStreamAdapter, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := readMonitorOutputStream(strings.NewReader(tc.stream), tc.adapter, time.Now())
			require.NoError(t, err)
			require.True(t, m.complete)
			require.True(t, m.content)
			require.NotNil(t, m.first)
			require.Equal(t, tc.tokens, m.outputTokens)
		})
	}
}

func TestMonitorOutputStreamLargeMetadataAndLimits(t *testing.T) {
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
		"data: {\"type\":\"response.metadata\",\"metadata\":\"" + strings.Repeat("x", 70*1024) + "\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"output_tokens\":12}}}\n\n"
	m, err := readMonitorOutputStream(strings.NewReader(stream), openAIResponsesStreamAdapter, time.Now())
	require.NoError(t, err)
	require.True(t, m.complete)
	require.True(t, m.content)
	require.Equal(t, int64(12), m.outputTokens)
	_, err = readMonitorOutputStream(strings.NewReader("data: {\"padding\":\""+strings.Repeat("x", monitorStreamMaxBytes)+"\"}\n\n"), openAIResponsesStreamAdapter, time.Now())
	require.ErrorContains(t, err, "exceeds response limit")
	_, err = readMonitorOutputStream(strings.NewReader("data: {invalid}\n\n"), openAIResponsesStreamAdapter, time.Now())
	require.ErrorContains(t, err, "invalid stream JSON")
}

func TestMonitorOutputProbeReadsAfterFirstFrameAndDoesNotPing(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"include_usage":true`)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(120 * time.Millisecond)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"completion_tokens\":20}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	got, _ := runStreamingCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "test-key", "test-model", &CheckOptions{APIMode: MonitorAPIModeChatCompletions, HTTPClient: server.Client()})
	require.Equal(t, 1, calls)
	require.Equal(t, MonitorStatusOperational, got.Status, got.Message)
	require.NotNil(t, got.OutputTokensPerSecond)
	require.Greater(t, *got.OutputTokensPerSecond, 0.0)
	require.Greater(t, *got.LatencyMs, *got.FirstTokenMs)
	require.Nil(t, got.PingLatencyMs)
}

func TestMonitorOutputStreamMissingUsageAndTruncationNeverInventCounts(t *testing.T) {
	adapter := newOpenAICompatibleChatStreamAdapter(providerOpenAIPath)
	for _, stream := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n",
	} {
		m, err := readMonitorOutputStream(strings.NewReader(stream), adapter, time.Now())
		require.NoError(t, err)
		require.True(t, m.content)
		require.Zero(t, m.outputTokens)
	}
	m, err := readMonitorOutputStream(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"usage\":{\"completion_tokens\":12},\"choices\":[]}\n\n"), adapter, time.Now())
	require.NoError(t, err)
	require.False(t, m.complete, "usage alone cannot prove terminal success")
}

func TestMonitorOutputProbeFailureAfterTextNeverReportsSpeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"upstream failed\"}}}\n\n")
	}))
	defer server.Close()
	got, _ := runStreamingCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "test-key", "test-model", &CheckOptions{APIMode: MonitorAPIModeResponses, HTTPClient: server.Client()})
	require.Equal(t, MonitorStatusError, got.Status)
	require.Nil(t, got.OutputTokensPerSecond)
	require.Contains(t, got.Message, "upstream failed")
}
