//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPlazaDeepSeekOfficialPeakReferenceDoesNotChangeSellingPrices(t *testing.T) {
	models := []struct {
		model string
		input, output, read float64
	}{
		{"deepseek-v4.1-flash", 0.30e-6, 1.20e-6, 0.006e-6},
		{"deepseek-v4-flash", 0.30e-6, 1.20e-6, 0.006e-6},
		{"deepseek-v4-flash-0731", 0.30e-6, 1.20e-6, 0.006e-6},
		{"deepseek-v4-pro", 1.32e-6, 3.96e-6, 0.044e-6},
		{"deepseek-v4-pro-0813", 1.32e-6, 3.96e-6, 0.044e-6},
	}
	for _, tc := range models {
		t.Run(tc.model, func(t *testing.T) {
			channel := plazaPricedChannel(1, "deepseek", []int64{10}, "deepseek", tc.model)
			channel.ModelPricing[0].InputPrice = testPtrFloat64(9e-6)
			channel.ModelPricing[0].OutputPrice = testPtrFloat64(27e-6)
			channel.ModelPricing[0].CacheReadPrice = testPtrFloat64(0.3e-6)
			svc := newPlazaServiceWithBilling([]Channel{channel}, []Group{{ID: 10, Platform: "deepseek", RateMultiplier: 0.07}}, map[int64]string{10: "deepseek"}, nil)
			out, err := svc.ListGroups(context.Background())
			require.NoError(t, err)
			require.Len(t, out, 1)
			m := out[0].Models[0]
			require.InDelta(t, 9e-6, *m.Pricing.InputPrice, 1e-12)
			require.InDelta(t, 27e-6, *m.Pricing.OutputPrice, 1e-12)
			require.InDelta(t, 0.3e-6, *m.Pricing.CacheReadPrice, 1e-12)
			require.Equal(t, 0.07, out[0].RateMultiplier)
			require.InDelta(t, tc.input, *m.OfficialPricing.InputPrice, 1e-12)
			require.InDelta(t, tc.output, *m.OfficialPricing.OutputPrice, 1e-12)
			require.InDelta(t, tc.read, *m.OfficialPricing.CacheReadPrice, 1e-12)
			require.Nil(t, m.OfficialPricing.CacheWritePrice)
		})
	}
}

func TestKimiK3ConfiguredDollarRatesOverrideStaleConvertedCatalog(t *testing.T) {
	stale := &LiteLLMModelPricing{
		InputCostPerToken: 2.94806975133e-6, OutputCostPerToken: 14.7403487567e-6,
		CacheReadInputTokenCost: 0.294806975133e-6,
		CacheCreationInputTokenCost: 2.94806975133e-6,
		CacheCreationInputTokenCostAbove1hr: 2.94806975133e-6,
	}
	catalog := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{"kimi-k3": stale})
	bs := NewBillingService(&config.Config{}, catalog)
	pricing, err := bs.GetModelPricing("kimi-k3")
	require.NoError(t, err)
	require.Equal(t, 20e-6, pricing.InputPricePerToken)
	require.Equal(t, 100e-6, pricing.OutputPricePerToken)
	require.Equal(t, 2e-6, pricing.CacheReadPricePerToken)
	require.Zero(t, pricing.CacheCreationPricePerToken)
	require.False(t, pricing.SupportsCacheBreakdown)
	require.Equal(t, 2.94806975133e-6, stale.InputCostPerToken, "shared catalog must remain unchanged")
	cost, err := bs.CalculateCost("kimi-k3", UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}, 0.07)
	require.NoError(t, err)
	require.InDelta(t, 122.0, cost.TotalCost, 1e-10)
	require.InDelta(t, 8.54, cost.ActualCost, 1e-10)

	custom, err := bs.GetModelPricingWithChannel("kimi-k3", &ChannelModelPricing{InputPrice: testPtrFloat64(7e-6), OutputPrice: testPtrFloat64(8e-6), CacheReadPrice: testPtrFloat64(0.9e-6)})
	require.NoError(t, err)
	require.Equal(t, 7e-6, custom.InputPricePerToken)
	require.Equal(t, 8e-6, custom.OutputPricePerToken)
	require.Equal(t, 0.9e-6, custom.CacheReadPricePerToken)

	svc := newPlazaServiceWithBilling([]Channel{plazaPricedChannel(1, "kimi", []int64{12}, PlatformKimi, "kimi-k3")}, []Group{{ID: 12, Platform: PlatformKimi, RateMultiplier: 0.07}}, map[int64]string{12: PlatformKimi}, catalog)
	memo := make(map[string]*PlazaOfficialPricing)
	official := svc.lookupOfficialPricing(context.Background(), "kimi-k3", memo)
	require.Equal(t, 20e-6, *official.InputPrice)
	require.Equal(t, 100e-6, *official.OutputPrice)
	require.Equal(t, 2e-6, *official.CacheReadPrice)
	require.Nil(t, official.CacheWritePrice)
}
