package admin

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/pkg/errs"
)

// TestSetup_SecurityTablesReadOnly locks the security boundary: generic
// REST CRUD must not expose write routes for the permission catalog,
// role-permission junction, audit log, or login log. Those tables are
// managed by Setup/Seed/services; writable generic routes would let a
// role manager edit the catalog or grant itself permissions.
func TestSetup_SecurityTablesReadOnly(t *testing.T) {
	db, srv := newAuditServer(t)

	for _, path := range []string{
		"/system/sys_permission",
		"/system/sys_role_permission",
		"/system/sys_audit",
		"/system/sys_login_log",
	} {
		rec := doReq(t, srv, http.MethodPost, path, `{"data":"x"}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404 (read-only)", path, rec.Code)
		}
	}

	// Role.IsSuper and Role.Key are not part of the create/update
	// scenarios. A REST create that tries to set them must not grant a
	// super role.
	rec := doReq(t, srv, http.MethodPost, "/system/sys_role",
		`{"key":"ops","name":"Ops","is_super":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create role = %d body=%s", rec.Code, rec.Body.String())
	}
	var role models.Role
	if err := db.Where("`key` = ?", "ops").First(&role).Error; err != nil {
		t.Fatalf("load role: %v", err)
	}
	if role.IsSuper {
		t.Fatal("REST create must not set is_super=true")
	}

	// Updating an existing role cannot flip is_super or rename its Key.
	rec = doReq(t, srv, http.MethodPut, "/system/sys_role/"+strconv.FormatUint(uint64(role.ID), 10),
		`{"key":"admin","name":"Ops2","is_super":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update role = %d body=%s", rec.Code, rec.Body.String())
	}
	var updated models.Role
	if err := db.First(&updated, role.ID).Error; err != nil {
		t.Fatalf("reload role: %v", err)
	}
	if updated.IsSuper {
		t.Fatal("REST update must not flip is_super")
	}
	if updated.Key != "ops" {
		t.Fatalf("REST update changed key to %q, want ops", updated.Key)
	}
	if updated.Name != "Ops2" {
		t.Fatalf("ordinary field update was dropped: name=%q", updated.Name)
	}

	// User.RoleKey is create-only; an update cannot promote the user to
	// an admin role.
	u := models.User{UID: "u0001", Username: "alice", RoleKey: "ops", DeptID: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	rec = doReq(t, srv, http.MethodPut, "/system/sys_user/"+strconv.FormatUint(uint64(u.ID), 10),
		`{"role_key":"admin","username":"alice2"}`)
	if code, _ := decodeEnvelope(t, rec.Body.Bytes()); code != int(errs.CodePermissionDenied) {
		t.Fatalf("non-super role change code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}
	var got models.User
	if err := db.First(&got, u.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.RoleKey != "ops" {
		t.Fatalf("REST update changed role_key to %q, want ops", got.RoleKey)
	}

	// Creating a user whose role is super is only allowed for a super
	// caller. u0001's role is "ops" (non-super), so the create must be
	// rejected with the permission-denied business code.
	superRole := models.Role{Key: "root", Name: "Root", IsSuper: true}
	if err := db.Create(&superRole).Error; err != nil {
		t.Fatalf("create super role: %v", err)
	}
	rec = doReq(t, srv, http.MethodPost, "/system/sys_user",
		`{"uid":"u0002","username":"bob","role_key":"root","dept_id":1}`)
	code, _ := decodeEnvelope(t, rec.Body.Bytes())
	if code != int(errs.CodePermissionDenied) {
		t.Fatalf("super-role user create code = %d, want %d body=%s",
			code, errs.CodePermissionDenied, rec.Body.String())
	}
}
