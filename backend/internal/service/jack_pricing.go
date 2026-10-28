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

func isJackDefaultPricingPath(path string) bool {
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
	if !isJackDefaultPricingPath(path) {
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

// A bundled fallback-only startup creates no remote cache. Rebuild from the
// effective fallback so defaults, edits to the vendor path and custom overrides
// can still be hot-reloaded while the remote catalog is unavailable.
func (s *PricingService) readJackPricingBase(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) && isJackDefaultPricingPath(s.cfg.Pricing.FallbackFile) {
		return readJackPricingFile(s.cfg.Pricing.FallbackFile)
	}
	return body, err
}
