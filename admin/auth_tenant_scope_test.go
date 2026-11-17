package admin

import (
	"context"
	"testing"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/admin/service"
)

// TestAuthWithFailClosedTenantScope covers the auth RPCs that run on the
// JWT allowlist — no claims are in ctx when the handler runs. With the
// production tenant callbacks installed, Login and RefreshToken must
// bypass the fail-closed scope while still filtering tenant_id
// explicitly; otherwise every login/refresh fails with
// ErrTenantRequired.
func TestAuthWithFailClosedTenantScope(t *testing.T) {
	db, _ := withTenantDB(t, &models.Role{}, &models.User{})

	// Seed through a bypass handle: fixtures run without a request ctx.
	seed := bypassDB(db)
	if err := seed.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Key:         "admin",
		Name:        "admin",
		Status:      "enabled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := seed.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "t1"},
		UID:         "admin",
		Username:    "admin",
		RoleKey:     "admin",
		Status:      "normal",
		Password:    "pass1234",
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := service.NewAuthService(
		service.WithAuthServiceDB(db),
		service.WithAuthSecret("test-secret"),
	)

	login, err := svc.Login(context.Background(), &pb.LoginRequest{
		Username: "admin",
		Password: "pass1234",
	})
	if err != nil {
		t.Fatalf("login with tenant scope installed: %v", err)
	}
	if login.TenantId != "t1" {
		t.Fatalf("login tenant = %q, want t1", login.TenantId)
	}

	refreshed, err := svc.RefreshToken(context.Background(), &pb.RefreshTokenRequest{
		RefreshToken: login.RefreshToken,
	})
	if err != nil {
		t.Fatalf("refresh with tenant scope installed: %v", err)
	}
	if refreshed.AccessToken == "" {
		t.Fatal("refresh returned an empty access token")
	}
}
