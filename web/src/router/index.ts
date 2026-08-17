import { createRouter, createWebHashHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import DefaultLayout from '../layouts/DefaultLayout.vue'
import LoginView from '../views/LoginView.vue'
import PlaceholderView from '../views/PlaceholderView.vue'
import NotFoundView from '../views/NotFoundView.vue'
import { useAuthStore } from '../stores/auth'
import { useMenuStore } from '../stores/menu'
import { collectMenuUris, titleForUri } from '../stores/menuGroups'

export const LOGIN_PATH = '/login'

// name 必须存在:addRoute('default', …) 以名字引用父路由
const defaultRoute: RouteRecordRaw = {
  path: '/',
  name: 'default',
  component: DefaultLayout,
  children: [],
}

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: LOGIN_PATH, name: 'login', component: LoginView },
    defaultRoute,
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView },
  ],
})

/** 把菜单树里所有非空 uri 注册为业务路由(幂等,去重)。 */
export async function registerMenuRoutes(): Promise<void> {
  const menu = useMenuStore()
  const existing = new Set(
    router.getRoutes().filter((r) => r.name?.toString().startsWith('menu:')).map((r) => r.path),
  )
  for (const uri of collectMenuUris(menu.tree)) {
    if (existing.has(uri)) continue
    router.addRoute('default', {
      path: uri,
      name: `menu:${uri}`,
      component: PlaceholderView,
      meta: { title: titleForUri(uri, menu.tree) },
    })
  }
}

function isRegistered(path: string): boolean {
  return router.getRoutes().some((r) => r.path === path)
}

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.path === LOGIN_PATH) {
    // 已登录访问登录页 → 回首页
    if (auth.accessToken) return { path: '/', replace: true }
    return true
  }
  if (!auth.accessToken) {
    return { path: LOGIN_PATH, query: { redirect: to.fullPath }, replace: true }
  }

  // 已登录:强制保证菜单已加载、路由已注册——bootstrap 是 first line,这里是兜底。
  // 关键:不要在 addRoute 之后用 `return to` 重放当前导航(vue-router 4 不一定
  // 重算 to.matched);用「未注册则重定向到 /」保证渲染的一定是已知路由。
  const menu = useMenuStore()
  if (menu.tree.length === 0) {
    try {
      await menu.load()
    } catch {
      // 网络/业务错误已 toast,这里不阻断 → 让用户至少能进入 layout 看到空菜单
    }
  }
  await registerMenuRoutes()

  if (to.path === '/') {
    const first = collectMenuUris(menu.tree)[0]
    return first ? { path: first, replace: true } : true
  }

  // 路径不在菜单里 → 重定向到 '/'('/' 分支会再次处理)
  if (!isRegistered(to.path)) {
    return { path: '/', replace: true }
  }

  return true
})
