/** admin 统一响应信封(responder.go):HTTP 状态恒为 200,业务结果在 code。 */
export interface Envelope<T = unknown> {
  code: number
  message: string
  data?: T
}

/** /user/menus 返回的节点(menu.proto MenuEntry)。
 *  后端以平铺形式下发(每行带 parent 字段),前端 buildTree 构造
 *  children 树;组件(component)作为稳定 ID 参与路由与面包屑,name
 *  作为显示标题。section container(parent="", uri="")由前端识别
 * 为分组标题。 */
export interface MenuNode {
  name: string
  /** Menu.Component,稳定标识(后端 UserCenter/Logs/Settings 等);用于路由 key、面包屑与 Parent 引用。 */
  component: string
  /** 父节点的 component;空串表示顶级节点(section container 即典型顶级)。 */
  parent: string
  /**
   * 服务端下发的视图路径,相对 src/views,不含扩展名。
   * 例 "@/views/system/sys_user/Index.vue" → 客户端做归一化。
   * 空串表示该节点不路由(纯分组,如 section container)。
   */
  view_path: string
  /** 路由 URI(/system/sys-users);空串表示该节点不路由(纯分组)。 */
  uri: string
  icon: string
  hidden: boolean
  public: boolean
  /** 由 buildTree 从 parent 字段构造;原始 /user/menus 响应里这个数组是空。 */
  children: MenuNode[]
}

/** POST /auth/login 响应(auth.proto LoginResponse)。 */
export interface LoginResponse {
  uid: string
  username: string
  expires: number
  access_token: string
  refresh_token: string
  tenant_id: string
  tenant_name: string
}

/** POST /auth/refresh-token 响应:只回填 uid/expires/access_token。 */
export interface RefreshTokenResponse {
  uid?: string
  expires: number
  access_token: string
}

/** GET /user/profile 响应(user.proto UserProfile)。
   后端不返回 tenant_* 字段;租户来自 LoginResponse,
   所以这里只覆盖用户自身属性。 */
export interface UserProfile {
  uid: string
  username: string
  email: string
  gender: string
  description: string
  avatar: string
  role: string
  dept_id: number
}