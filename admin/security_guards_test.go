package admin

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/admin/service"
	"github.com/sjlit/aeus/pkg/errs"
)

func TestSecurityGuards_NonSuperCannotChangeRoleOrTouchSuperTargets(t *testing.T) {
	db, srv := newAuditServer(t)

	ops := &models.Role{Key: "ops", Name: "Ops"}
	root := &models.Role{Key: "root", Name: "Root", IsSuper: true}
	viewer := &models.Role{Key: "viewer", Name: "Viewer"}
	for _, role := range []*models.Role{ops, root, viewer} {
		if err := db.Create(role).Error; err != nil {
			t.Fatalf("create role %s: %v", role.Key, err)
		}
	}
	caller := &models.User{UID: "u0001", Username: "alice", RoleKey: "ops", DeptID: 1}
	target := &models.User{UID: "u0002", Username: "bob", RoleKey: "ops", DeptID: 1}
	superTarget := &models.User{UID: "u0003", Username: "carol", RoleKey: "root", DeptID: 1}
	for _, u := range []*models.User{caller, target, superTarget} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.UID, err)
		}
	}

	// A non-super caller may not change any user's role, even to a
	// non-super role: otherwise a user manager could grant themselves a
	// more privileged role.
	rec := doReq(t, srv, http.MethodPut, "/system/sys_user/"+strconv.FormatUint(uint64(target.ID), 10),
		`{"username":"bob","role_key":"viewer"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodePermissionDenied) {
		t.Fatalf("non-super role change code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}

	// A no-op role field (same value) is not a role change and must not
	// require a super caller.
	rec = doReq(t, srv, http.MethodPut, "/system/sys_user/"+strconv.FormatUint(uint64(target.ID), 10),
		`{"username":"bob","role_key":"ops"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodeOK) {
		t.Fatalf("same-role update code = %d, want OK body=%s", code, rec.Body.String())
	}

	// A non-super caller may not disable a super user.
	rec = doReq(t, srv, http.MethodPut, "/system/sys_user/"+strconv.FormatUint(uint64(superTarget.ID), 10),
		`{"username":"carol","status":"disabled","role_key":"root"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodePermissionDenied) {
		t.Fatalf("non-super disable super user code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}

	// A non-super caller may not disable or delete the super role.
	rec = doReq(t, srv, http.MethodPut, "/system/sys_role/"+strconv.FormatUint(uint64(root.ID), 10),
		`{"status":"disabled"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodePermissionDenied) {
		t.Fatalf("non-super disable super role code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}
	rec = doReq(t, srv, http.MethodDelete, "/system/sys_role/"+strconv.FormatUint(uint64(root.ID), 10), "")
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodePermissionDenied) {
		t.Fatalf("non-super delete super role code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}
	rec = doReq(t, srv, http.MethodDelete, "/system/sys_user/"+strconv.FormatUint(uint64(superTarget.ID), 10), "")
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodePermissionDenied) {
		t.Fatalf("non-super delete super user code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}
}

func TestSecurityGuards_SuperCallerCanAssignRole(t *testing.T) {
	db, srv := newAuditServer(t, WithUserResolve(func(context.Context, *http.Request) (string, error) {
		return "supercaller", nil
	}))

	root := &models.Role{Key: "root", Name: "Root", IsSuper: true}
	ops := &models.Role{Key: "ops", Name: "Ops"}
	if err := db.Create(root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(ops).Error; err != nil {
		t.Fatal(err)
	}
	caller := &models.User{UID: "supercaller", Username: "supercaller", RoleKey: "root", DeptID: 1}
	target := &models.User{UID: "u0002", Username: "bob", RoleKey: "ops", DeptID: 1}
	for _, u := range []*models.User{caller, target} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}

	rec := doReq(t, srv, http.MethodPut, "/system/sys_user/"+strconv.FormatUint(uint64(target.ID), 10),
		`{"username":"bob","role_key":"root"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodeOK) {
		t.Fatalf("super role assignment code = %d, want OK body=%s", code, rec.Body.String())
	}
}

func TestSecurityGuards_RejectsCrossFieldLoginCollision(t *testing.T) {
	db, srv := newAuditServer(t)

	ops := &models.Role{Key: "ops", Name: "Ops"}
	if err := db.Create(ops).Error; err != nil {
		t.Fatal(err)
	}
	// Victim's UID is "victim"; it must not be usable as another user's
	// username (Login would then return two candidates and reject both).
	victim := &models.User{UID: "victim", Username: "alice", RoleKey: "ops", DeptID: 1}
	if err := db.Create(victim).Error; err != nil {
		t.Fatal(err)
	}
	rec := doReq(t, srv, http.MethodPost, "/system/sys_user",
		`{"uid":"u0004","username":"victim","role_key":"ops","dept_id":1}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodeInvalid) {
		t.Fatalf("cross-field create code = %d, want %d body=%s",
			code, errs.CodeInvalid, rec.Body.String())
	}

	other := &models.User{UID: "u0005", Username: "bob", RoleKey: "ops", DeptID: 1}
	if err := db.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	rec = doReq(t, srv, http.MethodPut, "/system/sys_user/"+strconv.FormatUint(uint64(other.ID), 10),
		`{"username":"victim"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodeInvalid) {
		t.Fatalf("cross-field update code = %d, want %d body=%s",
			code, errs.CodeInvalid, rec.Body.String())
	}
}

// TestSingleTenant_RESTCreateAdminThenLogin reproduces the most common
// single-tenant bootstrap flow: Seed, then add a second super admin via
// REST, then log in. Before the default-tenant hooks both rows used
// different tenant ids and Login failed with "role not found".
func TestSingleTenant_RESTCreateAdminThenLogin(t *testing.T) {
	db, srv := newAuditServer(t, WithUserResolve(func(context.Context, *http.Request) (string, error) {
		return "admin", nil
	}))
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	rec := doReq(t, srv, http.MethodPost, "/system/sys_user",
		`{"uid":"admin2","username":"admin2","role_key":"admin","dept_id":1,"password":"Admin123","gender":"other"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodeOK) {
		t.Fatalf("create second admin code = %d, want OK body=%s", code, rec.Body.String())
	}

	var created models.User
	if err := db.Where("uid = ?", "admin2").First(&created).Error; err != nil {
		t.Fatalf("load admin2: %v", err)
	}
	if created.TenantID != models.DefaultTenantID {
		t.Fatalf("admin2 tenant = %q, want %q", created.TenantID, models.DefaultTenantID)
	}

	authSvc := service.NewAuthService(
		service.WithAuthServiceDB(db),
		service.WithAuthSecret("test-secret"),
	)
	res, err := authSvc.Login(context.Background(), &pb.LoginRequest{
		Username: "admin2",
		Password: "Admin123",
	})
	if err != nil {
		t.Fatalf("Login(admin2): %v", err)
	}
	if res.TenantId != models.DefaultTenantID {
		t.Fatalf("login tenant = %q, want %q", res.TenantId, models.DefaultTenantID)
	}
}
