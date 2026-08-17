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
  const menu = useMenuStore()
  // '/' 与深链接:确保菜单已加载、路由已注册
  if (to.path === '/' || !isRegistered(to.path)) {
    await menu.load()
    await registerMenuRoutes()
    if (to.path === '/') {
      const first = collectMenuUris(menu.tree)[0]
      return first ? { path: first, replace: true } : true
    }
    // 深链接:注册后重放一次导航;仍不匹配 → 回首页
    if (!isRegistered(to.path)) return { path: '/', replace: true }
  }
  return true
})
