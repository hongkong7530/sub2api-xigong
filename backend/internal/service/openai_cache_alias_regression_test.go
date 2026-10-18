package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeepseekNativeCacheAliasBillsCachedInputSeparately(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"usage":{"prompt_tokens":1200,"completion_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}}`))
	require.True(t, ok)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	// 隔离缓存拆桶与价格来源：DeepSeek 官方价卡会按时段覆盖目录价格，
	// 本测试用独立价卡验证解析所得缓存确实以缓存单价计费，不改产品价卡。
	svc.billingService = NewBillingService(svc.cfg, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"cache-alias-fixture": {InputCostPerToken: 2e-6, OutputCostPerToken: 8e-6, CacheReadInputTokenCost: 0.04e-6},
	}})
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "cache-alias-billing", Model: "cache-alias-fixture", Usage: usage, Duration: time.Second},
		APIKey: &APIKey{ID: 101}, User: &User{ID: 201}, Account: &Account{ID: 301},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 400, usageRepo.lastLog.InputTokens)
	require.Equal(t, 800, usageRepo.lastLog.CacheReadTokens)
	require.Equal(t, 1230, usageRepo.lastLog.TotalTokens())
	require.InDelta(t, 400*2e-6, usageRepo.lastLog.InputCost, 1e-12)
	require.InDelta(t, 800*0.04e-6, usageRepo.lastLog.CacheReadCost, 1e-12)
	require.InDelta(t, 30*8e-6, usageRepo.lastLog.OutputCost, 1e-12)
}

func TestDeepseekNativeCacheAliasPreservesUsageBuckets(t *testing.T) {
	for _, tc := range []struct {
		name string
		usage string
		cached int
	}{
		{"native", `{"prompt_tokens":1200,"completion_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}`, 800},
		{"nested_authoritative_zero", `{"prompt_tokens":1200,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":0},"prompt_cache_hit_tokens":800}`, 0},
		{"nested_precedence", `{"prompt_tokens":1200,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":600},"prompt_cache_hit_tokens":800}`, 600},
		{"top_level_precedence", `{"prompt_tokens":1200,"completion_tokens":30,"cache_read_input_tokens":700,"prompt_cache_hit_tokens":800}`, 700},
		{"no_cache", `{"prompt_tokens":1200,"completion_tokens":30}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"usage":` + tc.usage + `}`)
			jsonUsage, ok := extractOpenAIUsageFromJSONBytes(body)
			require.True(t, ok)
			streamUsage := extractCCStreamUsage(string(body))
			require.NotNil(t, streamUsage)
			require.Equal(t, jsonUsage, *streamUsage)
			require.Equal(t, 1200, jsonUsage.InputTokens)
			require.Equal(t, 30, jsonUsage.OutputTokens)
			require.Equal(t, tc.cached, jsonUsage.CacheReadInputTokens)
			require.Equal(t, 1200-tc.cached, jsonUsage.InputTokens-jsonUsage.CacheReadInputTokens-jsonUsage.CacheCreationInputTokens)
		})
	}
}
