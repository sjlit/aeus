package models

import "gorm.io/gorm"

// DefaultTenantID is the stable id of the tenant Seed creates. In
// single-tenant mode (admin.WithMultiTenant(false), the default) there
// is no GORM tenant callback, but auth still carries a tenant id in the
// JWT and authorization rows are tenant-scoped. Tenant-scoped models
// therefore fall back to this id when the caller leaves TenantID empty,
// so users/roles created through REST and the rows Seed bootstraps share
// one tenant namespace.
const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

type (
	// BaseModel is the shared base embedded by every model: a uint
	// primary key, Unix-second timestamps, and soft delete.
	BaseModel struct {
		ID        uint           `json:"id" yaml:"id" xml:"id" gorm:"primaryKey;column:id"`
		CreatedAt int64          `json:"created_at" yaml:"createdAt" xml:"createdAt" gorm:"column:created_at" comment:"创建时间" scenarios:"view;export"`
		UpdatedAt int64          `json:"updated_at" yaml:"updatedAt" xml:"updatedAt" gorm:"index;autoUpdateTime;column:updated_at" comment:"更新时间" scenarios:"view;export"`
		DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
	}

	// TenantModel embeds BaseModel plus the tenant_id column that
	// scopes every row to its tenant via the GORM callbacks installed
	// by admin.Server.Setup.
	TenantModel struct {
		BaseModel
		TenantID string `json:"tenant_id" gorm:"column:tenant_id;type:char(60);index" scenarios:"view;export" props:"readonly:update" comment:"所属租户"`
	}
)

// ApplyDefaultTenant fills TenantID with DefaultTenantID when the
// caller did not set one. It is called from the BeforeCreate hooks of
// the tenant-scoped models whose rows participate in auth (User, Role,
// RolePermission, Department). Audit/LoginLog deliberately opt out:
// a failed login has no tenant to attribute.
//
// In multi-tenant mode the GORM create callback runs after BeforeCreate
// and overwrites this value with the resolver's tenant, so the fallback
// only has an effect when tenant scoping is disabled.
func (m *TenantModel) ApplyDefaultTenant() {
	if m.TenantID == "" {
		m.TenantID = DefaultTenantID
	}
}
