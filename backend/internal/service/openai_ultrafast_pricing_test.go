package service

import (
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGPT61SolUltrafastOfficialPricingAcrossCatalogAndLongContext(t *testing.T) {
	data, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	catalog := &PricingService{}
	catalog.pricingData, err = catalog.parsePricingData(data)
	require.NoError(t, err)
	for _, svc := range []*BillingService{newTestBillingService(), NewBillingService(&config.Config{}, &PricingService{}), NewBillingService(&config.Config{}, catalog)} {
		for _, model := range []string{"gpt-6.1-sol", "openai/gpt-6.1-sol", "gpt-6.1-sol-max"} {
			for _, n := range []int{499999, 500000, 500001} {
				tokens := UsageTokens{InputTokens: n - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
				cost, err := svc.CalculateCostWithServiceTier(model, tokens, 0.188, "ultrafast")
				require.NoError(t, err)
				im, om := 1.0, 1.0
				if n > 500000 {
					im, om = 2, 1.5
				}
				require.InDelta(t, float64(n-3000)*12e-6*im, cost.InputCost, 1e-10)
				require.InDelta(t, 2000*0.6e-6*im, cost.CacheReadCost, 1e-10)
				require.InDelta(t, 1000*15e-6*im, cost.CacheCreationCost, 1e-10)
				require.InDelta(t, 500*60e-6*om, cost.OutputCost, 1e-10)
				require.InDelta(t, cost.TotalCost*0.188, cost.ActualCost, 1e-10)
			}
		}
	}
}
func TestUltrafastGroupOfficialCardsApplySixTimesAndGroupDiscountOnce(t *testing.T) {
	f := func(value float64) *float64 { return &value }
	billing := newTestBillingService()
	resolver := NewModelPricingResolver(nil, billing)
	group := &Group{ID: 23, Platform: PlatformOpenAI, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{
		{Platform: PlatformOpenAI, Models: []string{"gpt-6-astra"}, BillingMode: BillingModeToken, InputPrice: f(10e-6), OutputPrice: f(50e-6), CacheWritePrice: f(12.5e-6), CacheReadPrice: f(1e-6)},
		{Platform: PlatformOpenAI, Models: []string{"gpt-6.1-sol"}, BillingMode: BillingModeToken, InputPrice: f(2e-6), OutputPrice: f(10e-6), CacheWritePrice: f(2.5e-6), CacheReadPrice: f(0.1e-6)},
	}}
	for _, test := range []struct {
		model                      string
		threshold                  int
		input, output, write, read float64
	}{
		{"gpt-6-astra", 272000, 60e-6, 300e-6, 75e-6, 6e-6},
		{"gpt-6.1-sol", 500000, 12e-6, 60e-6, 15e-6, 0.6e-6},
	} {
		for _, total := range []int{test.threshold, test.threshold + 1} {
			tokens := UsageTokens{InputTokens: total - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
			cost, err := billing.CalculateCostUnified(CostInput{Model: test.model, Group: group, GroupID: &group.ID, Tokens: tokens, RateMultiplier: 0.188, ServiceTier: "ultrafast", Resolver: resolver})
			require.NoError(t, err)
			im, om := 1.0, 1.0
			if total > test.threshold {
				im, om = 2, 1.5
			}
			require.InDelta(t, float64(tokens.InputTokens)*test.input*im, cost.InputCost, 1e-10)
			require.InDelta(t, 2000*test.read*im, cost.CacheReadCost, 1e-10)
			require.InDelta(t, 1000*test.write*im, cost.CacheCreationCost, 1e-10)
			require.InDelta(t, 500*test.output*om, cost.OutputCost, 1e-10)
			require.InDelta(t, cost.TotalCost*0.188, cost.ActualCost, 1e-10)
		}
	}
}

func TestUltrafastOfficialDowngradeDoesNotOverbill(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, tier := range []string{"priority", "default", "flex"} {
		r := ResolveOpenAIServiceTierBilling(account, "ultrafast", tier)
		require.True(t, r.Downgraded)
		require.Equal(t, tier, r.Billing)
	}
}
