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
app.use(ElementPlus)
// 注意:router **不在这里** use——vue-router 4 在 app.use(router) 后会立刻触发
// 初始导航(microtask),那时 bootstrap 还没拿到菜单。下面 bootstrap 完成后
// 再 use(router),保证首次导航时路由表已经备好。

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
  // bootstrap 完成后再装路由——此时菜单已就位、路由已注册,
  // 首次导航能直接匹配上,不会落到 catch-all(404)。
  app.use(router)
  app.mount('#app')
})