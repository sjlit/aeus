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