import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    // EP 组件 JS 按需引入:CSS 走全量(见 main.ts 的 `element-plus/dist/index.css`),
    // 这里只让 resolver 解析模板里的 <el-*>,自动 import 对应 JS 模块;指令
    // (v-loading 等) 也由 directives: true 自动 import。
    //
    // importStyle: false —— 全量 CSS 已经在 main.ts 引入了,这里不再让
    // resolver 给每个组件再 import 一份 CSS,避免重复。
    Components({
      resolvers: [
        ElementPlusResolver({
          importStyle: false,
          directives: true,
        }),
      ],
      // dts 让 IDE 能识别自动导入的组件类型(无需手写 import)。
      dts: 'src/components.d.ts',
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ''),
      },
    },
  },
  build: {
    target: 'es2022',
    cssCodeSplit: true,
    sourcemap: false,
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        // vendor 分包:EP、vue 生态、rest-ui、axios 各走独立 chunk,
        // 浏览器可独立缓存;同时保证路由懒加载 chunk 不背 vendor。
        manualChunks: {
          'vendor-vue': ['vue', 'vue-router', 'pinia'],
          'vendor-rest': ['@sjlit/rest-ui'],
          'vendor-axios': ['axios'],
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    // vitest 在 jsdom 下不会处理 .scss / .css,把它们 mock 成空模块,
    // 否则 unplugin-vue-components 自动 import element-plus CSS 会抛
    // "Unknown file extension '.css'"。生产构建走 vite 自身的 CSS 管线,
    // 不受此处影响。
    css: {
      include: [],
    },
    // 让 element-plus 走 vite 的依赖内联管线,vite 自带 CSS 处理,
    // 避免 vitest 默认把它当纯 ESM 处理 .css 时报 Unknown file extension。
    server: {
      deps: {
        inline: ['element-plus', '@element-plus/icons-vue', '@sjlit/rest-ui'],
      },
    },
  },
})
