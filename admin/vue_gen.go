package admin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjlit/rest/v3"
)

// vueTemplate is the canonical Index.vue body generated for each
// auto-registered model.  It mirrors the hand-written sys_users view
// (admin/web/src/views/system/sys_user/Index.vue) one-to-one so that
// what Setup writes on a fresh project and what developers see in
// version control share the same indentation, comment density, and
// component shape.
//
// Two placeholders are substituted at generation time:
//
//   - {{COMPONENT_NAME}} — the PascalCase(module)+PascalCase(plural)
//     component name used in defineOptions (keep-alive :include
//     matching this against router meta.componentName).
//   - {{MODULE}} / {{SINGULAR}} — the (module, singular) tuple passed
//     to <SchemaViewer> as its module/table props.
//   - {{TABLE}} — the physical table name, used as the fallback
//     page title when route.meta.title isn't filled in by the menu
//     loader.
//
// Single-quote / double-quote style is preserved verbatim from the
// template — changing them would invalidate visual diffs against the
// committed file.
const vueTemplate = `<script setup lang="ts">
// 与 router/index.ts 里菜单路由 meta.componentName (= deriveComponentName('@/views/{{MODULE}}/{{SINGULAR}}/Index.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: '{{COMPONENT_NAME}}' })

import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { SchemaViewer } from '@sjlit/rest-ui'

const route = useRoute()

// 页面标题跟随菜单(route meta.title 由路由注册时从菜单写入),
// 避免 SchemaViewer 回落到英文表名
const title = computed(() => (route.meta.title as string | undefined) || '{{TABLE}}')

</script>

<!--
  注意:不要传空的 #gridview 插槽——插槽一旦提供就会替换默认表格渲染,
  空模板等于把整张表藏掉。需要定制表格时,在插槽里给出完整实现。
-->
<template>
  <SchemaViewer module="{{MODULE}}" table="{{TABLE}}" :title="title" />
</template>
`

// vuePathForModel composes the absolute path of the generated
// Index.vue file for one model. The directory layout mirrors
// deriveViewPath (the Vue path stored in sys_menus.view_path), so
// the file ends up at the same logical address the front-end
// resolves from the menu row: <outputDir>/<module>/<singular>/Index.vue.
//
// outputDir is expected to be the conventional Vite `views/`
// directory, already absolute or resolved relative to the project
// root by the caller (so any mkdir -p failures surface here, not
// somewhere down the rabbit hole of Vite alias resolution).
//
// A model with an empty ModuleName or Singular is the same skip
// signal as deriveViewPath — the same edge case (no ModuleNamer or
// gorm.Tabler) that already skips the menu / permission rows.
// Treating it consistently here keeps the on-disk footprint in
// lock-step with the database state.
//
// The path-traversal guard rejects module/singular values that
// contain ".." or path separators. Today, rest/v3's inflector won't
// produce such strings for any plausible input — ModuleName comes
// from a hand-written method and Singular from inflector.Singularize
// of TableName. The guard exists so a future copy-paste of this
// pattern into a less-trusted input source (config-driven TableName
// resolver, runtime-discovered model registry) can't accidentally
// turn the generator into a write-anywhere primitive. Rejected
// inputs are the same empty return as the (empty ModuleName,
// empty Singular) skip signal — quietly no-op at logger-info level.
func vuePathForModel(outputDir string, module, singular string) string {
	if module == "" || singular == "" {
		return ""
	}
	if strings.Contains(module, "..") || strings.Contains(singular, "..") ||
		strings.ContainsAny(module, `/\`) || strings.ContainsAny(singular, `/\`) {
		return ""
	}
	return filepath.Join(outputDir, strings.ToLower(module), singular, "Index.vue")
}

// generateVueFile writes one Index.vue for a single registered
// model, skipping silently when the file already exists. The
// "skip existing" rule is what makes re-runs of Setup idempotent:
// operators that hand-edit the generated view (e.g. swapping the
// default SchemaViewer for a custom <template>) are never
// overwritten.
//
// Returns:
//
//   - written=true, err=nil when the file was newly created;
//   - written=false, err=nil when the file already existed
//     (callers should treat this as a successful no-op);
//   - written=false, err=non-nil on any FS failure (caller decides
//     whether to abort or to log-warn and continue).
//
// The error path is intentionally left for the caller to handle:
// the menu / permission inserts that run before this step should
// never be rolled back just because a Vue write failed (Vue lives
// on the front-end and doesn't affect server correctness), and
// Setup's caller has the logger context that an isolated Warn
// needs.
func generateVueFile(path, content string) (written bool, err error) {
	if path == "" {
		return false, nil
	}
	// os.Stat instead of os.IsNotExist — the latter is a checked
	// call only after the FS round-trip anyway, so checking up
	// front costs nothing and avoids the distinction between
	// "file" and "directory" when an operator has created a
	// placeholder directory at the target location.
	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("stat %s: %w", path, statErr)
	}
	// MkdirAll on every write keeps the call site minimal: a fresh
	// project only needs to set VueOutputDir and re-run Setup.
	// Mode is 0o755 to match what Vue CLI / Vite scaffolds by
	// default — a stricter mode (0o750) would surprise operators
	// who pull the generated tree into a container.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// buildVueContent resolves the four placeholders of vueTemplate
// against one registered model. Splitting generation off from
// generateVueFile lets tests pin each placeholder independently
// without spinning up a tmpdir per assertion.
//
// componentName follows the project's existing convention used in
// admin/web/src/views/system/sys_user/Index.vue: PascalCase of the
// module joined to the PascalCase pluralized table name
// ("system" + "sys_users" -> "SystemSysUsers"). The router's
// deriveComponentName helper computes a slightly different shape
// (PathHasAllSegments joined), but the hand-rolled convention is
// what keeps the on-disk template looking the same — and it
// matches every view already in the repo, so we stay consistent
// with what an operator reading both files side-by-side expects.
//
// Note on case normalisation: vuePathForModel lowercases the
// module for the directory name (matching the conventional Vite
// `views/<module>/<singular>/` layout) while buildVueContent
// applies pascalWords to the same ModuleName. Both inputs converge
// on a single PascalString in the component name regardless of
// input casing ("WORKSPACE" or "Workspace" both -> "Workspace"), so
// the asymmetry is invisible on the front-end — only the dir's
// lowercase form is opinionated.
func buildVueContent(resource *rest.Resource) (string, error) {
	if resource == nil {
		return "", fmt.Errorf("buildVueContent: nil resource")
	}
	n := resource.ModelValue().GetNaming()
	module := n.ModuleName
	table := n.TableName
	singular := n.Singular
	if module == "" || table == "" || singular == "" {
		// Empty module / table / singular is the no-menu no-permission
		// skip signal (see modelNaming) — propagating it as an empty
		// body keeps registerModel's caller from writing a half-
		// populated view (e.g. one with module="" rendered into the
		// template, which SchemaViewer would then quietly reject
		// at runtime).
		return "", nil
	}
	componentName := pascalWords(module) + pascalWords(n.Pluralize)
	body := vueTemplate
	body = strings.ReplaceAll(body, "{{MODULE}}", strings.ToLower(module))
	body = strings.ReplaceAll(body, "{{TABLE}}", table)
	body = strings.ReplaceAll(body, "{{SINGULAR}}", singular)
	body = strings.ReplaceAll(body, "{{COMPONENT_NAME}}", componentName)
	return body, nil
}
