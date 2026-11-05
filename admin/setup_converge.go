package admin

import (
	"errors"
	"fmt"
	"slices"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// syncRoleKeyCreateRule backfills the create-scenario required flag on
// the existing sys_schemas row for User.RoleKey.
//
// rest/v3's schema.AutoMigrate is insert-only: it writes a metadata row
// the first time a column appears but never refreshes Rules for an
// existing row. Fresh databases pick up `rule:"required:create"` from
// the struct tag; databases created before that tag existed would keep
// the empty Rules set and never show the field as required. Update every
// matching row (including multi-tenant schema clones) in place while
// preserving any other required scenarios an operator added.
func syncRoleKeyCreateRule(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	var rows []schema.Schema
	if err := db.Where(
		"module_name = ? AND table_name = ? AND column = ?",
		"system", "sys_users", "role_key",
	).Find(&rows).Error; err != nil {
		return fmt.Errorf("load role_key schema metadata: %w", err)
	}
	changed := false
	for i := range rows {
		if slices.Contains(rows[i].Rules.Required, schema.ScenarioCreate) {
			continue
		}
		rows[i].Rules.Required = append(rows[i].Rules.Required, schema.ScenarioCreate)
		if err := db.Model(&schema.Schema{}).
			Where("id = ?", rows[i].Id).
			Update("rules", rows[i].Rules).Error; err != nil {
			return fmt.Errorf("update role_key schema metadata: %w", err)
		}
		changed = true
	}
	if changed {
		schema.InvalidateCache("system", "sys_users")
	}
	return nil
}

// tenantManagementMenuIdentity is the untouched shape ensureMenuRow
// writes for models.Tenant. Matching every field keeps the single-tenant
// cleanup from deleting a row an operator has deliberately repurposed.
var tenantManagementMenuIdentity = struct {
	component, name, parent, uri, viewPath string
}{
	component: "SystemSysTenants",
	name:      "租户管理",
	parent:    "SystemSettings",
	uri:       "/system/sys-tenants",
	viewPath:  "@/views/system/sys_tenant/Index.vue",
}

// tenantResourcePermissionData is the exact catalog produced by
// rest/v3's six CRUD scenarios for models.Tenant. These rows are only
// reachable while the Tenant REST resource is registered.
var tenantResourcePermissionData = []string{
	"POST /system/sys_tenant",
	"PUT /system/sys_tenant/:id",
	"DELETE /system/sys_tenant/:id",
	"GET /system/sys_tenants",
	"GET /system/sys_tenant/detail/:id",
	"GET /system/sys_tenant/export",
}

// removeTenantManagementArtifacts hard-deletes the auto-generated tenant
// menu and CRUD permission rows in single-tenant mode. The corresponding
// REST resource is not registered in that mode, so leaving them behind
// would send the sidebar to a non-existent API and keep a dead "租户管理"
// group in the permission catalog.
//
// Hard delete (rather than soft delete) is intentional: ensureMenuRow
// and ensurePermissionRows treat soft-deleted rows as "exists, do
// nothing", so a soft delete would prevent them from coming back when an
// operator later enables multi-tenant mode. Menu.AfterDelete still purges
// the menu grants for the row.
func removeTenantManagementArtifacts(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	ident := tenantManagementMenuIdentity
	var row models.Menu
	err := db.Unscoped().Where(
		"component = ? AND name = ? AND parent = ? AND uri = ? AND view_path = ?",
		ident.component, ident.name, ident.parent, ident.uri, ident.viewPath,
	).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup tenant management menu: %w", err)
	}
	if err == nil {
		if err := db.Unscoped().Delete(&row).Error; err != nil {
			return fmt.Errorf("remove tenant management menu: %w", err)
		}
	}
	// Permission rows have no cascade hook; purge the role grants first
	// so the catalog and sys_role_permissions stay consistent. Guard the
	// tables because AutoMigrate=false deployments may not have created
	// them yet when Setup fails earlier in the model loop.
	if db.Migrator().HasTable(&models.RolePermission{}) {
		if err := db.Unscoped().
			Where("type = ? AND data IN ?", models.RolePermissionTypePermission, tenantResourcePermissionData).
			Delete(&models.RolePermission{}).Error; err != nil {
			return fmt.Errorf("remove tenant resource grants: %w", err)
		}
	}
	if db.Migrator().HasTable(&models.Permission{}) {
		if err := db.Unscoped().
			Where("data IN ?", tenantResourcePermissionData).
			Delete(&models.Permission{}).Error; err != nil {
			return fmt.Errorf("remove tenant resource permissions: %w", err)
		}
	}
	return nil
}
