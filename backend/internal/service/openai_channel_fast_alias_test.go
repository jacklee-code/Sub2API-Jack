//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIChannelFastAliasRoutingAndPricing(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1)
	channel := Channel{
		ID: 11, Status: StatusActive, GroupIDs: []int64{groupID},
		BillingModelSource: BillingModelSourceUpstream,
		ModelMapping: map[string]map[string]string{PlatformOpenAI: {
			"gpt-6.2-sol-fast":   "gpt-6.2-sol",
			"gpt-6.2-astra-fast": "gpt-6.2-astra",
			"gpt-6-luna-fast":    "gpt-6-luna",
			"gpt-6.1-sol-fa*":    "gpt-6.1-sol",
		}},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-6.2-sol-fast"}, FastMultiplier: pricingMultiplier(3)},
			{Platform: PlatformOpenAI, Models: []string{"gpt-6.2-astra-fast"}, FastMultiplier: pricingMultiplier(3.5)},
			{Platform: PlatformOpenAI, Models: []string{"gpt-6.1-sol-fast"}, FastMultiplier: pricingMultiplier(9)},
		},
	}
	channels := newTestChannelService(makeStandardRepo(channel, map[int64]string{groupID: PlatformOpenAI}))
	billing := &BillingService{fallbackPrices: make(map[string]*ModelPricing)}
	billing.initFallbackPricing()
	resolver := NewModelPricingResolver(channels, billing)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 1000, CacheReadTokens: 100}

	for _, example := range []struct {
		base string
		rate float64
	}{
		{base: "gpt-6.2-sol", rate: 3},
		{base: "gpt-6.2-astra", rate: 3.5},
	} {
		alias := example.base + "-fast"
		mapping := channels.ResolveChannelMapping(ctx, groupID, alias)
		require.Equal(t, example.base, mapping.MappedModel)
		require.Equal(t, OpenAIFastTierPriority, mapping.OpenAIServiceTier)
		require.Equal(t, BillingModelSourceRequested, mapping.ToUsageFields(alias, example.base).BillingModelSource)
		request, err := mapping.ApplyOpenAIServiceTier([]byte(`{"service_tier":"default"}`))
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(request, "service_tier").String())

		standard := resolver.Resolve(ctx, PricingInput{Model: example.base, GroupID: &groupID})
		fast := resolver.Resolve(ctx, PricingInput{Model: alias, GroupID: &groupID})
		require.Equal(t, PricingSourceChannel, fast.Source)
		normalCost := billing.computeTokenBreakdown(standard.BasePricing, tokens, 1, "", false)
		nativeFastCost := billing.computeTokenBreakdown(standard.BasePricing, tokens, 1, OpenAIFastTierPriority, false)
		aliasFastCost := billing.computeTokenBreakdown(fast.BasePricing, tokens, 1, OpenAIFastTierPriority, false)
		aliasStandardCost := billing.computeTokenBreakdown(fast.BasePricing, tokens, 1, "", false)
		require.Positive(t, normalCost.TotalCost)
		require.InDelta(t, normalCost.TotalCost*2, nativeFastCost.TotalCost, 1e-12)
		require.InDelta(t, normalCost.TotalCost*example.rate, aliasFastCost.TotalCost, 1e-12)
		require.InDelta(t, normalCost.TotalCost, aliasStandardCost.TotalCost, 1e-12)
	}
	for _, model := range []string{"gpt-6.2-sol", "gpt-6.2-sol-ultrafast", "gpt-6-luna-fast", "gpt-6.1-sol-fast"} {
		require.Empty(t, channels.ResolveChannelMapping(ctx, groupID, model).OpenAIServiceTier, model)
	}
	require.Empty(t, channels.ResolveChannelMapping(ctx, groupID+1, "gpt-6.2-sol-fast").OpenAIServiceTier)

	group := &Group{ID: groupID, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{
		Enabled: true, Models: []string{"gpt-6.2-sol", "gpt-6.2-sol-fast", "gpt-6.2-astra"},
	}}
	gateway := &OpenAIGatewayService{channelService: channels}
	manifest, err := BuildCodexModelsManifest([]string{"gpt-6.2-sol", "gpt-6.2-astra"})
	require.NoError(t, err)
	manifest, changed, err := gateway.addOpenAIChannelFastAliases(ctx, group, manifest)
	require.NoError(t, err)
	require.True(t, changed)
	base := gjson.GetBytes(manifest, `models.#(slug=="gpt-6.2-sol")`)
	alias := gjson.GetBytes(manifest, `models.#(slug=="gpt-6.2-sol-fast")`)
	require.Equal(t, base.Get("display_name").String()+" Fast", alias.Get("display_name").String())
	require.Equal(t, base.Get("supported_reasoning_levels").Raw, alias.Get("supported_reasoning_levels").Raw)
	require.Equal(t, base.Get("context_window").Int(), alias.Get("context_window").Int())
	require.False(t, gjson.GetBytes(manifest, `models.#(slug=="gpt-6.2-astra-fast")`).Exists())
}

func TestOpenAIChannelFastAliasWebSocketTurnTier(t *testing.T) {
	hooks := &OpenAIWSIngressHooks{RequestServiceTier: func(turn int) string {
		if turn == 2 {
			return OpenAIFastTierPriority
		}
		return ""
	}}
	request := []byte(`{"type":"response.create","model":"gpt-6.2-sol"}`)
	for turn, expected := range []string{"", OpenAIFastTierPriority, ""} {
		body, err := hooks.applyChannelServiceTier(turn+1, request)
		require.NoError(t, err)
		require.Equal(t, expected, gjson.GetBytes(body, "service_tier").String())
	}
}
