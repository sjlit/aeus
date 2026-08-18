package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BeforeCreate backfills ID with a fresh uuid when the caller left it
// blank, so tenants created through the REST API (or any other path)
// always carry the stable string id that other tables' tenant_id
// columns reference.
func (m *Tenant) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return nil
}

// Tenant is the root of the tenant tree: its ID is the value every
// TenantModel row carries in tenant_id. It deliberately embeds neither
// BaseModel (uint ID conflict) nor TenantModel (a tenant row must NOT
// be tenant-scoped — the GORM callbacks skip models without a
// tenant_id column, which is exactly the global visibility tenant
// management requires).
type Tenant struct {
	ID        string         `json:"id" yaml:"id" xml:"id" gorm:"primaryKey;column:id;type:char(60)" comment:"租户ID" props:"readonly:update" scenarios:"update;view;list;search;detail;export"`
	Name      string         `json:"name" yaml:"name" xml:"name" gorm:"size:60;column:name" comment:"租户名称" rule:"required" scenarios:"create;update;view;list;search;export"`
	Status    string         `json:"status" yaml:"status" xml:"status" gorm:"size:20;default:enabled;column:status" comment:"状态" scenarios:"create;update;view;list;search;export" enum:"enabled:启用;disabled:禁用"`
	CreatedAt int64          `json:"created_at" yaml:"createdAt" xml:"createdAt" gorm:"column:created_at" comment:"创建时间" scenarios:"view;export"`
	UpdatedAt int64          `json:"updated_at" yaml:"updatedAt" xml:"updatedAt" gorm:"index;autoUpdateTime;column:updated_at" comment:"更新时间" scenarios:"view;export"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *Tenant) TableName() string {
	return "sys_tenants"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *Tenant) ModuleName() string {
	return "system"
}

// MenuEntry exposes tenants as a navigable item under the 系统设置
// section.  Parent references SystemSettings; Sort places Tenant
// first inside the section.
func (m *Tenant) MenuEntry() MenuSpec {
	return MenuSpec{Name: "租户管理", Parent: "SystemSettings", Sort: 10}
}
