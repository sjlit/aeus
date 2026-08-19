import { http } from './http'
import type { LoginResponse, RefreshTokenResponse } from '../types'

export async function login(form: { username: string; password: string }): Promise<LoginResponse> {
  const data = await http.post<LoginResponse>('/auth/login', form)
  return data
}

export async function refresh(refreshToken: string): Promise<RefreshTokenResponse> {
  const data = await http.post<RefreshTokenResponse>('/auth/refresh-token', {
    refresh_token: refreshToken,
  })
  return data
}

export async function logout(accessToken: string): Promise<void> {
  await http.post('/auth/logout', { access_token: accessToken })
}