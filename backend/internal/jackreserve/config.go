// Package jackreserve owns the ordered, single-level reserve-energy plan.
package jackreserve

import (
	"encoding/json"
	"fmt"
)

const EnabledKey = "jack_reserve_enabled"
const AccountsKey = "jack_reserve_account_ids"
const MaxAccounts = 16

type Config struct {
	Enabled    bool
	AccountIDs []int64
}

func HasConfig(extra map[string]any) bool {
	_, enabled := extra[EnabledKey]
	_, accounts := extra[AccountsKey]
	return enabled || accounts
}

// WithoutConfig drops installation-local account references from data imports.
// The source map belongs to the uploaded document and is never modified.
func WithoutConfig(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	copy := make(map[string]any, len(extra))
	for key, value := range extra {
		if key != EnabledKey && key != AccountsKey {
			copy[key] = value
		}
	}
	return copy
}

func Parse(extra map[string]any) (Config, error) {
	var c Config
	if raw, ok := extra[EnabledKey]; ok && raw != nil {
		var valid bool
		c.Enabled, valid = raw.(bool)
		if !valid {
			return c, fmt.Errorf("reserve enabled must be a boolean")
		}
	}
	if raw, ok := extra[AccountsKey]; ok && raw != nil {
		data, err := json.Marshal(raw)
		if err != nil {
			return c, err
		}
		if err := json.Unmarshal(data, &c.AccountIDs); err != nil {
			return c, fmt.Errorf("reserve accounts must be integer account IDs")
		}
	}
	if len(c.AccountIDs) > MaxAccounts {
		return c, fmt.Errorf("at most %d reserve accounts are allowed", MaxAccounts)
	}
	seen := make(map[int64]bool, len(c.AccountIDs))
	for _, id := range c.AccountIDs {
		if id <= 0 || id > 9007199254740991 || seen[id] {
			return c, fmt.Errorf("reserve account IDs must be positive, unique safe integers")
		}
		seen[id] = true
	}
	if c.Enabled && len(c.AccountIDs) == 0 {
		return c, fmt.Errorf("select at least one reserve account")
	}
	return c, nil
}
