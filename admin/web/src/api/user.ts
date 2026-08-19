import { http } from './http'
import type { UserProfile } from '../types'

/** GET /user/profile —— 当前登录用户的资料(信封已由 http 拦截器解包)。 */
export async function fetchProfile(): Promise<UserProfile> {
  const data = await http.get<UserProfile>('/user/profile')
  return data
}