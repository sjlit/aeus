package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
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
	if store.revoked == nil {
		t.Fatal("default store has nil denylist map")
	}
}

// TestMemoryTokenStore_RevokeAndRevoked covers the denylist round
// trip: Revoke makes a jti read as revoked; an unknown jti does not.
func TestMemoryTokenStore_RevokeAndRevoked(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	base := time.Now()
	store.now = func() time.Time { return base }

	until := base.Add(time.Hour)
	if err := store.Revoke(ctx, "jti-a", until); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, err := store.Revoked(ctx, "jti-a")
	if err != nil {
		t.Fatalf("Revoked: %v", err)
	}
	if !got {
		t.Fatal("revoked jti reads as not-revoked")
	}
	if got, _ = store.Revoked(ctx, "jti-unknown"); got {
		t.Fatal("unknown jti reads as revoked")
	}
}

// TestMemoryTokenStore_ExpiredEntryNotRevoked: once the window passes,
// the entry must read as clean (the token is dead anyway) and be
// dropped from the map.
func TestMemoryTokenStore_ExpiredEntryNotRevoked(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	base := time.Now()
	store.now = func() time.Time { return base }

	if err := store.Revoke(ctx, "jti-a", base.Add(time.Second)); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	store.now = func() time.Time { return base.Add(2 * time.Second) }
	if got, _ := store.Revoked(ctx, "jti-a"); got {
		t.Fatal("expired entry still reads as revoked")
	}
	if len(store.revoked) != 0 {
		t.Fatalf("expired entry was not dropped: %v", store.revoked)
	}
}

// TestMemoryTokenStore_PastWindowIgnored: revoking into the past is a
// no-op, and re-revoking overwrites (idempotent logout).
func TestMemoryTokenStore_PastWindowIgnoredAndIdempotent(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	base := time.Now()
	store.now = func() time.Time { return base }

	if err := store.Revoke(ctx, "gone", base.Add(-time.Second)); err != nil {
		t.Fatalf("Revoke past: %v", err)
	}
	if len(store.revoked) != 0 {
		t.Fatal("past-window revoke inserted an entry")
	}
	until := base.Add(time.Hour)
	if err := store.Revoke(ctx, "jti-a", until); err != nil {
		t.Fatalf("Revoke 1: %v", err)
	}
	if err := store.Revoke(ctx, "jti-a", until); err != nil {
		t.Fatalf("Revoke 2: %v", err)
	}
	if len(store.revoked) != 1 {
		t.Fatalf("re-revoke duplicated entries: %v", store.revoked)
	}
}

// TestMemoryTokenStore_CapacityEvictsSoonestExpiry pins the bound:
// exceeding capacity drops the soonest-expiring entry, never the new
// one.
func TestMemoryTokenStore_CapacityEvictsSoonestExpiry(t *testing.T) {
	store := NewMemoryTokenStore()
	store.capacity = 2
	ctx := context.Background()
	base := time.Now()
	store.now = func() time.Time { return base }

	if err := store.Revoke(ctx, "soon", base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(ctx, "late", base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(ctx, "newest", base.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(store.revoked) != 2 {
		t.Fatalf("capacity not enforced: %v", store.revoked)
	}
	if _, ok := store.revoked["soon"]; ok {
		t.Fatal("soonest-expiring entry should have been evicted")
	}
	for _, keep := range []string{"late", "newest"} {
		if _, ok := store.revoked[keep]; !ok {
			t.Fatalf("%s was evicted instead of the soonest entry", keep)
		}
	}
}

// TestMemoryTokenStore_ConcurrentRevokeRevoked exercises the store
// under parallel access; run with -race to verify the mutex actually
// guards the map.
func TestMemoryTokenStore_ConcurrentRevokeRevoked(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := store.Revoke(ctx, "t", time.Now().Add(time.Hour)); err != nil {
				t.Errorf("Revoke: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := store.Revoked(ctx, "t"); err != nil {
				t.Errorf("Revoked: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestMemoryTokenStore_RevokeIfNotRevoked pins the CAS semantics:
// first caller wins; a live existing entry loses; an expired entry can
// be re-claimed.
func TestMemoryTokenStore_RevokeIfNotRevoked(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()
	base := time.Now()
	store.now = func() time.Time { return base }

	won, err := store.RevokeIfNotRevoked(ctx, "jti-a", base.Add(time.Hour))
	if err != nil || !won {
		t.Fatalf("first claim: won=%v err=%v, want true/nil", won, err)
	}
	won, _ = store.RevokeIfNotRevoked(ctx, "jti-a", base.Add(time.Hour))
	if won {
		t.Fatal("second concurrent claim should lose")
	}

	// Expired entries are reclaimable.
	store.now = func() time.Time { return base.Add(2 * time.Hour) }
	if won, _ = store.RevokeIfNotRevoked(ctx, "jti-a", base.Add(3*time.Hour)); !won {
		t.Fatal("claim on expired entry should win")
	}
}

// TestLoginDoesNotWriteStore pins the denylist semantics: Login must
// NOT touch the store — only explicit logout inserts entries.
func TestLoginDoesNotWriteStore(t *testing.T) {
	db := newLoginDB(t)
	svc := loginAsAdmin(t, db)
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	store := svc.opts.TokenStore.(*MemoryTokenStore)
	if len(store.revoked) != 0 {
		t.Fatalf("Login wrote to the denylist: %v", store.revoked)
	}
	_ = res
}

// TestLogoutRevokesAccessAndRefresh walks the end-to-end path: Logout
// with both tokens must denylist both jtis — no explicit WithTokenStore
// in between.
func TestLogoutRevokesAccessAndRefresh(t *testing.T) {
	db := newLoginDB(t)
	svc := loginAsAdmin(t, db)
	ctx := context.Background()

	res, err := svc.Login(ctx, &pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	store := svc.opts.TokenStore.(*MemoryTokenStore)

	jtiOf := func(raw string) string {
		claims := &auth.Claims{}
		if _, err := jwt.ParseWithClaims(raw, claims, svc.keyfunc()); err != nil {
			t.Fatalf("parse token: %v", err)
		}
		return claims.ID
	}
	accessJTI := jtiOf(res.AccessToken)
	refreshJTI := jtiOf(res.RefreshToken)

	if _, err := svc.Logout(ctx, &pb.LogoutRequest{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if got, _ := store.Revoked(ctx, accessJTI); !got {
		t.Error("Logout did not revoke the access token jti")
	}
	if got, _ := store.Revoked(ctx, refreshJTI); !got {
		t.Error("Logout did not revoke the refresh token jti")
	}
}

// TestLogoutRequiresAtLeastOneToken: neither field present is a 1001.
func TestLogoutRequiresAtLeastOneToken(t *testing.T) {
	db := newLoginDB(t)
	svc := loginAsAdmin(t, db)
	if _, err := svc.Logout(context.Background(), &pb.LogoutRequest{}); err == nil {
		t.Fatal("empty LogoutRequest did not fail")
	}
}
