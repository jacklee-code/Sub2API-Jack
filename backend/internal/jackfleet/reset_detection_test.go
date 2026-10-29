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
	require.Equal(t, "early", detectReset(before, after), "idle baselines remain useful")
	c := ResetCandidate{Before: before, First: after, Kind: "early"}
	require.False(t, confirmsReset(c, after))
	after.At = now.Add(time.Minute)
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
	after.At = now.Add(time.Minute)
	after.Used = 15
	require.True(t, confirmsReset(c, after))
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
