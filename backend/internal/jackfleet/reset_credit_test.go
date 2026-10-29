//go:build unit

package jackfleet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreditSucceeded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    string
		windows int
		want    bool
	}{
		{"upstream reset response", "reset", 1, true},
		{"multiple windows", "reset", 2, true},
		{"case insensitive", "RESET", 1, true},
		{"ok alias", "ok", 1, true},
		{"success alias", "success", 1, true},
		{"no windows reset", "reset", 0, false},
		{"negative windows", "reset", -1, false},
		{"ok without windows", "ok", 0, false},
		{"success without windows", "success", 0, false},
		{"no credit", "no_credit", 0, false},
		{"unknown code with windows", "unknown", 1, false},
		{"empty code with windows", "", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, CreditSucceeded(tc.code, tc.windows))
		})
	}
}
