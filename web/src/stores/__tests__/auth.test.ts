import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../auth'
import * as authApi from '../../api/auth'

vi.mock('../../api/auth', () => ({
  login: vi.fn(),
  refresh: vi.fn(),
  logout: vi.fn(),
}))

const loginResp = {
  uid: 'admin',
  username: 'admin',
  expires: 7200,
  access_token: 'at-1',
  refresh_token: 'rt-1',
  tenant_id: 't-1',
  tenant_name: '默认租户',
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('login 保存 token、profile 并持久化 access_token', async () => {
    vi.mocked(authApi.login).mockResolvedValue(loginResp)
    const s = useAuthStore()
    await s.login({ username: 'admin', password: 'Admin123' })
    expect(s.accessToken).toBe('at-1')
    expect(s.refreshToken).toBe('rt-1')
    expect(s.profile?.username).toBe('admin')
    expect(s.profile?.tenant_name).toBe('默认租户')
    expect(localStorage.getItem('aeus.access')).toBe('at-1')
    expect(authApi.login).toHaveBeenCalledWith({ username: 'admin', password: 'Admin123' })
  })

  it('refresh 只更新 token/expiresAt,不覆盖 profile', async () => {
    vi.mocked(authApi.login).mockResolvedValue(loginResp)
    const s = useAuthStore()
    await s.login({ username: 'admin', password: 'Admin123' })
    vi.mocked(authApi.refresh).mockResolvedValue({ uid: 'admin', expires: 3600, access_token: 'at-2' })
    const ok = await s.refresh()
    expect(ok).toBe(true)
    expect(s.accessToken).toBe('at-2')
    expect(s.refreshToken).toBe('rt-1') // refresh token 不轮换
    expect(s.profile?.username).toBe('admin') // profile 未被覆盖
    expect(s.expiresAt).toBeGreaterThan(Date.now() + 3500 * 1000)
    expect(localStorage.getItem('aeus.access')).toBe('at-2')
  })

  it('无 refreshToken 时 refresh 返回 false', async () => {
    const s = useAuthStore()
    expect(await s.refresh()).toBe(false)
  })

  it('logout 调用接口并清空状态与 localStorage', async () => {
    vi.mocked(authApi.login).mockResolvedValue(loginResp)
    const s = useAuthStore()
    await s.login({ username: 'admin', password: 'Admin123' })
    vi.mocked(authApi.logout).mockResolvedValue(undefined)
    await s.logout()
    expect(authApi.logout).toHaveBeenCalledWith('at-1')
    expect(s.accessToken).toBeNull()
    expect(s.refreshToken).toBeNull()
    expect(s.profile).toBeNull()
    expect(localStorage.getItem('aeus.access')).toBeNull()
  })
})