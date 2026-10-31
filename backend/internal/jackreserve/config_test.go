package jackreserve

import "testing"

func TestConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra map[string]any
		valid bool
	}{
		{"absent", nil, true},
		{"ordered", map[string]any{EnabledKey: true, AccountsKey: []any{float64(9), float64(3)}}, true},
		{"empty", map[string]any{EnabledKey: true}, false},
		{"duplicate", map[string]any{EnabledKey: true, AccountsKey: []int64{2, 2}}, false},
		{"negative", map[string]any{AccountsKey: []int64{-1}}, false},
		{"fraction", map[string]any{AccountsKey: []float64{1.5}}, false},
		{"string", map[string]any{AccountsKey: []string{"2"}}, false},
		{"boolean", map[string]any{EnabledKey: "true"}, false},
		{"unsafe", map[string]any{AccountsKey: []int64{9007199254740992}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse(tc.extra)
			if (err == nil) != tc.valid {
				t.Fatalf("config=%+v err=%v", c, err)
			}
			if tc.name == "ordered" && (c.AccountIDs[0] != 9 || c.AccountIDs[1] != 3) {
				t.Fatal("order changed")
			}
		})
	}
}

func TestImportDropsLocalAccountReferences(t *testing.T) {
	source := map[string]any{EnabledKey: true, AccountsKey: []int64{3}, "quota_limit": 100.0}
	copy := WithoutConfig(source)
	if HasConfig(copy) || copy["quota_limit"] != 100.0 || !HasConfig(source) {
		t.Fatal("import must preserve other settings and leave source untouched")
	}
}
