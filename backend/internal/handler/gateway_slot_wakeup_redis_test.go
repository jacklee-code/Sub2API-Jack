//go:build unit

package handler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type wakeupWorkerKey struct{}
type observedSlotCache struct {
	service.ConcurrencyCache
	tries   atomic.Int64
	blocked chan int
}

func (c *observedSlotCache) observe(ctx context.Context, acquired bool, err error) {
	c.tries.Add(1)
	if worker, ok := ctx.Value(wakeupWorkerKey{}).(int); ok && !acquired && err == nil {
		select {
		case c.blocked <- worker:
		default:
		}
	}
}
func (c *observedSlotCache) AcquireUserSlot(ctx context.Context, id int64, max int, rid string) (bool, error) {
	ok, err := c.ConcurrencyCache.AcquireUserSlot(ctx, id, max, rid)
	c.observe(ctx, ok, err)
	return ok, err
}
func (c *observedSlotCache) AcquireAccountSlot(ctx context.Context, id int64, max int, rid string) (bool, error) {
	ok, err := c.ConcurrencyCache.AcquireAccountSlot(ctx, id, max, rid)
	c.observe(ctx, ok, err)
	return ok, err
}

func TestSlotWakeupsRedisBoundedConcurrency(t *testing.T) {
	for _, kind := range []string{"user", "account"} {
		t.Run(kind, func(t *testing.T) {
			rdb := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: rdb.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			cache := &observedSlotCache{ConcurrencyCache: repository.NewConcurrencyCache(client, 15, 900), blocked: make(chan int, 128)}
			svc := service.NewConcurrencyService(cache)
			helper := NewConcurrencyHelper(svc, SSEPingFormatNone, time.Second)
			const limit, workers = 3, 12
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var holders []func()
			for i := 0; i < limit; i++ {
				var result *service.AcquireResult
				var err error
				if kind == "user" {
					result, err = svc.AcquireUserSlot(ctx, 7, limit)
				} else {
					result, err = svc.AcquireAccountSlot(ctx, 7, limit)
				}
				require.NoError(t, err)
				require.True(t, result.Acquired)
				holders = append(holders, result.ReleaseFunc)
			}
			var wg sync.WaitGroup
			var active, peak atomic.Int64
			errs := make(chan error, workers)
			for i := 0; i < workers; i++ {
				c, _ := newHelperTestContext("POST", "/v1/responses")
				c.Request = c.Request.WithContext(context.WithValue(ctx, wakeupWorkerKey{}, i))
				wg.Add(1)
				go func() {
					defer wg.Done()
					started := false
					var release func()
					var err error
					if kind == "user" {
						release, err = helper.acquireUserSlotWithWaitTimeout(c, 7, limit, 5*time.Second, false, &started)
					} else {
						release, err = helper.AcquireAccountSlotWithWaitTimeout(c, 7, limit, 5*time.Second, false, &started)
					}
					if err != nil {
						errs <- err
						return
					}
					n := active.Add(1)
					for old := peak.Load(); n > old; old = peak.Load() {
						if peak.CompareAndSwap(old, n) {
							break
						}
					}
					time.Sleep(10 * time.Millisecond)
					active.Add(-1)
					release()
				}()
			}
			// Every worker has failed its recheck after registering for hints.
			seen := make(map[int]int)
			ready := 0
			for ready < workers {
				select {
				case worker := <-cache.blocked:
					seen[worker]++
					if seen[worker] == 2 {
						ready++
					}
				case <-ctx.Done():
					t.Fatal("workers did not reach wait state")
				}
			}
			require.Zero(t, active.Load())
			began := time.Now()
			for _, release := range holders {
				release()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			require.LessOrEqual(t, peak.Load(), int64(limit))
			require.Positive(t, peak.Load())
			// A burst should not fan out a Redis query to every queued request
			// on every release. Allow scheduling jitter and fallback polling.
			require.LessOrEqual(t, cache.tries.Load(), int64(workers*6+limit))
			userCount, err := cache.GetUserConcurrency(context.Background(), 7)
			require.NoError(t, err)
			require.Zero(t, userCount)
			accountCount, err := cache.GetAccountConcurrency(context.Background(), 7)
			require.NoError(t, err)
			require.Zero(t, accountCount)
			t.Logf("limit=%d requests=%d peak=%d acquire_attempts=%d drain_ms=%.3f", limit, workers, peak.Load(), cache.tries.Load(), float64(time.Since(began))/float64(time.Millisecond))
		})
	}
}
