package models

import (
	"context"

	"gorm.io/gorm"
)

// AfterDelete purges the RolePermission rows referencing the deleted
// menu, avoiding orphans.  Menu itself carries no tenant_id, so no
// tenant filter is applied; purgeRolePermissions keeps the
// NewDB/SkipHooks boilerplate for this asymmetric choice inside the
// models package.
func (m *Menu) AfterDelete(tx *gorm.DB) error {
	return purgeRolePermissions(tx, func(db *gorm.DB) *gorm.DB {
		return db.Where("type = ?", RolePermissionTypeMenu).Where("data = ?", m.Component)
	})
}

// VisibleMenusByComponents returns the menus whose Component is in
// components, ordered so the frontend can rebuild the tree
// deterministically (parent ASC, then id).
func (*Menu) VisibleMenusByComponents(db *gorm.DB, ctx context.Context, components []string) ([]Menu, error) {
	var menus []Menu
	err := db.WithContext(ctx).
		Where("component IN ?", components).
		Order("parent ASC, id ASC").
		Find(&menus).Error
	return menus, err
}

// Menu is a global (non-tenant-scoped) navigation entry shared across
// tenants.
type Menu struct {
	BaseModel
	// Parent stores a Menu.Component reference, so its width must match
	// Component's size:120 (SQLite masks length issues; MySQL strict mode
	// rejects the mismatch).
	Parent      string `json:"parent" yaml:"parent" xml:"parent" gorm:"index;size:120;column:parent" comment:"父级菜单(引用 Menu.Component)" scenarios:"create;update;view;export" format:"menu" props:"readonly:update" live:"type:dropdown;url:/rest/model-tiers/system/sys_menus?parent=parent&label=name&value=component"`
	Name        string `json:"name" yaml:"name" xml:"name" gorm:"index;size:60;column:name" comment:"菜单标题" props:"readonly:update" rule:"required"`
	Component   string `json:"component" yaml:"component" xml:"component" gorm:"size:120;column:component" comment:"组件名称" rule:"required;unique"`
	Uri         string `json:"uri" yaml:"uri" xml:"uri" gorm:"size:512;column:uri" comment:"菜单链接" scenarios:"create;update;view;export" rule:"required"`
	ViewPath    string `json:"view_path" yaml:"viewPath" xml:"viewPath" gorm:"size:512;column:view_path" comment:"视图路径" scenarios:"create;update;view;export"`
	Icon        string `json:"icon" yaml:"icon" xml:"icon" gorm:"size:60;column:icon" comment:"菜单图标" scenarios:"create;update;view;export"`
	Hidden      bool   `json:"hidden" yaml:"hidden" xml:"hidden" gorm:"column:hidden" comment:"是否隐藏" scenarios:"create;update;view;export"`
	Public      bool   `json:"public" yaml:"public" xml:"public" gorm:"column:public" comment:"是否公开" scenarios:"create;update;view;export"`
	Sort        int64  `json:"sort" yaml:"sort" xml:"sort" gorm:"column:sort" comment:"排序" scenarios:"create;update"`
	Description string `json:"description" yaml:"description" xml:"description" gorm:"size:1024;column:description" comment:"备注说明" scenarios:"create;update;view;export" format:"textarea"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *Menu) TableName() string {
	return "sys_menus"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *Menu) ModuleName() string {
	return "system"
}

// MenuEntry auto-registers this Menu model as a navigable menu item
// under the 系统设置 section so admins can reach /system/sys-menus to
// manage menus themselves.  Parent references SystemSettings; Sort
// places Menu between Tenant and Permission inside the section.
// Component/Uri are left empty so the framework derives them from
// ModuleName + TableName (see MenuProvider doc).
func (m *Menu) MenuEntry() MenuSpec {
	return MenuSpec{Name: "菜单管理", Parent: "SystemSettings", Sort: 20}
}

// MenuSpec describes one auto-generated sys_menus row.  Fields left zero
// are derived from the model:
//
//   - Component: PascalCase(ModuleName) + PascalCase(TableName), e.g.
//     "system" + "sys_users" -> "SystemSysUsers".  Only consulted when the
//     model implements rest.ModuleNamer and gorm.Tabler.
//   - Uri: "/" + module (with "_" -> "-") + "/" + table (with "_" -> "-")
//     when both module and table are set.  An empty module or empty
//     table yields "" so the row stores NULL sys_menus.uri — the
//     frontend falls back to the REST resource path (per Q3).  A
//     half-formed "/module/" path is worse than NULL for downstream
//     breadcrumb logic, so neither side is auto-padded.
//   - ViewPath: derived from module + table by deriveViewPath (the
//     singular form comes from rest/v3's Naming, which uses its
//     internal inflector).  Empty when either side is empty, so the
//     row stores NULL sys_menus.view_path — the frontend falls back to
//     its MENU_VIEW_MAP static table for legacy components.  Callers
//     that want a custom path set ViewPath explicitly in MenuEntry();
//     an empty result is the skip signal.
//
// Parent, Sort, Icon, Name, Hidden, Public, Description carry no
// canonical derivation and MUST be set explicitly by the model's
// MenuEntry() — otherwise the framework skips the row (Component or
// Name empty).  Parent MUST reference an existing Menu.Component;
// Setup fails fast with the dangling reference(s) listed if it does
// not.
type MenuSpec struct {
	Component   string
	Uri         string
	ViewPath    string
	Name        string
	Icon        string
	Parent      string
	Sort        int64
	Hidden      bool
	Public      bool
	Description string
}

// MenuProvider is implemented by models that want admin.Server.Setup to
// auto-create a sys_menus row on their behalf.  ModuleName() and
// TableName() are NOT part of this interface — the framework uses
// rest.ModuleNamer / gorm.Tabler type assertions only when Component or
// Uri in MenuEntry() is empty.
type MenuProvider interface {
	MenuEntry() MenuSpec
}

// MenuTreeNode is a plain Go representation of the menu tree; models
// must not import the generated proto types, so the wire-level MenuNode
// in admin/pb is a separate type. Service.convert.go bridges them.
//
// Component is the stable identifier (matches Menu.Component); Title is
// the human-friendly display label (matches Menu.Name).
type MenuTreeNode struct {
	Component string
	Title     string
	Uri       string
	Icon      string
	Hidden    bool
	Public    bool
	Children  []MenuTreeNode
}

// BuildTree turns a flat slice of Menu rows into a nested []MenuTreeNode
// keyed on Menu.Component (parent references a child's Component).  Rows
// whose parent cannot be matched in the input are surfaced as roots — a
// defensive fallback for orphaned rows, since the model has rule:"unique"
// on Component but no FK constraint on Parent.
func (*Menu) BuildTree(menus []Menu) []MenuTreeNode {
	byComponent := make(map[string]int, len(menus))
	roots := make([]MenuTreeNode, 0)
	for i := range menus {
		byComponent[menus[i].Component] = i
	}
	children := make(map[string][]int, len(menus))
	for i, m := range menus {
		if _, ok := byComponent[m.Parent]; !ok || m.Parent == "" {
			continue
		}
		children[m.Parent] = append(children[m.Parent], i)
	}
	for i, m := range menus {
		_, hasParent := byComponent[m.Parent]
		if m.Parent == "" || !hasParent {
			roots = append(roots, buildSubtree(menus, children, i))
		}
	}
	return roots
}

func buildSubtree(menus []Menu, children map[string][]int, idx int) MenuTreeNode {
	m := menus[idx]
	node := MenuTreeNode{
		Component: m.Component,
		Title:     m.Name,
		Uri:       m.Uri,
		Icon:      m.Icon,
		Hidden:    m.Hidden,
		Public:    m.Public,
	}
	for _, cidx := range children[m.Component] {
		node.Children = append(node.Children, buildSubtree(menus, children, cidx))
	}
	return node
}

// PathTo walks the Parent chain of the row identified by id, returning
// the Components from root to that row.  If a parent link points to a
// missing row, the walk stops and returns what it has (degraded
// behaviour; see spec §7 for the rationale).
func (*Menu) PathTo(db *gorm.DB, ctx context.Context, id uint) ([]string, error) {
	var start Menu
	if err := db.WithContext(ctx).First(&start, id).Error; err != nil {
		return nil, err
	}
	path := []string{start.Component}
	visited := map[string]bool{start.Component: true}
	cur := start.Parent
	for cur != "" {
		if visited[cur] {
			// Parent cycle (e.g. A -> B -> A): stop instead of looping
			// forever. The walk degrades to the path collected so far,
			// mirroring the missing-parent behaviour below.
			break
		}
		visited[cur] = true
		var p Menu
		if err := db.WithContext(ctx).Where("component = ?", cur).First(&p).Error; err != nil {
			break
		}
		path = append([]string{p.Component}, path...)
		cur = p.Parent
	}
	return path, nil
}

// VisibleByRole returns the Menu rows that the given role is granted
// access to.  It composes the two existing primitives in
// models/role_permission.go and models/menu.go rather than adding a new
// query — same plumbing as UserService.ListVisibleMenus but
// parameterised by role.
func (*Menu) VisibleByRole(db *gorm.DB, ctx context.Context, role string) ([]Menu, error) {
	components, err := (&RolePermission{}).MenuPermissionDatas(db, ctx, role)
	if err != nil {
		return nil, err
	}
	if len(components) == 0 {
		return nil, nil
	}
	return (&Menu{}).VisibleMenusByComponents(db, ctx, components)
}
