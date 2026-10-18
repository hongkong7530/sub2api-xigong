package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func ultrafastGroupPolicy() *OpenAIFastPolicySettings {
	rules := []OpenAIFastPolicyRule{}
	for _, tier := range []string{OpenAIFastTierAny, OpenAIFastTierMissing} {
		rules = append(rules, OpenAIFastPolicyRule{ServiceTier: tier, Action: OpenAIFastPolicyActionForceUltrafast, Scope: BetaPolicyScopeAPIKey, GroupIDs: []int64{23}, ModelWhitelist: []string{"gpt-6-astra", "gpt-6.1-sol"}, FallbackAction: BetaPolicyActionBlock, FallbackErrorMessage: "Ultrafast group supports only GPT-6 Astra and GPT-6.1 Sol"})
	}
	return &OpenAIFastPolicySettings{Rules: rules}
}
func ultrafastGroupContext(id int64) context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, &Group{ID: id, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true})
}
func TestUltrafastGroupHTTPAndWSForceOnlyTargetGroup(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, ultrafastGroupPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol"} {
		for _, tier := range []string{"", "default", "auto", "flex", "fast", "priority", "ultrafast"} {
			t.Run(model+"/"+tier, func(t *testing.T) {
				field := ""
				if tier != "" {
					field = fmt.Sprintf(",\"service_tier\":%q", tier)
				}
				body := []byte(fmt.Sprintf(`{"type":"response.create","model":%q,"input":"hi"%s}`, model, field))
				updated, err := svc.applyOpenAIFastPolicyToBody(ultrafastGroupContext(23), account, model, body)
				require.NoError(t, err)
				require.Equal(t, "ultrafast", gjson.GetBytes(updated, "service_tier").String())
				frame, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ultrafastGroupContext(23), account, model, body)
				require.NoError(t, err)
				require.Nil(t, blocked)
				require.Equal(t, "ultrafast", gjson.GetBytes(frame, "service_tier").String())
				other, err := svc.applyOpenAIFastPolicyToBody(ultrafastGroupContext(11), account, model, body)
				require.NoError(t, err)
				want := tier
				if tier == "fast" {
					want = "priority"
				}
				require.Equal(t, want, gjson.GetBytes(other, "service_tier").String())
			})
		}
	}
}
func TestUltrafastGroupRejectsUnsupportedModelsIncludingMissingTier(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, ultrafastGroupPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, body := range []string{`{"type":"response.create","model":"gpt-5.6-sol"}`, `{"type":"response.create","model":"gpt-6-sol","service_tier":"default"}`} {
		_, err := svc.applyOpenAIFastPolicyToBody(ultrafastGroupContext(23), account, gjson.Get(body, "model").String(), []byte(body))
		require.ErrorAs(t, err, new(*OpenAIFastBlockedError))
		_, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ultrafastGroupContext(23), account, gjson.Get(body, "model").String(), []byte(body))
		require.NoError(t, err)
		require.NotNil(t, blocked)
	}
	frame := []byte(`{"type":"response.cancel"}`)
	actual, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ultrafastGroupContext(23), account, "gpt-6-sol", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, frame, actual)
}
func TestUltrafastGroupSettingsRoundTripAndValidation(t *testing.T) {
	repo := &openAIFastPolicyRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	require.NoError(t, svc.SetOpenAIFastPolicySettings(context.Background(), ultrafastGroupPolicy()))
	got, err := svc.GetOpenAIFastPolicySettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, ultrafastGroupPolicy(), got)
	for _, ids := range [][]int64{{0}, {-1}, {23, 23}} {
		policy := ultrafastGroupPolicy()
		policy.Rules[0].GroupIDs = ids
		require.Error(t, svc.SetOpenAIFastPolicySettings(context.Background(), policy))
	}
}
func TestUltrafastGroupScopeRequiresTrustedHydratedGroup(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, ultrafastGroupPolicy())
	body := []byte(`{"model":"gpt-6.1-sol"}`)
	for _, ctx := range []context.Context{context.Background(), context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 23})} {
		actual, err := svc.applyOpenAIFastPolicyToBody(ctx, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-6.1-sol", body)
		require.NoError(t, err)
		require.Equal(t, body, actual)
	}
}
