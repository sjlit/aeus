package service

import (
	"context"
	"errors"
	"testing"

	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
)

// TestLogout_ExpiredAccessStillRevokesRefresh pins the recovery path
// that matters when a tab wakes up after the access token expired:
// Logout must ignore the dead access token and still denylist the
// paired refresh token, otherwise a stolen refresh token outlives the
// user's explicit logout.
func TestLogout_ExpiredAccessStillRevokesRefresh(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	ctx := context.Background()

	res, err := svc.Login(ctx, &pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	expiredAccess, err := svc.createToken("u0001", "admin", "t1", tokenTypeAccess, -60)
	if err != nil {
		t.Fatalf("create expired access token: %v", err)
	}
	if _, err := svc.Logout(ctx, &pb.LogoutRequest{
		AccessToken:  expiredAccess,
		RefreshToken: res.RefreshToken,
	}); err != nil {
		t.Fatalf("Logout with expired access token: %v", err)
	}

	if _, err := svc.RefreshToken(ctx,
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken}); !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("refresh after logout: got %v, want ErrAccessDenied", err)
	}
}

// TestLogin_DuplicateIdentifierRejected: Login has no tenant selector,
// so the same uid/username in two tenants is ambiguous. It must fail
// closed instead of picking an arbitrary row.
func TestLogin_DuplicateIdentifierRejected(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	seedAccount(t, db, "u0001", "alice", "admin", "t2")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))

	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("duplicate identifier login: got %v, want ErrInvalidPassword", err)
	}
	if res != nil {
		t.Fatalf("duplicate identifier login returned a session: %+v", res)
	}
}
