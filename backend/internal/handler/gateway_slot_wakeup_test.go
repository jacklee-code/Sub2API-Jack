// Regression for released capacity sitting idle during concurrency backoff.
package handler

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type slotWakeupTestCache struct {
	helperConcurrencyCacheStub
	lock      sync.Mutex
	occupied  bool
	lastTry   time.Time
	ready     chan struct{}
	signalled bool
}

func (s *slotWakeupTestCache) AcquireUserSlot(ctx context.Context, id int64, max int, rid string) (bool, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.occupied {
		s.occupied = true
		return true, nil
	}
	now := time.Now()
	if !s.lastTry.IsZero() && now.Sub(s.lastTry) >= 800*time.Millisecond && !s.signalled {
		s.signalled = true
		close(s.ready)
	}
	s.lastTry = now
	return false, nil
}
func (s *slotWakeupTestCache) ReleaseUserSlot(ctx context.Context, id int64, rid string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.occupied = false
	return nil
}
func TestConcurrencySlotWakeupReleasedCapacity(t *testing.T) {
	synctest.Test(t, testConcurrencySlotWakeupReleasedCapacity)
}

func testConcurrencySlotWakeupReleasedCapacity(t *testing.T) {
	cache := &slotWakeupTestCache{ready: make(chan struct{})}
	svc := service.NewConcurrencyService(cache)
	holder, err := svc.AcquireUserSlot(context.Background(), 1001, 1)
	if err != nil || !holder.Acquired {
		t.Fatal("cannot acquire initial slot", err)
	}
	helper := NewConcurrencyHelper(svc, SSEPingFormatNone, time.Second)
	c, _ := newHelperTestContext("POST", "/v1/responses")
	done := make(chan error, 1)
	go func() {
		started := false
		release, err := helper.waitForSlotWithPingTimeout(c, "user", 1001, 1, 20*time.Second, false, &started, true)
		if release != nil {
			release()
		}
		done <- err
	}()
	select {
	case <-cache.ready:
	case <-time.After(18 * time.Second):
		holder.ReleaseFunc()
		t.Fatal("backoff did not reach measurement window")
	}
	freed := time.Now()
	holder.ReleaseFunc()
	err = <-done
	if err != nil {
		t.Fatal(err)
	}
	idle := time.Since(freed)
	t.Logf("one queued request; one slot free; idle_capacity_ms=%.3f", float64(idle)/float64(time.Millisecond))
	if idle > 250*time.Millisecond {
		t.Errorf("queued request left free capacity unused for %v (target <=250ms)", idle)
	}
}

func TestConcurrencySlotWakeupReleaseBeforeSubscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &slotWakeupTestCache{ready: make(chan struct{})}
		cache.waitAllowed = true
		svc := service.NewConcurrencyService(cache)
		holder, err := svc.AcquireUserSlot(context.Background(), 1001, 1)
		if err != nil || !holder.Acquired {
			t.Fatal("initial slot", err)
		}
		// Release between the caller's first failure and subscribing. There
		// are no subscribers yet, so only the post-subscribe recheck finds it.
		cache.waitIncrementHook = holder.ReleaseFunc
		helper := NewConcurrencyHelper(svc, SSEPingFormatNone, time.Second)
		c, _ := newHelperTestContext("POST", "/v1/responses")
		started := false
		before := time.Now()
		release, err := helper.AcquireUserSlotWithWait(c, 1001, 1, false, &started)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if elapsed := time.Since(before); elapsed != 0 {
			t.Fatalf("missed available capacity for %v", elapsed)
		}
	})
}

func TestConcurrencySlotWakeupOtherProcessPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &slotWakeupTestCache{ready: make(chan struct{})}
		local := service.NewConcurrencyService(cache)
		otherProcess := service.NewConcurrencyService(cache)
		holder, err := otherProcess.AcquireUserSlot(context.Background(), 1001, 1)
		if err != nil || !holder.Acquired {
			t.Fatal("initial slot", err)
		}
		helper := NewConcurrencyHelper(local, SSEPingFormatNone, time.Second)
		c, _ := newHelperTestContext("POST", "/v1/responses")
		done := make(chan error, 1)
		go func() {
			started := false
			release, err := helper.waitForSlotWithPingTimeout(c, "user", 1001, 1, 3*time.Second, false, &started, true)
			if release != nil {
				release()
			}
			done <- err
		}()
		synctest.Wait()
		// This service instance has no local subscribers. The waiter's
		// original timer must still discover the shared cache's free slot.
		holder.ReleaseFunc()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
