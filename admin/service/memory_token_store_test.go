package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sjlit/aeus/admin/pb"
)

// TestNewAuthService_DefaultsToMemoryTokenStore pins the default
// wiring: without WithTokenStore, AuthService must fall back to the
// in-memory store instead of nil (nil used to mean "no revocation").
func TestNewAuthService_DefaultsToMemoryTokenStore(t *testing.T) {
	svc := newAuthSvcForTest(t)
	store, ok := svc.opts.TokenStore.(*MemoryTokenStore)
	if !ok {
		t.Fatalf("default TokenStore is %T, want *MemoryTokenStore", svc.opts.TokenStore)
	}
	if store.tokens == nil {
		t.Fatal("default store has nil token map")
	}
}

// TestMemoryTokenStore_PutAndDel covers record / remove / idempotent
// remove: revoking an unknown token must not error (logout twice is
// not a failure).
func TestMemoryTokenStore_PutAndDel(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()

	if err := store.Put(ctx, "tok-a", 3600); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := store.tokens["tok-a"]; !ok {
		t.Fatal("Put did not record the token")
	}
	if err := store.Del(ctx, "tok-a"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	if _, ok := store.tokens["tok-a"]; ok {
		t.Fatal("Del did not remove the token")
	}
	if err := store.Del(ctx, "tok-a"); err != nil {
		t.Fatalf("second Del: %v", err)
	}
}

// TestMemoryTokenStore_PutRecordsExpiry pins the ttl unit: ttl is
// SECONDS, expiry is now + ttl seconds.
func TestMemoryTokenStore_PutRecordsExpiry(t *testing.T) {
	store := NewMemoryTokenStore()
	base := time.Now()
	store.now = func() time.Time { return base }

	if err := store.Put(context.Background(), "tok-a", 7200); err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := base.Add(7200 * time.Second)
	if got := store.tokens["tok-a"]; !got.Equal(want) {
		t.Fatalf("expiry = %v, want %v", got, want)
	}
}

// TestMemoryTokenStore_PutPrunesExpiredTokens covers the lazy sweep:
// every Put first drops entries whose expiry has passed, so the map
// only ever holds live tokens.
func TestMemoryTokenStore_PutPrunesExpiredTokens(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	base := time.Now()
	store.now = func() time.Time { return base }

	if err := store.Put(ctx, "expired", -1); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Advance past the expired entry's expiry; the next Put sweeps it.
	store.now = func() time.Time { return base.Add(2 * time.Second) }
	if err := store.Put(ctx, "fresh", 3600); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := store.tokens["expired"]; ok {
		t.Fatal("expired token was not pruned by Put")
	}
	if _, ok := store.tokens["fresh"]; !ok {
		t.Fatal("unexpired token was pruned")
	}
}

// TestMemoryTokenStore_ConcurrentPutDel exercises the store under
// parallel writers; run with -race to verify the mutex actually
// guards the map.
func TestMemoryTokenStore_ConcurrentPutDel(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := store.Put(ctx, "t", 3600); err != nil {
				t.Errorf("Put: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := store.Del(ctx, "t"); err != nil {
				t.Errorf("Del: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestLoginStoresTokenAndLogoutRevokes walks the end-to-end path:
// Login must Put the issued access token into the default store, and
// Logout must Del it — no explicit WithTokenStore in between.
func TestLoginStoresTokenAndLogoutRevokes(t *testing.T) {
	db := newLoginDB(t)
	svc := loginAsAdmin(t, db)
	ctx := context.Background()

	res, err := svc.Login(ctx, &pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	store := svc.opts.TokenStore.(*MemoryTokenStore)
	if _, ok := store.tokens[res.AccessToken]; !ok {
		t.Fatal("Login did not Put the access token in the default store")
	}

	if _, err := svc.Logout(ctx, &pb.LogoutRequest{AccessToken: res.AccessToken}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, ok := store.tokens[res.AccessToken]; ok {
		t.Fatal("Logout did not revoke the access token")
	}
}
