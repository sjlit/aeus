/** localStorage 安全封装:隐私模式/配额溢出时静默降级,不让存储问题拖垮应用。 */

/** 字符串版:原样读写(token、开关等字符串值用)。 */
export function safeGetString(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

export function safeSetString(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    // 配额/隐私模式:忽略
  }
}

export function safeRemove(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch {
    // 忽略
  }
}

/** JSON 泛型版:读取并 JSON.parse,缺失/损坏返回 fallback(多标签持久化等用)。 */
export function safeGet<T>(key: string, fallback: T): T {
  const raw = safeGetString(key)
  if (raw === null) return fallback
  try {
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

/** JSON 泛型版:JSON.stringify 后写入,失败静默忽略。 */
export function safeSet(key: string, value: unknown): void {
  safeSetString(key, JSON.stringify(value))
}
