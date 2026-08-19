import { http } from './http'
import type { MenuNode, UserProfile } from '../types'

/** GET /user/profile —— 当前登录用户的资料(信封已由 http 拦截器解包)。 */
export function fetchProfile(): Promise<UserProfile> {
  return http.get<UserProfile>('/user/profile')
}

/** GET /user/menus 信封 data 为 { total_count, menus } (user.proto
 *  ListVisibleMenusResponse)。后端以平铺形式下发,每行带 parent
 *  字段,前端 store.load 用 buildTree 构造成嵌套树。 */
export async function fetchMenuTree(): Promise<MenuNode[]> {
  const data = await http.get<{ menus: MenuNode[] }>('/user/menus')
  return data?.menus ?? []
}
