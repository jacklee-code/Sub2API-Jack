package service

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/resources"
)

// The image marks its original fallback file with a digest. Unmodified image
// defaults follow the binary's bundle; edited/mounted operator files remain live.
const jackBundledPricingPath = "resources/model-pricing/model_prices_and_context_window.json"

func isJackBundledPricingFile(path string) bool {
	if path == "" {
		return false
	}
	want, err := filepath.Abs(jackBundledPricingPath)
	if err != nil {
		return false
	}
	got, err := filepath.Abs(path)
	return err == nil && got == want
}

func readJackPricingFile(path string) ([]byte, error) {
	body, _, err := jackPricingSource(path)
	return body, err
}

func jackPricingSource(path string) ([]byte, bool, error) {
	body, err := os.ReadFile(path)
	if !isJackBundledPricingFile(path) {
		return body, false, err
	}
	if os.IsNotExist(err) {
		return []byte(resources.ModelPricing()), true, nil
	}
	if err != nil {
		return nil, false, err
	}
	digest, digestErr := os.ReadFile(path + ".jack-sha256")
	if string(body) == resources.ModelPricing() || (digestErr == nil && strings.TrimSpace(string(digest)) == fmt.Sprintf("%x", sha256.Sum256(body))) {
		return []byte(resources.ModelPricing()), true, nil
	}
	return body, false, nil
}

// A fallback-only startup deliberately creates no remote cache. Rebuild from the
// bundled primary layer so the current binary's defaults and custom overrides can
// still be hot-reloaded while the remote catalog is unavailable.
func (s *PricingService) readJackPricingBase(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if fallback, bundled, sourceErr := jackPricingSource(s.cfg.Pricing.FallbackFile); sourceErr == nil && bundled {
			return fallback, nil
		}
	}
	return body, err
}
