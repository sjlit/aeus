package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// sectionMenuSpecs lists the first-level navigation containers created
// by EnsureSectionMenus.  Each entry has a stable Component (used by
// future MenuEntry().Parent references once Setup gains a pre-loop
// insertion step) and a Chinese Name shown in the sidebar.  Uri and
// ViewPath are intentionally empty — section containers are not
// navigable; the sidebar's resolveIcon treats them as plain-text
// grouping headers today (uri="" + no children → nav-group-label), and
// a future frontend pass will render them as proper section dividers.
//
// Icons follow Element Plus naming (@element-plus/icons-vue) so the
// sidebar's resolveIcon lookup hits the export directly.  Operators
// can rename / re-icon via /system/sys-menus; EnsureSectionMenus is
// FirstOrCreate and never overwrites an existing row.
//
// Adding / reordering rows here is safe; changing an existing
// Component is not — Setup's auto-registration keys on the model's
// derived Component and Parent references would silently break.
var sectionMenuSpecs = []models.MenuSpec{
	{Component: "SystemUserCenter", Name: "用户中心", Icon: "UserFilled", Sort: 10},
	{Component: "SystemLogs", Name: "日志记录", Icon: "Tickets", Sort: 20},
	{Component: "SystemSettings", Name: "系统设置", Icon: "Tools", Sort: 30},
}

// rolePermissionMenuSpec registers the dedicated role-permission
// assignment page reachable from /system/sys_role/perm/:roleKey.
// Parent references the auto-derived SystemSysRoles component
// (pascalWords("system") + pascalWords("sys_roles")); that row is
// inserted by Setup via ensureMenuRow when sys_role registers, so by
// the time Seed runs the parent is guaranteed to exist.
//
// Component matches the Vue page's defineOptions({ name: ... })
// (= "SystemSysRolesPermission") so <keep-alive :include> in
// router/index.ts can match the resolved component name against
// tabs.cachedViews.
//
// Uri carries the :roleKey placeholder verbatim — vue-router's hash
// mode treats it as a dynamic segment and the per-row Index.vue
// link (router-link :to=`/system/sys_role/perm/${key}`) is the only
// way to land here.
var rolePermissionMenuSpec = models.MenuSpec{
	Component: "SystemSysRolesPermission",
	Parent:    "SystemSysRoles",
	Name:      "权限配置",
	Uri:       "/system/sys_role/perm/:roleKey",
	ViewPath:  "@/views/system/sys_role/Permission.vue",
	Sort:      10,
}

// EnsureSectionMenus FirstOrCreates each sectionMenuSpecs entry into
// sys_menus.  Idempotent — runs on every Seed call without duplicating
// rows.  Existing rows are NEVER overwritten, so operator renames and
// icon changes survive subsequent server starts.
//
// Scope: this function only creates the section containers.  It does
// NOT re-point child menus' Parent at these containers — that would
// require Setup to insert section menus BEFORE validateMenuParentsRef
// runs (currently impossible without modifying Setup), and changing
// MenuEntry().Parent to reference these Components on every child
// model.  After this Seed-only step, sys_menus carries 3 standalone
// section rows (Parent="") in addition to the 9 child rows that
// MenuEntry() registers; the actual nesting / sidebar grouping is a
// separate task.
//
// Unscoped() mirrors ensureMenuRow's contract: a previously
// soft-deleted row counts as "exists, do nothing" so an operator who
// removed a section via /system/sys-menus is not silently resurrected.
func EnsureSectionMenus(db *gorm.DB) error {
	for _, spec := range sectionMenuSpecs {
		if spec.Component == "" || spec.Name == "" {
			continue
		}
		var existing models.Menu
		err := db.Unscoped().Where("component = ?", spec.Component).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup section menu %q: %w", spec.Component, err)
		}
		row := models.Menu{
			Component: spec.Component,
			Name:      spec.Name,
			Icon:      spec.Icon,
			Sort:      spec.Sort,
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("insert section menu %q: %w", spec.Component, err)
		}
	}
	return nil
}

// EnsureRolePermissionMenu FirstOrCreates the role-permission
// assignment page row (rolePermissionMenuSpec).  Idempotent on
// Component; the operator's manual edits survive subsequent runs the
// same way EnsureSectionMenus preserves section containers.
//
// Runs from Seed after Setup has populated the auto-derived
// SystemSysRoles row (ensureMenuRow fires during sys_role's
// registerModel call), so the Parent reference always resolves to a
// real Component on a fresh database.  On legacy databases that
// pre-date the auto-menu wiring, the parent might still be missing —
// in that case the INSERT succeeds (no FK on Parent) and the row
// simply renders as a top-level sidebar entry until the operator
// re-parents it through /system/sys-menus.
func EnsureRolePermissionMenu(db *gorm.DB) error {
	spec := rolePermissionMenuSpec
	if spec.Component == "" || spec.Name == "" {
		return nil
	}
	var existing models.Menu
	err := db.Unscoped().Where("component = ?", spec.Component).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup role permission menu %q: %w", spec.Component, err)
	}
	row := models.Menu{
		Component: spec.Component,
		Parent:    spec.Parent,
		Name:      spec.Name,
		Uri:       spec.Uri,
		ViewPath:  spec.ViewPath,
		Sort:      spec.Sort,
	}
	return db.Create(&row).Error
}

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
//   - a default sys_tenants entity row: id = the fixed uuid
//     "00000000-0000-0000-0000-000000000000", Name="默认租户",
//     Status="enabled".  Materialized BEFORE the role/user ensures
//     so a fresh role's tenant_id and the user that follows both
//     resolve to a real tenant row in the same pass.
//   - a sys_tenants entity row for the role's tenant when it differs
//     from the default: Name="默认租户", Status="enabled",
//     id = the role's tenant_id.  A pre-existing role's tenant id is
//     followed, so databases bootstrapped before the tenant model
//     existed converge to a real tenant row instead of an orphan uuid.
//   - full grants for every IsSuper role: each run diffs the global
//     catalogs (sys_menus components + sys_permissions datas) against
//     the role's sys_role_permissions rows and inserts the missing
//     ones — add-only.  Models registered by an application upgrade
//     are picked up automatically, and grants deleted out-of-band are
//     healed; manual edits to a super role's grants are rejected at
//     the API layer (RoleService.ReplaceRolePermissions), this heal is
//     the safety net for anything that slips through.
//   - per-tenant sys_schemas clones: every active row in sys_tenants
//     gets its own copy of every schema template row — those written
//     by rest/v3's schema.AutoMigrate with tenant_id left at the Go
//     zero value (empty string).  The copy's tenant_id is set to the
//     tenant id, primary key / timestamps reset so the database
//     assigns fresh values, and the (module_name, table_name, column)
//     triple is the per-tenant uniqueness key.  Runs LAST so any
//     new schema templates registered by an application upgrade are
//     picked up on the next Seed call and fanned out to every tenant
//     in a single pass.
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

	tenantModel := &models.Tenant{
		ID:     "00000000-0000-0000-0000-000000000000",
		Name:   "默认租户",
		Status: "enabled",
	}

	roleModel := &models.Role{
		TenantModel: models.TenantModel{TenantID: tenantModel.ID},
		Key:         "admin",
		Name:        "系统管理员",
		Status:      "enabled",
		Builtin:     true,
		IsSuper:     true,
		DataScope:   "all",
	}

	userModel := &models.User{
		TenantModel: models.TenantModel{TenantID: tenantModel.ID},
		UID:         "admin",
		Username:    adminUser,
		RoleKey:     roleModel.Key,
		Status:      "normal",
		Password:    adminPassword, // BeforeCreate bcrypts it
		Gender:      "other",
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Section containers are inserted FIRST so the structural menu
		// catalog is in place before role / user / grants; this is also
		// the only place Seed touches the menu catalog — Setup owns the
		// model-derived menus, Seed owns the section scaffolding.
		if err := EnsureSectionMenus(tx); err != nil {
			return err
		}

		// Dedicated role-permission assignment page (/system/sys_role/perm/:roleKey).
		// Parented at the auto-derived SystemSysRoles component which
		// Setup inserted via ensureMenuRow when sys_role registered;
		// FirstOrCreate so an operator who deleted the row via
		// /system/sys-menus is not silently resurrected.
		if err := EnsureRolePermissionMenu(tx); err != nil {
			return err
		}

		// Ensure the default tenant row exists, mirroring userModel's
		// FirstOrCreate below.  Idempotent — re-runs on every Seed
		// without touching a row that's already there.
		if err := tx.Where("id = ?", tenantModel.ID).Attrs(*tenantModel).FirstOrCreate(tenantModel).Error; err != nil {
			return err
		}

		// FirstOrCreate with a key-clause is the canonical "ensure exists"
		// idiom.  RowsAffected==0 means the row was already there (any
		// tenant_id) and has been loaded into role — including its
		// existing tenant_id, which the user ensure below follows.
		res := tx.Where("key = ?", roleModel.Key).Attrs(*roleModel).FirstOrCreate(roleModel)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 && roleModel.Builtin && !roleModel.IsSuper {
			// Upgrade a pre-existing builtin admin role created before
			// IsSuper existed so the converge pass below covers it.
			if err := tx.Model(&models.Role{}).
				Where("id = ?", roleModel.ID).
				Update("is_super", true).Error; err != nil {
				return err
			}
			roleModel.IsSuper = true
		}

		// Ensure a sys_tenants entity row for the role's tenant so
		// Login's resolveTenant returns a real name.  The default
		// tenant is already materialized above; this branch only fires
		// when a pre-existing role carries a different tenant id
		// (legacy orphan uuid gains its row here).  Guarded on
		// non-empty: roles created by out-of-band tooling may have no
		// tenant id, and a tenant row keyed on "" would be meaningless.
		if roleModel.TenantID != "" && roleModel.TenantID != tenantModel.ID {
			tenant := &models.Tenant{ID: roleModel.TenantID, Name: "默认租户", Status: "enabled"}
			if err := tx.Where("id = ?", tenant.ID).Attrs(*tenant).FirstOrCreate(tenant).Error; err != nil {
				return err
			}
		}

		// user was built against the fresh tenant before the role lookup;
		// re-point it at the tenant the role actually lives on.
		userModel.TenantID = tenantModel.ID
		if err := tx.Where("uid = ?", userModel.UID).Attrs(*userModel).FirstOrCreate(userModel).Error; err != nil {
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

		// Materialize per-tenant sys_schemas clones last.  Depends on
		// the tenant rows being in place (the default tenant above and
		// any legacy tenant ensured earlier), and on the catalog diff
		// having no business competing for the same rows.  A failure
		// here rolls back the entire transaction so the database never
		// lands in a half-bootstrapped state.
		if err := ensureTenantSchemas(tx); err != nil {
			return err
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

// schemaTuple is the per-tenant uniqueness key for a sys_schemas
// row.  Matches the implicit dedupe used by rest/v3's
// schema.AutoMigrate, which treats a column as uniquely identified
// by its (module, table, column) triple within a given tenant.
type schemaTuple struct {
	module string
	table  string
	column string
}

// ensureTenantSchemas fans the schema templates (every sys_schemas
// row whose tenant_id is the empty string) out to every active row
// in sys_tenants.  Rest/v3's schema.AutoMigrate writes templates
// with TenantID at its Go zero value during Setup — that produces
// a single, global catalog the rest of the codebase queries
// module-by-module — and this function is the bridge that gives
// each tenant its own scoped copy so future per-tenant reads can
// filter by tenant_id without losing the column catalog.
//
// Order of operations inside Seed matters: this runs AFTER
// grantFullCatalog so the role/user/grants converge is already
// durable when the schema clones start landing.  The function
// makes no assumption about which tenants exist beyond what's in
// sys_tenants at call time — adding a new tenant and re-running
// Seed will clone the templates for it.
//
// Idempotent: the (module, table, column) set already present
// for each tenant is computed in memory after a single bulk
// fetch, so a fully-cloned tenant makes this loop a no-op.
// Templates that vanish between Setup and Seed are silently left
// in the per-tenant copies — no DELETE — because rest/v3 does not
// expose a removal path through the schema endpoint, and a
// template that returns later (e.g. after an upgrade) would
// otherwise need a manual cleanup.  Add-only is the safe default.
//
// No row update: if a template's column metadata changes (label,
// type, format, rules, …) the existing per-tenant copy is left
// alone and the operator is expected to reconcile through the
// rest/v3 schema surface.  Updating would silently overwrite any
// per-tenant customization, which we don't yet have a contract
// for.
//
// Seed runs without JWT claims, so the tenant callbacks'
// resolver returns "" and the create-callback bails out before
// backfilling TenantID — explicit TenantID values on each copy
// therefore win, as intended.
func ensureTenantSchemas(tx *gorm.DB) error {
	var tenants []models.Tenant
	if err := tx.Find(&tenants).Error; err != nil {
		return fmt.Errorf("ensureTenantSchemas: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}

	var templates []schema.Schema
	if err := tx.Where("tenant_id = ?", "").Find(&templates).Error; err != nil {
		return fmt.Errorf("ensureTenantSchemas: list schema templates: %w", err)
	}
	if len(templates) == 0 {
		return nil
	}

	for i := range tenants {
		tenant := &tenants[i]
		var existing []schema.Schema
		if err := tx.Where("tenant_id = ?", tenant.ID).
			Find(&existing).Error; err != nil {
			return fmt.Errorf("ensureTenantSchemas: list schemas for tenant %q: %w", tenant.ID, err)
		}
		seen := make(map[schemaTuple]struct{}, len(existing))
		for _, e := range existing {
			seen[schemaTuple{e.ModuleName, e.TableName, e.Column}] = struct{}{}
		}

		missing := make([]schema.Schema, 0, len(templates))
		for _, tmpl := range templates {
			key := schemaTuple{tmpl.ModuleName, tmpl.TableName, tmpl.Column}
			if _, ok := seen[key]; ok {
				continue
			}
			cp := tmpl
			cp.Id = 0        // let the DB assign a fresh primary key
			cp.TenantID = tenant.ID
			cp.CreatedAt = 0 // let autoCreateTime stamp the new row
			cp.UpdatedAt = 0 // let autoUpdateTime stamp the new row
			missing = append(missing, cp)
		}
		if len(missing) == 0 {
			continue
		}
		if err := tx.Create(&missing).Error; err != nil {
			return fmt.Errorf("ensureTenantSchemas: insert tenant schemas for %q: %w", tenant.ID, err)
		}
	}
	return nil
}
