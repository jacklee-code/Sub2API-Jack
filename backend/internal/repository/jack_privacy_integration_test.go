//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
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
	a := mustCreateAccount(t, client, &service.Account{Name: "privacy-upstream"})
	r := newUsageLogRepositoryWithSQL(client, tx)
	requested, actual, mapped := "xhigh", "low", "private-model"
	entry := &service.UsageLog{UserID: u.ID, APIKeyID: k.ID, AccountID: a.ID, RequestID: "jack-privacy-roundtrip", Model: mapped, RequestedModel: "public-model", RequestedReasoningEffort: &requested, ReasoningEffort: &actual, UpstreamModel: &mapped}
	_, err := r.Create(ctx, entry)
	require.NoError(t, err)
	read, err := r.GetByID(ctx, entry.ID)
	require.NoError(t, err)
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
}
