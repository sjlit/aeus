/**
 * tabs store · 多标签页状态管理(迁移自参考项目)
 *
 * 功能:
 * - 管理打开的标签列表(路由 afterEach 自动加标签)
 * - 自动持久化到 localStorage
 * - 支持右键菜单操作(关闭其他、关闭左侧、关闭右侧)
 * - 配合 KeepAlive 实现页面缓存与「刷新当前页」
 *
 * 缓存策略:
 * - cachedViews 从路由表静态推导(meta.keepAlive !== false 即缓存)
 *   与「打开的 tab 列表」解耦 → 首访即缓存,不依赖 addTab 时机
 * - cachedViews 给 <keep-alive :include> 提供 componentName 白名单
 *
 * 身份策略(沿用参考项目 2026-07-20 的修复):
 * - 用 route.path 作为 :key 身份(App 内 :key="${r.path}::${refreshToken}")
 * - 多 route 共享同一 view 时 KeepAlive 缓存 slot 不碰撞
 * - 同 route 改 query 不重新 mount(r.path 不含 query)
 *
 * 与参考项目的差异:
 * - home tab 不硬编码 /dashboard:落地页 = 菜单第一项(menu.uris[0]),
 *   由路由 afterEach 计算 closable,store 不感知首页概念;
 *   不做 DEFAULT_TAB / ensureHomeTab——首页标签在首次导航时自然产生
 * - localStorage key 用 aeus.* 前缀
 *
 * 注意:不在模块顶层 import router,避免 @/router → guards → tabs → router
 *       的循环;由 setTabsRouter() 在 main.ts 启动时同步注入。
 */
import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import type { Router } from 'vue-router'
import { safeGet, safeSet } from '../utils/storage'

const TABS_KEY = 'aeus.app.tabs'
const ACTIVE_TAB_KEY = 'aeus.app.activeTab'

/** 单例 router 引用,main.ts 启动时同步注入(避免 @/router → guards → tabs → router 的循环) */
let routerInstance: Router | null = null
export function setTabsRouter(router: Router): void {
  routerInstance = router
}

export interface Tab {
  path: string
  name: string
  title: string
  icon?: string
  closable: boolean
  query?: Record<string, string>
}

export const useTabsStore = defineStore('tabs', () => {
  // State
  const tabs = ref<Tab[]>(safeGet<Tab[]>(TABS_KEY, []))
  const activeTab = ref<string>(safeGet<string>(ACTIVE_TAB_KEY, ''))
  // 每个 route.path 一个 refresh token;bump 该值即触发对应视图重渲染
  const refreshTokens = ref<Record<string, number>>({})

  // 给 KeepAlive :include 用:从路由表按 meta.keepAlive 派生(缺省 true)
  // setTabsRouter() 在启动阶段注入,getRoutes() 反映最新注册的菜单路由
  const cachedViews = computed<string[]>(() => {
    const r = routerInstance
    if (!r) return []
    return r.getRoutes()
      .filter((route) => route.meta?.keepAlive !== false)
      .map((route) => route.meta?.componentName as string)
      .filter((n): n is string => Boolean(n))
  })

  // 持久化
  watch(tabs, (v) => { safeSet(TABS_KEY, v) }, { deep: true })
  watch(activeTab, (v) => { safeSet(ACTIVE_TAB_KEY, v) })

  // Actions
  function addTab(tab: Tab) {
    const existing = tabs.value.find(t => t.path === tab.path)
    if (existing) {
      // 已存在则更新信息并激活;closable 按守卫当前值刷新,
      // 避免「落地页随菜单变更」后旧会话持久化的 closable 漂移
      existing.title = tab.title
      existing.icon = tab.icon
      existing.query = tab.query
      existing.closable = tab.closable
    } else {
      // 新增标签(插入到激活标签后面)
      const activeIdx = tabs.value.findIndex(t => t.path === activeTab.value)
      const insertIdx = activeIdx >= 0 ? activeIdx + 1 : tabs.value.length
      tabs.value.splice(insertIdx, 0, tab)
    }
    activeTab.value = tab.path
  }

  function removeTab(path: string) {
    const idx = tabs.value.findIndex(t => t.path === path)
    if (idx === -1) return

    // 不可关闭的标签跳过
    if (!tabs.value[idx]!.closable) return

    // 移除标签
    tabs.value.splice(idx, 1)

    // 如果关闭的是当前激活标签,切换到相邻标签
    if (activeTab.value === path) {
      const newActive = tabs.value[Math.min(idx, tabs.value.length - 1)]
      if (newActive) {
        activeTab.value = newActive.path
      }
    }
  }

  function removeOtherTabs(path: string) {
    tabs.value = tabs.value.filter(t => !t.closable || t.path === path)
    activeTab.value = path
  }

  function removeLeftTabs(path: string) {
    const idx = tabs.value.findIndex(t => t.path === path)
    if (idx <= 0) return

    tabs.value = [
      ...tabs.value.filter((t, i) => i >= idx || !t.closable),
    ]
    // 如果当前激活标签被关闭,切换到目标标签
    if (!tabs.value.find(t => t.path === activeTab.value)) {
      activeTab.value = path
    }
  }

  function removeRightTabs(path: string) {
    const idx = tabs.value.findIndex(t => t.path === path)
    if (idx === -1) return

    tabs.value = [
      ...tabs.value.filter((t, i) => i <= idx || !t.closable),
    ]
    // 如果当前激活标签被关闭,切换到目标标签
    if (!tabs.value.find(t => t.path === activeTab.value)) {
      activeTab.value = path
    }
  }

  function removeAllTabs() {
    tabs.value = tabs.value.filter(t => !t.closable)
    // 确保有激活标签
    if (!tabs.value.find(t => t.path === activeTab.value)) {
      activeTab.value = tabs.value[0]?.path || ''
    }
  }

  function setActiveTab(path: string) {
    activeTab.value = path
  }

  /**
   * 强制刷新指定路径对应的视图实例。
   * 把 path 对应的 token +1,router-view 上对应 :key 变化,
   * 旧实例被销毁、新实例挂载(onMounted 重新执行、数据重新加载)。
   * 同一 view 被多 route 共享时,只会重渲染匹配 path 的那个实例
   * (每个 path 独立 cache slot,即 :key 唯一)。
   */
  function refreshTab(path: string): void {
    if (!path) return
    refreshTokens.value = {
      ...refreshTokens.value,
      [path]: (refreshTokens.value[path] ?? 0) + 1,
    }
  }

  /** 读取某个 path 的 refresh token,供 :key 计算使用。封装后不暴露内部 ref */
  function getRefreshToken(path: string): number {
    return refreshTokens.value[path] ?? 0
  }

  return {
    tabs,
    activeTab,
    cachedViews,
    getRefreshToken,
    addTab,
    removeTab,
    removeOtherTabs,
    removeLeftTabs,
    removeRightTabs,
    removeAllTabs,
    setActiveTab,
    refreshTab,
  }
})
