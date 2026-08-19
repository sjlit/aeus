import { http } from './http'
import type { LoginResponse, RefreshTokenResponse } from '../types'

export function login(form: { username: string; password: string }): Promise<LoginResponse> {
  return http.post<LoginResponse>('/auth/login', form)
}

export function refresh(refreshToken: string): Promise<RefreshTokenResponse> {
  return http.post<RefreshTokenResponse>('/auth/refresh-token', {
    refresh_token: refreshToken,
  })
}

export function logout(accessToken: string): Promise<void> {
  return http.post<void>('/auth/logout', { access_token: accessToken })
}
