package models

// Audit is a tenant-scoped operation log row appended by the
// application when a resource changes.
type Audit struct {
	TenantModel
	UID    string `json:"uid" yaml:"uid" xml:"uid" gorm:"index;size:20;column:uid" comment:"用户" format:"user" props:"readonly:update" rule:"required"`
	Action string `json:"action" yaml:"action" xml:"action" gorm:"index;size:20;not null;default:'';column:action" comment:"行为" scenarios:"search;list;create;update;view;export" props:"match:exactly" rule:"required" enum:"create:新建#198754;update:更新#f09d00;delete:删除#e63757"`
	Module string `json:"module" yaml:"module" xml:"module" gorm:"size:60;not null;default:'';column:module" comment:"模块" scenarios:"search;list;create;update;view;export"`
	Table  string `json:"table" yaml:"table" xml:"table" gorm:"size:60;not null;default:'';column:table" comment:"表名" scenarios:"list;create;update;view;export"`
	Data   string `json:"data" yaml:"data" xml:"data" gorm:"size:10240;not null;default:'';column:data" comment:"变更内容" scenarios:"list;create;update;view;export"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *Audit) TableName() string {
	return "sys_audits"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *Audit) ModuleName() string {
	return "system"
}

// MenuEntry exposes the audit log as a navigable item under the
// 日志记录 section.  Parent references SystemLogs; Sort puts Audit
// first, above LoginLog, inside the section.
func (m *Audit) MenuEntry() MenuSpec {
	return MenuSpec{Name: "操作日志", Parent: "SystemLogs", Sort: 10}
}
