//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamingMonitorResponsesDoesNotSwitchProtocolOnFailure(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/v1/responses", r.URL.Path)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"message":"Responses unavailable"}}`))
			}))
			defer server.Close()
			result, supported := runStreamingCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "test-key", "gpt-test", &CheckOptions{
				HTTPClient: server.Client(), APIMode: MonitorAPIModeResponses,
			})
			require.True(t, supported)
			require.Equal(t, MonitorStatusError, result.Status)
			require.Contains(t, result.Message, "Responses unavailable")
			require.Equal(t, 1, calls)
		})
	}
}

func TestStreamingMonitorZhipuRelayUsesGatewayChatPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
	}))
	defer server.Close()
	result, supported := runStreamingCheckForModel(context.Background(), MonitorProviderZhipu, server.URL, "test-key", "glm-test", &CheckOptions{HTTPClient: server.Client()})
	require.True(t, supported)
	require.Equal(t, MonitorStatusOperational, result.Status, result.Message)
	require.NotNil(t, result.FirstTokenMs)
}
