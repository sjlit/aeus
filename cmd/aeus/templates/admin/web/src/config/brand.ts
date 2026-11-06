/** 运行时品牌配置：由 public/app-config.js 在应用启动前挂到 window。 */
export interface BrandRuntimeConfig {
  appName?: string
  appMark?: string
}

export interface Brand {
  name: string
  mark: string
}

const DEFAULT_NAME = 'Board'
const DEFAULT_MARK = 'D'

function clean(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

/**
 * 解析品牌配置。
 *
 * - appName 为空时回退到默认值；
 * - appMark 优先；未配置时自定义名称取首字符，默认名称保留 D；
 * - 运行时配置来自未受类型约束的 JS，所有字段都先做字符串校验。
 */
export function resolveBrand(runtime?: BrandRuntimeConfig | null): Brand {
  const configuredName = clean(runtime?.appName)
  const name = configuredName || DEFAULT_NAME
  const configuredMark = clean(runtime?.appMark)
  const derivedMark = configuredName
    ? Array.from(configuredName)[0] ?? DEFAULT_MARK
    : DEFAULT_MARK
  const markChar = Array.from(configuredMark || derivedMark)[0] ?? DEFAULT_MARK
  const mark = Array.from(markChar.toUpperCase())[0] ?? DEFAULT_MARK
  return { name, mark }
}

export const brand = resolveBrand(
  typeof window !== 'undefined' ? window.__AEUS_CONFIG__ : undefined,
)
