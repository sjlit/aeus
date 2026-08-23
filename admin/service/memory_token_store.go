package service

import (
	"context"
	"sync"
	"time"
)

// __revocationCapacity bounds how many revoked-jti entries the default
// in-memory store will hold.  Entries are denylist records (one logout
// = one row), so the working set is bounded by revocation rate × TTL,
// not by session count — a few thousand is already generous.  When the
// cap is hit the soonest-expiring entries are evicted first, degrading
// revocation memory gracefully instead of growing without bound.
const __revocationCapacity = 65536

// MemoryTokenStore is the default in-memory TokenStore: a DENYLIST of
// revoked JWT ids.  Unlike the previous whitelist design (which stored
// every issued access token), nothing is written on login — only an
// explicit logout inserts an entry — so the map stays small and the
// hot login path touches no shared state.
//
// Expired entries are lazily swept on write.  In-memory revocation is
// per-process: multi-instance deployments must plug a shared store via
// WithTokenStore (Redis etc.) for logout to take effect everywhere.
type MemoryTokenStore struct {
	mu       sync.Mutex
	now      func() time.Time
	capacity int
	revoked  map[string]time.Time // jti -> revoke holds until this instant
}

// NewMemoryTokenStore builds an empty in-memory denylist store.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{
		now:      time.Now,
		capacity: __revocationCapacity,
		revoked:  make(map[string]time.Time),
	}
}

// Revoke marks jti as unusable until (inclusive).  A jti whose window
// has already passed is ignored — its token cannot authenticate anyone
// anymore.  Idempotent: re-revoking overwrites the window.
func (s *MemoryTokenStore) Revoke(_ context.Context, jti string, until time.Time) error {
	if jti == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !until.After(now) {
		return nil
	}
	s.sweepLocked(now)
	if _, ok := s.revoked[jti]; !ok && len(s.revoked) >= s.capacity {
		s.evictOneLocked()
	}
	s.revoked[jti] = until
	return nil
}

// RevokeIfNotRevoked atomically claims jti for revocation: it inserts
// the denylist entry and reports true only when THIS call was first —
// an already-revoked (and unexpired) jti is left untouched and reports
// false.  RefreshToken uses it as a replay fence: two concurrent
// refreshes of the same token serialize, exactly one wins.
func (s *MemoryTokenStore) RevokeIfNotRevoked(_ context.Context, jti string, until time.Time) (bool, error) {
	if jti == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !until.After(now) {
		return false, nil
	}
	if existing, ok := s.revoked[jti]; ok && existing.After(now) {
		return false, nil
	}
	s.sweepLocked(now)
	if _, ok := s.revoked[jti]; !ok && len(s.revoked) >= s.capacity {
		s.evictOneLocked()
	}
	s.revoked[jti] = until
	return true, nil
}

// Revoked reports whether jti currently sits on the denylist.  An
// entry whose window has passed reads as not-revoked (the token is
// dead anyway) and is dropped opportunistically.
func (s *MemoryTokenStore) Revoked(_ context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.revoked[jti]
	if !ok {
		return false, nil
	}
	if !until.After(s.now()) {
		delete(s.revoked, jti)
		return false, nil
	}
	return true, nil
}

// sweepLocked drops every expired entry.  Caller must hold mu.
func (s *MemoryTokenStore) sweepLocked(now time.Time) {
	for jti, until := range s.revoked {
		if !until.After(now) {
			delete(s.revoked, jti)
		}
	}
}

// evictOneLocked drops the soonest-expiring entry when the store is at
// capacity — the entry closest to being irrelevant anyway.  Caller
// must hold mu and guarantee len(revoked) > 0.
func (s *MemoryTokenStore) evictOneLocked() {
	var (
		victim    string
		victimTil time.Time
		first     = true
	)
	for jti, until := range s.revoked {
		if first || until.Before(victimTil) {
			victim, victimTil, first = jti, until, false
		}
	}
	if !first {
		delete(s.revoked, victim)
	}
}
