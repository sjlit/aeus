import { http } from './http'
import type { MenuNode } from '../types'

/** GET /menu/tree 信封 data 为 { items: MenuNode[] }(menu.proto MenuTreeResponse)。 */
export async function fetchMenuTree(): Promise<MenuNode[]> {
  const { data } = await http.get<{ menus: MenuNode[] }>('/user/menus')
  return data?.menus ?? []
}