/** admin 统一响应信封(responder.go):HTTP 状态恒为 200,业务结果在 code。 */
export interface Envelope<T = unknown> {
  code: number
  message: string
  data?: T
}

/** MenuTree 返回的节点(menu.proto MenuNode)。 */
export interface MenuNode {
  name: string
  title: string
  component: string
  uri: string
  icon: string
  hidden: boolean
  public: boolean
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