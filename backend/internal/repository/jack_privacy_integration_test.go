//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestJackPrivacyUsageProvenanceSurvivesRoundtrip(t *testing.T) {
	tx := testEntTx(t)
	client := tx.Client()
	ctx := context.Background()
	u := mustCreateUser(t, client, &service.User{Email: "jack-privacy@example.test"})
	k := mustCreateApiKey(t, client, &service.APIKey{UserID: u.ID, Key: "sk-jack-privacy", Name: "privacy"})
	a := mustCreateAccount(t, client, &service.Account{Name: "privacy-upstream", Platform: service.PlatformOpenAI})
	r := newUsageLogRepositoryWithSQL(client, tx)
	requested, actual, mapped := "xhigh", "low", "private-model"
	entry := &service.UsageLog{UserID: u.ID, APIKeyID: k.ID, AccountID: a.ID, RequestID: "jack-privacy-roundtrip", Model: mapped, RequestedModel: "public-model", RequestedReasoningEffort: &requested, ReasoningEffort: &actual, UpstreamModel: &mapped}
	entry.CacheCreation30mTokens = 17
	entry.CacheCreation12hTokens = 29
	_, err := r.Create(ctx, entry)
	require.NoError(t, err)
	read, err := r.GetByID(ctx, entry.ID)
	require.NoError(t, err)
	require.Equal(t, 17, read.CacheCreation30mTokens)
	require.Equal(t, 29, read.CacheCreation12hTokens)
	require.False(t, read.LegacyRequestMetadata)
	require.Equal(t, "public-model", dto.UsageLogFromService(read).Model)
	// Simulate an old row/old writer: its database default is deliberately false.
	_, err = r.sql.ExecContext(ctx, "UPDATE usage_logs SET jack_request_metadata_verified=DEFAULT WHERE id=$1", entry.ID)
	require.NoError(t, err)
	read, err = r.GetByID(ctx, entry.ID)
	require.NoError(t, err)
	require.True(t, read.LegacyRequestMetadata)
	public := dto.UsageLogFromService(read)
	require.Empty(t, public.Model)
	require.Nil(t, public.ReasoningEffort)
	admin := dto.UsageLogFromServiceAdmin(read)
	require.Equal(t, a.ID, admin.AccountID)
	require.Equal(t, "public-model", admin.Model)
	require.Equal(t, mapped, *admin.UpstreamModel)
	require.Equal(t, requested, *admin.ReasoningEffort)
	require.Equal(t, actual, *admin.UpstreamReasoningEffort)
	for _, tc := range []struct{ source, model string }{
		{usagestats.ModelSourcePublicRequested, "unknown"},
		{usagestats.ModelSourceRequested, "public-model"},
		{usagestats.ModelSourceUpstream, mapped},
	} {
		stats, err := r.GetModelStatsWithUsageFiltersBySource(ctx, read.CreatedAt.Add(-time.Hour), read.CreatedAt.Add(time.Hour), usagestats.UsageLogFilters{UserID: u.ID}, tc.source)
		require.NoError(t, err)
		require.Len(t, stats, 1)
		require.Equal(t, tc.model, stats[0].Model)
	}
	// A legacy row from another provider retains its original public values and
	// account ID, even when queried alongside OpenAI under the same API key.
	other := mustCreateAccount(t, client, &service.Account{Name: "unchanged-anthropic", Platform: service.PlatformAnthropic})
	otherEntry := &service.UsageLog{RoutingProvider: service.PlatformAnthropic, UserID: u.ID, APIKeyID: k.ID, AccountID: other.ID, RequestID: "jack-privacy-other-provider", Model: "anthropic-original", ReasoningEffort: &actual}
	_, err = r.Create(ctx, otherEntry)
	require.NoError(t, err)
	_, err = r.sql.ExecContext(ctx, "UPDATE usage_logs SET jack_request_metadata_verified=DEFAULT WHERE id=$1", otherEntry.ID)
	require.NoError(t, err)
	otherRead, err := r.GetByID(ctx, otherEntry.ID)
	require.NoError(t, err)
	otherDTO := dto.UsageLogFromService(otherRead)
	require.Equal(t, "anthropic-original", otherRead.RequestedModel)
	require.NotNil(t, otherDTO.AccountID)
	require.Equal(t, other.ID, *otherDTO.AccountID)
	require.Equal(t, "anthropic-original", otherDTO.Model)
	require.NotNil(t, otherDTO.ReasoningEffort)
	require.Equal(t, actual, *otherDTO.ReasoningEffort)
	// Raw list readers also skip association hydration.
	listed, _, err := r.ListByUser(ctx, u.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, listed, 2)
	for i := range listed {
		row := dto.UsageLogFromService(&listed[i])
		if listed[i].AccountID == other.ID {
			require.Equal(t, service.PlatformAnthropic, listed[i].RoutingPrivacyPlatform())
			require.NotNil(t, row.AccountID)
		} else {
			require.Equal(t, service.PlatformOpenAI, listed[i].RoutingPrivacyPlatform())
			require.Nil(t, row.AccountID)
		}
	}
	stats, err := r.GetModelStatsWithUsageFiltersBySource(ctx, otherRead.CreatedAt.Add(-time.Hour), otherRead.CreatedAt.Add(time.Hour), usagestats.UsageLogFilters{UserID: u.ID}, usagestats.ModelSourcePublicRequested)
	require.NoError(t, err)
	var names []string
	for _, stat := range stats {
		names = append(names, stat.Model)
	}
	require.ElementsMatch(t, []string{"unknown", "anthropic-original"}, names)
}
