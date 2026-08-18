package models

import (
	"context"

	"gorm.io/gorm"
)

// RolePermission is the per-tenant junction between a role and either a
// menu (type=menu) or a catalog permission (type=permission).  The data
// column holds a Menu.Component when type=menu and a Permission.Data
// when type=permission.  No DB FK or CHECK constraint enforces the
// reference; the service layer validates before writing.
type RolePermission struct {
	TenantModel
	// RoleKey holds Role.Key (machine identifier); size:30 matches Role.Key.
	RoleKey string `json:"role_key" yaml:"roleKey" xml:"roleKey" gorm:"size:30;not null;default:'';column:role_key;index:idx_rp_role_key" comment:"角色 Key" rule:"required"`
	Type    string `json:"type" yaml:"type" xml:"type" gorm:"size:20;not null;default:'';column:type" comment:"绑定类型" rule:"required" enum:"menu:菜单;permission:权限"`
	// Data holds Menu.Component when type=menu — Component is size:120,
	// so the width must match (MySQL strict mode rejects the mismatch).
	Data string `json:"data" yaml:"data" xml:"data" gorm:"size:120;not null;default:'';column:data" comment:"绑定目标 (Menu.Component 或 Permission.Data)"`
}

// RolePermission.Type values.
const (
	RolePermissionTypeMenu       = "menu"
	RolePermissionTypePermission = "permission"
)

// TableName returns the physical table name (gorm.Tabler).
func (m *RolePermission) TableName() string {
	return "sys_role_permissions"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *RolePermission) ModuleName() string {
	return "system"
}

// MenuEntry exposes role-permission grants as a navigable item under
// the 用户中心 section.  This is the canonical surface for editing
// per-role menu and permission bindings — separate from Role, which
// only manages the role list itself.  Parent references
// SystemUserCenter; Sort places RolePermission below Role inside the
// section.
func (m *RolePermission) MenuEntry() MenuSpec {
	return MenuSpec{Name: "角色授权", Parent: "SystemUserCenter", Sort: 30}
}

// MenuPermissionDatas returns the distinct Menu.Component values a role
// is granted access to (RolePermission.Type == "menu").  Used by
// UserService.ListVisibleMenus to derive which Menu rows to fetch.
func (*RolePermission) MenuPermissionDatas(db *gorm.DB, ctx context.Context, roleKey string) ([]string, error) {
	var components []string
	err := db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("role_key = ? AND type = ?", roleKey, RolePermissionTypeMenu).
		Distinct("data").
		Pluck("data", &components).Error
	return components, err
}

// APIPermissionDatas returns the distinct Permission.Data values a role
// is granted access to (RolePermission.Type == "permission").  These
// are permission codes (catalog identifiers), not free strings; the
// caller should join against sys_permissions to get descriptions.
func (*RolePermission) APIPermissionDatas(db *gorm.DB, ctx context.Context, roleKey string) ([]string, error) {
	var datas []string
	err := db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("role_key = ? AND type = ?", roleKey, RolePermissionTypePermission).
		Distinct("data").
		Pluck("data", &datas).Error
	return datas, err
}

// ValidateMenuData returns the subset of components that do NOT exist
// in sys_menus.
func (*RolePermission) ValidateMenuData(tx *gorm.DB, ctx context.Context, components []string) ([]string, error) {
	return missingDataValues(tx, ctx, &Menu{}, "component", components)
}

// ValidatePermissionData returns the subset of Permission.Data values
// that do NOT exist in the catalog.
func (*RolePermission) ValidatePermissionData(tx *gorm.DB, ctx context.Context, datas []string) ([]string, error) {
	return missingDataValues(tx, ctx, &Permission{}, "data", datas)
}

// missingDataValues returns the subset of values that do NOT exist in
// the given column of the given model. A single IN query is used
// regardless of len(values) (the upper bound is enforced at the proto
// layer via max_items=1024). An empty input returns nil so callers can
// short-circuit. column is an internal literal (not user input), so the
// concatenation is safe.
func missingDataValues(tx *gorm.DB, ctx context.Context, model any, column string, values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var existing []string
	if err := tx.WithContext(ctx).
		Model(model).
		Where(column+" IN ?", values).
		Pluck(column, &existing).Error; err != nil {
		return nil, err
	}
	have := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		have[n] = struct{}{}
	}
	var missing []string
	for _, n := range values {
		if _, ok := have[n]; !ok {
			missing = append(missing, n)
		}
	}
	return missing, nil
}
