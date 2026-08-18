package models

import (
	"context"

	"gorm.io/gorm"
)

// BeforeUpdate synchronizes two cascade scenarios:
//  1. When Key changes, re-point the references in sys_role_permissions
//     / sys_users at the new Key.  Historically sys_users.role_key was
//     missed, so a rename would strip the user of every permission
//     (claims.Role decoupled from sys_users.role_key).
//  2. When DeletedAt changes (soft delete), purge the RolePermission
//     rows under this Key.  AfterDelete only fires on a real DELETE, so
//     the soft-delete path must be intercepted here — otherwise the key
//     would resurrect with stale permissions attached.
//
// m.Key / m.TenantID are only populated after GORM loads the row; when
// a column update goes through db.Model(...).Update(...) directly,
// m.TenantID is empty, and blindly appending tenant_id = "" would match
// zero rows and leave orphans.  Both scenarios therefore SELECT the row
// first to get old.Key / old.TenantID.
func (m *Role) BeforeUpdate(tx *gorm.DB) error {
	keyChanged := tx.Statement.Changed("Key")
	deletedChanged := tx.Statement.Changed("DeletedAt")
	// db.Model(&role).Update("`key`", v) goes through the column-update
	// path: GORM stuffs the new value into tx.Statement.Dest as
	// map[string]any{"`key`": v}, while the role.Key field itself is
	// untouched, so Changed("Key") returns false.  Inspect the dest map
	// as a fallback; backticks are required for MySQL, and both
	// spellings are accepted here.
	newKey := ""
	if dest, ok := tx.Statement.Dest.(map[string]any); ok {
		if v, ok := dest["`key`"].(string); ok {
			newKey = v
		} else if v, ok := dest["key"].(string); ok {
			newKey = v
		}
	}
	if newKey != "" {
		keyChanged = true
	}
	if !keyChanged && !deletedChanged {
		return nil
	}
	db := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true})
	var old Role
	if err := db.Where("id = ?", m.ID).First(&old).Error; err != nil {
		return err
	}
	if old.TenantID == "" {
		return nil
	}

	if keyChanged {
		if newKey == "" {
			newKey = m.Key
		}
		if newKey != "" && newKey != old.Key {
			// Re-point sys_role_permissions.role_key
			if err := db.Model(&RolePermission{}).
				Where("role_key = ? AND tenant_id = ?", old.Key, old.TenantID).
				Update("role_key", newKey).Error; err != nil {
				return err
			}
			// Re-point sys_users.role_key
			if err := db.Model(&User{}).
				Where("role_key = ? AND tenant_id = ?", old.Key, old.TenantID).
				Update("role_key", newKey).Error; err != nil {
				return err
			}
		}
	}

	if deletedChanged {
		// Soft delete: purge the RolePermission rows under the old Key
		// so the role can't resurrect with stale permissions.  AfterDelete
		// does not fire here (this is an Update, not a DELETE).
		if err := db.Where("role_key = ? AND tenant_id = ?", old.Key, old.TenantID).
			Delete(&RolePermission{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// AfterDelete purges the role's RolePermission rows after a hard
// delete, avoiding orphans.  The soft-delete path is handled by
// BeforeUpdate (via the DeletedAt change); this only covers hard
// deletes.
func (m *Role) AfterDelete(tx *gorm.DB) error {
	if m.TenantID == "" {
		return nil
	}
	return purgeRolePermissions(tx, func(db *gorm.DB) *gorm.DB {
		return db.Where("role_key = ? AND tenant_id = ?", m.Key, m.TenantID)
	})
}

// Role is a tenant-scoped role.  Key is the stable machine identifier
// within a tenant (read-only after creation); Name is the
// human-readable name (renameable); Status controls whether the role
// is enabled; Builtin marks system-preset roles that must not be
// deleted or have their Key changed; DataScope is the row-level data
// scope.
type Role struct {
	TenantModel
	Key         string `json:"key" yaml:"key" xml:"key" gorm:"size:30;column:key;index:idx_sys_roles_key" comment:"角色 Key(机器标识,创建后不可改)" props:"readonly:update" rule:"required;regexp:^[a-z][a-z0-9_]*$"`
	Name        string `json:"name" yaml:"name" xml:"name" gorm:"size:60;column:name;index:idx_role_tenant_name" comment:"角色名称(可改名)" rule:"required"`
	Status      string `json:"status" yaml:"status" xml:"status" gorm:"size:20;default:enabled;column:status" comment:"状态" scenarios:"create;update;list;export" enum:"enabled:启用;disabled:禁用"`
	Builtin     bool   `json:"builtin" yaml:"builtin" xml:"builtin" gorm:"default:false;column:builtin" comment:"系统预置(禁止删除/改 Key)" scenarios:"list;view"`
	IsSuper     bool   `json:"is_super" yaml:"isSuper" xml:"isSuper" gorm:"default:false;column:is_super" comment:"超级管理员(拥有全部权限,授权自动管理)" scenarios:"create;update;list;view"`
	DataScope   string `json:"data_scope" yaml:"dataScope" xml:"dataScope" gorm:"size:20;default:all;column:data_scope" comment:"数据范围" scenarios:"create;update;view" enum:"all:全部;dept:本部门;self:仅本人;custom:自定义"`
	Sort        int64  `json:"sort" yaml:"sort" xml:"sort" gorm:"default:0;column:sort" comment:"排序" scenarios:"create;update;list"`
	CreatedBy   string `json:"created_by" yaml:"createdBy" xml:"createdBy" gorm:"size:20;column:created_by" comment:"创建人" scenarios:"view"`
	Description string `json:"description" yaml:"description" xml:"description" gorm:"size:1024;column:description" comment:"备注说明" scenarios:"list;create;update;export" format:"textarea"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *Role) TableName() string {
	return "sys_roles"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *Role) ModuleName() string {
	return "system"
}

// MenuEntry exposes roles as a navigable item under the 用户中心
// section.  Parent references SystemUserCenter; Sort places Role below
// User and above RolePermission inside the section.
func (m *Role) MenuEntry() MenuSpec {
	return MenuSpec{Name: "角色管理", Parent: "SystemUserCenter", Sort: 20}
}

// ReplacePermissions atomically deletes every existing sys_role_permissions
// row for the given role key and writes a fresh batch — one row per
// Menu.Component (type="menu") and one per Permission.Data (type="permission").
// The caller is responsible for the upstream role-exists, menu-component, and
// permission-data checks; this method only does the write.  The transaction
// handle `tx` is supplied so the caller can roll the validation queries and
// the write into a single atomic unit (see service/role.go
// ReplaceRolePermissions).
func (*Role) ReplacePermissions(tx *gorm.DB, ctx context.Context, roleKey string, menus, perms []string) error {
	if err := tx.WithContext(ctx).Where("role_key = ?", roleKey).Delete(&RolePermission{}).Error; err != nil {
		return err
	}
	total := len(menus) + len(perms)
	if total == 0 {
		return nil
	}
	rows := make([]RolePermission, 0, total)
	for _, m := range menus {
		rows = append(rows, RolePermission{RoleKey: roleKey, Type: RolePermissionTypeMenu, Data: m})
	}
	for _, p := range perms {
		rows = append(rows, RolePermission{RoleKey: roleKey, Type: RolePermissionTypePermission, Data: p})
	}
	return tx.WithContext(ctx).Create(&rows).Error
}

// GrantMissingPermissions inserts one sys_role_permissions row per menu /
// permission entry the role does not already hold — the add-only
// counterpart of ReplacePermissions, used by Seed's converge pass so
// existing grants (and revoked-then-healed ones) are never deleted.
//
// tenantID is an explicit parameter because Seed runs without JWT
// claims: the tenant callbacks have nothing to backfill from, and the
// junction is tenant-scoped while Menu / Permission are global
// catalogs.  The diff tracks menu and permission grants separately, so
// a Menu.Component that collides with a Permission.Data string still
// lands in the right type bucket.  Returns nil when nothing is missing.
func (*Role) GrantMissingPermissions(tx *gorm.DB, ctx context.Context, roleKey, tenantID string, menus, perms []string) error {
	if len(menus) == 0 && len(perms) == 0 {
		return nil
	}
	var existing []RolePermission
	if err := tx.WithContext(ctx).
		Where("role_key = ? AND tenant_id = ?", roleKey, tenantID).
		Find(&existing).Error; err != nil {
		return err
	}
	menuHave := make(map[string]struct{}, len(existing))
	permHave := make(map[string]struct{}, len(existing))
	for _, rp := range existing {
		switch rp.Type {
		case RolePermissionTypeMenu:
			menuHave[rp.Data] = struct{}{}
		case RolePermissionTypePermission:
			permHave[rp.Data] = struct{}{}
		}
	}
	rows := make([]RolePermission, 0, len(menus)+len(perms))
	for _, m := range menus {
		if _, ok := menuHave[m]; ok {
			continue
		}
		rows = append(rows, RolePermission{
			TenantModel: TenantModel{TenantID: tenantID},
			RoleKey:     roleKey,
			Type:        RolePermissionTypeMenu,
			Data:        m,
		})
	}
	for _, p := range perms {
		if _, ok := permHave[p]; ok {
			continue
		}
		rows = append(rows, RolePermission{
			TenantModel: TenantModel{TenantID: tenantID},
			RoleKey:     roleKey,
			Type:        RolePermissionTypePermission,
			Data:        p,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&rows).Error
}
