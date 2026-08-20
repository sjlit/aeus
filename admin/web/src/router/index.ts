import { createRouter, createWebHashHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import DefaultLayout from '../layouts/default/Layout.vue'
import LoginView from '../views/public/LoginView.vue'
import NotFoundView from '../views/public/NotFoundView.vue'
import ProfileView from '../views/system/profile/Index.vue'
import { useAuthStore } from '../stores/auth'
import { useMenuStore } from '../stores/menu'
import { useTabsStore } from '../stores/tabs'
import type { MenuNode } from '../types'
import { normalizeGlobPath, deriveComponentName } from './viewPath'

export const LOGIN_PATH = '/login'
/** 个人中心路由:固定写在前端,不依赖服务端菜单下发,所以也用 `menu:` 前缀,
 *  让 router.beforeEach 里的 hasRoute 检查通过,不会被重定向回首页。 */
export const PROFILE_PATH = '/profile'

export function goToLogin(redirect?: string) {
  return {
    path: LOGIN_PATH,
    query: redirect ? { redirect } : undefined
  }
}

const defaultRoute: RouteRecordRaw = {
  path: '/',
  name: 'default',
  component: DefaultLayout,
  children: [
    {
      // 个人中心:不走菜单分发,前端硬编码;名字用 menu: 前缀以便
      // beforeEach 的 hasRoute 检查命中。挂在 default 下,
      // 这样它会进入 DefaultLayout(顶栏/侧栏/tabs 全部保留)。
      path: PROFILE_PATH,
      name: `menu:${PROFILE_PATH}`,
      component: ProfileView,
      meta: {
        title: '个人中心',
        componentName: deriveComponentName('@/views/system/profile/Index.vue'),
      },
    },
  ],
}

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: LOGIN_PATH,
      name: 'login',
      component: LoginView,
      meta: { componentName: deriveComponentName('@/views/public/LoginView.vue') },
    },
    defaultRoute,
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: NotFoundView,
      meta: { componentName: deriveComponentName('@/views/public/NotFoundView.vue') },
    },
  ],
})

/**
 * Vite 静态扫描 src/views 下全部 .vue,key 是相对路径,value 是懒加载器。
 * 让每个业务页面走独立 chunk,首屏不需要背全部业务代码。
 * view_path 来自服务端,形如 "@/views/system/sys_user/Index.vue",
 * 通过 normalizeGlobPath 转成 "../views/system/sys_user/Index.vue" 作为查表 key。
 */
const viewModules = import.meta.glob('../views/**/*.vue') as Record<string, () => Promise<unknown>>

/** 同一 view_path 不重复查表;命中失败也缓存,避免每次导航都打 warn。 */
const resolvedViewCache = new Map<string, () => Promise<unknown>>()

function resolveView(viewPath: string | undefined): () => Promise<unknown> {
  if (!viewPath) {
    return () => Promise.resolve(NotFoundView)
  }
  if (resolvedViewCache.has(viewPath)) {
    return resolvedViewCache.get(viewPath)!
  }
  const load = viewModules[normalizeGlobPath(viewPath)]
  if (load) {
    resolvedViewCache.set(viewPath, load)
    return load
  }
  if (import.meta.env.DEV) {
    console.warn(
      `[router] view_path "${viewPath}" 对应的视图文件不存在;回落 NotFoundView。`
    )
  }
  const fallback = () => Promise.resolve(NotFoundView)
  resolvedViewCache.set(viewPath, fallback)
  return fallback
}

let registeredFor: MenuNode[] | null = null

let readiness: Promise<void> | null = null

function registerMenuRoutes(menu: ReturnType<typeof useMenuStore>): void {
  if (menu.tree === registeredFor) return
  registeredFor = menu.tree
  for (const uri of menu.uris) {
    if (router.hasRoute(`menu:${uri}`)) continue
    const viewPath = menu.viewsByUri.get(uri)
    // meta.componentName 是 <keep-alive :include>(= tabs.cachedViews) 命中的依据。
    // 优先用服务端 menu.component(稳定 ID,view 文件用 defineOptions({ name })
    // 对齐它);缺失时回退 deriveComponentName(viewPath),避免无 component 字段
    // 的菜单数据破坏多 tab 缓存。
    const componentName =
      menu.componentsByUri.get(uri) ?? deriveComponentName(viewPath)
    router.addRoute('default', {
      path: uri,
      name: `menu:${uri}`,
      component: resolveView(viewPath),
      meta: {
        title: menu.titlesByUri.get(uri),
        componentName,
      },
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
    if (auth.accessToken) return {
      path: '/', replace: true
    }
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
