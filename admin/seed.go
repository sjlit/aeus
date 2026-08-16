package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// Seed converges a database to the bootstrap state the admin module
// expects, and is safe to run on every startup:
//
//   - a super-admin role: Key="admin", Name="系统管理员",
//     Status="enabled", Builtin=true, IsSuper=true, DataScope="all",
//     tenant-scoped to a fresh uuid on first creation.  A pre-existing
//     Builtin role with Key="admin" is upgraded to IsSuper=true in
//     place; a pre-existing non-builtin role with that key is the
//     operator's own construct and is left untouched.
//   - a user bound to that role: UID="admin", Username=adminUser,
//     Password=bcrypt(adminPassword), Status="normal", on the role's
//     tenant.  The user is ensured even when the role already existed,
//     so a half-bootstrapped database converges instead of
//     short-circuiting.
//   - a sys_tenants entity row for the role's tenant: Name="默认租户",
//     Status="enabled", id = the role's tenant_id.  A pre-existing
//     role's tenant id is followed, so databases bootstrapped before
//     the tenant model existed converge to a real tenant row instead
//     of an orphan uuid.
//   - full grants for every IsSuper role: each run diffs the global
//     catalogs (sys_menus components + sys_permissions datas) against
//     the role's sys_role_permissions rows and inserts the missing
//     ones — add-only.  Models registered by an application upgrade
//     are picked up automatically, and grants deleted out-of-band are
//     healed; manual edits to a super role's grants are rejected at
//     the API layer (RoleService.ReplaceRolePermissions), this heal is
//     the safety net for anything that slips through.
//
// The grants reflect the catalogs as of the Seed call: run Setup (which
// populates sys_menus / sys_permissions) before Seed so a fresh database
// is granted in the same pass; anything registered later is picked up
// on the next run.
//
// Idempotent: when nothing is missing the converge checks and the
// catalog diff perform no writes.
//
// The role + user + grant writes run inside a single transaction so a
// failure partway (e.g. the bcrypt BeforeCreate hook) leaves no
// partial state behind.
func Seed(db *gorm.DB, adminUser, adminPassword string) error {
	if db == nil {
		return errs.New(errs.CodeInvalid, "admin: Seed requires a non-nil *gorm.DB")
	}
	if adminUser == "" || adminPassword == "" {
		return errs.New(errs.CodeInvalid, "admin: Seed requires non-empty adminUser and adminPassword")
	}

	role := &models.Role{
		TenantModel: models.TenantModel{TenantID: uuid.NewString()},
		Key:         "admin",
		Name:        "系统管理员",
		Status:      "enabled",
		Builtin:     true,
		IsSuper:     true,
		DataScope:   "all",
	}
	user := &models.User{
		TenantModel: models.TenantModel{TenantID: role.TenantID},
		UID:         "admin",
		Username:    adminUser,
		RoleKey:     role.Key,
		Status:      "normal",
		Password:    adminPassword, // BeforeCreate bcrypts it
		Gender:      "other",
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// FirstOrCreate with a key-clause is the canonical "ensure exists"
		// idiom.  RowsAffected==0 means the row was already there (any
		// tenant_id) and has been loaded into role — including its
		// existing tenant_id, which the user ensure below follows.
		res := tx.Where("key = ?", role.Key).Attrs(*role).FirstOrCreate(role)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 && role.Builtin && !role.IsSuper {
			// Upgrade a pre-existing builtin admin role created before
			// IsSuper existed so the converge pass below covers it.
			if err := tx.Model(&models.Role{}).
				Where("id = ?", role.ID).
				Update("is_super", true).Error; err != nil {
				return err
			}
			role.IsSuper = true
		}

		// Ensure the sys_tenants entity row for the role's tenant so
		// Login's resolveTenant returns a real name.  Runs after the
		// role ensure because a pre-existing role carries its own
		// tenant id (a legacy orphan uuid gains its row here), while a
		// fresh role uses the uuid generated above.  Guarded on
		// non-empty: roles created by out-of-band tooling may have no
		// tenant id, and a tenant row keyed on "" would be meaningless.
		if role.TenantID != "" {
			tenant := &models.Tenant{ID: role.TenantID, Name: "默认租户", Status: "enabled"}
			if err := tx.Where("id = ?", tenant.ID).Attrs(*tenant).FirstOrCreate(tenant).Error; err != nil {
				return err
			}
		}

		// user was built against the fresh tenant before the role lookup;
		// re-point it at the tenant the role actually lives on.
		user.TenantID = role.TenantID
		if err := tx.Where("uid = ?", user.UID).Attrs(*user).FirstOrCreate(user).Error; err != nil {
			return err
		}

		// Top up the grants of every IsSuper role.  The freshly ensured
		// role is included via its in-memory IsSuper, so a brand-new
		// database gets its full catalog in this same pass.
		var supers []models.Role
		if err := tx.Where("is_super = ?", true).Find(&supers).Error; err != nil {
			return err
		}
		for i := range supers {
			if err := grantFullCatalog(tx, &supers[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// grantFullCatalog reads the global catalogs and inserts the
// sys_role_permissions rows the super role is still missing.  Menu and
// Permission are tenant-less; the junction rows are tenant-scoped to
// the role's tenant.  Seed runs without JWT claims, so the tenant
// callbacks (query filters and create backfill) are inert here and
// tenantID flows explicitly into GrantMissingPermissions.
func grantFullCatalog(tx *gorm.DB, role *models.Role) error {
	ctx := context.Background()

	var menus []string
	if err := tx.WithContext(ctx).Model(&models.Menu{}).Pluck("component", &menus).Error; err != nil {
		return err
	}
	var datas []string
	if err := tx.WithContext(ctx).Model(&models.Permission{}).Pluck("data", &datas).Error; err != nil {
		return err
	}

	return (&models.Role{}).GrantMissingPermissions(tx, ctx, role.Key, role.TenantID, menus, datas)
}
