package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	adminauth "github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

func newUserSvcDB(t *testing.T) *gorm.DB {
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

// claimsCtx wraps ctx with JWT claims the same way the JWT middleware
// does, so the service's auth.ClaimsFromContext reads them.
func claimsCtx(uid, role, tenant string) context.Context {
	return mwauth.NewContext(context.Background(), &adminauth.Claims{
		UID:      uid,
		Role:     role,
		TenantID: tenant,
	})
}

func TestUserService_ResetPassword_SuperRoleAllowed(t *testing.T) {
	db := newUserSvcDB(t)
	if err := db.Create(&models.Role{Key: "ops", Name: "运维", IsSuper: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{UID: "bob", Username: "bob", RoleKey: "ops", Password: "oldpass1"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	res, err := svc.ResetPassword(claimsCtx("alice", "ops", "t1"), &pb.ResetPasswordRequest{
		TargetUid:   "bob",
		NewPassword: "newpass1",
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if res.TargetUid != "bob" {
		t.Errorf("TargetUid = %q, want bob", res.TargetUid)
	}
	var target models.User
	if err := db.Where("uid = ?", "bob").First(&target).Error; err != nil {
		t.Fatal(err)
	}
	if !target.ValidatePassword("newpass1") {
		t.Error("target password should be the new one")
	}
}

func TestUserService_ResetPassword_NonSuperDenied(t *testing.T) {
	db := newUserSvcDB(t)
	if err := db.Create(&models.Role{Key: "viewer", Name: "访客", IsSuper: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{UID: "bob", Username: "bob", RoleKey: "viewer", Password: "oldpass1"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	_, err := svc.ResetPassword(claimsCtx("alice", "viewer", "t1"), &pb.ResetPasswordRequest{
		TargetUid:   "bob",
		NewPassword: "newpass",
	})
	if err == nil {
		t.Fatal("want error")
	}
	wantPkError(t, err, errs.CodeAccessDenied)
}

func TestUserService_ResetPassword_UnknownRoleDenied(t *testing.T) {
	// role row missing entirely (e.g. deleted after the token was
	// issued): the caller cannot act on a NotFound, so deny access.
	db := newUserSvcDB(t)
	svc := NewUserService(WithUserServiceDB(db))

	_, err := svc.ResetPassword(claimsCtx("alice", "ghost", "t1"), &pb.ResetPasswordRequest{
		TargetUid:   "bob",
		NewPassword: "newpass1",
	})
	if err == nil {
		t.Fatal("want error")
	}
	wantPkError(t, err, errs.CodeAccessDenied)
}

// TestUserService_ChangePassword_WeakNewPasswordRejected: the new
// password must pass the policy even when the old one verifies.
func TestUserService_ChangePassword_WeakNewPasswordRejected(t *testing.T) {
	db := newUserSvcDB(t)
	if err := db.Create(&models.User{
		UID: "alice", Username: "alice", RoleKey: "ops", Password: "pass1234",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	_, err := svc.ChangePassword(claimsCtx("alice", "ops", "t1"), &pb.ChangePasswordRequest{
		OldPassword: "pass1234",
		NewPassword: "weakpass",
	})
	wantPkError(t, err, errs.CodeInvalid)

	var u models.User
	if err := db.Where("uid = ?", "alice").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if !u.ValidatePassword("pass1234") {
		t.Error("password should be unchanged after a rejected change")
	}
}

// TestUserService_ChangePassword_SamePasswordRejected: the new password
// must differ from the old one.
func TestUserService_ChangePassword_SamePasswordRejected(t *testing.T) {
	db := newUserSvcDB(t)
	if err := db.Create(&models.User{
		UID: "alice", Username: "alice", RoleKey: "ops", Password: "pass1234",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	_, err := svc.ChangePassword(claimsCtx("alice", "ops", "t1"), &pb.ChangePasswordRequest{
		OldPassword: "pass1234",
		NewPassword: "pass1234",
	})
	wantPkError(t, err, errs.CodeInvalid)
}

// TestUserService_ChangePassword_Success: a compliant new password
// replaces the old one.
func TestUserService_ChangePassword_Success(t *testing.T) {
	db := newUserSvcDB(t)
	if err := db.Create(&models.User{
		UID: "alice", Username: "alice", RoleKey: "ops", Password: "pass1234",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	_, err := svc.ChangePassword(claimsCtx("alice", "ops", "t1"), &pb.ChangePasswordRequest{
		OldPassword: "pass1234",
		NewPassword: "newpass1",
	})
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	var u models.User
	if err := db.Where("uid = ?", "alice").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if !u.ValidatePassword("newpass1") {
		t.Error("password should be the new one")
	}
}

// TestUserService_ResetPassword_WeakNewPasswordRejected: admin resets
// must not install a weak password.
func TestUserService_ResetPassword_WeakNewPasswordRejected(t *testing.T) {
	db := newUserSvcDB(t)
	if err := db.Create(&models.Role{Key: "ops", Name: "运维", IsSuper: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		UID: "bob", Username: "bob", RoleKey: "ops", Password: "pass1234",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	_, err := svc.ResetPassword(claimsCtx("alice", "ops", "t1"), &pb.ResetPasswordRequest{
		TargetUid:   "bob",
		NewPassword: "weakpass",
	})
	wantPkError(t, err, errs.CodeInvalid)

	var target models.User
	if err := db.Where("uid = ?", "bob").First(&target).Error; err != nil {
		t.Fatal(err)
	}
	if !target.ValidatePassword("pass1234") {
		t.Error("target password should be unchanged after a rejected reset")
	}
}
