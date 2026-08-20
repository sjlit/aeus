import { http } from './http'

/** RoleService.RolePermissionsResponse (admin/pb/role.proto) —
 *  当前角色的菜单 / API 授权快照。type=UNSPECIFIED 时 menus + apis
 *  都有值;type=MENU/API 时只填对应一边。 */
export interface RolePermissions {
  menus: string[]
  apis: string[]
}

/** GET /role/permissions?role={role}&type=0 — 拉取角色授权快照。
 *  type 固定为 0(UNSPECIFIED)——分配页两个 tab 都需要。 */
export function fetchRolePermissions(role: string): Promise<RolePermissions> {
  return http.get<RolePermissions>(
    `/role/permissions?role=${encodeURIComponent(role)}&type=0`,
  )
}

/** PUT /role/permissions — 整体替换该角色的菜单 + API 授权。
 *  后端是单事务整体替换(见 role.proto ReplaceRolePermissions),
 *  所以前端每次保存都必须带「不变项」,否则另一侧会被清空。 */
export function replaceRolePermissions(payload: {
  role: string
  menus: string[]
  apis: string[]
}): Promise<void> {
  return http.put<void>('/role/permissions', payload)
}