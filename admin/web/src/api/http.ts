import axios from 'axios'
import type {
  AxiosInstance,
  AxiosRequestConfig,
  AxiosResponse,
  InternalAxiosRequestConfig,
} from 'axios'
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
export function bindHttpContext(context: HttpContext): void {
  ctx = context
}

/**
 * 响应拦截器已解包业务信封,业务码 0 的成功路径直接把 data 作为结果返回。
 * SchemaUIPlugin 等按"data"约定使用的 httpClient 因此可以共用同一个实例。
 */
type UnwrappedHttp = Omit<
  AxiosInstance,
  'get' | 'post' | 'put' | 'delete' | 'patch' | 'request'
> & {
  get<T = unknown>(url: string, config?: AxiosRequestConfig): Promise<T>
  post<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T>
  put<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T>
  delete<T = unknown>(url: string, config?: AxiosRequestConfig): Promise<T>
  patch<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T>
  request<T = unknown>(config: AxiosRequestConfig): Promise<T>
}

export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE,
  timeout: 10_000,
}) as unknown as UnwrappedHttp

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
): Promise<unknown> {
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
  // 重试走本 http,会再次经过响应拦截器,成功时返回的是解包后的 data。
  return http.request(config)
}

http.interceptors.response.use(
  (response) => {
    const body = response.data
    const code = envelopeCode(body)
    if (code === 0) {
      // 成功:把信封解包后直接返回 data,调用方和 SchemaUIPlugin 都不再需要 .data。
      // 这里 cast 是为了让 axios 拦截器签名满意,运行时返回的就是业务数据。
      return (body?.data ?? null) as unknown as AxiosResponse
    }
    if (isAuthFailureCode(code)) {
      // 重试走本 http,响应拦截器会把成功路径再解包为 data;这里 cast 是为了让
      // axios 拦截器签名满意,运行时与成功路径行为一致。
      return refreshAndRetry(response.config) as Promise<AxiosResponse>
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