import { http } from './http'

/** PermissionService.ListPermissionItem (admin/pb/permission.proto) —
 *  管理 UI 用的完整 catalog 行,带 id / type / data / description。 */
export interface PermissionItem {
  id: number
  /** 'api' | 'button' | 'data_scope' —— models.PermissionType
   *  常量,与后端 sys_permissions.type 字段对齐。 */
  type: string
  /** 形如 'POST /system/sys_user'。由后端 derive.go permissionCode 拼装。 */
  data: string
  /** 形如 '创建 用户管理'。 */
  description: string
}

/** 拆 'POST /system/sys_user' 为 uri 部分。derive.go permissionCode 保证
 *  输出永远 'METHOD<空格>URI' 格式;无空格的 row 是损坏数据,UI 跳过。 */
export function parseApiUri(data: string): string {
  const idx = data.indexOf(' ')
  return idx < 0 ? '' : data.slice(idx + 1)
}

/** GET /permission/list?type=api —— 拉取指定类型的完整权限(用于权限配置 UI)。
 *  type 传 '' 时返回全量;默认 'api' 匹配分配页当前需求。 */
export function fetchPermissionList(
  type: string = 'api',
): Promise<{ items: PermissionItem[] }> {
  return http.get<{ items: PermissionItem[] }>(
    `/permission/list?type=${encodeURIComponent(type)}`,
  )
}