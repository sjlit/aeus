package models

// Department is a tenant-scoped organizational unit.  ParentID indexes
// for subtree lookups.
type Department struct {
	TenantModel
	// ParentID indexes for subtree lookups ("give me all departments under X");
	// without it the tree query falls back to a full table scan.
	ParentID    uint   `json:"parent_id" yaml:"parentId" xml:"parentId" gorm:"index;column:parent_id" comment:"父级部门" format:"department" live:"type:dropdown;url:/rest/model-tiers/system/sys_departments?parent=parent_id&label=name&value=id&valueType=uint64"`
	Name        string `json:"name" yaml:"name" xml:"name" gorm:"size:120;column:name" comment:"部门名称" rule:"required"`
	Description string `json:"description" yaml:"description" xml:"description" gorm:"size:1024;column:description" comment:"备注说明" scenarios:"create;update;view;export;list" format:"textarea"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *Department) TableName() string {
	return "sys_departments"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *Department) ModuleName() string {
	return "system"
}

// MenuEntry exposes departments as a navigable item under the
// 用户中心 section.  Parent references SystemUserCenter; Sort places
// Department last inside the section (below RolePermission).
func (m *Department) MenuEntry() MenuSpec {
	return MenuSpec{Name: "部门管理", Parent: "SystemUserCenter", Sort: 40}
}
