//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func officialUltrafastPolicy() *OpenAIFastPolicySettings {
	policy := ultrafastGroupPolicy()
	policy.Rules = append(policy.Rules, OpenAIFastPolicyRule{
		ServiceTier: OpenAIFastTierUltrafast, Action: BetaPolicyActionPass,
		Scope: BetaPolicyScopeAPIKey, GroupIDs: []int64{15},
		ModelWhitelist: []string{"gpt-6-astra", "gpt-6.1-sol"},
	})
	return policy
}

func officialUltrafastContext() context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 15, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, RateMultiplier: 0.3,
	})
}

func TestOfficialUltrafastOptionalFinalOutbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 组15仅新增显式Ultrafast pass；独立组23的强制规则保持。
	for _, path := range []string{"responses", "passthrough", "chat-responses", "chat-raw", "messages-responses", "messages-chat"} {
		for _, tier := range []string{"", "default", "auto", "priority", "flex", "ultrafast", " ULTRAFAST "} {
			t.Run(path+"/"+tier, func(t *testing.T) {
				svc := newOpenAIGatewayServiceWithSettings(t, officialUltrafastPolicy())
				svc.cfg = &config.Config{}
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"test stops after final outbound capture"}}`)),
				}}
				svc.httpUpstream = upstream
				account := &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
					Credentials: map[string]any{"api_key": "mock-key", "base_url": "https://api.openai.com"},
					Extra:       map[string]any{"openai_responses_supported": true}}
				payload := map[string]any{"model": "gpt-6.1-sol", "stream": false, "input": "hello"}
				if strings.HasPrefix(path, "chat") || strings.HasPrefix(path, "messages") {
					delete(payload, "input")
					payload["messages"] = []map[string]any{{"role": "user", "content": "hello"}}
				}
				if strings.HasPrefix(path, "messages") {
					payload["max_tokens"] = 16
				}
				if tier != "" {
					payload["service_tier"] = tier
				}
				body, err := json.Marshal(payload)
				require.NoError(t, err)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+path, bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				if path == "passthrough" {
					account.Extra["openai_passthrough"] = true
				}
				if path == "chat-raw" || path == "messages-chat" {
					account.Extra["openai_responses_supported"] = false
				}
				switch {
				case path == "messages-chat":
					_, err = svc.forwardAnthropicViaRawChatCompletions(officialUltrafastContext(), c, account, body, "")
				case path == "messages-responses":
					_, err = svc.ForwardAsAnthropic(officialUltrafastContext(), c, account, body, "", "")
				case strings.HasPrefix(path, "chat"):
					_, err = svc.ForwardAsChatCompletions(officialUltrafastContext(), c, account, body, "", "gpt-6.1-sol")
				default:
					_, err = svc.Forward(officialUltrafastContext(), c, account, body)
				}
				require.Error(t, err, "mock rejects only after the outbound body is built")
				require.NotNil(t, upstream.lastReq, "must reach the final upstream boundary")
				want := strings.ToLower(strings.TrimSpace(tier))
				if strings.HasPrefix(path, "messages") && want != "ultrafast" {
					want = "" // 非Ultrafast扩展继续沿用Messages原行为。
				}
				require.Equal(t, want, gjson.GetBytes(upstream.lastBody, "service_tier").String())
				if tier == "" {
					require.False(t, gjson.GetBytes(upstream.lastBody, "service_tier").Exists(), "missing must not become ultrafast")
				}
			})
		}
	}
}

func TestOfficialUltrafastExplicitTierWinsCompatFastHeader(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, officialUltrafastPolicy())
	svc.cfg = &config.Config{}
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"capture"}}`))}}
	svc.httpUpstream = upstream
	account := &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "mock-key", "base_url": "https://api.openai.com"}, Extra: map[string]any{"openai_responses_supported": true}}
	body := []byte(`{"model":"gpt-6.1-sol","max_tokens":16,"service_tier":"ultrafast","messages":[{"role":"user","content":"hello"}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("anthropic-beta", claude.BetaFastMode)
	_, err := svc.ForwardAsAnthropic(officialUltrafastContext(), c, account, body, "", "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "ultrafast", gjson.GetBytes(upstream.lastBody, "service_tier").String())
}

func TestOfficialUltrafastOptionalWSAndGroupIsolation(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, officialUltrafastPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, tier := range []string{"", "default", "priority", "flex", "ultrafast"} {
		field := ""
		if tier != "" {
			field = fmt.Sprintf(",\"service_tier\":%q", tier)
		}
		frame := []byte(`{"type":"response.create","model":"gpt-6.1-sol","input":[]` + field + `}`)
		got, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(officialUltrafastContext(), account, "gpt-6.1-sol", frame)
		require.NoError(t, err)
		require.Nil(t, blocked)
		require.Equal(t, tier, gjson.GetBytes(got, "service_tier").String())
		forced, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ultrafastGroupContext(23), account, "gpt-6.1-sol", frame)
		require.NoError(t, err)
		require.Nil(t, blocked)
		require.Equal(t, "ultrafast", gjson.GetBytes(forced, "service_tier").String(), "独立组23仍沿用强制规则")
	}
}

func TestOfficialUltrafastSixTimesAndGroupRateOnce(t *testing.T) {
	billing := newTestBillingService()
	resolver := NewModelPricingResolver(nil, billing)
	group := contextThresholdGroup(15, 272000)
	group.RateMultiplier = 0.3
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol"} {
		for _, inputTotal := range []int{271999, 272000, 272001} {
			tokens := UsageTokens{InputTokens: inputTotal - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
			base, err := billing.CalculateCostUnified(CostInput{Model: model, Group: group, GroupID: &group.ID, Tokens: tokens, Resolver: resolver, RateMultiplier: 1, ServiceTier: "default"})
			require.NoError(t, err)
			ultra, err := billing.CalculateCostUnified(CostInput{Model: model, Group: group, GroupID: &group.ID, Tokens: tokens, Resolver: resolver, RateMultiplier: 0.3, ServiceTier: "ultrafast"})
			require.NoError(t, err)
			require.InDelta(t, base.InputCost*6, ultra.InputCost, 1e-10)
			require.InDelta(t, base.OutputCost*6, ultra.OutputCost, 1e-10)
			require.InDelta(t, base.CacheReadCost*6, ultra.CacheReadCost, 1e-10)
			require.InDelta(t, base.CacheCreationCost*6, ultra.CacheCreationCost, 1e-10)
			require.InDelta(t, base.TotalCost*6*0.3, ultra.ActualCost, 1e-10)
		}
	}
}

func TestOfficialUltrafastRecordUsageChargesResolvedTier(t *testing.T) {
	for _, observed := range []string{"ultrafast", "", "default"} {
		t.Run("observed/"+observed, func(t *testing.T) {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			users := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			group := contextThresholdGroup(15, 272000)
			group.RateMultiplier = 0.3
			tier := "ultrafast"
			usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 500, CacheReadInputTokens: 200, CacheCreationInputTokens: 100}
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{RequestID: "mock-official-" + observed, Model: "gpt-6.1-sol", ServiceTier: &tier, UpstreamResponseServiceTier: observed, Usage: usage, Duration: time.Second},
				APIKey: &APIKey{ID: 1, GroupID: &group.ID, Group: group}, User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			})
			require.NoError(t, err)
			wantTier, multiplier := "ultrafast", 6.0
			if observed == "default" {
				wantTier, multiplier = "default", 1
			}
			require.NotNil(t, logs.lastLog)
			require.Equal(t, wantTier, *logs.lastLog.ServiceTier)
			base, err := svc.billingService.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", Group: group, GroupID: &group.ID,
				Tokens: UsageTokens{InputTokens: 700, OutputTokens: 500, CacheReadTokens: 200, CacheCreationTokens: 100}, Resolver: svc.resolver, RateMultiplier: 1, ServiceTier: "default"})
			require.NoError(t, err)
			require.InDelta(t, base.TotalCost*multiplier*0.3, logs.lastLog.ActualCost, 1e-10)
			require.Equal(t, 1, users.deductCalls)
			require.InDelta(t, logs.lastLog.ActualCost, users.lastAmount, 1e-10)
		})
	}
}
