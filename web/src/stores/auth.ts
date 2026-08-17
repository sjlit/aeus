import { defineStore } from 'pinia'
import { login as apiLogin, logout as apiLogout, refresh as apiRefresh } from '../api/auth'
import type { LoginResponse } from '../types'

const LS_ACCESS = 'aeus.access'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    accessToken: localStorage.getItem(LS_ACCESS),
    refreshToken: null as string | null,
    expiresAt: 0,
    profile: null as LoginResponse | null,
  }),
  actions: {
    async login(form: { username: string; password: string }) {
      const resp = await apiLogin(form)
      this.accessToken = resp.access_token
      this.refreshToken = resp.refresh_token
      this.expiresAt = Date.now() + resp.expires * 1000
      this.profile = resp
      localStorage.setItem(LS_ACCESS, resp.access_token)
    },
    // 只更新 token/expiresAt —— RefreshTokenResponse 不含 username/tenant,绝不能覆盖 profile
    async refresh(): Promise<boolean> {
      if (!this.refreshToken) return false
      const resp = await apiRefresh(this.refreshToken)
      this.accessToken = resp.access_token
      this.expiresAt = Date.now() + resp.expires * 1000
      localStorage.setItem(LS_ACCESS, resp.access_token)
      return true
    },
    async logout() {
      try {
        if (this.accessToken) await apiLogout(this.accessToken)
      } catch {
        // 撤销失败(网络等)不阻塞本地登出
      }
      this.accessToken = null
      this.refreshToken = null
      this.expiresAt = 0
      this.profile = null
      localStorage.removeItem(LS_ACCESS)
    },
  },
})