import { http } from './http'
import type { MenuNode, UserProfile } from '../types'

/** GET /user/profile —— 当前登录用户的资料(信封已由 http 拦截器解包)。 */
export function fetchProfile(): Promise<UserProfile> {
  return http.get<UserProfile>('/user/profile')
}

/** PATCH /user/profile —— 部分更新当前用户资料。
 *  username/email/gender 为空字符串时后端保留原值;description 始终生效
 *  (传 '' 表示清空)。成功后回填 UserProfile。 */
export function updateProfile(payload: {
    username?: string
    email?: string
    gender?: string
    description?: string
}): Promise<UserProfile> {
    return http.patch<UserProfile>('/user/profile', payload)
}

/** POST /user/change-password —— 修改自己的密码。
 *  old_password 必须与现库一致;成功后服务端返回空 body,
 *  调用方按成功 toast 即可。 */
export function changePassword(payload: {
    old_password: string
    new_password: string
}): Promise<void> {
    return http.post<void>('/user/change-password', payload)
}

/** GET /user/menus 信封 data 为 { total_count, menus } (user.proto
 *  ListVisibleMenusResponse)。后端以平铺形式下发,每行带 parent
 *  字段,前端 store.load 用 buildTree 构造成嵌套树。 */
export async function fetchMenuTree(): Promise<MenuNode[]> {
  const data = await http.get<{ menus: MenuNode[] }>('/user/menus')
  return data?.menus ?? []
}
