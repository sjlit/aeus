import axios from 'axios'
import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import { envelopeCode, envelopeMessage, isAuthFailureCode } from './envelope'

/** 拦截器需要的运行时依赖,由 main.ts 注入,避免与 stores/router 循环引用。 */
export interface HttpContext {
  getToken(): string | null
  getCurrentPath(): string
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
  const token = ctx?.getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

/** 会话彻底失效:清本地状态并跳登录页,带上当前路径以便登录后回跳。 */
function handleSessionExpired(): Promise<never> {
  if (ctx) {
    ctx.logout()
    ctx.pushLogin(ctx.getCurrentPath())
  }
  return Promise.reject(new Error('session expired'))
}

/** refresh 请求自身:再失败也不能再去 refresh,否则会无限循环。 */
function isRefreshRequest(config: InternalAxiosRequestConfig | undefined): boolean {
  const url = config?.url ?? ''
  // baseURL 已通过 http.defaults.baseURL 拼装,这里只看相对路径段。
  return url.includes('/auth/refresh-token')
}

// 并发 401 共享同一次刷新(single-flight)
let refreshing: Promise<boolean> | null = null

/**
 * 收到鉴权失败(信封 4001/4002/4006 或 HTTP 401)时,先静默 refresh 一次,
 * 成功则用新 token 重试原请求;失败/refresh 抛错/refresh 请求自身失败 → 会话失效。
 */
async function refreshAndRetry(
  config: InternalAxiosRequestConfig | undefined,
): Promise<AxiosResponse> {
  if (!config || isRefreshRequest(config)) return handleSessionExpired()
  refreshing ??= (ctx?.refresh() ?? Promise.resolve(false)).finally(() => {
    refreshing = null
  })
  let ok = false
  try {
    ok = await refreshing
  } catch {
    // refresh() 自身抛错(网络抖动等):与 refresh 返回 false 同等待遇。
    return handleSessionExpired()
  }
  if (!ok) return handleSessionExpired()
  // 仅复用原请求配置;Authorization 由请求拦截器在重试时重新注入。
  return http.request(config)
}

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
      return refreshAndRetry(response.config)
    }
    const msg = envelopeMessage(body) || `业务错误 code=${code}`
    ElMessage.error(msg)
    return Promise.reject(new Error(msg))
  },
  (error) => {
    // 网络层失败(后端未启动等)
    if (!error.response) {
      ElMessage.error('Network unreachable')
      return Promise.reject(error)
    }
    // 后端对 CodeTokenExpired/CodeUnauthorized/CodeTokenInvalid 映射 HTTP 401
    // (pkg/errs/error.go HTTPStatus):同样走静默刷新 → 重试路径,
    // 否则 token 过期时不会自动跳登录页。
    if (error.response.status === 401) {
      return refreshAndRetry(error.config)
    }
    return Promise.reject(error)
  },
)