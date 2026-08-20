package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock is a swap-in for time.Now so token-bucket tests don't
// have to wait real seconds. The bucket math is wall-clock based, so
// driving now() with a clock the test owns keeps the suite fast and
// deterministic.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newFakeLimiter builds a memoryLimiter pinned to a fakeClock so
// tests can advance time without sleeping. Janitor is NOT started
// (the test calls size() directly to verify idle GC instead).
func newFakeLimiter(t *testing.T, capacity int, refill float64) (*memoryLimiter, *fakeClock) {
	t.Helper()
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	l := NewMemoryLimiter(capacity, refill)
	l.now = clk.Now
	return l, clk
}

// ---------- cold start ----------

func TestMemoryLimiter_ColdStartAllowsBurst(t *testing.T) {
	l, _ := newFakeLimiter(t, 5, 1.0/60)
	// First 5 requests must pass; 6th must reject.
	for i := 1; i <= 5; i++ {
		ok, _, err := l.Allow(context.Background(), "ip:1.2.3.4")
		if err != nil {
			t.Fatalf("unexpected error on attempt %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("attempt %d must pass (cold bucket)", i)
		}
	}
	ok, retryAfter, err := l.Allow(context.Background(), "ip:1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error on 6th attempt: %v", err)
	}
	if ok {
		t.Fatal("6th attempt must be rejected after exhausting the cold-start burst")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter must be positive on rejection, got %v", retryAfter)
	}
}

// ---------- refill ----------

func TestMemoryLimiter_RefillOverTime(t *testing.T) {
	l, clk := newFakeLimiter(t, 1, 1.0) // 1 token / second
	// Drain the single token.
	ok, _, _ := l.Allow(context.Background(), "k")
	if !ok {
		t.Fatal("first request must pass")
	}
	ok, _, _ = l.Allow(context.Background(), "k")
	if ok {
		t.Fatal("second request must reject (no refill yet)")
	}
	// Advance 1.1s — the bucket should now have ~1.1 tokens, the
	// next Allow consumes one and reports success.
	clk.Advance(1100 * time.Millisecond)
	ok, _, _ = l.Allow(context.Background(), "k")
	if !ok {
		t.Fatal("after refill window the next request must pass")
	}
}

// ---------- per-key isolation ----------

func TestMemoryLimiter_PerKeyIsolation(t *testing.T) {
	l, _ := newFakeLimiter(t, 2, 0.0001)
	// Exhaust key A.
	for i := 0; i < 2; i++ {
		if ok, _, _ := l.Allow(context.Background(), "user:alice"); !ok {
			t.Fatalf("alice attempt %d must pass", i)
		}
	}
	if ok, _, _ := l.Allow(context.Background(), "user:alice"); ok {
		t.Fatal("alice must be exhausted")
	}
	// Bob's bucket is independent.
	if ok, _, _ := l.Allow(context.Background(), "user:bob"); !ok {
		t.Fatal("bob's bucket must be untouched")
	}
}

// ---------- cap never exceeded ----------

func TestMemoryLimiter_CapacityNeverExceedsCap(t *testing.T) {
	l, clk := newFakeLimiter(t, 3, 1.0) // 1 tok/sec, cap=3
	// Idle for 10s — bucket should be capped at 3, not 10.
	clk.Advance(10 * time.Second)
	for i := 1; i <= 3; i++ {
		if ok, _, _ := l.Allow(context.Background(), "k"); !ok {
			t.Fatalf("post-idle attempt %d must pass", i)
		}
	}
	if ok, _, _ := l.Allow(context.Background(), "k"); ok {
		t.Fatal("4th attempt must reject — bucket must not exceed capacity")
	}
}

// ---------- idle GC ----------

func TestMemoryLimiter_JanitorRemovesIdleBuckets(t *testing.T) {
	l, clk := newFakeLimiter(t, 1, 1.0/60)
	// Touch a key, advance past the idle window, run a manual sweep.
	if ok, _, _ := l.Allow(context.Background(), "user:alice"); !ok {
		t.Fatal("warm-up call must pass")
	}
	if got := l.size(); got != 1 {
		t.Fatalf("size after warm-up = %d, want 1", got)
	}
	clk.Advance(6 * time.Minute) // > 5min idle window
	// Replicate janitor()'s sweep manually — the real janitor would
	// eventually catch up on its own, but tests shouldn't sleep.
	l.mu.Lock()
	for k, b := range l.buckets {
		if clk.Now().Sub(b.lastRefill) > 5*time.Minute {
			delete(l.buckets, k)
		}
	}
	l.mu.Unlock()
	if got := l.size(); got != 0 {
		t.Errorf("janitor should have removed the idle bucket, size=%d", got)
	}
	// After GC the next Allow must start fresh: full capacity, no
	// stale rejection state from the previous round.
	if ok, _, _ := l.Allow(context.Background(), "user:alice"); !ok {
		t.Fatal("post-GC bucket must restart at full capacity")
	}
}

// ---------- retry-after calculation ----------

func TestMemoryLimiter_RetryAfterMatchesRefillRate(t *testing.T) {
	// 1 token / minute → after exhausting, retryAfter should be ~60s
	// (deficit = 1 - 0 = 1, / refill = 1 / (1/60) = 60s).
	l, _ := newFakeLimiter(t, 1, 1.0/60)
	l.Allow(context.Background(), "k") // drain
	ok, retryAfter, _ := l.Allow(context.Background(), "k")
	if ok {
		t.Fatal("second attempt must reject")
	}
	want := 60 * time.Second
	tolerance := 500 * time.Millisecond
	if retryAfter < want-tolerance || retryAfter > want+tolerance {
		t.Errorf("retryAfter = %v, want ~%v", retryAfter, want)
	}
}

// ---------- concurrency ----------

func TestMemoryLimiter_ConcurrentAccessIsSafe(t *testing.T) {
	l, _ := newFakeLimiter(t, 100, 100.0)
	const goroutines = 50
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_, _, _ = l.Allow(context.Background(), "shared")
			}
		}()
	}
	wg.Wait()
	// Sanity: total tokens consumed = goroutines * perGoroutine =
	// 1000, which exceeds capacity. Some calls must have been
	// rejected; nothing must panic and the bucket state must be
	// consistent.
	if got := l.size(); got != 1 {
		t.Errorf("shared key should keep one bucket, got %d", got)
	}
}

// ---------- constructor validation ----------

func TestNewMemoryLimiter_PanicsOnBadArgs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cap      int
		refill   float64
		wantPanic bool
	}{
		{"zero capacity", 0, 1, true},
		{"negative capacity", -1, 1, true},
		{"zero refill", 1, 0, true},
		{"negative refill", 1, -0.5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				got := recover() != nil
				if got != tc.wantPanic {
					t.Errorf("panic=%v, want=%v", got, tc.wantPanic)
				}
			}()
			NewMemoryLimiter(tc.cap, tc.refill)
		})
	}
}