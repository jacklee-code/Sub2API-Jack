package jackfleet

import (
	"crypto/sha256"
	"fmt"
	"math"
	"time"
)

// Observation is an actual upstream sample, never a normalized display value.
// Identity is a digest, not a credential or upstream account identifier.
type Observation struct {
	Identity string    `json:"identity"`
	Plan     string    `json:"plan,omitempty"`
	Used     float64   `json:"used"`
	ResetAt  time.Time `json:"reset_at"`
	At       time.Time `json:"at"`
	Source   string    `json:"source"`
}

func (o Observation) Valid(now time.Time) bool {
	return o.Identity != "" && !math.IsNaN(o.Used) && !math.IsInf(o.Used, 0) && o.Used >= 0 && o.Used <= 100 && !o.At.IsZero() && !o.At.After(now.Add(5*time.Second)) && now.Sub(o.At) <= 2*time.Minute && o.ResetAt.After(o.At)
}
func sameIdentity(a, b Observation) bool {
	return a.Identity == b.Identity && (a.Plan == "" || b.Plan == "" || a.Plan == b.Plan)
}

type ResetCandidate struct {
	Before Observation `json:"before"`
	First  Observation `json:"first"`
	Kind   string      `json:"kind"`
}

func detectReset(before, after Observation) string {
	if !sameIdentity(before, after) || !after.At.After(before.At) {
		return ""
	}
	if !after.At.Before(before.ResetAt) && after.ResetAt.After(before.ResetAt.Add(time.Minute)) {
		return "scheduled"
	}
	if before.Used >= 5 && after.Used <= 1 {
		return "early"
	}
	return ""
}
func confirmsReset(c ResetCandidate, o Observation) bool {
	// A separate authenticated usage query confirms the observation immediately;
	// repeated request headers alone must never grant quota.
	if o.Source != "query" || !sameIdentity(c.First, o) || !o.At.After(c.First.At) {
		return false
	}
	if c.Kind == "early" {
		// Allow small amounts of new usage while the confirmation is in flight.
		return math.Abs(o.Used-c.First.Used) <= 2 && o.Used < c.Before.Used
	}
	delta := o.ResetAt.Sub(c.First.ResetAt)
	return c.Kind == "scheduled" && delta >= -time.Minute && delta <= time.Minute
}

type ResetSync struct {
	Enabled      bool            `json:"enabled"`
	AccountID    int64           `json:"account_id"`
	Generation   int64           `json:"generation"`
	ConfiguredAt time.Time       `json:"configured_at"`
	Baseline     *Observation    `json:"-"`
	Candidate    *ResetCandidate `json:"-"`
	NextResetAt  *time.Time      `json:"next_reset_at"`
	ObservedAt   *time.Time      `json:"observed_at"`
	Status       string          `json:"status"`
	ErrorCode    string          `json:"error_code,omitempty"`
	Rebaseline   bool            `json:"-"`
}
type ResetEvent struct {
	ID           int64        `json:"id"`
	AccountID    int64        `json:"account_id"`
	Source       string       `json:"source"`
	Before       *Observation `json:"before,omitempty"`
	After        *Observation `json:"after,omitempty"`
	Members      int          `json:"members"`
	CreatedAt    time.Time    `json:"created_at"`
	CachePending bool         `json:"cache_pending"`
}

// SourceIdentity excludes rotating tokens and plan metadata. Plan changes are
// checked separately for passive detection; a confirmed manual redemption still
// applies when token refresh updates the same account's plan metadata.
func SourceIdentity(id int64, account, organization, email string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", id, account, organization, email))))
}
