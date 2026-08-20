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
    return r
      .getRoutes()
      // 动态段 URI(占位符 :roleKey 等)承载路由参数,缓存会让 a→b 切角色时
      // 看到旧角色的已分配授权;按 path 含 ':' 直接排除。
      // meta.keepAlive 缺省 true,显式 false 一并尊重。
      .filter((route) => route.meta?.keepAlive !== false && !(route.path ?? '').includes(':'))
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

    tabs.value = tabs.value.filter((t, i) => i >= idx || !t.closable)
    // 如果当前激活标签被关闭,切换到目标标签
    reassignActive(path)
  }

  function removeRightTabs(path: string) {
    const idx = tabs.value.findIndex(t => t.path === path)
    if (idx === -1) return

    tabs.value = tabs.value.filter((t, i) => i <= idx || !t.closable)
    // 如果当前激活标签被关闭,切换到目标标签
    reassignActive(path)
  }

  function removeAllTabs() {
    tabs.value = tabs.value.filter(t => !t.closable)
    // 确保有激活标签
    reassignActive(tabs.value[0]?.path ?? '')
  }

  /** 当前激活 tab 不在剩余列表里时,切到 fallback;否则保持不变。
   *  集中处理"关闭后掉激活"的语义,left/right/all 三个分支共用。 */
  function reassignActive(fallback: string) {
    if (!tabs.value.find(t => t.path === activeTab.value)) {
      activeTab.value = fallback
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
