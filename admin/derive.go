package admin

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/rest/v3"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// pascalWords returns PascalCase of s with both "_" and "-" as word
// boundaries — the union mirrors what the front-end's
// deriveComponentName helper (admin/web/src/router/viewPath.ts) does
// for keep-alive component-name lookups, so the auto-generated
// Vue template's defineOptions name matches whichever segment
// convention a future model happens to use.
//
// Empty input returns "". Empty segments (caused by leading /
// trailing / consecutive separators) are dropped so "__sys_users"
// -> "SysUsers" rather than "_SysUsers".
func pascalWords(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		r, size := utf8.DecodeRuneInString(p)
		out = append(out, string(unicode.ToUpper(r))+strings.ToLower(p[size:]))
	}
	return strings.Join(out, "")
}

// deriveUri returns "/" + module-with-dashes + "/" + table-with-dashes
// when both pieces are present, "" otherwise.  An empty module means
// "no Uri derivation possible" — per the Q3 contract, that becomes a
// NULL sys_menus.uri, with the frontend falling back to the REST
// resource path.  An empty table while module is set is similarly
// treated as "skip"; a half-formed /module/ path is worse than NULL
// for downstream breadcrumb logic.
func deriveUri(module, table string) string {
	if module == "" || table == "" {
		return ""
	}
	return "/" + strings.ReplaceAll(module, "_", "-") +
		"/" + strings.ReplaceAll(table, "_", "-")
}

// deriveViewPath returns the Vue view path "@/views/<module>/<singular>/Index.vue"
// for the resource's registered model.  Mirrors deriveUri's empty-means-skip
// contract: returns "" when either ModuleName or Singular is empty, so a caller
// that needs both Uri and ViewPath can short-circuit on a single empty check.
//
// ModuleName is lowercased (not dash-converted) because Vue file paths use the
// bare lowercase name as the directory — "system" rather than "sys-".  Singular
// comes from rest/v3's Naming (populated from gorm.Tabler + its internal
// inflector), so it stays consistent with the URI rest/v3 actually mounts.
//
// An override via MenuSpec.ViewPath wins when the singular misjudges (truly
// irregular plurals), so the framework is forgiving rather than relying on
// inflection being exhaustive.
func deriveViewPath(resource *rest.Resource) string {
	n := resource.ModelValue().GetNaming()
	if n.ModuleName == "" || n.Singular == "" {
		return ""
	}
	return path.Join("@/views", strings.ToLower(n.ModuleName), n.Singular, "Index.vue")
}

// modelNaming resolves (module, table) from rest/v3's Naming, which is the
// canonical source for both fields: ModuleName comes from the model's
// rest.ModuleNamer implementation (or empty if absent) and TableName from
// gorm.Tabler.  Either field being absent yields an empty string — the caller
// decides whether that's a skip signal or a Uri input.
func (s *Server) modelNaming(resource *rest.Resource) (module, table string) {
	n := resource.ModelValue().GetNaming()
	return n.ModuleName, n.TableName
}

// fillDerivedSpec populates Component, Uri, and ViewPath from model
// metadata when the spec leaves them blank.  Models that implement
// neither rest.ModuleNamer nor gorm.Tabler get an empty Component —
// callers must treat that as "skip menu creation" rather than letting
// it through as a real but unique-collision-prone identifier.
//
// modelNaming is two type assertions; calling it unconditionally
// keeps the per-field guards below trivial to read.  Takes
// *rest.Resource so the helper stays symmetric with modelNaming and
// ensurePermissionRows; the underlying model is recovered via
// resource.ModelValue().
func (s *Server) fillDerivedSpec(resource *rest.Resource, spec models.MenuSpec) models.MenuSpec {
	module, table := s.modelNaming(resource)
	if spec.Component == "" {
		spec.Component = pascalWords(module) + pascalWords(table)
	}
	if spec.Uri == "" {
		spec.Uri = deriveUri(module, table)
	}
	if spec.ViewPath == "" {
		spec.ViewPath = deriveViewPath(resource)
	}
	return spec
}

// ensureMenuRow FirstOrCreates one sys_menus row keyed on Component.
// Existing rows are NEVER updated — manual overrides (icon, sort,
// parent rearranged by an operator) survive subsequent server starts.
// Returns created=true only when the row was newly inserted.
//
// Unscoped() is required: Menu carries gorm.DeletedAt, so the default
// First would add `deleted_at IS NULL` and treat a previously
// soft-deleted row as "missing" — then INSERT would collide with the
// unique Component index and fail.  Treating a soft-deleted row as
// "exists, do nothing" preserves the operator's intent (delete) while
// keeping Setup idempotent.
//
// Called from Server.registerModel when the registered model
// implements MenuProvider, so the same auto-menu logic applies to
// built-in models wired via Setup's getModels() loop AND to
// application models registered from outside the loop.
func (s *Server) ensureMenuRow(db *gorm.DB, spec models.MenuSpec) (created bool, err error) {
	if spec.Component == "" || spec.Name == "" {
		return false, nil
	}
	var existing models.Menu
	err = db.Unscoped().Where("component = ?", spec.Component).First(&existing).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	row := models.Menu{
		Component:   spec.Component,
		Uri:         spec.Uri,
		ViewPath:    spec.ViewPath,
		Name:        spec.Name,
		Icon:        spec.Icon,
		Parent:      spec.Parent,
		Sort:        spec.Sort,
		Hidden:      spec.Hidden,
		Public:      spec.Public,
		Description: spec.Description,
	}
	return true, db.Create(&row).Error
}

// validateMenuParentsRef scans sys_menus for any row whose Parent
// points at a Component that doesn't exist.  An orphan Parent is
// silent data corruption: Menu.BuildTree defensively promotes the row
// to a root, but the navigation structure an operator wired up via
// MenuEntry().Parent is gone, and the resulting menu tree puts the
// item at the wrong level with no log/return path to spot it.
//
// Setup runs this at the end of its registration loop.  External
// callers that batch their own registerModel calls — for example, an
// application that registers app-specific models after Setup — should
// also call this once at the end of their batch to surface dangling
// references as a single error.
//
// The check covers every row in sys_menus, not just ones this Setup
// run touched — pre-existing orphans from operator mistakes or prior
// buggy runs should surface here too.  Soft-deleted parents are NOT
// counted as "exists" (default scope), so a parent that was deleted
// without cleanup is also reported.
func (s *Server) validateMenuParentsRef(db *gorm.DB) error {
	var rows []models.Menu
	if err := db.Find(&rows).Error; err != nil {
		return fmt.Errorf("list sys_menus: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	have := make(map[string]struct{}, len(rows))
	for _, m := range rows {
		have[m.Component] = struct{}{}
	}
	var orphans []string
	for _, m := range rows {
		if m.Parent == "" {
			continue
		}
		if _, ok := have[m.Parent]; !ok {
			orphans = append(orphans, fmt.Sprintf("%q -> %q", m.Component, m.Parent))
		}
	}
	if len(orphans) > 0 {
		return fmt.Errorf("%d menu row(s) reference missing parent: %s",
			len(orphans), strings.Join(orphans, "; "))
	}
	return nil
}

// _canonicalPermissionScenarios is the scenario set rest/v3's buildUri
// actually mounts routes for.  The schema package also defines the
// ScenarioList / ScenarioImport constants, but buildUri has no matching
// case, so no permission should be registered for them either.
//
// "openapi" is a rest/v3 internal literal (not part of the schema.*
// constant set), used only by the OpenAPI JSON endpoint.  It is not an
// authorizable business API, so it is excluded here — PermissionChecker
// treats `*/openapi.json` as infrastructure and lets JWT-authenticated
// users read it. If an application declares it explicitly via
// ScenarioProvider, a permission row is generated from the literal
// (also the intended behavior).
var _canonicalPermissionScenarios = []string{
	schema.ScenarioCreate,
	schema.ScenarioUpdate,
	schema.ScenarioDelete,
	schema.ScenarioSearch,
	schema.ScenarioDetail,
	schema.ScenarioExport,
}

// scenarioCNLabel returns the Chinese verb for a scenario, used when
// building the permission Description.  Unknown scenarios fall back to
// the raw string so the description is never empty.
func scenarioCNLabel(scenario string) string {
	switch scenario {
	case schema.ScenarioCreate:
		return "创建"
	case schema.ScenarioUpdate:
		return "更新"
	case schema.ScenarioDelete:
		return "删除"
	case schema.ScenarioSearch:
		return "列表"
	case schema.ScenarioDetail:
		return "详情"
	case schema.ScenarioExport:
		return "导出"
	}
	return scenario
}

// permissionScenarios resolves the scenario set declared by a model.
//
//   - Models implementing rest.ScenarioProvider register only the
//     scenarios they declare.  This mirrors rest/v3's route mounting —
//     HasScenario decides which routes get registered, and permissions
//     should mirror that same set.
//   - Otherwise it falls back to all 6 canonical scenarios, matching
//     the default behavior of every aeus built-in model (no
//     ScenarioProvider implementation = every scenario exposed).
//
// The default branch returns a fresh slice so callers never share the
// package-level slice's backing array; the ScenarioProvider branch
// passes the model's own declaration through as-is (the model is
// responsible for not reusing/mutating the returned slice).
//
// Takes *rest.Resource rather than the raw model any so the helper
// stays symmetric with modelNaming and ensurePermissionRows; the
// underlying model is recovered via resource.ModelValue().
func permissionScenarios(resource *rest.Resource) []string {
	if resource == nil {
		return nil
	}
	// A model that declares its own ScenarioProvider keeps ownership of
	// the list (including custom scenario literals). Otherwise ask the
	// resolved rest.Model which canonical scenarios it actually mounted;
	// this is what respects the read-only WithScenarios(...) override
	// applied for security/audit tables, so we never create write
	// permission rows for routes that are not mounted.
	modelType := resource.ModelValue().ModelType()
	if sp, ok := reflect.New(modelType).Interface().(rest.ScenarioProvider); ok {
		return sp.Scenarios()
	}
	out := make([]string, 0, len(_canonicalPermissionScenarios))
	for _, scenario := range _canonicalPermissionScenarios {
		if resource.ModelValue().HasScenario(scenario) {
			out = append(out, scenario)
		}
	}
	return out
}

// permissionResourceLabel extracts the model's human-readable Chinese
// label, used as the body of Description.  MenuEntry().Name wins (every
// built-in admin model implements MenuProvider and maintains it
// already); the module/table path is the fallback.
//
// No new interface is introduced: MenuProvider is reused from the menu
// feature rather than adding an aeus abstraction.  This keeps
// permission auto-registration non-invasive for models — it only reads
// MenuEntry's existing fields.
//
// Takes *rest.Resource rather than the raw model any so the helper
// stays symmetric with modelNaming and ensurePermissionRows; the
// underlying model is recovered via resource.ModelValue().
func permissionResourceLabel(resource *rest.Resource) string {
	if resource == nil {
		return ""
	}
	modelType := resource.ModelValue().ModelType()
	if p, ok := reflect.New(modelType).Interface().(models.MenuProvider); ok {
		if name := p.MenuEntry().Name; name != "" {
			return name
		}
	}
	module, table := resource.ModelValue().GetNaming().ModuleName,
		resource.ModelValue().GetNaming().TableName
	if module != "" && table != "" {
		return module + "/" + table
	}
	return table
}

// permissionGroupProvider lets a model override the permission group
// without changing its menu label. Generated business entities expose
// this when (aeus.admin).permission_group is set.
type permissionGroupProvider interface {
	PermissionGroup() string
}

func permissionGroup(resource *rest.Resource) string {
	if resource == nil {
		return ""
	}
	modelType := resource.ModelValue().ModelType()
	if p, ok := reflect.New(modelType).Interface().(permissionGroupProvider); ok {
		if group := p.PermissionGroup(); group != "" {
			return group
		}
	}
	return permissionResourceLabel(resource)
}

// permissionCode builds the Data, Description and Group of one
// permission row.  Data looks like "POST /system/sys_user" (METHOD +
// single space + URI); Description looks like "创建 用户管理"; Group
// looks like "用户管理" (the menu's Chinese name — the same label the
// sidebar shows for the matching module/table).  Group is what the
// admin UI uses to cluster permissions under one el-collapse item;
// keeping it auto-derived from MenuEntry().Name means every model that
// registers through Setup gets the right cluster without per-model
// configuration.  Operators can override Group by editing the row
// directly; ensurePermissionRows never overwrites a populated value on
// a re-seed. A model implementing permissionGroupProvider can override
// the group while keeping its menu label for descriptions.
//
// Takes *rest.Resource so the same model source is used to read
// ModuleName/TableName (via resource.ModelValue().GetNaming()) and to
// detect MenuProvider/ScenarioProvider — three queries against one
// canonical handle instead of three.
func permissionCode(resource *rest.Resource, scenario string) (data, description, group string) {
	if resource == nil {
		return "", "", ""
	}
	method, uri := resource.BuildUri(scenario)
	if uri == "" {
		return "", "", ""
	}
	// path.Join with an empty prefix returns "system/sys_user" without
	// a leading slash.  Permission.Data wire format has always carried
	// the leading slash (matching HTTP request URLs and the form
	// validator's permissionDataPattern), so we re-add it explicitly.
	if uri[0] != '/' {
		uri = "/" + uri
	}
	label := permissionResourceLabel(resource)
	return fmt.Sprintf("%s %s", method, uri),
		fmt.Sprintf("%s %s", scenarioCNLabel(scenario), label),
		permissionGroup(resource)
}

// _endpointPermissionCodes registers the metadata/business RPC endpoints
// in the permission catalog so they are RBAC-enforced like any other API.
// With the checker's fail-closed default, an uncatalogued route is denied;
// cataloging these rows is what lets Seed grant them to super roles and
// lets operators grant them to other roles deliberately.
//
// Data matches the gin route TEMPLATE exactly: transport/http writes
// RequestRoute as gin's FullPath, and the checker compares
// "<METHOD> <FullPath>" verbatim, so ":module"/":table" literals must
// be preserved.  Seed's grantFullCatalog diff picks these rows up on
// the next boot and grants them to super roles automatically; ordinary
// roles receive them through the role-permission UI as needed.
var _endpointPermissionCodes = []struct {
	data, description, group string
}{
	// Model metadata endpoints.
	{"GET /rest/model-types/:module/:table", "选项查询 元数据下拉", "系统"},
	{"GET /rest/model-tiers/:module/:table", "层级查询 树形选择", "系统"},
	{"GET /schema/:module/:table", "Schema 元数据", "系统"},
	{"GET /dashboard/overview", "查询 工作台概览", "工作台"},

	// Business RPCs. Self-service reads/writes (profile, menu, password,
	// avatar) are also in the checker's built-in allowlist; cataloging
	// them keeps the super-role grant set complete and makes the
	// fail-closed behavior explicit for non-super roles.
	{"GET /role/options", "选项查询 角色下拉", "角色"},
	{"GET /role/permissions", "查询 角色授权", "角色"},
	{"PUT /role/permissions", "更新 角色授权", "角色"},
	{"GET /role/menus", "查询 角色菜单", "角色"},
	{"GET /permission/catalog", "查询 权限目录", "权限"},
	{"GET /permission/list", "查询 权限列表", "权限"},
	{"GET /menu/tree", "查询 菜单树", "菜单"},
	{"GET /menu/all", "查询 全部菜单", "菜单"},
	{"GET /menu/options", "查询 菜单选项", "菜单"},
	{"GET /menu/breadcrumb", "查询 面包屑", "菜单"},
	{"GET /tenant/options", "选项查询 租户下拉", "租户"},
	{"GET /tenant/detail", "查询 租户详情", "租户"},
	{"POST /user/reset-password", "重置 用户密码", "用户"},
}

// ensureEndpointPermissions idempotently inserts one sys_permissions
// row per entry of _endpointPermissionCodes.  Same FirstOrCreate-style
// contract as ensurePermissionRows: existing rows are never touched,
// so operator edits (description / group) survive restarts.
//
// Called from Setup after the model loop. The defensive AutoMigrate is
// gated on Server.AutoMigrate so a production deployment that manages
// its own schema never gets a surprise DDL from Setup.
func (s *Server) ensureEndpointPermissions(db *gorm.DB) error {
	if s.opts.AutoMigrate {
		if err := db.AutoMigrate(&models.Permission{}); err != nil {
			return fmt.Errorf("migrate sys_permissions: %w", err)
		}
	}
	for _, code := range _endpointPermissionCodes {
		var existing models.Permission
		err := db.Unscoped().Where("data = ?", code.data).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup sys_permission %q: %w", code.data, err)
		}
		row := models.Permission{
			Type:        string(models.PermissionTypeAPI),
			Data:        code.data,
			Description: code.description,
			Group:       code.group,
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("auto-create endpoint permission %q: %w", code.data, err)
		}
	}
	return nil
}

// ensurePermissionRows walks one model's scenario set and turns each
// scenario into one sys_permissions row.  AutoMigrate guarantees the
// table exists; FirstOrCreate-style existence checks (Unscoped)
// guarantee idempotency across restarts.
//
// Unlike ensureMenuRow — one row per menu vs. many per permission —
// this is a slice of rows, but the semantics carry over: existing rows
// are left untouched, missing ones are inserted, and fields an operator
// edited manually (Description / Type / Group, etc.) are never
// overwritten.  An existing row whose Group is empty is back-filled
// on re-seed — this lets a database that pre-dates the Group column
// pick up clustering labels in one Setup pass without the operator
// having to touch every row.
//
// overrideScenarios lets per-call RegisterModel options pin the
// scenario set explicitly.  Precedence:
//
//   - override == nil:  fall through to permissionScenarios(resource)
//     (ScenarioProvider or canonical 6).
//   - override != nil:  use exactly this slice, even when empty.  An
//     empty slice suppresses permission seeding for the model
//     entirely, matching the "no api permission catalog for this
//     model" intent — useful for write-only audit / log models whose
//     security posture is enforced elsewhere.
//
// db is s.opts.DB, not resourceDB: permission inserts are plain data
// writes and don't need rest/v3's detached session, which would in
// fact misbehave on auto-timestamp fields because its statement still
// points at the previous model's schema (same root cause as
// ensureMenuRow).
func (s *Server) ensurePermissionRows(db *gorm.DB, resource *rest.Resource, overrideScenarios []string) error {
	if s.opts.AutoMigrate {
		if err := db.AutoMigrate(&models.Permission{}); err != nil {
			return fmt.Errorf("migrate sys_permissions: %w", err)
		}
	}
	module, table := s.modelNaming(resource)
	if module == "" || table == "" {
		// The model has neither ModuleName nor TableName, so nothing
		// can be derived for a permission row — symmetric with
		// ensureMenuRow's "empty Component means skip", this returns nil
		// without error.
		return nil
	}
	scenarios := permissionScenarios(resource)
	if overrideScenarios != nil {
		scenarios = overrideScenarios
	}
	for _, sc := range scenarios {
		data, description, group := permissionCode(resource, sc)
		if data == "" {
			continue
		}
		var existing models.Permission
		err := db.Unscoped().Where("data = ?", data).First(&existing).Error
		if err == nil {
			// Row exists.  Only back-fill Group when empty —
			// never overwrite an operator-set value, even on re-seed.
			if existing.Group == "" && group != "" {
				if err := db.Model(&existing).Update("group", group).Error; err != nil {
					return fmt.Errorf("back-fill group on %q: %w", data, err)
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup sys_permission %q: %w", data, err)
		}
		row := models.Permission{
			Type:        string(models.PermissionTypeAPI),
			Data:        data,
			Description: description,
			Group:       group,
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("auto-create permission %q: %w", data, err)
		}
	}
	return nil
}
