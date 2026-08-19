import { http } from './http'

/** MenuService.MenuItem (admin/pb/menu.proto) —— 后端 /menu/all
 *  返回的扁平行；前端用 parent 字段自构嵌套树。 字段按 MenuItem
 *  proto 顺序写出,后续新增字段时同步这边。 */
export interface MenuItem {
  id: number
  parent: string
  name: string
  component: string
  uri: string
  view_path: string
  icon: string
  hidden: boolean
  public: boolean
  sort: number
  description: string
  created_at: number
  updated_at: number
}

/** MenuService.MenuOptionsResponse 里 OptionNode —— /menu/options
 *  返回的级联选项。 value = Menu.Component,作为 Menu.Parent 入参。 */
export interface MenuOptionNode {
  value: string
  label: string
  parent: string
  children: MenuOptionNode[]
}

/** MenuService.ListAll 返回 `{ items: MenuItem[] }`(信已被 http 拦截器解包)。 */
export interface MenuListAllResponse {
  items: MenuItem[]
}

/** GET /menu/all —— 全部菜单(扁平,不分页)。 */
export function fetchMenuAll(): Promise<MenuListAllResponse> {
  return http.get<MenuListAllResponse>('/menu/all')
}

/** 父级选择器级联选项。 */
export function fetchMenuOptions(): Promise<{ items: MenuOptionNode[] }> {
  return http.get<{ items: MenuOptionNode[] }>('/menu/options')
}

/** REST CRUD(`/system/sys_menus`)—— 创建/更新/删除走通用 rest/v3 端点,
 *  鉴权 / 校验 / 软删除一律由后端处理,前端只关心字段名。 */
export interface MenuCreatePayload {
  parent: string
  name: string
  component: string
  uri: string
  view_path?: string
  icon?: string
  hidden?: boolean
  public?: boolean
  sort?: number
  description?: string
}

export function createMenu(payload: MenuCreatePayload): Promise<MenuItem> {
  return http.post<MenuItem>('/system/sys_menus', payload)
}

export function updateMenu(id: number, payload: MenuCreatePayload): Promise<MenuItem> {
  return http.put<MenuItem>(`/system/sys_menus/${id}`, payload)
}

export function deleteMenu(id: number): Promise<unknown> {
  return http.delete(`/system/sys_menus/${id}`)
}
