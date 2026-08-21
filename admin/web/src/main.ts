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

// 先挂路由 + mount,再后台 bootstrap:
// - 无 token:无操作,用户立即看到 LoginView。
// - 有 stale token:用户先看到 LoginView / Layout 框架,
//   路由守卫在首次导航前再调一次 ensureMenuRoutes() 等菜单数据回来。
//   失败由 http 拦截器统一处理(认证失败 → logout + pushLogin;网络失败 → toast)。
async function bootstrap(): Promise<void> {
  if (!auth.accessToken) return
  try {
    await Promise.all([
      auth.fetchProfile().catch(() => {
        // 跳转到登录页;router 守卫看到 accessToken=null 会放过。
      }),
      ensureMenuRoutes(),
    ])
  } catch {
    // 已在每个分支内部处理;这里吞掉异常,避免 unhandledrejection
  }
}

// 立即挂载,不等待 bootstrap。路由守卫负责按需拉取菜单 / profile;
// 路由被 vue-router 设计为「守卫异步后再放行」,所以首屏跳转也不会
// 落到 catch-all(404)。
app.use(router)
app.mount('#app')

// 后台 bootstrap:不阻塞首屏 paint;若 token 仍在,守卫触发前完成即可。
queueMicrotask(() => void bootstrap())
