package service

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// newAuthDB migrates the tables Login / RefreshToken touch. Login runs
// without claims in ctx, so its status lookups must filter tenant_id
// explicitly — the GORM tenant callbacks cannot backfill a scope here.
func newAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Role{}, &models.User{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// testLoginPassword is policy-compliant (8 chars, letter+digit), so
// every fixture goes through the BeforeCreate hook unchanged.
const testLoginPassword = "pass1234"

// seedAccount creates an enabled role + a normal user on the same
// tenant, so Login / RefreshToken have a healthy row pair to hit.
func seedAccount(t *testing.T, db *gorm.DB, uid, username, roleKey, tenantID string) {
	t.Helper()
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: tenantID},
		Key:         roleKey,
		Name:        roleKey,
		Status:      "enabled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: tenantID},
		UID:         uid,
		Username:    username,
		RoleKey:     roleKey,
		Status:      "normal",
		Password:    testLoginPassword,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func newAuthSvcForTest(t *testing.T) *AuthService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
}

func TestNewAuthService_RequiresDBAndSecret(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(){
		"missing DB":     func() { NewAuthService(WithAuthSecret("s")) },
		"missing secret": func() { NewAuthService(WithAuthServiceDB(db)) },
		"empty secret":   func() { NewAuthService(WithAuthServiceDB(db), WithAuthSecret("")) },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("NewAuthService did not panic")
				}
			}()
			fn()
		})
	}
}

func TestRefreshToken_RejectsAccessToken(t *testing.T) {
	svc := newAuthSvcForTest(t)
	access, err := svc.createToken("u0001", "admin", "t1", tokenTypeAccess, 7200)
	if err != nil {
		t.Fatalf("createToken: %v", err)
	}
	_, err = svc.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: access})
	if err == nil {
		t.Fatal("got nil error, want ErrAccessDenied for an access token")
	}
	if !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("got %v, want ErrAccessDenied", err)
	}
}

func TestRefreshToken_IssuesFreshAccessToken(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "u0001", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	refresh, err := svc.createToken("u0001", "admin", "t1", tokenTypeRefresh, __refreshTokenTTL)
	if err != nil {
		t.Fatalf("createToken: %v", err)
	}
	res, err := svc.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: refresh})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if res.AccessToken == "" {
		t.Error("got empty access token")
	}
	if res.Uid != "u0001" {
		t.Errorf("Uid = %q, want u0001", res.Uid)
	}
	if res.RefreshToken != refresh {
		t.Error("refresh token should be echoed unchanged")
	}
	parsed, err := jwt.ParseWithClaims(res.AccessToken, &auth.Claims{}, svc.keyfunc())
	if err != nil {
		t.Fatalf("parse issued access token: %v", err)
	}
	claims, ok := parsed.Claims.(*auth.Claims)
	if !ok {
		t.Fatalf("claims type = %T, want *auth.Claims", parsed.Claims)
	}
	if claims.TokenType != tokenTypeAccess {
		t.Errorf("issued token_type = %q, want %q", claims.TokenType, tokenTypeAccess)
	}
}

// TestLogin_DisabledUserDenied: a disabled account must not receive
// tokens even with correct credentials.
func TestLogin_DisabledUserDenied(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	if err := db.Model(&models.User{}).Where("uid = ?", "u0001").
		Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))

	_, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	wantPkError(t, err, errs.CodePermissionDenied)
}

// TestLogin_DisabledRoleDenied: a disabled role must not receive
// tokens even with correct credentials.
func TestLogin_DisabledRoleDenied(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	if err := db.Model(&models.Role{}).
		Where("key = ? AND tenant_id = ?", "admin", "t1").
		Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))

	_, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	wantPkError(t, err, errs.CodePermissionDenied)
}

// TestLogin_MissingRoleDenied: a user whose role row is gone (deleted
// role) must not log in.
func TestLogin_MissingRoleDenied(t *testing.T) {
	db := newAuthDB(t)
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "t1"},
		UID:         "u0001",
		Username:    "alice",
		RoleKey:     "ghost",
		Status:      "normal",
		Password:    testLoginPassword,
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))

	_, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	wantPkError(t, err, errs.CodePermissionDenied)
}

// TestLogin_CrossTenantRoleIsolation: the role lookup must scope to the
// USER's tenant. A same-key role on another tenant must not satisfy the
// check — without the explicit tenant_id filter this test would pass
// the login (t1's role would answer for t2's user).
func TestLogin_CrossTenantRoleIsolation(t *testing.T) {
	db := newAuthDB(t)
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Key:         "admin",
		Name:        "admin",
		Status:      "enabled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "t2"},
		UID:         "u0001",
		Username:    "bob",
		RoleKey:     "admin",
		Status:      "normal",
		Password:    testLoginPassword,
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))

	_, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "bob", Password: testLoginPassword})
	wantPkError(t, err, errs.CodePermissionDenied)
}

// TestRefreshToken_DisabledUserDenied: the refresh path must re-check
// status — a disabled account keeps a valid refresh token otherwise,
// and would keep minting access tokens for the refresh TTL.
func TestRefreshToken_DisabledUserDenied(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := db.Model(&models.User{}).Where("uid = ?", "u0001").
		Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}

	_, err = svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken})
	wantPkError(t, err, errs.CodePermissionDenied)
}

// TestRefreshToken_DisabledRoleDenied: same for a disabled role.
func TestRefreshToken_DisabledRoleDenied(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := db.Model(&models.Role{}).
		Where("key = ? AND tenant_id = ?", "admin", "t1").
		Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}

	_, err = svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken})
	wantPkError(t, err, errs.CodePermissionDenied)
}

// TestRefreshToken_MissingUserDenied: a deleted user must not refresh.
func TestRefreshToken_MissingUserDenied(t *testing.T) {
	db := newAuthDB(t)
	seedAccount(t, db, "u0001", "alice", "admin", "t1")
	svc := NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
	res, err := svc.Login(context.Background(),
		&pb.LoginRequest{Username: "alice", Password: testLoginPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := db.Where("uid = ?", "u0001").Delete(&models.User{}).Error; err != nil {
		t.Fatal(err)
	}

	_, err = svc.RefreshToken(context.Background(),
		&pb.RefreshTokenRequest{RefreshToken: res.RefreshToken})
	wantPkError(t, err, errs.CodeAccessDenied)
}
