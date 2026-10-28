//go:build unit

package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const jackDefaultPricingPath = "./resources/model-pricing/model_prices_and_context_window.json"

func TestJackPricingUsesBinaryBundleWithStaleOrMissingImageResources(t *testing.T) {
	// Read the source catalog before moving into an isolated old container root.
	source, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{FallbackFile: jackDefaultPricingPath, DataDir: "data"}}, nil)
	expected, err := svc.parsePricingData(source)
	require.NoError(t, err)
	require.NotEmpty(t, expected)
	require.NoError(t, os.MkdirAll(filepath.Dir(jackDefaultPricingPath), 0755))
	require.NoError(t, os.MkdirAll("data", 0755))
	// A stale image copy must never outrank the catalog belonging to the new binary.
	require.NoError(t, os.WriteFile(jackDefaultPricingPath, []byte(`{"stale-image-only":{"input_cost_per_token":0.01,"output_cost_per_token":0.02}}`), 0644))
	image, err := os.ReadFile(jackDefaultPricingPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(jackDefaultPricingPath+".jack-sha256", []byte(fmt.Sprintf("%x", sha256.Sum256(image))), 0644))
	for _, imageExists := range []bool{true, false} {
		if !imageExists {
			require.NoError(t, os.Remove(jackDefaultPricingPath))
		}
		actual := svc.mergeFallbackPricingData(nil)
		require.Equal(t, expected, actual)
		require.NoError(t, svc.useFallbackPricing())
		require.Equal(t, expected, svc.pricingData)
		// Bundled defaults must not become a persistent remote cache that overrides
		// the next binary's defaults after an upgrade or rollback.
		_, err = os.Stat(svc.getPricingFilePath())
		require.True(t, os.IsNotExist(err))
	}
}

func TestJackPricingPreservesCustomFilesAndRemotePrecedence(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{Pricing: config.PricingConfig{FallbackFile: "custom.json", OverrideFile: "override.json", DataDir: "data"}}
	svc := NewPricingService(cfg, nil)
	require.NoError(t, os.WriteFile("custom.json", []byte(`{"custom-only":{"input_cost_per_token":0.01,"output_cost_per_token":0.02}}`), 0644))
	require.NoError(t, os.WriteFile("override.json", []byte(`{"override-only":{"input_cost_per_token":0.03,"output_cost_per_token":0.04}}`), 0644))
	actual, _, err := svc.buildPricingData([]byte(`{"remote":{"input_cost_per_token":0.05,"output_cost_per_token":0.06}}`))
	require.NoError(t, err)
	require.Len(t, actual, 3)
	require.Equal(t, 0.01, actual["custom-only"].InputCostPerToken)
	require.Equal(t, 0.03, actual["override-only"].InputCostPerToken)
	cfg.Pricing.FallbackFile = jackDefaultPricingPath
	bundled := svc.mergeFallbackPricingData(nil)
	require.NotEmpty(t, bundled)
	var model string
	for name := range bundled {
		model = name
		break
	}
	remote, err := json.Marshal(map[string]any{model: map[string]float64{"input_cost_per_token": 123, "output_cost_per_token": 456}})
	require.NoError(t, err)
	actual, _, err = svc.buildPricingData(remote)
	require.NoError(t, err)
	require.Equal(t, 123.0, actual[model].InputCostPerToken)
	// Offline fallback still supports hot reloading the user's override layer.
	require.NoError(t, os.MkdirAll("data", 0755))
	require.NoError(t, svc.useFallbackPricing())
	require.NoError(t, os.WriteFile("override.json", []byte(`{"override-only":{"input_cost_per_token":0.07,"output_cost_per_token":0.08}}`), 0644))
	require.NoError(t, svc.reloadCustomPricingLayers())
	require.Equal(t, 0.07, svc.pricingData["override-only"].InputCostPerToken)
}

func TestJackPricingPreservesEditedVendorPath(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Dir(jackDefaultPricingPath), 0755))
	require.NoError(t, os.WriteFile(jackDefaultPricingPath+".jack-sha256", []byte("digest-of-original-image-data"), 0644))
	edited := []byte(`{"operator-model":{"input_cost_per_token":0.01,"output_cost_per_token":0.02}}`)
	require.NoError(t, os.WriteFile(jackDefaultPricingPath, edited, 0644))
	for _, markerExists := range []bool{true, false} {
		if !markerExists {
			require.NoError(t, os.Remove(jackDefaultPricingPath+".jack-sha256"))
		}
		body, bundled, err := jackPricingSource(jackDefaultPricingPath)
		require.NoError(t, err)
		require.False(t, bundled)
		require.Equal(t, edited, body)
	}
}

func TestJackPricingEditedVendorFileHotReloadsAfterOfflineStartup(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Dir(jackDefaultPricingPath), 0755))
	require.NoError(t, os.MkdirAll("data", 0755))
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{FallbackFile: jackDefaultPricingPath, DataDir: "data"}}, nil)
	// Begin without an image resource or remote cache, using bundled defaults.
	require.NoError(t, svc.useFallbackPricing())
	require.NoError(t, os.WriteFile(jackDefaultPricingPath, []byte(`{"operator-model":{"input_cost_per_token":0.09,"output_cost_per_token":0.1}}`), 0644))
	require.NoError(t, svc.reloadCustomPricingLayers())
	require.Len(t, svc.pricingData, 1)
	require.Equal(t, 0.09, svc.pricingData["operator-model"].InputCostPerToken)
}
