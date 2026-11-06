/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

interface Window {
  /** public/app-config.js 在应用启动前写入的运行时品牌配置。 */
  __AEUS_CONFIG__?: {
    appName?: string
    appMark?: string
  }
}
