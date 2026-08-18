/** localStorage 安全封装:隐私模式/配额溢出时静默降级,不让存储问题拖垮应用。 */

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
