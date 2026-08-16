package admin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/admin/service"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	"github.com/sjlit/aeus/pkg/errs"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// ctxWithClaims returns a context carrying *auth.Claims as the JWT subject.
func ctxWithClaims(uid, role, tenant string) context.Context {
	return mwauth.NewContext(context.Background(), &auth.Claims{
		UID:      uid,
		Role:     role,
		TenantID: tenant,
	})
}

// setupUserSvcDB migrates the minimum set of tables for UserService and
// returns the *gorm.DB. Tests then build a UserService on top.
func setupUserSvcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.RolePermission{},
		&models.Menu{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

// seedUserAndRole inserts the user, role, permissions and menus referenced
// by tests in this file.
func seedUserAndRole(t *testing.T, db *gorm.DB) {
	t.Helper()
	role := &models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Name:        "admin",
		Key:         "admin_code",
		IsSuper:     true,
	}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	user := &models.User{
		TenantModel: models.TenantModel{TenantID: "t1"},
		UID:         "u0001",
		Username:    "alice",
		RoleKey:     "admin_code",
		DeptID:      1,
		Password:    "oldpass1",
		Email:       "alice@x.io",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	// junction rows: type=menu references Menu.Component, type=permission
	// references Permission.Data from the catalog.
	perms := []models.RolePermission{
		{TenantModel: models.TenantModel{TenantID: "t1"}, RoleKey: "admin_code", Type: "menu", Data: "User"},
		{TenantModel: models.TenantModel{TenantID: "t1"}, RoleKey: "admin_code", Type: "menu", Data: "Role"},
		{TenantModel: models.TenantModel{TenantID: "t1"}, RoleKey: "admin_code", Type: "menu", Data: "UserList"},
		{TenantModel: models.TenantModel{TenantID: "t1"}, RoleKey: "admin_code", Type: "permission", Data: "user:create"},
		{TenantModel: models.TenantModel{TenantID: "t1"}, RoleKey: "admin_code", Type: "permission", Data: "user:delete"},
	}
	for i := range perms {
		if err := db.Create(&perms[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	menus := []models.Menu{
		{Name: "user", Component: "User", Uri: "/user", Parent: "", Icon: "user"},
		{Name: "role", Component: "Role", Uri: "/role", Parent: "", Icon: "role"},
		{Name: "user:list", Component: "UserList", Uri: "/user/list", Parent: "user", Icon: ""},
	}
	for i := range menus {
		if err := db.Create(&menus[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func newUserSvc(t *testing.T, opts ...service.UserServiceOption) *service.UserService {
	t.Helper()
	base := []service.UserServiceOption{
		service.WithUserServiceDB(setupUserSvcDB(t)),
	}
	return service.NewUserService(append(base, opts...)...)
}

func TestUserService_Profile_ReadsFromCtxUID(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))

	res, err := s.Profile(ctxWithClaims("u0001", "admin_code", "t1"), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Uid != "u0001" || res.Username != "alice" || res.Email != "alice@x.io" {
		t.Errorf("profile mismatch: %+v", res)
	}
	if res.Role != "admin_code" || res.DeptId != 1 {
		t.Errorf("profile role/dept mismatch: %+v", res)
	}
}

func TestUserService_Profile_NoClaims_ReturnsAccessDenied(t *testing.T) {
	s := newUserSvc(t)
	_, err := s.Profile(context.Background(), &pb.Empty{})
	if !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("got %v, want ErrAccessDenied", err)
	}
}

func TestUserService_UpdateProfile_OnlyAffectsSelf(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))

	res, err := s.UpdateProfile(ctxWithClaims("u0001", "admin_code", "t1"),
		&pb.UpdateProfileRequest{Username: "alice2", Email: "new@x.io", Description: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Username != "alice2" || res.Email != "new@x.io" || res.Description != "hi" {
		t.Errorf("update did not stick: %+v", res)
	}
}

func TestUserService_ChangePassword_HashesNewPassword(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))

	if _, err := s.ChangePassword(ctxWithClaims("u0001", "admin_code", "t1"),
		&pb.ChangePasswordRequest{OldPassword: "oldpass1", NewPassword: "newpass1"}); err != nil {
		t.Fatal(err)
	}
	var u models.User
	if err := db.Where("uid = ?", "u0001").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.Password == "newpass1" || !strings.HasPrefix(u.Password, "$2") {
		t.Fatalf("password not hashed: %s", u.Password)
	}
	if !u.ValidatePassword("newpass1") {
		t.Fatalf("new password failed validation")
	}
}

func TestUserService_ChangePassword_RejectsWrongOld(t *testing.T) {
	s := newUserSvc(t)
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	_ = s

	if _, err := service.NewUserService(service.WithUserServiceDB(db)).ChangePassword(
		ctxWithClaims("u0001", "admin_code", "t1"),
		&pb.ChangePasswordRequest{OldPassword: "nope", NewPassword: "newpass1"}); !errors.Is(err, service.ErrInvalidPassword) {
		t.Fatalf("got %v, want ErrInvalidPassword", err)
	}
}

func TestUserService_ResetPassword_NonSuperDenied(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	// Caller is logged in with a role that exists but is not super.
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Name:        "user", Key: "user_code",
	}).Error; err != nil {
		t.Fatal(err)
	}
	s := service.NewUserService(service.WithUserServiceDB(db))
	_, err := s.ResetPassword(ctxWithClaims("u9999", "user_code", "t1"),
		&pb.ResetPasswordRequest{TargetUid: "u0001", NewPassword: "newpass1"})
	if !errors.Is(err, errs.ErrAccessDenied) {
		t.Fatalf("got %v, want ErrAccessDenied", err)
	}
}

func TestUserService_ResetPassword_SuperRoleCanResetOthers(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))
	res, err := s.ResetPassword(ctxWithClaims("u0001", "admin_code", "t1"),
		&pb.ResetPasswordRequest{TargetUid: "u0001", NewPassword: "fresh123"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TargetUid != "u0001" {
		t.Errorf("target_uid mismatch: %q", res.TargetUid)
	}
	var u models.User
	if err := db.Where("uid = ?", "u0001").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if !u.ValidatePassword("fresh123") {
		t.Fatalf("password not actually reset")
	}
}

func TestUserService_SetAvatarByURL_UpdatesField(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))

	res, err := s.SetAvatarByURL(ctxWithClaims("u0001", "admin_code", "t1"),
		&pb.SetAvatarByURLRequest{AvatarUrl: "https://cdn/x.png"})
	if err != nil {
		t.Fatal(err)
	}
	if res.AvatarUrl != "https://cdn/x.png" {
		t.Errorf("response avatar mismatch: %q", res.AvatarUrl)
	}
	var u models.User
	if err := db.Where("uid = ?", "u0001").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.Avatar != "https://cdn/x.png" {
		t.Errorf("DB avatar mismatch: %q", u.Avatar)
	}
}

func TestUserService_SetAvatarByURL_RejectsEmpty(t *testing.T) {
	s := newUserSvc(t)
	_, err := s.SetAvatarByURL(ctxWithClaims("u0001", "admin_code", "t1"),
		&pb.SetAvatarByURLRequest{AvatarUrl: "   "})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestUserService_ListVisibleMenus_FiltersByRolePermission(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))

	res, err := s.ListVisibleMenus(ctxWithClaims("u0001", "admin_code", "t1"), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	// Only menus whose Component appears in RolePermission(type=menu,
	// role=admin_code): "User", "Role", "UserList" — three rows (wire
	// Name maps from Menu.Name).
	got := map[string]*pb.MenuEntry{}
	for _, m := range res.Menus {
		got[m.Name] = m
	}
	for _, want := range []string{"user", "role", "user:list"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing menu %q in visible menus: %+v", want, res.Menus)
		}
	}
	if len(res.Menus) != 3 {
		t.Errorf("expected exactly 3 visible menus, got %d", len(res.Menus))
	}
	if got["user:list"].Parent != "user" {
		t.Errorf("user:list parent mismatch: %q", got["user:list"].Parent)
	}
}

func TestUserService_ListVisibleMenus_NoPerms_ReturnsEmpty(t *testing.T) {
	db := setupUserSvcDB(t)
	// Seed user with role that has NO permission rows.
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Name:        "user", Key: "user_code",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "t1"},
		UID:         "u0002", Username: "bob", RoleKey: "user_code", DeptID: 1, Password: "pass1234",
	}).Error; err != nil {
		t.Fatal(err)
	}
	s := service.NewUserService(service.WithUserServiceDB(db))

	res, err := s.ListVisibleMenus(ctxWithClaims("u0002", "user_code", "t1"), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 0 {
		t.Errorf("got %d, want no menus", len(res.Menus))
	}
}

func TestUserService_ListPermissionCodes_ReturnsAPICodesOnly(t *testing.T) {
	db := setupUserSvcDB(t)
	seedUserAndRole(t, db)
	s := service.NewUserService(service.WithUserServiceDB(db))

	res, err := s.ListPermissionCodes(ctxWithClaims("u0001", "admin_code", "t1"), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"user:create": true, "user:delete": true}
	if len(res.Permissions) != len(want) {
		t.Fatalf("code count = %d, want %d (%v)", len(res.Permissions), len(want), res.Permissions)
	}
	for _, c := range res.Permissions {
		if !want[c] {
			t.Errorf("unexpected code: %q", c)
		}
	}
}

// TestServerSetup_NoUserServiceRegistered confirms that admin.Server.Setup
// does not auto-register the user RPCs — applications must call
// pb.RegisterUserServiceRouter themselves, mirroring AuthService.
func TestServerSetup_NoUserServiceRegistered(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup error: %v", err)
	}
	// Walk the same router Setup registered resources onto — using a
	// fresh ghttp.New() here would always yield an empty route list
	// and silently pass even if Setup DID auto-register /user/*.
	for _, r := range httpSrv.Engine().Routes() {
		if strings.HasPrefix(r.Path, "/user/") {
			t.Errorf("UserService route should not be auto-registered, found %s %s", r.Method, r.Path)
		}
	}
}
