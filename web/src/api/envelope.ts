import type { Envelope } from '../types'

/** 触发静默刷新的业务码(pkg/errs/const.go)。 */
export const AUTH_FAILURE_CODES = [4001, 4002, 4006] as const

/** 取信封 code;非信封(网络层错误等)返回 -1。 */
export function envelopeCode(body: unknown): number {
  if (body && typeof body === 'object' && 'code' in body) {
    const c = Number((body as Record<string, unknown>).code)
    return Number.isFinite(c) ? c : -1
  }
  return -1
}

/** 取信封 message;缺失时返回空串。 */
export function envelopeMessage(body: unknown): string {
  if (body && typeof body === 'object' && 'message' in body) {
    const m = (body as Record<string, unknown>).message
    return typeof m === 'string' ? m : ''
  }
  return ''
}

/** code ∈ {4001 Unauthorized, 4002 TokenExpired, 4006 TokenInvalid}。 */
export function isAuthFailureCode(code: number): boolean {
  return (AUTH_FAILURE_CODES as readonly number[]).includes(code)
}

/** 类型守卫:code===0 时 data 可用。 */
export function isOkEnvelope<T = unknown>(body: unknown): body is Envelope<T> {
  return envelopeCode(body) === 0
}