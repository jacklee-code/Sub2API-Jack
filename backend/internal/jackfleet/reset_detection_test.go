package jackfleet

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

func TestResetDetectionUsesRealTransitions(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	before := Observation{Identity: "one", Plan: "pro", Used: 70, At: now.Add(-48 * time.Hour), ResetAt: now.Add(3 * 24 * time.Hour)}
	after := before
	after.At = now
	after.Used = .4
	after.Source = "query"
	require.Equal(t, "early", detectReset(before, after), "idle baselines remain useful")
	c := ResetCandidate{Before: before, First: after, Kind: "early"}
	require.False(t, confirmsReset(c, after))
	after.At = now.Add(time.Millisecond)
	require.True(t, confirmsReset(c, after))
	after.Used = 20
	require.False(t, confirmsReset(c, after))
	after.Identity = "different"
	after.Used = 0
	require.Empty(t, detectReset(before, after))
	after = before
	after.At = now
	after.Plan = "plus"
	after.Used = 0
	require.Empty(t, detectReset(before, after))
	after = before
	after.At = now
	after.ResetAt = before.ResetAt.Add(time.Hour)
	require.Empty(t, detectReset(before, after))
	before.Used = 0
	after = before
	after.At = now
	require.Empty(t, detectReset(before, after))
	before.ResetAt = now.Add(-time.Minute)
	after.ResetAt = now.Add(7 * 24 * time.Hour)
	require.Equal(t, "scheduled", detectReset(before, after))
	c = ResetCandidate{Before: before, First: after, Kind: "scheduled"}
	after.At = now.Add(time.Millisecond)
	after.Source = "query"
	after.Used = 15
	require.True(t, confirmsReset(c, after))
	after.ResetAt = after.ResetAt.Add(time.Hour)
	require.False(t, confirmsReset(c, after))
}

func TestResetConfirmationRequiresMatchingFreshQuery(t *testing.T) {
	now := time.Now()
	before := Observation{Identity: "one", Plan: "pro", Used: 80, At: now.Add(-time.Minute), ResetAt: now.Add(3 * 24 * time.Hour)}
	first := before
	first.At, first.Used, first.Source = now, 0, "request"
	c := ResetCandidate{Before: before, First: first, Kind: "early"}
	for _, tc := range []struct {
		name   string
		change func(*Observation)
		want   bool
	}{
		{"zero immediately", func(o *Observation) {}, true},
		{"small new usage", func(o *Observation) { o.Used = 1.5 }, true},
		{"two point boundary", func(o *Observation) { o.Used = 2 }, true},
		{"larger change", func(o *Observation) { o.Used = 2.01 }, false},
		{"old high usage", func(o *Observation) { o.Used = 80 }, false},
		{"request headers only", func(o *Observation) { o.Source = "request" }, false},
		{"same observation", func(o *Observation) { o.At = first.At }, false},
		{"older query", func(o *Observation) { o.At = first.At.Add(-time.Second) }, false},
		{"different identity", func(o *Observation) { o.Identity = "two" }, false},
		{"different plan", func(o *Observation) { o.Plan = "plus" }, false},
		{"early reset may move deadline", func(o *Observation) { o.ResetAt = first.ResetAt.Add(time.Hour) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := first
			o.At, o.Source = now.Add(time.Millisecond), "query"
			tc.change(&o)
			require.Equal(t, tc.want, confirmsReset(c, o))
		})
	}
}
func TestResetObservationValidity(t *testing.T) {
	now := time.Now()
	o := Observation{Identity: "one", Used: 0, At: now, ResetAt: now.Add(time.Hour)}
	require.True(t, o.Valid(now))
	for _, used := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		bad := o
		bad.Used = used
		require.False(t, bad.Valid(now))
	}
	o.ResetAt = now
	require.False(t, o.Valid(now))
	o.ResetAt = now.Add(time.Hour)
	o.At = now.Add(-3 * time.Minute)
	require.False(t, o.Valid(now))
}
