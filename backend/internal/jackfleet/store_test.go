package jackfleet

import (
	"testing"
	"time"
)

func TestAdjustExpiry(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.FixedZone("SG", 8*3600))
	future := now.AddDate(0, 0, 15)
	past := now.AddDate(0, 0, -15)
	for _, tc := range []struct {
		name string
		old  time.Time
		days int
		want time.Time
	}{{"active", future, 30, future.AddDate(0, 0, 30)}, {"expired", past, 30, now.AddDate(0, 0, 30)}, {"shorten", future, -30, future.AddDate(0, 0, -30)}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := adjustExpiry(tc.old, now, tc.days, nil)
			if err != nil || !got.Equal(tc.want) {
				t.Fatalf("got %v %v; want %v", got, err, tc.want)
			}
		})
	}
	if _, err := adjustExpiry(future, now, 0, nil); err == nil {
		t.Fatal("zero adjustment accepted")
	}
	if _, err := adjustExpiry(future, now, 30, &future); err == nil {
		t.Fatal("ambiguous input accepted")
	}
	if _, err := adjustExpiry(future, now, 36500, nil); err == nil {
		t.Fatal("out-of-range date accepted")
	}
}
