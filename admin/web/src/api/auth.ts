import { http } from './http'
import type { LoginResponse, RefreshTokenResponse } from '../types'

export function login(form: { username: string; password: string }): Promise<LoginResponse> {
  return http.post<LoginResponse>('/auth/login', form)
}

// 刷新会同时返回新的 access_token 和轮换后的 refresh_token:
// 旧 refresh_token 已被服务端吊销,调用方必须持久化新值。
export function refresh(refreshToken: string): Promise<RefreshTokenResponse> {
  return http.post<RefreshTokenResponse>('/auth/refresh-token', {
    refresh_token: refreshToken,
  })
}

// 登出吊销 access_token;携带 refresh_token 时一并吊销,
// 防止被盗的 refresh token 在登出后仍可续期。
export function logout(accessToken?: string, refreshToken?: string): Promise<void> {
  return http.post<void>('/auth/logout', {
    ...(accessToken ? { access_token: accessToken } : {}),
    ...(refreshToken ? { refresh_token: refreshToken } : {}),
  })
}
