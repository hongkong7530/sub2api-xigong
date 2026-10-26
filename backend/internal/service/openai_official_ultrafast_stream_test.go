//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func officialUltrafastResponseStream(observed string, complete bool) string {
	response := map[string]any{"id": "resp_official", "object": "response", "model": "gpt-6.1-sol", "status": "completed",
		"output": []map[string]any{{"id": "msg_official", "type": "message", "role": "assistant", "status": "completed", "content": []map[string]any{{"type": "output_text", "text": "ok"}}}},
		"usage":  map[string]any{"input_tokens": 1000, "output_tokens": 500}}
	if observed != "" {
		response["service_tier"] = observed
	}
	created, _ := json.Marshal(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_official", "model": "gpt-6.1-sol", "status": "in_progress"}})
	progress, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "item_id": "msg_official", "output_index": 0, "content_index": 0, "delta": "ok", "usage": response["usage"]})
	body := "data: " + string(created) + "\n\ndata: " + string(progress) + "\n\n"
	if complete {
		terminal, _ := json.Marshal(map[string]any{"type": "response.completed", "response": response})
		body += "data: " + string(terminal) + "\n\ndata: [DONE]\n\n"
	}
	return body
}

func officialUltrafastChatReply(observed string, stream bool) string {
	base := map[string]any{"id": "chatcmpl_official", "object": "chat.completion", "model": "gpt-6.1-sol",
		"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 1000, "completion_tokens": 500, "total_tokens": 1500}}
	if observed != "" {
		base["service_tier"] = observed
	}
	if !stream {
		raw, _ := json.Marshal(base)
		return string(raw)
	}
	chunk := map[string]any{"id": "chatcmpl_official", "object": "chat.completion.chunk", "model": "gpt-6.1-sol",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": nil}}}
	if observed != "" {
		chunk["service_tier"] = observed
	}
	first, _ := json.Marshal(chunk)
	chunk["choices"] = []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}
	chunk["usage"] = base["usage"]
	last, _ := json.Marshal(chunk)
	return "data: " + string(first) + "\n\ndata: " + string(last) + "\n\ndata: [DONE]\n\n"
}

func forwardOfficialUltrafastFixture(t *testing.T, path string, stream bool, observed string, complete bool) (*OpenAIForwardResult, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newOpenAIGatewayServiceWithSettings(t, officialUltrafastPolicy())
	svc.cfg = &config.Config{}
	rawChat := path == "chat-raw" || path == "messages-chat"
	responseBody, contentType := officialUltrafastResponseStream(observed, complete), "text/event-stream"
	if rawChat {
		responseBody = officialUltrafastChatReply(observed, stream)
		if !stream {
			contentType = "application/json"
		}
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200,
		Header: http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"mock-official-success"}},
		Body:   io.NopCloser(strings.NewReader(responseBody))}}
	svc.httpUpstream = upstream
	account := &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "mock-key", "base_url": "https://api.openai.com"},
		Extra:       map[string]any{"openai_responses_supported": !rawChat}}
	if path == "passthrough" {
		account.Extra["openai_passthrough"] = true
	}
	payload := map[string]any{"model": "gpt-6.1-sol", "service_tier": "ultrafast", "stream": stream, "input": "hello"}
	if strings.HasPrefix(path, "chat") || strings.HasPrefix(path, "messages") {
		delete(payload, "input")
		payload["messages"] = []map[string]any{{"role": "user", "content": "hello"}}
	}
	if strings.HasPrefix(path, "messages") {
		payload["max_tokens"] = 16
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+path, bytes.NewReader(body))
	var result *OpenAIForwardResult
	switch {
	case path == "messages-chat":
		beginUpstreamResponseModelObservation(c)
		result, err = svc.forwardAnthropicViaRawChatCompletions(officialUltrafastContext(), c, account, body, "")
	case path == "messages-responses":
		result, err = svc.ForwardAsAnthropic(officialUltrafastContext(), c, account, body, "", "")
	case strings.HasPrefix(path, "chat"):
		result, err = svc.ForwardAsChatCompletions(officialUltrafastContext(), c, account, body, "", "gpt-6.1-sol")
	default:
		result, err = svc.Forward(officialUltrafastContext(), c, account, body)
	}
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "ultrafast", gjson.GetBytes(upstream.lastBody, "service_tier").String())
	return result, err
}

func assertOfficialUltrafastBillingCommand(t *testing.T, result *OpenAIForwardResult, observed string) {
	t.Helper()
	logs := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	users := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, billingRepo, users, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	group := contextThresholdGroup(15, 272000)
	group.RateMultiplier = 0.3
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: result, APIKey: &APIKey{ID: 1, GroupID: &group.ID, Group: group, Quota: 100, RateLimit5h: 100},
		User: &User{ID: 2}, Account: &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{},
	})
	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Zero(t, users.deductCalls, "仓库结算不能再走兼容扣款分支")
	cmd := billingRepo.lastCmd
	require.NotNil(t, cmd)
	require.NotNil(t, cmd.UsageLog)
	wantTier, multiple := "ultrafast", 6.0
	if observed == "default" {
		wantTier, multiple = "default", 1
	}
	require.Equal(t, wantTier, cmd.ServiceTier)
	base, err := svc.billingService.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", Group: group, GroupID: &group.ID,
		Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, Resolver: svc.resolver, RateMultiplier: 1, ServiceTier: "default"})
	require.NoError(t, err)
	want := base.TotalCost * multiple * 0.3
	require.InDelta(t, want, cmd.BalanceCost, 1e-9)
	require.InDelta(t, want, cmd.APIKeyQuotaCost, 1e-9)
	require.InDelta(t, want, cmd.APIKeyRateLimitCost, 1e-9)
	require.InDelta(t, want, cmd.UsageLog.ActualCost, 1e-9)
	require.InDelta(t, want, logs.lastLog.ActualCost, 1e-9)
}

func TestOfficialUltrafastSuccessFlowsReachBillingRepository(t *testing.T) {
	for _, path := range []string{"responses", "passthrough", "chat-responses", "chat-raw", "messages-responses", "messages-chat"} {
		for _, stream := range []bool{false, true} {
			for _, observed := range []string{"ultrafast", "default", ""} {
				t.Run(path+"/"+map[bool]string{false: "json", true: "stream"}[stream]+"/"+observed, func(t *testing.T) {
					result, err := forwardOfficialUltrafastFixture(t, path, stream, observed, true)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, "ultrafast", optionalStringValue(result.ServiceTier))
					require.Equal(t, observed, result.UpstreamResponseServiceTier)
					require.Equal(t, 1000, result.Usage.InputTokens)
					require.Equal(t, 500, result.Usage.OutputTokens)
					assertOfficialUltrafastBillingCommand(t, result, observed)
				})
			}
		}
	}
}

func TestOfficialUltrafastInterruptedMessagesKeepBillableTier(t *testing.T) {
	result, err := forwardOfficialUltrafastFixture(t, "messages-responses", true, "", false)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1000, result.Usage.InputTokens)
	require.Equal(t, 500, result.Usage.OutputTokens)
	require.Equal(t, "ultrafast", optionalStringValue(result.ServiceTier), "部分流仍按最终出站档位结算")
	assertOfficialUltrafastBillingCommand(t, result, "")
}

func TestOfficialUltrafastMessagesPassStillHonoursEarlierGroupPolicy(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"service_tier":"ultrafast"}`)
	for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionBlock, OpenAIFastPolicyActionForcePriority} {
		t.Run(action, func(t *testing.T) {
			policy := officialUltrafastPolicy()
			policy.Rules = append([]OpenAIFastPolicyRule{{
				ServiceTier: OpenAIFastTierUltrafast, Action: action,
				Scope: BetaPolicyScopeAPIKey, GroupIDs: []int64{15},
			}}, policy.Rules...)
			svc := newOpenAIGatewayServiceWithSettings(t, policy)
			require.Empty(t, svc.openAICompatMessagesServiceTier(officialUltrafastContext(), account, "gpt-6.1-sol", body))
			got, _ := svc.evaluateOpenAIFastPolicy(officialUltrafastContext(), account, "gpt-6.1-sol", "ultrafast")
			require.Equal(t, action, got, "后置pass不能绕过先匹配的分组规则")
		})
	}
}

func TestOfficialUltrafastMessagesPermissionIsExplicitAndScoped(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"service_tier":"ultrafast"}`)
	for _, test := range []struct {
		name        string
		policy      *OpenAIFastPolicySettings
		ctx         context.Context
		account     *Account
		model, want string
	}{
		{"allowed", officialUltrafastPolicy(), officialUltrafastContext(), account, "gpt-6.1-sol", "ultrafast"},
		{"legacy default pass", ultrafastGroupPolicy(), officialUltrafastContext(), account, "gpt-6.1-sol", ""},
		{"other group", officialUltrafastPolicy(), ultrafastGroupContext(11), account, "gpt-6.1-sol", ""},
		{"no group", officialUltrafastPolicy(), context.Background(), account, "gpt-6.1-sol", ""},
		{"unhydrated group", officialUltrafastPolicy(), context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 15}), account, "gpt-6.1-sol", ""},
		{"other model", officialUltrafastPolicy(), officialUltrafastContext(), account, "gpt-5.5", ""},
		{"oauth", officialUltrafastPolicy(), officialUltrafastContext(), &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-6.1-sol", ""},
		{"other provider", officialUltrafastPolicy(), officialUltrafastContext(), &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, "gpt-6.1-sol", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := newOpenAIGatewayServiceWithSettings(t, test.policy)
			require.Equal(t, test.want, svc.openAICompatMessagesServiceTier(test.ctx, test.account, test.model, body))
		})
	}
}
