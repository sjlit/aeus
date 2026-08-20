import { http } from './http'

/** PermissionService.ListPermissionItem (admin/pb/permission.proto) —
 *  管理 UI 用的完整 catalog 行,带 id / type / data / description / group。 */
export interface PermissionItem {
  id: number
  /** 'api' | 'button' | 'data_scope' —— models.PermissionType
   *  常量,与后端 sys_permissions.type 字段对齐。 */
  type: string
  /** 形如 'POST /system/sys_user'。由后端 derive.go permissionCode 拼装。 */
  data: string
  /** 形如 '创建 用户管理'。 */
  description: string
  /** UI 聚合键,例如 "审计" / "用户"。derive.go 从 MenuEntry().Name 自动
   *  写入,运营手工新增 permission 时也可手填;空串表示「未分组」,UI 兜底
   *  进 "未分组" 组。 */
  group: string
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