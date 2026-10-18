package service

import (
	"bytes"
	"context"
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

func TestUltrafastGroupForwardCapturesFinalTierAndRejectsBeforeUpstream(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, test := range []struct {
			model, tier string
			blocked     bool
		}{
			{"gpt-6-astra", "", false}, {"gpt-6.1-sol", "default", false},
			{"gpt-6-sol", "", true}, {"gpt-5.6-sol", "priority", true},
		} {
			t.Run(test.model+test.tier+map[bool]string{false: "/native", true: "/passthrough"}[passthrough], func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
				c.Request.Header.Set("User-Agent", "curl/8.0")
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_mock","model":"` + test.model + `","service_tier":"ultrafast","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))}}
				svc := newOpenAIGatewayServiceWithSettings(t, ultrafastGroupPolicy())
				svc.cfg = &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}}
				svc.httpUpstream = upstream
				account := &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "mock-key", "base_url": "https://api.openai.com"}, Extra: map[string]any{"openai_passthrough": passthrough}}
				body := []byte(`{"model":"` + test.model + `","stream":false,"input":"hi"}`)
				if test.tier != "" {
					body = []byte(`{"model":"` + test.model + `","stream":false,"service_tier":"` + test.tier + `","input":"hi"}`)
				}
				ctx := ultrafastGroupContext(23)
				result, err := svc.Forward(ctx, c, account, body)
				if test.blocked {
					require.ErrorAs(t, err, new(*OpenAIFastBlockedError))
					require.Nil(t, upstream.lastReq)
					require.Equal(t, 403, rec.Code)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, upstream.lastReq)
				require.Equal(t, "ultrafast", gjson.GetBytes(upstream.lastBody, "service_tier").String())
				require.Equal(t, "ultrafast", optionalStringValue(result.ServiceTier))
				require.Equal(t, "ultrafast", result.UpstreamResponseServiceTier)
				require.False(t, ApplyOpenAIServiceTierBillingResolution(account, result).Downgraded)
			})
		}
	}
}

func TestUltrafastGroupScopedRuleCannotAffectOtherAccountsOrGroups(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, ultrafastGroupPolicy())
	for _, account := range []*Account{{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, {Platform: PlatformAnthropic, Type: AccountTypeAPIKey}} {
		action, _ := svc.evaluateOpenAIFastPolicy(ultrafastGroupContext(23), account, "gpt-6.1-sol", "priority")
		require.Equal(t, BetaPolicyActionPass, action)
	}
	action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-6.1-sol", "priority")
	require.Equal(t, BetaPolicyActionPass, action)
}
