import { defineStore } from 'pinia'
import { login as apiLogin, logout as apiLogout, refresh as apiRefresh } from '../api/auth'
import { fetchProfile as apiFetchProfile } from '../api/user'
import { safeGetString, safeRemove, safeSetString } from '../utils/storage'
import { useMenuStore } from './menu'
import type { LoginResponse, UserProfile } from '../types'

const LS_ACCESS = 'aeus.access'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    accessToken: safeGetString(LS_ACCESS),
    refreshToken: null as string | null,
    profile: null as LoginResponse | null,
    // 来自 GET /user/profile;刷新后用于恢复侧栏用户信息。
    // profile(LoginResponse)保留为登录瞬态数据,登录后会再次被覆盖。
    userProfile: null as UserProfile | null,
  }),
  getters: {
    // userProfile 与 profile 都可能先到,谁非空谁生效;UI 只读这一个,
    // 不用每个组件各自写 `userProfile ?? profile` 的优先级。
    currentUser(): (UserProfile | LoginResponse) | null {
      return this.userProfile ?? this.profile
    },
    displayName(): string {
      return this.currentUser?.username || '—'
    },
    initials(): string {
      const u = this.currentUser?.username ?? ''
      return u.slice(0, 2).toUpperCase() || '?'
    },
  },
  actions: {
    async login(form: { username: string; password: string }) {
      const resp = await apiLogin(form)
      this.accessToken = resp.access_token
      this.refreshToken = resp.refresh_token
      this.profile = resp
      safeSetString(LS_ACCESS, resp.access_token)
      // 换用户后菜单可能不同:清空菜单缓存,路由守卫会在首次导航时重新拉取
      useMenuStore().$reset()
    },
    // 只更新 token —— RefreshTokenResponse 不含 username/tenant,绝不能覆盖 profile
    async refresh(): Promise<boolean> {
      if (!this.refreshToken) return false
      const resp = await apiRefresh(this.refreshToken)
      this.accessToken = resp.access_token
      safeSetString(LS_ACCESS, resp.access_token)
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
      safeRemove(LS_ACCESS)
    },
  },
})
