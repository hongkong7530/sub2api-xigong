//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func contextThresholdGroup(id int64, threshold int) *Group {
	return &Group{ID: id, Platform: PlatformOpenAI, LongContextPricingEnabled: true,
		ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-6.1-sol"}, BillingMode: BillingModeToken, LongContextInputThreshold: &threshold}}}
}

func TestGroupContextThreshold272BoundariesAndIsolation(t *testing.T) {
	billing := newTestBillingService()
	resolver := NewModelPricingResolver(nil, billing)
	ctx := context.Background()
	for _, id := range []int64{11, 15, 27} {
		group := contextThresholdGroup(id, 272000)
		for _, tier := range []string{"default", "priority", "flex", "ultrafast"} {
			for _, n := range []int{271999, 272000, 272001, 499999, 500000, 500001} {
				tokens := UsageTokens{InputTokens: n - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
				got, err := billing.CalculateCostUnified(CostInput{Ctx: ctx, Model: "gpt-6.1-sol", Group: group, GroupID: &id, Tokens: tokens, Resolver: resolver, RateMultiplier: 0.18, ServiceTier: tier})
				require.NoError(t, err)
				shortTokens := UsageTokens{InputTokens: 1000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
				short, err := billing.CalculateCostWithServiceTier("gpt-6.1-sol", shortTokens, 1, tier)
				require.NoError(t, err)
				im, om := 1.0, 1.0
				if n > 272000 {
					im, om = 2, 1.5
				}
				require.InDelta(t, short.InputCost/1000*float64(tokens.InputTokens)*im, got.InputCost, 1e-10)
				require.InDelta(t, short.CacheReadCost*im, got.CacheReadCost, 1e-10)
				require.InDelta(t, short.CacheCreationCost*im, got.CacheCreationCost, 1e-10)
				require.InDelta(t, short.OutputCost*om, got.OutputCost, 1e-10)
				require.InDelta(t, got.TotalCost*0.18, got.ActualCost, 1e-10)
			}
		}
		resolved := resolver.Resolve(ctx, PricingInput{Model: "gpt-6.1-sol", Group: group, GroupID: &id})
		require.Equal(t, 272000, resolved.BasePricing.LongContextInputThreshold)
		plain := resolver.Resolve(ctx, PricingInput{Model: "gpt-6.1-sol", Group: &Group{ID: 23, Platform: PlatformOpenAI, LongContextPricingEnabled: true}})
		require.Equal(t, 500000, plain.BasePricing.LongContextInputThreshold)
		other := resolver.Resolve(ctx, PricingInput{Model: "gpt-6-astra", Group: group})
		require.False(t, other.BasePricing.LongContextThresholdExplicit)
	}
}

func TestGroupContextThresholdUltrafastOfficialCard272(t *testing.T) {
	billing := newTestBillingService()
	resolver := NewModelPricingResolver(nil, billing)
	group := contextThresholdGroup(23, 272000)
	card := &group.ModelPricing[0]
	card.InputPrice = testPtrFloat64(2e-6)
	card.OutputPrice = testPtrFloat64(10e-6)
	card.CacheReadPrice = testPtrFloat64(0.1e-6)
	card.CacheWritePrice = testPtrFloat64(2.5e-6)
	for _, total := range []int{271999, 272000, 272001, 500001} {
		tokens := UsageTokens{InputTokens: total - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
		cost, err := billing.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", Group: group, GroupID: &group.ID, Tokens: tokens, Resolver: resolver, RateMultiplier: 0.185, ServiceTier: "ultrafast"})
		require.NoError(t, err)
		im, om := 1.0, 1.0
		if total > 272000 {
			im, om = 2, 1.5
		}
		require.InDelta(t, float64(tokens.InputTokens)*12e-6*im, cost.InputCost, 1e-10)
		require.InDelta(t, 2000*0.6e-6*im, cost.CacheReadCost, 1e-10)
		require.InDelta(t, 1000*15e-6*im, cost.CacheCreationCost, 1e-10)
		require.InDelta(t, 500*60e-6*om, cost.OutputCost, 1e-10)
		require.InDelta(t, cost.TotalCost*0.185, cost.ActualCost, 1e-10)
	}
	schedule, err := billing.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol", Platform: PlatformOpenAI, Group: group})
	require.NoError(t, err)
	require.Len(t, schedule.Tiers, 2)
	require.Equal(t, 272000, *schedule.Tiers[0].MaxTokens)
	require.Equal(t, 272000, schedule.Tiers[1].MinTokens)
}

func TestGroupContextThresholdScheduleMatchesBillingAndToggle(t *testing.T) {
	billing := newTestBillingService()
	resolver := NewModelPricingResolver(nil, billing)
	group := contextThresholdGroup(11, 272000)
	for _, enabled := range []bool{true, false} {
		group.LongContextPricingEnabled = enabled
		schedule, err := billing.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol", Platform: PlatformOpenAI, Group: group})
		require.NoError(t, err)
		if enabled {
			require.Len(t, schedule.Tiers, 2)
			require.Equal(t, 272000, *schedule.Tiers[0].MaxTokens)
			require.Equal(t, 272000, schedule.Tiers[1].MinTokens)
			require.InDelta(t, *schedule.Tiers[0].Input*2, *schedule.Tiers[1].Input, 1e-12)
			require.InDelta(t, *schedule.Tiers[0].Output*1.5, *schedule.Tiers[1].Output, 1e-12)
		} else {
			require.Len(t, schedule.Tiers, 1)
		}
	}
}

func TestGroupContextThresholdOnlyCardPreservesInheritedPrices(t *testing.T) {
	input, output := 7e-6, 13e-6
	billing, resolver := newTokenCostTestEnv(t, PlatformOpenAI, []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-6.1-sol"}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output}}, nil)
	group := contextThresholdGroup(100, 272000)
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "gpt-6.1-sol", Group: group, GroupID: &group.ID})
	require.Equal(t, PricingSourceChannel, resolved.Source)
	require.Equal(t, input, resolved.BasePricing.InputPricePerToken)
	require.Equal(t, output, resolved.BasePricing.OutputPricePerToken)
	tokens := UsageTokens{InputTokens: 272001, OutputTokens: 100}
	cost, err := billing.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", Group: group, GroupID: &group.ID, Tokens: tokens, Resolver: resolver, RateMultiplier: 0.3})
	require.NoError(t, err)
	require.InDelta(t, 272001*input*2, cost.InputCost, 1e-10)
	require.InDelta(t, 100*output*1.5, cost.OutputCost, 1e-10)
}

func TestGroupContextThresholdValidationAndJSONRoundtrip(t *testing.T) {
	for _, value := range []int{0, -1} {
		_, err := normalizeGroupModelPricing(PlatformOpenAI, contextThresholdGroup(11, value).ModelPricing)
		require.Error(t, err)
	}
	pricing, err := normalizeGroupModelPricing(PlatformOpenAI, contextThresholdGroup(11, 272000).ModelPricing)
	require.NoError(t, err)
	data, err := json.Marshal(pricing)
	require.NoError(t, err)
	var decoded []ChannelModelPricing
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, 272000, *decoded[0].LongContextInputThreshold)
	decoded[0].BillingMode = BillingModePerRequest
	_, err = normalizeGroupModelPricing(PlatformOpenAI, decoded)
	require.Error(t, err)
}
