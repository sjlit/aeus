import axios from 'axios'
import { ElMessage } from 'element-plus'
import { envelopeCode, envelopeMessage, isAuthFailureCode } from './envelope'

/** 拦截器需要的运行时依赖,由 main.ts 注入,避免与 stores/router 循环引用。 */
export interface HttpContext {
  getToken(): string | null
  getTenantId(): string | null
  refresh(): Promise<boolean>
  logout(): void
  pushLogin(redirect: string): void
}

let ctx: HttpContext | null = null

/** 必须在应用启动时调用一次。 */
export function bindHttpContext(c: HttpContext): void {
  ctx = c
}

export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE,
  timeout: 10_000,
})

http.interceptors.request.use((config) => {
  if (ctx?.getToken()) {
    config.headers.Authorization = `Bearer ${ctx.getToken()}`
  }
  if (ctx?.getTenantId()) {
    // NOTE: 后端当前从 JWT TenantID claim 派生租户(admin/middleware/auth/jwt.go),
    // 此 header 被静默忽略;契约保留以便后续多租户接入时启用。
    config.headers['X-Tenant-Id'] = ctx.getTenantId()
  }
  return config
})

// 并发 401 共享同一次刷新(single-flight)
let refreshing: Promise<boolean> | null = null

http.interceptors.response.use(
  async (response) => {
    const body = response.data
    const code = envelopeCode(body)
    if (code === 0) {
      // 成功:把信封解包,调用方拿到 data
      response.data = body?.data ?? null
      return response
    }
    if (isAuthFailureCode(code)) {
      // 认证失败:静默刷新一次后重试原请求
      refreshing ??= (ctx?.refresh() ?? Promise.resolve(false)).finally(() => {
        refreshing = null
      })
      let ok = false
      try {
        ok = await refreshing
      } catch {
        // refresh() 自身抛错(网络抖动等):与 refresh 返回 false 同等待遇。
        ctx?.logout()
        ctx?.pushLogin(window.location.hash.replace(/^#/, '') || '/')
        return Promise.reject(new Error('session expired'))
      }
      if (ok) {
        // 仅复用原请求配置;Authorization 由请求拦截器在重试时重新注入。
        const retryConfig = { ...response.config }
        return http.request(retryConfig)
      }
      ctx?.logout()
      ctx?.pushLogin(window.location.hash.replace(/^#/, '') || '/')
      return Promise.reject(new Error('session expired'))
    }
    const msg = envelopeMessage(body) || `业务错误 code=${code}`
    ElMessage.error(msg)
    return Promise.reject(new Error(msg))
  },
  (error) => {
    // 网络层失败(后端未启动等)
    if (!error.response) {
      ElMessage.error('Network unreachable')
    }
    return Promise.reject(error)
  },
)