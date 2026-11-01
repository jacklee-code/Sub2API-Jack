package service

import "sync"

type concurrencySlotKey struct {
	account bool
	id      int64
}

type concurrencySlotWaiters struct {
	wake chan struct{}
	refs int
}

// concurrencySlotWakeups provides process-local hints, never slot ownership.
// All woken requests must still acquire through the authoritative Redis cache.
// Other processes and expired slots remain covered by the handler's polling.
type concurrencySlotWakeups struct {
	mu      sync.Mutex
	waiters map[concurrencySlotKey]*concurrencySlotWaiters
}

func signalSlotWaiter(wake chan struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (w *concurrencySlotWakeups) watch(key concurrencySlotKey) (<-chan struct{}, func()) {
	w.mu.Lock()
	if w.waiters == nil {
		w.waiters = make(map[concurrencySlotKey]*concurrencySlotWaiters)
	}
	entry := w.waiters[key]
	if entry == nil {
		entry = &concurrencySlotWaiters{wake: make(chan struct{}, 1)}
		w.waiters[key] = entry
	}
	entry.refs++
	w.mu.Unlock()
	var once sync.Once
	return entry.wake, func() {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			entry.refs--
			if entry.refs == 0 {
				delete(w.waiters, key)
				return
			}
			// Pass the hint on when a waiter succeeds, fails or is cancelled.
			// This drains multiple available slots after coalesced releases and
			// avoids losing a hint consumed by a request that then exits.
			signalSlotWaiter(entry.wake)
		})
	}
}

func (w *concurrencySlotWakeups) released(key concurrencySlotKey) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if entry := w.waiters[key]; entry != nil {
		// A shared channel wakes one waiter, not every request for this key.
		signalSlotWaiter(entry.wake)
	}
}

// WatchUserSlotRelease subscribes to local availability hints. Stop must be
// called on every exit. Subscribe before rechecking Redis to close the race
// between an unsuccessful acquisition and registering the waiter.
func (s *ConcurrencyService) WatchUserSlotRelease(userID int64) (<-chan struct{}, func()) {
	return s.slotWakeups.watch(concurrencySlotKey{id: userID})
}

// WatchAccountSlotRelease is the account-scoped counterpart of WatchUserSlotRelease.
func (s *ConcurrencyService) WatchAccountSlotRelease(accountID int64) (<-chan struct{}, func()) {
	return s.slotWakeups.watch(concurrencySlotKey{account: true, id: accountID})
}
