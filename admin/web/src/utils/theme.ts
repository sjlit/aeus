/**
 * theme · 主题模式解析与 DOM 应用
 * 与 index.html 的内联首帧脚本共用 localStorage key 与取值约定,
 * 保证"首帧前"和"运行时"算出同一个 data-theme,避免闪色。
 */
import { safeGetString, safeSetString } from './storage'

export type ThemeMode = 'light' | 'dark' | 'system'

export const THEME_STORAGE_KEY = 'aeus.theme'

const VALID_MODES: readonly string[] = ['light', 'dark', 'system']

function isThemeMode(value: string | null): value is ThemeMode {
  return value !== null && VALID_MODES.includes(value)
}

/** 读取持久化主题;缺失或非法值回退到"跟随系统"。 */
export function readStoredTheme(): ThemeMode {
  const raw = safeGetString(THEME_STORAGE_KEY)
  return isThemeMode(raw) ? raw : 'system'
}

export function storeTheme(mode: ThemeMode): void {
  safeSetString(THEME_STORAGE_KEY, mode)
}

/** 系统当前是否偏好深色;旧环境回退 false。 */
export function systemPrefersDark(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: dark)').matches
  )
}

/** 把"三态"解析成最终落地的 light / dark。 */
export function resolveTheme(mode: ThemeMode): 'light' | 'dark' {
  if (mode === 'system') return systemPrefersDark() ? 'dark' : 'light'
  return mode
}

/** 应用主题到 <html>:data-theme 驱动 token 覆盖,color-scheme 驱动原生控件/滚动条。 */
export function applyTheme(mode: ThemeMode): void {
  if (typeof document === 'undefined') return
  const resolved = resolveTheme(mode)
  const root = document.documentElement
  root.dataset.theme = resolved
  root.style.colorScheme = resolved
}

/** 监听系统主题变化;返回反注册函数。仅在 mode === 'system' 时才需响应。 */
export function onSystemThemeChange(handler: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }
  // 兼容旧版 Safari
  mq.addListener(handler)
  return () => mq.removeListener(handler)
}
