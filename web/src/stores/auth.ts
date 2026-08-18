import { defineStore } from 'pinia'
import { login as apiLogin, logout as apiLogout, refresh as apiRefresh } from '../api/auth'
import { fetchProfile as apiFetchProfile } from '../api/user'
import type { LoginResponse, UserProfile } from '../types'

const LS_ACCESS = 'aeus.access'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    accessToken: localStorage.getItem(LS_ACCESS),
    refreshToken: null as string | null,
    profile: null as LoginResponse | null,
    // 来自 GET /user/profile;刷新后用于恢复侧栏用户信息。
    // profile(LoginResponse)保留为登录瞬态数据,登录后会再次被覆盖。
    userProfile: null as UserProfile | null,
  }),
  actions: {
    async login(form: { username: string; password: string }) {
      const resp = await apiLogin(form)
      this.accessToken = resp.access_token
      this.refreshToken = resp.refresh_token
      this.profile = resp
      localStorage.setItem(LS_ACCESS, resp.access_token)
    },
    // 只更新 token —— RefreshTokenResponse 不含 username/tenant,绝不能覆盖 profile
    async refresh(): Promise<boolean> {
      if (!this.refreshToken) return false
      const resp = await apiRefresh(this.refreshToken)
      this.accessToken = resp.access_token
      localStorage.setItem(LS_ACCESS, resp.access_token)
      return true
    },
    // 拉取当前用户资料。失败抛错(由调用方/http 拦截器处理);
    // 成功时同步写入 userProfile 供 UI 渲染。
    async fetchProfile(): Promise<UserProfile> {
      const resp = await apiFetchProfile()
      this.userProfile = resp
      return resp
    },
    async logout() {
      try {
        if (this.accessToken) await apiLogout(this.accessToken)
      } catch {
        // 撤销失败(网络等)不阻塞本地登出
      }
      this.accessToken = null
      this.refreshToken = null
      this.profile = null
      this.userProfile = null
      localStorage.removeItem(LS_ACCESS)
    },
  },
})