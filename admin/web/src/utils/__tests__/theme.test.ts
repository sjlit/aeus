import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  applyTheme,
  readStoredTheme,
  resolveTheme,
  storeTheme,
  THEME_STORAGE_KEY,
  type ThemeMode,
} from '../theme'

function mockMatchSystem(prefersDark: boolean | null): void {
  if (prefersDark === null) {
    vi.stubGlobal('matchMedia', undefined)
    return
  }
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: prefersDark,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
  })))
}

describe('theme 工具', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.unstubAllGlobals()
    document.documentElement.removeAttribute('data-theme')
    document.documentElement.style.colorScheme = ''
  })

  it('读取持久化主题;缺失或非法值回退到 system', () => {
    expect(readStoredTheme()).toBe('system')
    localStorage.setItem(THEME_STORAGE_KEY, 'dark')
    expect(readStoredTheme()).toBe('dark')
    localStorage.setItem(THEME_STORAGE_KEY, 'bogus')
    expect(readStoredTheme()).toBe('system')
  })

  it('resolveTheme:显式模式直接返回,system 跟随系统偏好', () => {
    expect(resolveTheme('light')).toBe('light')
    expect(resolveTheme('dark')).toBe('dark')
    mockMatchSystem(true)
    expect(resolveTheme('system')).toBe('dark')
    mockMatchSystem(false)
    expect(resolveTheme('system')).toBe('light')
    // 旧环境无 matchMedia 时回退浅色
    mockMatchSystem(null)
    expect(resolveTheme('system')).toBe('light')
  })

  it('applyTheme 写入 data-theme 与 color-scheme', () => {
    applyTheme('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')

    applyTheme('light')
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(document.documentElement.style.colorScheme).toBe('light')
  })

  it('system 模式下落地的主题随系统偏好变化', () => {
    mockMatchSystem(true)
    applyTheme('system')
    expect(document.documentElement.dataset.theme).toBe('dark')

    mockMatchSystem(false)
    applyTheme('system')
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('storeTheme 持久化后 readStoredTheme 可读回', () => {
    const modes: ThemeMode[] = ['light', 'dark', 'system']
    for (const mode of modes) {
      storeTheme(mode)
      expect(readStoredTheme()).toBe(mode)
    }
  })
})
