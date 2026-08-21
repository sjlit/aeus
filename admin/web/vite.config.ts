import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    // EP 按需引入:模板里用到 el-* 组件 / v-loading 指令时,自动 import
    // 对应模块与 CSS;避免 main.ts 一次性 app.use(ElementPlus) 引爆全量。
    // importStyle: 'css' 让组件 CSS 走 Vite 自动拆分;noStylesComponents
    // 让 ElMessage / ElMessageBox / ElNotification 这类不挂载到 DOM 的服务
    // 跳过 CSS(它们的样式已合并在 EP 通用弹层样式里)。
    Components({
      resolvers: [
        ElementPlusResolver({
          importStyle: 'css',
          directives: true,
          noStylesComponents: ['ElMessage', 'ElMessageBox', 'ElNotification'],
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
