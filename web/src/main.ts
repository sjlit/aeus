import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import { router, registerMenuRoutes } from './router'
import { bindHttpContext } from './api/http'
import { useAuthStore } from './stores/auth'
import { useMenuStore } from './stores/menu'
import './styles/app.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(ElementPlus)

const auth = useAuthStore()
bindHttpContext({
  getToken: () => auth.accessToken,
  getTenantId: () => auth.profile?.tenant_id ?? null,
  refresh: () => auth.refresh(),
  logout: () => void auth.logout(),
  pushLogin: (redirect) => {
    void router.push({ path: '/login', query: { redirect } })
  },
})

// 启动时 bootstrap:有 token 时拉取 profile + 菜单并预注册路由,
// 避免刷新后出现 user-pill 为空、菜单页 404 的问题。
// 失败由 http 拦截器统一处理(认证失败 → logout + pushLogin;网络失败 → toast)。
// 用 IIFE 避免顶层 await(Vite 默认 target 不支持)。
const menu = useMenuStore()
async function bootstrap(): Promise<void> {
  if (!auth.accessToken) return
  await Promise.all([
    auth.fetchProfile().catch(() => {}),
    menu.load().catch(() => {}),
  ])
  if (menu.tree.length > 0) {
    registerMenuRoutes()
  }
}

bootstrap().finally(() => {
  app.mount('#app')
})