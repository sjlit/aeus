import { createApp } from 'vue'
import { createPinia } from 'pinia'
// 自托管可变字体:仅 latin + latin-ext 子集(见 src/styles/fonts.scss),
// 替代 fontsource 默认 index.css 一次性打包全部子集。
// 必须在 app.scss 之前引入,保证 @font-face 先于使用处加载。
import './styles/fonts.scss'
import App from './App.vue'
import { router, ensureMenuRoutes, goToLogin } from './router'
import { bindHttpContext, http } from './api/http'
import { useAuthStore } from './stores/auth'
import { setTabsRouter } from './stores/tabs'
import { SchemaUIPlugin, SchemaUIConfig } from '@sjlit/rest-ui'
import '@sjlit/rest-ui/dist/style.css'
// EP CSS 全量引入。原因:按需 importStyle: 'css' 不会自动加载命令式 service
// (ElMessage / ElMessageBox / ElNotification) 与共享 base.css(--el-* 变量、
// fade/zoom 动画),需要逐个手动补 base.css + 三个 service CSS 兜底,等于
// 半全量。按需 JS 仍由 vite.config.ts 里的 ElementPlusResolver 自动解析
// 模板里的 <el-*>,这里只把 CSS 一次性补齐。体积差异在可接受范围(主 chunk
// +32 KB raw / +32 KB gzip),换来所有弹层/动画/toast/loading 不再缺样式。
import 'element-plus/dist/index.css'
import './styles/app.scss'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
// 注意:ElementPlus 走 unplugin-vue-components 自动按需引入(vite.config.ts),
// 不再 app.use(ElementPlus)。v-loading 等指令由 resolver 的 directives: true
// 自动 import;ElMessage / ElMessageBox 等服务在用到的地方显式 import。
app.use(SchemaUIPlugin, <SchemaUIConfig>{
  httpClient: http,
  apiPrefix: '',
})

const auth = useAuthStore()
// 多标签的 cachedViews 依赖 router.getRoutes(),必须在首航前注入
// (依赖注入模式,避免 store → router 的循环依赖)
setTabsRouter(router)
bindHttpContext({
  getToken: () => auth.accessToken,
  getCurrentPath: () => router.currentRoute.value.fullPath,
  refresh: () => auth.refresh(),
  logout: () => void auth.logout(),
  pushLogin: (redirect) => {
    void router.push(goToLogin(redirect))
  },
})

// bootstrap 完成后再挂路由。
// 必须 await:vue-router 4 的 pushWithRedirect 在首航时就把 targetLocation.matched
// 锁死,守卫里 router.addRoute() 后续注册的菜单路由不会回填,会落到 catch-all
// 渲染成 404(P0-4 改成 queueMicrotask 后台跑就引入了这个回归)。
async function bootstrap(): Promise<void> {
  if (!auth.accessToken) return
  try {
    await Promise.all([
      auth.fetchProfile().catch((err) => {
        // http 拦截器已分别处理:401 → refreshAndRetry(失败则 logout + pushLogin),
        // 网络/业务错误 → ElMessage.error。这里只吞 promise,dev 下留痕便于排查
        // 「菜单有了但 profile 没回来」之类的中间态。
        if (import.meta.env.DEV) console.warn('[bootstrap] fetchProfile failed:', err)
      }),
      ensureMenuRoutes(), // 内部 try/catch 已吞网络错误,菜单失败不阻断挂载
    ])
  } catch (err) {
    // ensureMenuRoutes 内部已吞错,理论上不会进这里;留兜底避免 unhandledrejection。
    if (import.meta.env.DEV) console.error('[bootstrap] unexpected error:', err)
  }
}

bootstrap().finally(() => {
  app.use(router)
  app.mount('#app')
})
