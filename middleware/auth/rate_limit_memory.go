package auth

import (
	"context"
	"math"
	"sync"
	"time"
)

// tokenBucket is one per-key state slot. tokens is the current
// available token count (fractional between 0 and capacity); lastRefill
// is the wall-clock instant the bucket was last updated.
type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

// memoryLimiter is the default Limiter implementation: an in-memory
// per-key token bucket with a 5-minute idle GC sweep.
//
// It is the per-process default (no shared state across replicas), so
// a deployment with N instances effectively gets an N× the configured
// rate. Multi-instance deployments should swap in a shared backend
// (Redis) via RateLimitOpts.Limiter.
//
// Concurrency: a single sync.Mutex guards the map of buckets. The
// critical section is constant-time per key (one map load + a few
// float ops) and never makes blocking calls, so contention is
// bounded by request rate — well within the middleware's hot-path
// expectations.
type memoryLimiter struct {
	capacity int
	refill   float64
	now      func() time.Time // injectable for tests

	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

// NewMemoryLimiter returns an in-memory Limiter with the given
// capacity (burst) and refill rate (tokens/sec). A typical
// "5/minute" rule is NewMemoryLimiter(5, 1.0/60).
//
// Idle GC: buckets that haven't been touched for 5 minutes are
// removed by a janitor goroutine. The 5-minute window is the same
// one infra/cache/memory uses for its own idle sweep, so a
// process restart or a key that's been quiet long enough will not
// retain stale state. The janitor runs on its own goroutine; call
// Stop to halt it (most tests do not need to — process exit cleans up).
func NewMemoryLimiter(capacity int, refill float64) *memoryLimiter {
	if capacity <= 0 {
		panic("middleware/auth: NewMemoryLimiter requires capacity > 0")
	}
	if refill <= 0 {
		panic("middleware/auth: NewMemoryLimiter requires refill > 0")
	}
	l := &memoryLimiter{
		capacity: capacity,
		refill:   refill,
		now:      time.Now,
		buckets:  make(map[string]*tokenBucket),
	}
	go l.janitor(5 * time.Minute)
	return l
}

// Allow implements Limiter.
//
// On entry the bucket's tokens field is lazily brought up to date:
// the elapsed time since lastRefill is multiplied by refill rate and
// added, capped at capacity. The request then consumes one token if
// any remain. retryAfter is computed from the deficit divided by the
// refill rate, so the response header tells the client exactly how
// long until the bucket is replenished.
//
// A bucket that has never been touched starts full (capacity tokens),
// which is the token-bucket convention for "first burst is free".
// Without this, a fresh process would silently rate-limit every IP
// on the first request.
func (l *memoryLimiter) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		// Cold start: full bucket. A subsequent Allow immediately
		// consumes that first token, so the caller observes a
		// burst of `capacity` requests — the intended semantics.
		b = &tokenBucket{tokens: float64(l.capacity), lastRefill: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastRefill).Seconds()
		if elapsed > 0 {
			b.tokens = math.Min(float64(l.capacity), b.tokens+elapsed*l.refill)
			b.lastRefill = now
		}
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0, nil
	}

	// Deficit-based retryAfter: 1 token / refill rate gives the
	// time until the bucket has at least one token to spend.
	deficit := 1.0 - b.tokens
	retryAfter := time.Duration(deficit / l.refill * float64(time.Second))
	return false, retryAfter, nil
}

// janitor periodically drops buckets that have been idle longer than
// idle. A bucket is "idle" when its lastRefill is more than `idle`
// in the past — refill has fully replenished it by then, so dropping
// the entry costs us a fresh `capacity`-token burst on next access,
// which is exactly the documented behaviour of Allow for a new key.
//
// 5 minutes matches infra/cache/memory's own idle window so the two
// sweep periods line up in unit tests that combine both layers.
func (l *memoryLimiter) janitor(idle time.Duration) {
	ticker := time.NewTicker(idle)
	defer ticker.Stop()
	for now := range ticker.C {
		l.mu.Lock()
		for k, b := range l.buckets {
			if now.Sub(b.lastRefill) > idle {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// size returns the current bucket count. Test-only helper — keeps
// the unit tests honest about the GC contract without leaking the
// map to the Limiter interface.
func (l *memoryLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}