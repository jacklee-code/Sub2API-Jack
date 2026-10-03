//go:build unit

package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type jackReleaseClient struct {
	updateServiceGitHubClientStub
	manifest JackReleaseManifest
}

func (c *jackReleaseClient) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return json.Marshal(c.manifest)
}
func TestJackVersionOrder(t *testing.T) {
	for _, p := range [][2]string{{"0.2.13-jack.1", "0.2.13-jack.2"}, {"0.2.13-jack.9", "0.2.13-jack.10"}, {"0.2.13-jack.99", "0.2.14-jack.1"}, {"0.2.13", "0.2.13-jack.1"}} {
		require.Less(t, compareVersions(p[0], p[1]), 0)
		require.Greater(t, compareVersions(p[1], p[0]), 0)
	}
	require.Zero(t, compareVersions("0.2.13-jack.1", "v0.2.13-jack.1"))
}
func TestJackAssetOwnership(t *testing.T) {
	require.NoError(t, validateJackAsset("https://github.com/jacklee-code/Sub2API-Jack/releases/download/v0.2.13-jack.1/checksums.txt"))
	for _, url := range []string{"https://github.com/Wei-Shaw/sub2api/releases/download/v0.2.13/checksums.txt", "https://github.com.evil.test/jacklee-code/Sub2API-Jack/releases/download/v1/file", "http://github.com/jacklee-code/Sub2API-Jack/releases/download/v1/file", "https://github.com/jacklee-code/Sub2API-Jack/releases/download/v1/../../other", "https://github.com/jacklee-code/Sub2API-Jack/releases/download/v1/file?redirect=1"} {
		require.Error(t, validateJackAsset(url))
	}
}
func TestJackRuntimeGateAndRollbackBoundary(t *testing.T) {
	old := JackRuntimeID
	JackRuntimeID = "runtime-one"
	t.Cleanup(func() { JackRuntimeID = old })
	asset := GitHubAsset{Name: "jack-release.json", BrowserDownloadURL: "https://github.com/jacklee-code/Sub2API-Jack/releases/download/v0.2.13-jack.1/jack-release.json"}
	client := &jackReleaseClient{manifest: JackReleaseManifest{Format: 1, Version: "0.2.13-jack.1", UpstreamVersion: "0.2.13", RuntimeID: "runtime-one", SchemaEpoch: 1}}
	client.recentReleases = []*GitHubRelease{{TagName: "v0.2.13-jack.1", Assets: []GitHubAsset{asset}}, {TagName: "v0.2.12-jack.8", Assets: []GitHubAsset{asset}}, {TagName: "v0.2.13", Assets: []GitHubAsset{asset}}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.2.13-jack.2", "release")
	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "0.2.13-jack.1", versions[0].Version)
	client.manifest.RuntimeID = "new-runtime"
	versions, err = svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Empty(t, versions)
	require.Error(t, svc.Rollback())
}

func TestFleetSubscriptionDisplayNormalizesWithoutChangingHistory(t *testing.T) {
	now := time.Now()
	old := now.AddDate(0, -2, 0)
	sub := UserSubscription{StartsAt: old, ExpiresAt: now.AddDate(0, 0, 30), Status: SubscriptionStatusActive, DailyWindowStart: &old, WeeklyWindowStart: &old, MonthlyWindowStart: &old, DailyUsageUSD: 10, WeeklyUsageUSD: 20, MonthlyUsageUSD: 30}
	view := NormalizeFleetSubscriptionView(sub)
	require.Zero(t, view.DailyUsageUSD)
	require.Zero(t, view.WeeklyUsageUSD)
	require.Zero(t, view.MonthlyUsageUSD)
	require.Equal(t, 10.0, sub.DailyUsageUSD)
	require.Equal(t, 20.0, sub.WeeklyUsageUSD)
	require.Equal(t, 30.0, sub.MonthlyUsageUSD)
}
