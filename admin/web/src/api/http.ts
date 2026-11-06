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
    // 文件下载（CSV/Blob）不是 JSON 信封，直接返回 AxiosResponse，
    // 交给 rest-ui 的 CRUD.__downloadFile 处理 headers/文件名。
    if (
      response.config?.responseType === 'blob' ||
      response.config?.responseType === 'arraybuffer'
    ) {
      return response as unknown as AxiosResponse
    }
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

    const body = error.response.data
    const code = envelopeCode(body)
    const message = envelopeMessage(body)

    // 传输层错误(JWT/限流/权限)也携带同一个 envelope。认证类错误
    // 优先尝试静默 refresh；其余错误至少把后端 message toast 出来,
    // 避免 axios 把 403/429 当异常后前端毫无提示。
    //
    // 4005 只在 HTTP 非 2xx 时视为会话失效:admin responder 也会用
    // 4005 表示"旧密码错误/非超管重置密码",那些响应是 HTTP 200,
    // 走成功拦截器,不应触发 refresh/logout。
    if (
      error.response.status === 401 ||
      (error.response.status === 403 && code === 4005) ||
      isAuthFailureCode(code)
    ) {
      return refreshAndRetry(error.config)
    }
    // 5xx 不把后端原始 message(可能含 SQL/内部路径)直接展示给用户。
    if (error.response.status >= 500) {
      ElMessage.error('internal server error')
      return Promise.reject(error)
    }
    if (error.response.status === 429) {
      const retryAfter = error.response.headers?.['retry-after']
      ElMessage.error(
        message || (retryAfter
          ? `too many attempts, retry after ${retryAfter}s`
          : 'too many attempts'),
      )
      return Promise.reject(error)
    }
    if (message) {
      ElMessage.error(message)
    } else {
      ElMessage.error(`HTTP ${error.response.status}`)
    }
    return Promise.reject(error)
  },
)