import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useWorkspaceStore, type WorkspaceLink } from '../workspace'

function link(overrides: Partial<WorkspaceLink> = {}): WorkspaceLink {
  return {
    path: '/system/sys_users',
    title: '用户管理',
    icon: 'User',
    section: '用户中心',
    ...overrides,
  }
}

describe('workspace store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('recordVisit 去重并把最近访问置顶', () => {
    const store = useWorkspaceStore()
    store.recordVisit(link({ path: '/a', title: 'A' }))
    store.recordVisit(link({ path: '/b', title: 'B' }))
    store.recordVisit(link({ path: '/a', title: 'A' }))
    expect(store.visits.map((item) => item.path)).toEqual(['/a', '/b'])
    expect(store.visits[0]!.visitedAt).toBeGreaterThanOrEqual(store.visits[1]!.visitedAt)
  })

  it('recordVisit 最多保留 12 条', () => {
    const store = useWorkspaceStore()
    for (let i = 0; i < 15; i++) {
      store.recordVisit(link({ path: `/page-${i}`, title: `Page ${i}` }))
    }
    expect(store.visits).toHaveLength(12)
    expect(store.visits[0]!.path).toBe('/page-14')
    expect(store.visits[11]!.path).toBe('/page-3')
  })

  it('favorites 可添加、取消,最多 8 个', () => {
    const store = useWorkspaceStore()
    expect(store.toggleFavorite(link({ path: '/a', title: 'A' }))).toBe(true)
    expect(store.isFavorite('/a')).toBe(true)
    expect(store.toggleFavorite(link({ path: '/a', title: 'A' }))).toBe(true)
    expect(store.isFavorite('/a')).toBe(false)

    for (let i = 0; i < 8; i++) {
      expect(store.toggleFavorite(link({ path: `/f-${i}`, title: `F ${i}` }))).toBe(true)
    }
    expect(store.favorites).toHaveLength(8)
    expect(store.toggleFavorite(link({ path: '/overflow', title: 'Overflow' }))).toBe(false)
    expect(store.favorites).toHaveLength(8)
  })

  it('clearVisits 只清访问历史,clear 清空全部并移除 localStorage key', () => {
    const store = useWorkspaceStore()
    store.recordVisit(link())
    store.toggleFavorite(link())
    expect(localStorage.getItem('aeus.app.visits')).not.toBeNull()
    expect(localStorage.getItem('aeus.app.favorites')).not.toBeNull()

    store.clearVisits()
    expect(store.visits).toHaveLength(0)
    expect(store.favorites).toHaveLength(1)
    expect(localStorage.getItem('aeus.app.visits')).toBeNull()

    store.clear()
    expect(store.favorites).toHaveLength(0)
    expect(localStorage.getItem('aeus.app.favorites')).toBeNull()
  })

  it('从 localStorage 恢复时忽略损坏数据和重复项', () => {
    localStorage.setItem(
      'aeus.app.visits',
      JSON.stringify([
        { path: '/a', title: 'A', icon: '', section: '', visitedAt: 10 },
        { path: '/a', title: 'A', icon: '', section: '', visitedAt: 20 },
        { path: '', title: 'bad', icon: '', section: '', visitedAt: 30 },
        { path: '/b', title: 'B', icon: '', section: '', visitedAt: 'bad' },
      ]),
    )
    const store = useWorkspaceStore()
    expect(store.visits).toHaveLength(1)
    expect(store.visits[0]!.path).toBe('/a')
  })
})
