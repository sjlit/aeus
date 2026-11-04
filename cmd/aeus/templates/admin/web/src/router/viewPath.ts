/**
 * 服务端下发的 view_path(如 "@/views/system/sys_user/Index.vue")
 * 转成 import.meta.glob 查表用的相对路径 key。
 *
 * 路由文件位于 src/router/,所以 src/views/... 对应 ../views/...。
 * "@/" 是 Vite 配置里指向 src/ 的别名;"/" 开头按项目根处理。
 * 已是相对路径的形式原样返回。
 */
export function normalizeGlobPath(viewPath: string): string {
  if (viewPath.startsWith('@/')) return '../' + viewPath.slice(2)
  if (viewPath.startsWith('/')) return '..' + viewPath
  return viewPath
}

/**
 * 从服务端下发的 view_path 派生出 stable component name。
 * 用于 <keep-alive :include> 与 SFC 的 defineOptions({ name }) 对齐:
 * 路由 meta.componentName 和 view 文件组件 name 一一对应,避免异步组件包装层
 * 丢失内部 name,导致 keep-alive 把所有视图视为「不缓存」而丢弃切换前的状态。
 *
 * 派生规则:
 * - 路径中第一个 views/ 之后各段用 PascalCase 拼起
 * - "-"/"_" 用作分隔符(转 camelCase)
 * - 去掉扩展名(.vue / .ts / .tsx 等)
 *
 * 例:
 *   "@/views/system/sys_user/Index.vue"      -> "SystemSysUserIndex"
 *   "../views/workspace/overview/Index.vue"   -> "WorkspaceOverviewIndex"
 *   "views/foo/Bar.vue"                       -> "FooBar"
 *   "@/views/foo/Bar.vue"                     -> "FooBar"
 */
export function deriveComponentName(viewPath: string | undefined): string {
  if (!viewPath) return ''
  const m = viewPath.match(/(?:^|\/)views\/(.+)$/)
  if (!m) return ''
  const stripped = m[1].replace(/\.[^./]+$/, '')
  return stripped
    .split('/')
    .filter(Boolean)
    .map(pascalize)
    .join('')
}

function pascalize(seg: string): string {
  // snake_case / kebab-case -> PascalCase("sys_user" -> "SysUser","foo-bar" -> "FooBar")
  // 已是大写开头的段("Bar")也保持兼容
  return seg
    .replace(/[-_]+(.)?/g, (_, c) => (c ? c.toUpperCase() : ''))
    .replace(/^(.)/, (_, c) => c.toUpperCase())
}