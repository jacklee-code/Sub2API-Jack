package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// A speed alias requires both an exact channel mapping and an exact token
// price card. Wildcards and unconfigured suffixes keep their existing meaning.
func openAIChannelFastAliasTarget(lk *channelLookup, groupID int64, model string) string {
	base, fast := strings.CutSuffix(model, "-fast")
	if !fast || lk.platform != PlatformOpenAI {
		return ""
	}
	switch base {
	case "gpt-6.2-astra", "gpt-6.2-sol", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-astra", "gpt-6-sol", "gpt-6-luna":
	default:
		return ""
	}
	key := channelModelKey{groupID: groupID, platform: PlatformOpenAI, model: model}
	if lk.cache.mappingByGroupModel[key] != base {
		return ""
	}
	pricing := lk.cache.pricingByGroupModel[key]
	if pricing == nil || (pricing.BillingMode != "" && pricing.BillingMode != BillingModeToken) ||
		pricing.FastMultiplier == nil || *pricing.FastMultiplier <= 0 {
		return ""
	}
	return base
}

// AppendOpenAIChannelFastModelIDs augments the standard /models list using
// the current channel and the already available base models for this group.
func (s *GatewayService) AppendOpenAIChannelFastModelIDs(ctx context.Context, groupID *int64, modelIDs []string) []string {
	if groupID == nil || s.channelService == nil || len(modelIDs) == 0 {
		return modelIDs
	}
	lk, err := s.channelService.lookupGroupChannel(ctx, *groupID)
	if err != nil || lk == nil || len(lk.channel.ModelMapping[PlatformOpenAI]) == 0 {
		return modelIDs
	}

	var aliases []string
	for _, base := range modelIDs {
		alias := base + "-fast"
		if base != "" && openAIChannelFastAliasTarget(lk, *groupID, alias) == base && !slices.Contains(modelIDs, alias) {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		return modelIDs
	}
	return append(slices.Clone(modelIDs), aliases...)
}

// ApplyOpenAIServiceTier materializes the requested alias before the normal
// account and Fast/Flex policy checks. Usage sees the final forwarded tier.
func (r ChannelMappingResult) ApplyOpenAIServiceTier(body []byte) ([]byte, error) {
	return applyOpenAIChannelServiceTier(body, r.OpenAIServiceTier)
}

func applyOpenAIChannelServiceTier(body []byte, tier string) ([]byte, error) {
	if tier == "" || gjson.GetBytes(body, "service_tier").String() == tier {
		return body, nil
	}
	return sjson.SetBytes(body, "service_tier", tier)
}

func (h *OpenAIWSIngressHooks) applyChannelServiceTier(turn int, body []byte) ([]byte, error) {
	if h == nil || h.RequestServiceTier == nil {
		return body, nil
	}
	return applyOpenAIChannelServiceTier(body, h.RequestServiceTier(turn))
}

func (s *OpenAIGatewayService) addOpenAIChannelFastAliases(ctx context.Context, group *Group, body []byte) ([]byte, bool, error) {
	if s.channelService == nil {
		return body, false, nil
	}
	lk, err := s.channelService.lookupGroupChannel(ctx, group.ID)
	if err != nil || lk == nil || len(lk.channel.ModelMapping[PlatformOpenAI]) == 0 {
		return body, false, err
	}

	// Inherit the live descriptor of the base model, including its reasoning
	// levels, tools and context window. An unavailable base has no alias here.
	entries := gjson.GetBytes(body, "models").Array()
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.Get("slug").String()] = true
	}
	models := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		models = append(models, json.RawMessage(entry.Raw))
		base := entry.Get("slug").String()
		alias := base + "-fast"
		if base == "" || entry.Get("visibility").String() != "list" || seen[alias] ||
			!group.ModelAllowlist.Allows(alias) || openAIChannelFastAliasTarget(lk, group.ID, alias) != base {
			continue
		}
		var descriptor map[string]json.RawMessage
		if err := json.Unmarshal([]byte(entry.Raw), &descriptor); err != nil {
			return body, false, fmt.Errorf("decode OpenAI Fast base descriptor: %w", err)
		}
		name := entry.Get("display_name").String()
		if name == "" {
			name = base
		}
		descriptor["slug"], _ = json.Marshal(alias)
		descriptor["display_name"], _ = json.Marshal(name + " Fast")
		descriptor["visibility"] = json.RawMessage(`"list"`)
		descriptor["supported_in_api"] = json.RawMessage("true")
		descriptor["is_default"] = json.RawMessage("false")
		descriptor["service_tiers"] = json.RawMessage("[]")
		descriptor["additional_speed_tiers"] = json.RawMessage("[]")
		descriptor["default_service_tier"] = json.RawMessage("null")
		added, err := json.Marshal(descriptor)
		if err != nil {
			return body, false, fmt.Errorf("encode OpenAI Fast descriptor: %w", err)
		}
		models = append(models, added)
		seen[alias] = true
	}
	if len(models) == len(entries) {
		return body, false, nil
	}
	updated, err := sjson.SetBytes(body, "models", models)
	return updated, err == nil, err
}
