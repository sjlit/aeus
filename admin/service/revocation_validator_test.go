package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
)

// ---------- RefreshToken rotation & revocation ----------

// TestRefreshToken_RotatesAndDenylistsOld: a successful refresh must
// return a NEW refresh token (fresh jti) and denylist the old one, so
// replaying the old token is rejected.
func TestRefreshToken_RotatesAndDenylistsOld(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	out, err := svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if out.RefreshToken == "" || out.RefreshToken == res.RefreshToken {
		t.Fatalf("refresh token not rotated: old=%q new=%q", res.RefreshToken, out.RefreshToken)
	}

	// The rotated-out token must now be on the denylist.
	oldClaims := parseClaimsForTest(t, svc, res.RefreshToken)
	newClaims := parseClaimsForTest(t, svc, out.RefreshToken)
	if oldClaims.ID == newClaims.ID {
		t.Fatal("rotated refresh token reused the same jti")
	}
	store := svc.opts.TokenStore.(*MemoryTokenStore)
	if revoked, _ := store.Revoked(context.Background(), oldClaims.ID); !revoked {
		t.Fatal("old refresh jti was not denylisted after rotation")
	}

	// Replaying the old refresh token must fail.
	if _, err := svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken}); !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("replayed refresh token: got %v, want ErrAccessDenied", err)
	}
}

// TestRefreshToken_UsesFreshRoleFromDB: the issued access token must
// carry the role currently stored in the DB, not the one baked into
// the old refresh token.
func TestRefreshToken_UsesFreshRoleFromDB(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// Operator demotes / re-points the user's role after login.
	if err := db.Model(&models.User{}).Where("uid = ?", "u0001").
		Update("role_key", "viewer").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Key:         "viewer",
		Name:        "viewer",
		Status:      "enabled",
	}).Error; err != nil {
		t.Fatal(err)
	}

	out, err := svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	claims := parseClaimsForTest(t, svc, out.AccessToken)
	if claims.Role != "viewer" {
		t.Fatalf("access token role = %q, want viewer (from DB)", claims.Role)
	}
}

// TestLogoutThenRefreshDenied: logout with only the access token must
// still kill the session — the paired refresh token is denylisted too.
func TestLogoutThenRefreshDenied(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := svc.Logout(context.Background(), &pb.LogoutRequest{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken}); !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("refresh after logout: got %v, want ErrAccessDenied", err)
	}
}

// TestRefreshToken_ConcurrentReplayFenced: N concurrent refreshes of
// the same token must yield exactly ONE success — RevokeIfNotRevoked
// serializes them, so replayed (or merely duplicated) requests lose.
func TestRefreshToken_ConcurrentReplayFenced(t *testing.T) {
	db := newAuthDB(t)
	// :memory: SQLite gives every pooled connection its own empty
	// database — pin the pool to one connection so all racers hit the
	// same data.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	const racers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		wins     int
		denied   int
		otherErr int
	)
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, rerr := svc.RefreshToken(context.Background(),
				&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case rerr == nil:
				wins++
			case errors.Is(rerr, errs.ErrAccessDenied):
				denied++
			default:
				otherErr++
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("concurrent refreshes: %d succeeded, want exactly 1 (denied=%d other=%v)",
			wins, denied, otherErr)
	}
	if denied != racers-1 {
		t.Fatalf("losers should all be AccessDenied: denied=%d want %d", denied, racers-1)
	}
}

func parseClaimsForTest(t *testing.T, svc *AuthService, raw string) *auth.Claims {
	t.Helper()
	claims := &auth.Claims{}
	if _, err := jwt.ParseWithClaims(raw, claims, svc.keyfunc()); err != nil {
		t.Fatalf("parse token: %v", err)
	}
	return claims
}

// ---------- RevocationValidator ----------

// TestRevocationValidator_RejectsRefreshToken: a refresh token must
// never authenticate API routes through the middleware hook.
func TestRevocationValidator_RejectsRefreshToken(t *testing.T) {
	svc := newAuthSvcForTest(t)
	refresh, err := svc.createToken("u0001", "admin", "t1", tokenTypeRefresh, 7200)
	if err != nil {
		t.Fatal(err)
	}
	v := NewRevocationValidator(svc.opts.TokenStore, "test-secret")
	if err := v.Validate(context.Background(), refresh); !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("refresh token passed validator: got %v, want ErrAccessDenied", err)
	}
}

// TestRevocationValidator_AllowsFreshAccessAndBlocksRevoked covers all
// three outcomes of the adapter in one flow.
func TestRevocationValidator_AllowsFreshAccessAndBlocksRevoked(t *testing.T) {
	svc := newAuthSvcForTest(t)
	store := svc.opts.TokenStore.(*MemoryTokenStore)
	v := NewRevocationValidator(store, "test-secret")
	ctx := context.Background()

	// Malformed input passes through — the middleware owns parse errors.
	if err := v.Validate(ctx, "garbage"); err != nil {
		t.Fatalf("malformed token should pass through, got %v", err)
	}

	access, err := svc.createToken("u0001", "admin", "t1", tokenTypeAccess, 7200)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Validate(ctx, access); err != nil {
		t.Fatalf("valid access token rejected: %v", err)
	}

	claims := parseClaimsForTest(t, svc, access)
	until := time.Now().Add(time.Hour)
	if claims.ExpiresAt != nil {
		until = claims.ExpiresAt.Time
	}
	if err := store.Revoke(ctx, claims.ID, until); err != nil {
		t.Fatal(err)
	}
	if err := v.Validate(ctx, access); !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("revoked token accepted: got %v, want ErrAccessDenied", err)
	}
}

// TestRevocationValidator_FailsClosedOnStoreError: a denylist outage
// must reject rather than silently resurrect logged-out sessions.
type failingStore struct{}

func (failingStore) Revoke(context.Context, string, time.Time) error { return errors.New("boom") }
func (failingStore) Revoked(context.Context, string) (bool, error)   { return false, errors.New("boom") }
func (failingStore) RevokeIfNotRevoked(context.Context, string, time.Time) (bool, error) {
	return false, errors.New("boom")
}

func TestRevocationValidator_FailsClosedOnStoreError(t *testing.T) {
	svc := newAuthSvcForTest(t)
	access, err := svc.createToken("u0001", "admin", "t1", tokenTypeAccess, 7200)
	if err != nil {
		t.Fatal(err)
	}
	v := NewRevocationValidator(failingStore{}, "test-secret")
	if err := v.Validate(context.Background(), access); !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("store error should fail closed: got %v", err)
	}
}
