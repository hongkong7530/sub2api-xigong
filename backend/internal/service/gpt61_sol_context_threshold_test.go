//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGPT61SolStaleCatalogUsesStrict500KThreshold(t *testing.T) {
	stale := &LiteLLMModelPricing{
		InputCostPerToken: 2e-6, OutputCostPerToken: 10e-6,
		CacheCreationInputTokenCost: 2.5e-6, CacheReadInputTokenCost: 0.1e-6,
		LongContextInputTokenThreshold: 272000,
		LongContextInputCostMultiplier: 2, LongContextOutputCostMultiplier: 1.5,
	}
	catalog := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{"gpt-6.1-sol": stale})
	billing := NewBillingService(&config.Config{}, catalog)
	for _, model := range []string{"gpt-6.1-sol", "openai/gpt-6.1-sol-max", "gpt-6.1-sol-openai-compact"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := billing.GetModelPricing(model)
			require.NoError(t, err)
			require.Equal(t, 500000, pricing.LongContextInputThreshold)
			for _, n := range []int{272001, 499999, 500000, 500001} {
				cost, err := billing.CalculateCost(model, UsageTokens{InputTokens: n - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}, 1)
				require.NoError(t, err)
				require.Equal(t, n > 500000, cost.LongContextBillingApplied)
			}
		})
	}
	require.Equal(t, 272000, stale.LongContextInputTokenThreshold, "do not mutate the shared source catalog")
	resolver := NewModelPricingResolver(nil, billing)
	schedule, err := billing.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol"})
	require.NoError(t, err)
	require.Len(t, schedule.Tiers, 2)
	require.Equal(t, 500000, *schedule.Tiers[0].MaxTokens)
	require.Equal(t, "≤500K", schedule.Tiers[0].Label)
	require.Equal(t, ">500K", schedule.Tiers[1].Label)
	require.InDelta(t, 4e-6, *schedule.Tiers[1].Input, 1e-12)
	require.InDelta(t, 15e-6, *schedule.Tiers[1].Output, 1e-12)
}
