import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// REST_UI_LOCAL=1 时,@sjlit/rest-ui 走本地源码 (/mobe/js/rest-ui/src),
// 改源码即时 HMR;不设置则走 node_modules 里的发布版本。
const restUiLocal = !!process.env.REST_UI_LOCAL
const restUiRoot = '/mobe/js/rest-ui'

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
    alias: [
      {
        find: /^@sjlit\/rest-ui$/,
        replacement: restUiLocal
          ? `${restUiRoot}/src/index.ts`
          : '@sjlit/rest-ui',
      },
      {
        find: '@sjlit/rest-ui/dist/style.css',
        replacement: restUiLocal
          ? `${restUiRoot}/src/styles/index.scss`
          : '@sjlit/rest-ui/dist/style.css',
      },
      {
        find: '@',
        replacement: fileURLToPath(new URL('./src', import.meta.url)),
      },
    ],
    // 本地源码调试时,rest-ui 源码里的 `import 'vue' / 'element-plus'` 按
    // Node 解析会命中 /mobe/js/rest-ui/node_modules 下的另一份拷贝(版本还
    // 不一致),导致 provide/inject 的 Symbol key 对不上 —— App 里的
    // ElConfigProvider locale 传不进 rest-ui 组件(表现为分页等仍是英文)。
    // dedupe 强制统一解析到本项目的拷贝。
    dedupe: restUiLocal
      ? ['vue', 'element-plus', '@element-plus/icons-vue']
      : [],
  },
  server: {
    // 本地源码在项目根目录之外,需要放行文件访问
    fs: {
      allow: restUiLocal
        ? [fileURLToPath(new URL('.', import.meta.url)), restUiRoot]
        : undefined,
    },
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ''),
      },
    },
  },
  // 本地源码调试时不要让 dep optimizer 预打包它(源码含 .vue/.scss 需走 vite 管线)
  optimizeDeps: {
    exclude: restUiLocal ? ['@sjlit/rest-ui'] : [],
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
