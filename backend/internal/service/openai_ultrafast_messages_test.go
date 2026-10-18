package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUltrafastGroupMessagesChatBridgeForcesFinalBodyAndBlocksOtherModels(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))
			c.Set(upstreamResponseModelObserverContextKey, &upstreamResponseModelObserver{})
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl_mock","model":"` + model + `","service_tier":"ultrafast","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))}}
			svc := newOpenAIGatewayServiceWithSettings(t, ultrafastGroupPolicy())
			svc.cfg = &config.Config{}
			svc.httpUpstream = upstream
			account := &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Credentials: map[string]any{"api_key": "mock-key", "base_url": "https://api.openai.com"}}
			body := []byte(`{"model":"` + model + `","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
			result, err := svc.forwardAnthropicViaRawChatCompletions(ultrafastGroupContext(23), c, account, body, "")
			if model == "gpt-6-sol" {
				require.ErrorAs(t, err, new(*OpenAIFastBlockedError))
				require.Nil(t, upstream.lastReq)
				require.Equal(t, http.StatusForbidden, rec.Code)
				require.Equal(t, "error", gjson.Get(rec.Body.String(), "type").String())
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "ultrafast", gjson.GetBytes(upstream.lastBody, "service_tier").String())
			require.Equal(t, "ultrafast", optionalStringValue(result.ServiceTier))
			require.Equal(t, "ultrafast", result.UpstreamResponseServiceTier)
		})
	}
}

func TestUltrafastObserverAcceptsTerminalResponseButNotCreatedEcho(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.ObserveOpenAI([]byte(`{"type":"response.created","response":{"model":"gpt-6.1-sol","service_tier":"ultrafast"}}`), "response.created")
	require.Empty(t, observer.ServiceTier())
	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-6.1-sol","service_tier":"ultrafast"}}`), "response.completed")
	require.Equal(t, "ultrafast", observer.ServiceTier())
}
