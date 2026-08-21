import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { setTabsRouter, notifyRoutesChanged, useTabsStore, type Tab } from '../tabs'

function tab(overrides: Partial<Tab> = {}): Tab {
  return {
    path: '/a',
    name: 'menu:/a',
    title: 'A',
    icon: 'user',
    closable: true,
    ...overrides,
  }
}

describe('tabs store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('addTab 插入到激活标签之后并激活', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b', title: 'B' }))
    store.addTab(tab({ path: '/c', title: 'C' }))
    // 激活是 /c,新标签插到它后面
    store.addTab(tab({ path: '/d', title: 'D' }))
    expect(store.tabs.map(t => t.path)).toEqual(['/home', '/b', '/c', '/d'])
    expect(store.activeTab).toBe('/d')
  })

  it('addTab 已存在则原地更新信息(含 closable 自愈),不重复插入', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b', title: 'B' }))
    store.addTab(tab({ path: '/b', title: 'B2', icon: 'Monitor', closable: false }))
    expect(store.tabs).toHaveLength(2)
    const b = store.tabs[1]!
    expect(b.title).toBe('B2')
    expect(b.icon).toBe('Monitor')
    expect(b.closable).toBe(false)
    expect(store.activeTab).toBe('/b')
  })

  it('removeTab 不可关闭的标签跳过', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.removeTab('/home')
    expect(store.tabs.map(t => t.path)).toEqual(['/home'])
  })

  it('关闭激活标签后切换到相邻标签', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b' }))
    store.addTab(tab({ path: '/c' }))
    store.removeTab('/b')
    // 关闭中间标签 → 激活右侧邻居;关闭末尾标签 → 激活前一个
    expect(store.activeTab).toBe('/c')
    store.removeTab('/c')
    expect(store.activeTab).toBe('/home')
  })

  it('removeOtherTabs 保留不可关闭标签与目标标签', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b' }))
    store.addTab(tab({ path: '/c' }))
    store.removeOtherTabs('/c')
    expect(store.tabs.map(t => t.path)).toEqual(['/home', '/c'])
    expect(store.activeTab).toBe('/c')
  })

  it('removeLeftTabs / removeRightTabs 只动一侧', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b' }))
    store.addTab(tab({ path: '/c' }))
    store.addTab(tab({ path: '/d' }))
    store.removeLeftTabs('/c')
    expect(store.tabs.map(t => t.path)).toEqual(['/home', '/c', '/d'])
    store.removeRightTabs('/c')
    expect(store.tabs.map(t => t.path)).toEqual(['/home', '/c'])
  })

  it('removeAllTabs 只留不可关闭标签', () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b' }))
    store.addTab(tab({ path: '/c' }))
    store.removeAllTabs()
    expect(store.tabs.map(t => t.path)).toEqual(['/home'])
    expect(store.activeTab).toBe('/home')
  })

  it('refreshTab 递增对应 path 的 token,getRefreshToken 缺省 0', () => {
    const store = useTabsStore()
    expect(store.getRefreshToken('/b')).toBe(0)
    store.refreshTab('/b')
    store.refreshTab('/b')
    store.refreshTab('/c')
    expect(store.getRefreshToken('/b')).toBe(2)
    expect(store.getRefreshToken('/c')).toBe(1)
    // 其他 path 不受影响
    expect(store.getRefreshToken('/home')).toBe(0)
  })

  it('标签与激活态持久化到 localStorage,新实例从缓存恢复', async () => {
    const store = useTabsStore()
    store.addTab(tab({ path: '/home', closable: false }))
    store.addTab(tab({ path: '/b', title: 'B' }))
    await nextTick()
    const saved = JSON.parse(localStorage.getItem('aeus.app.tabs')!)
    expect(saved.map((t: Tab) => t.path)).toEqual(['/home', '/b'])
    expect(localStorage.getItem('aeus.app.activeTab')).toBe('"/b"')

    // 新实例(新的 pinia)从 localStorage 恢复
    setActivePinia(createPinia())
    const restored = useTabsStore()
    expect(restored.tabs.map(t => t.path)).toEqual(['/home', '/b'])
    expect(restored.activeTab).toBe('/b')
  })

  it('localStorage 数据损坏时回退到空列表', () => {
    localStorage.setItem('aeus.app.tabs', '{broken json')
    const store = useTabsStore()
    expect(store.tabs).toEqual([])
  })

  it('cachedViews 从路由表推导:keepAlive 缺省缓存,取 componentName', () => {
    const store = useTabsStore()
    setTabsRouter({
      getRoutes: () => [
        { meta: { componentName: 'PlaceholderView' } },
        { meta: { componentName: 'PlaceholderView', keepAlive: false } },
        { meta: { title: '无 componentName 的路由' } },
      ],
    } as never)
    expect(store.cachedViews).toEqual(['PlaceholderView'])
  })

  it('cachedViews 跳过动态段路由:含 ":" 的 path 不缓存', () => {
    const store = useTabsStore()
    setTabsRouter({
      getRoutes: () => [
        { path: '/system/sys_user', meta: { componentName: 'SystemSysUsers' } },
        { path: '/system/sys_role/perm/:roleKey', meta: { componentName: 'SystemSysRolesPermission' } },
      ],
    } as never)
    expect(store.cachedViews).toEqual(['SystemSysUsers'])
  })

  it('动态注册路由后 notifyRoutesChanged 触发 cachedViews 重算', async () => {
    // 模拟非响应式 router:路由表是可变数组,getRoutes 每次返回最新快照
    const routes: { path: string; meta: Record<string, unknown> }[] = []
    setTabsRouter({ getRoutes: () => routes } as never)
    const store = useTabsStore()
    expect(store.cachedViews).toEqual([])
    // registerMenuRoutes 注册新路由后必须 bump,否则 computed 永不重算
    routes.push({ path: '/system/sys_user', meta: { componentName: 'SystemSysUsers' } })
    notifyRoutesChanged()
    await nextTick()
    expect(store.cachedViews).toEqual(['SystemSysUsers'])
  })
})
