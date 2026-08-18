import { createRouter, createWebHashHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import DefaultLayout from '../layouts/DefaultLayout.vue'
import LoginView from '../views/LoginView.vue'
import PlaceholderView from '../views/PlaceholderView.vue'
import NotFoundView from '../views/NotFoundView.vue'
import { useAuthStore } from '../stores/auth'
import { useMenuStore } from '../stores/menu'
import { useTabsStore } from '../stores/tabs'
import type { MenuNode } from '../types'

export const LOGIN_PATH = '/login'

/** 登录跳转统一出口:路由守卫、http 拦截器、用户菜单都从这里构造跳转。 */
export function goToLogin(redirect?: string) {
  return { path: LOGIN_PATH, query: redirect ? { redirect } : undefined }
}

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

// 已注册过的菜单树引用;树未变时 registerMenuRoutes 直接返回,避免每次导航重走 DFS。
let registeredFor: MenuNode[] | null = null
// 并发导航共享同一次加载(single-flight),避免同时打开多个 tab/深链时重复请求菜单。
let readiness: Promise<void> | null = null

function registerMenuRoutes(menu: ReturnType<typeof useMenuStore>): void {
  if (menu.tree === registeredFor) return
  registeredFor = menu.tree
  for (const uri of menu.uris) {
    if (router.hasRoute(`menu:${uri}`)) continue
    router.addRoute('default', {
      path: uri,
      name: `menu:${uri}`,
      component: PlaceholderView,
      // componentName: keep-alive include 白名单(多标签缓存)按它匹配,
      // 取组件文件名;真实页面落地后各自文件名即组件名,无需额外维护
      meta: { title: menu.titlesByUri.get(uri), componentName: 'PlaceholderView' },
    })
  }
}

/**
 * 保证菜单已加载且对应路由已注册——菜单就绪的唯一入口(幂等、单飞)。
 * main.ts 启动时与路由守卫都调用它,组件无需再自行 load/register。
 */
export function ensureMenuRoutes(): Promise<void> {
  readiness ??= (async () => {
    const menu = useMenuStore()
    try {
      await menu.load()
    } catch {
      // 网络/业务错误已由 http 拦截器 toast;不阻断导航 → 用户至少能进 layout 看到空菜单
    }
    registerMenuRoutes(menu)
  })().finally(() => {
    readiness = null
  })
  return readiness
}

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.path === LOGIN_PATH) {
    // 已登录访问登录页 → 回首页
    if (auth.accessToken) return { path: '/', replace: true }
    return true
  }
  if (!auth.accessToken) {
    return { ...goToLogin(to.fullPath), replace: true }
  }

  // 已登录:保证菜单已加载、路由已注册(ensureMenuRoutes 幂等,树未变时几乎零成本)。
  // 关键:不要在 addRoute 之后用 `return to` 重放当前导航(vue-router 4 不一定
  // 重算 to.matched);用「未注册则重定向到 /」保证渲染的一定是已知路由。
  await ensureMenuRoutes()
  const menu = useMenuStore()

  if (to.path === '/') {
    const first = menu.uris[0]
    return first ? { path: first, replace: true } : true
  }

  // 路径不在菜单里 → 重定向到 '/'('/' 分支会再次处理)
  if (!router.hasRoute(`menu:${to.path}`)) {
    return { path: '/', replace: true }
  }

  return true
})

// 多标签:菜单页(带 meta.title)自动开标签;首页(菜单第一项)不可关闭。
// 此时菜单必已加载(beforeEach 保证),home 推导安全;closable 随每次导航
// 刷新,落地页变更后旧会话持久化的标签也能自愈(见 tabs store addTab)。
router.afterEach((to) => {
  const title = to.meta?.title as string | undefined
  if (!title) return
  const menu = useMenuStore()
  const tabs = useTabsStore()
  tabs.addTab({
    path: to.path,
    name: to.name as string,
    title,
    icon: menu.iconsByUri.get(to.path),
    closable: to.path !== menu.uris[0],
    query: to.query as Record<string, string>,
  })
})
