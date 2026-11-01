package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestJackUserMetadataAndAdminPreservation(t *testing.T) {
	requested, actual, upstream := "xhigh", "low", "private-model"
	log := &service.UsageLog{AccountID: 73, Model: "private-model", RequestedModel: "public-model", RequestedReasoningEffort: &requested, ReasoningEffort: &actual, UpstreamModel: &upstream}
	user, err := json.Marshal(UsageLogFromService(log))
	require.NoError(t, err)
	require.NotContains(t, string(user), "account_id")
	require.NotContains(t, string(user), "private-model")
	require.Contains(t, string(user), `"reasoning_effort":"xhigh"`)
	admin, err := json.Marshal(UsageLogFromServiceAdmin(log))
	require.NoError(t, err)
	require.Contains(t, string(admin), `"account_id":73`)
	require.Contains(t, string(admin), `"upstream_model":"private-model"`)
	require.Contains(t, string(admin), `"upstream_reasoning_effort":"low"`)
	log.LegacyRequestMetadata = true
	legacy := UsageLogFromService(log)
	require.Empty(t, legacy.Model)
	require.Nil(t, legacy.ReasoningEffort)
	legacyAdmin := UsageLogFromServiceAdmin(log)
	require.Equal(t, "public-model", legacyAdmin.Model)
	require.Equal(t, "xhigh", *legacyAdmin.ReasoningEffort)
	require.Equal(t, "private-model", *legacyAdmin.UpstreamModel)
	group := &service.Group{Platform: service.PlatformOpenAI, MaxReasoningEffort: "low", ReasoningEffortMappings: []domain.ReasoningEffortMapping{{From: "xhigh", To: "low"}}}
	user, err = json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(user), "reasoning_effort")
	admin, err = json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	require.Contains(t, string(admin), `"max_reasoning_effort":"low"`)
	require.Contains(t, string(admin), `"from":"xhigh","to":"low"`)
}

func TestJackAdminMissingRequestedSettingsStayInMappingFields(t *testing.T) {
	actual := "low"
	admin := UsageLogFromServiceAdmin(&service.UsageLog{Model: "private-model", ReasoningEffort: &actual})
	require.Empty(t, admin.Model)
	require.Nil(t, admin.ReasoningEffort)
	require.Equal(t, "private-model", *admin.UpstreamModel)
	require.Equal(t, actual, *admin.UpstreamReasoningEffort)
}

func TestJackPrivacyOtherProviderUsageAndGroupUnchanged(t *testing.T) {
	for _, platform := range []string{service.PlatformAnthropic, service.PlatformGemini, service.PlatformGrok, service.PlatformAntigravity} {
		effort := "high"
		log := &service.UsageLog{AccountID: 73, Model: "original-model", ReasoningEffort: &effort, LegacyRequestMetadata: true, Account: &service.Account{Platform: platform}, Group: &service.Group{Platform: service.PlatformOpenAI}}
		row := UsageLogFromService(log)
		require.Equal(t, int64(73), *row.AccountID)
		require.Equal(t, "original-model", row.Model)
		require.Equal(t, effort, *row.ReasoningEffort)
		group := &service.Group{Platform: platform, MaxReasoningEffort: "low", ReasoningEffortMappings: []domain.ReasoningEffortMapping{{From: "high", To: "low"}}}
		body, err := json.Marshal(GroupFromService(group))
		require.NoError(t, err)
		require.Contains(t, string(body), `"max_reasoning_effort":"low"`)
		require.Contains(t, string(body), `"from":"high","to":"low"`)
	}
}
