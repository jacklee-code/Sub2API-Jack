//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlotWakeupsReleaseScopesAndErrors(t *testing.T) {
	for _, account := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			cache := &stubConcurrencyCacheForTest{acquireResult: true}
			if fail {
				cache.releaseErr = errors.New("release failed")
			}
			s := NewConcurrencyService(cache)
			user, stopUser := s.WatchUserSlotRelease(7)
			otherUser, stopOtherUser := s.WatchUserSlotRelease(8)
			acc, stopAcc := s.WatchAccountSlotRelease(7)
			otherAcc, stopOtherAcc := s.WatchAccountSlotRelease(8)
			var result *AcquireResult
			var err error
			if account {
				result, err = s.AcquireAccountSlot(context.Background(), 7, 3)
			} else {
				result, err = s.AcquireUserSlot(context.Background(), 7, 3)
			}
			require.NoError(t, err)
			result.ReleaseFunc()
			for ch, expected := range map[<-chan struct{}]bool{user: !account && !fail, acc: account && !fail, otherUser: false, otherAcc: false} {
				select {
				case <-ch:
					require.True(t, expected, "unexpected wakeup")
				default:
					require.False(t, expected, "missing wakeup")
				}
			}
			stopUser()
			stopOtherUser()
			stopAcc()
			stopOtherAcc()
			require.Empty(t, s.slotWakeups.waiters)
		}
	}
}

func TestSlotWakeupsCoalesceAndHandOff(t *testing.T) {
	var w concurrencySlotWakeups
	key := concurrencySlotKey{id: 7}
	a, stopA := w.watch(key)
	b, stopB := w.watch(key)
	c, stopC := w.watch(key)
	require.Equal(t, a, b)
	require.Equal(t, b, c)
	for i := 0; i < 100; i++ {
		w.released(key)
	}
	<-a
	select {
	case <-b:
		t.Fatal("broadcast or unbounded notification backlog")
	default:
	}
	// A consumed the hint but exits (success, cancellation or error). B must
	// get a chance even if several releases had coalesced into the one hint.
	stopA()
	stopA()
	select {
	case <-b:
	default:
		t.Fatal("lost handoff after waiter exit")
	}
	stopB()
	select {
	case <-c:
	default:
		t.Fatal("lost coalesced capacity")
	}
	stopC()
	require.Empty(t, w.waiters)
	w.released(key)
	d, stopD := w.watch(key)
	select {
	case <-d:
		t.Fatal("stale hint survived last waiter")
	default:
	}
	stopD()
}

func TestSlotWakeupsConcurrentLifecycle(t *testing.T) {
	var w concurrencySlotWakeups
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				key := concurrencySlotKey{account: id%2 == 0, id: int64(id % 3)}
				ch, stop := w.watch(key)
				w.released(key)
				select {
				case <-ch:
				default:
				}
				stop()
				stop()
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, w.waiters)
}
