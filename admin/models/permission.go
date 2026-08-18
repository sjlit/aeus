package models

import (
	"context"

	"gorm.io/gorm"
)

// Permission is the global catalog of permission definitions.  The
// junction between roles and these permissions lives in
// sys_role_permissions (RolePermission).  This model is intentionally
// tenant-less — every tenant shares the same catalog, and grants are
// scoped per-tenant via the junction.
type Permission struct {
	BaseModel
	// Data carries the auto-derived "<METHOD> <URI>" pair (e.g.
	// "POST /system/sys_user"); the regex mirrors rest/v3's
	// buildUri output. Width 60 covers deeper paths like
	// "GET /system/sys_user/detail/:id" with room.
	Type        string `json:"type" yaml:"type" xml:"type" gorm:"index;size:20;not null;default:'';column:type" comment:"权限类型" rule:"required" enum:"api:接口;button:按钮;data_scope:数据范围"`
	Data        string `json:"data" yaml:"data" xml:"data" gorm:"index;size:60;not null;default:'';column:data" comment:"权限标识" rule:"required"`
	Description string `json:"description" yaml:"description" xml:"description" gorm:"size:1024;not null;default:'';column:description" comment:"权限说明"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *Permission) TableName() string {
	return "sys_permissions"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *Permission) ModuleName() string {
	return "system"
}

// MenuEntry exposes the permission catalog as a navigable item under
// the 系统设置 section.  Parent references SystemSettings; Sort places
// Permission last inside the section (below Tenant and Menu).
func (m *Permission) MenuEntry() MenuSpec {
	return MenuSpec{Name: "权限管理", Parent: "SystemSettings", Sort: 30}
}

// PermissionType mirrors the proto enum, but as a string so models stay
// proto-free.  The zero value "" is "all" — used by callers who don't
// filter by type.
type PermissionType string

const (
	PermissionTypeUnspec PermissionType = ""
	PermissionTypeAPI    PermissionType = "api"
	PermissionTypeButton PermissionType = "button"
	PermissionTypeData   PermissionType = "data_scope"
)

// ListByType returns the distinct Permission.Data values from the
// catalog, optionally filtered by type.  An empty PermissionType means
// "all".
func (*Permission) ListByType(db *gorm.DB, ctx context.Context, t PermissionType) ([]string, error) {
	q := db.WithContext(ctx).Model(&Permission{}).Distinct("data")
	if t != PermissionTypeUnspec {
		q = q.Where("type = ?", string(t))
	}
	var out []string
	return out, q.Pluck("data", &out).Error
}
