package service

import (
	"context"
	"sync"
	"time"
)

// MemoryTokenStore is an in-memory TokenStore: Put records an access
// token with its expiry, Del drops it. Expired entries are lazily
// swept on Put, so the map only ever holds live tokens.
//
// It is the default store wired into NewAuthService when the caller
// supplies no WithTokenStore. In-memory revocation is per-process: in
// a multi-instance deployment a logout only revokes the token on the
// instance that served it — plug a shared store via WithTokenStore
// when that matters.
type MemoryTokenStore struct {
	mu     sync.Mutex
	now    func() time.Time
	tokens map[string]time.Time
}

// NewMemoryTokenStore builds an empty in-memory TokenStore.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{
		now:    time.Now,
		tokens: make(map[string]time.Time),
	}
}

// Put records token with an expiry of now + ttl seconds (ttl is
// SECONDS, matching AuthService.createToken). Entries whose expiry
// has already passed are pruned first.
func (s *MemoryTokenStore) Put(_ context.Context, token string, ttl int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for t, exp := range s.tokens {
		if !exp.After(now) {
			delete(s.tokens, t)
		}
	}
	s.tokens[token] = now.Add(time.Duration(ttl) * time.Second)
	return nil
}

// Del removes token from the store. Deleting an unknown token is a
// no-op so logout stays idempotent.
func (s *MemoryTokenStore) Del(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}
