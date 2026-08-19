package models

import "gorm.io/gorm"

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
