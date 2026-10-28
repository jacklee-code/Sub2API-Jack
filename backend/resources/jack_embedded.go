// Package resources contains application data that travels with the Jack binary.
package resources

import _ "embed"

//go:embed model-pricing/model_prices_and_context_window.json
var modelPricing string

// ModelPricing returns bundled fallback data. Keeping it in the executable makes
// checksum verification, atomic replacement, backup and rollback cover both.
func ModelPricing() string { return modelPricing }
