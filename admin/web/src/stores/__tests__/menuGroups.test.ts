import { describe, expect, it } from 'vitest'
import { flattenMenu, groupBySection, uriSegment } from '../menuGroups'
import type { MenuNode } from '../../types'

const tree: MenuNode[] = [
  {
    name: '用户管理',
    view_path: '@/views/system/sys_user/Index.vue',
    uri: '/system/sys-users',
    icon: '',
    hidden: false,
    public: false,
    children: [
      {
        name: '用户详情',
        view_path: '@/views/system/sys_user/Detail.vue',
        uri: '/system/sys-users/detail', icon: '', hidden: true, public: false, children: [],
      },
    ],
  },
  {
    name: '角色管理',
    view_path: '@/views/system/sys_role/Index.vue',
    uri: '/system/sys-roles', icon: '', hidden: false, public: false, children: [],
  },
  {
    name: '概览',
    view_path: '@/views/workspace/overview/Index.vue',
    uri: '/workspace/overview',
    icon: 'Monitor',
    hidden: false,
    public: false,
    children: [],
  },
  {
    name: '纯分组',
    view_path: '',
    uri: '',
    icon: '',
    hidden: false,
    public: false,
    children: [
      {
        name: '子项',
        view_path: '@/views/system/child/Index.vue',
        uri: '/system/child',
        icon: '',
        hidden: false,
        public: false,
        children: [],
      },
    ],
  },
]

describe('uriSegment', () => {
  it('取首段', () => {
    expect(uriSegment('/system/sys-users')).toBe('system')
    expect(uriSegment('system')).toBe('system')
  })
  it('空返回 general', () => {
    expect(uriSegment('')).toBe('general')
    expect(uriSegment('/')).toBe('general')
  })
})

describe('groupBySection', () => {
  it('按 uri 首段分组,保留出现顺序', () => {
    const secs = groupBySection(tree)
    expect(secs.map((s) => s.name)).toEqual(['system', 'workspace', 'general'])
    expect(secs[0].items.map((n) => n.name)).toEqual(['用户管理', '角色管理'])
    expect(secs[2].items.map((n) => n.name)).toEqual(['纯分组'])
  })
  it('空 uri 根节点归入 general', () => {
    const secs = groupBySection([tree[3]])
    expect(secs[0].name).toBe('general')
  })
})

describe('flattenMenu', () => {
  it('单次遍历收集全部非空 uri(含子节点),去重保序', () => {
    expect(flattenMenu(tree).uris).toEqual([
      '/system/sys-users',
      '/system/sys-users/detail',
      '/system/sys-roles',
      '/workspace/overview',
      '/system/child',
    ])
  })

  it('同时产出 uri → 标题映射', () => {
    const { titlesByUri } = flattenMenu(tree)
    expect(titlesByUri.get('/system/sys-roles')).toBe('角色管理')
    expect(titlesByUri.get('/system/sys-users/detail')).toBe('用户详情')
    expect(titlesByUri.has('/system/nope')).toBe(false)
  })

  it('同时产出 uri → 图标映射(多标签页标签图标用)', () => {
    const { iconsByUri } = flattenMenu(tree)
    expect(iconsByUri.get('/workspace/overview')).toBe('Monitor')
    expect(iconsByUri.get('/system/sys-users')).toBe('')
    expect(iconsByUri.has('/system/nope')).toBe(false)
  })

  it('同时产出 uri → 服务端 view_path 映射(保留 @/ 前缀原值)', () => {
    const { viewsByUri } = flattenMenu(tree)
    expect(viewsByUri.get('/system/sys-users')).toBe('@/views/system/sys_user/Index.vue')
    expect(viewsByUri.get('/system/sys-roles')).toBe('@/views/system/sys_role/Index.vue')
    expect(viewsByUri.get('/workspace/overview')).toBe('@/views/workspace/overview/Index.vue')
    expect(viewsByUri.has('/system/nope')).toBe(false)
  })

  it('空 view_path 不会进入映射(留给路由回落占位)', () => {
    const { viewsByUri } = flattenMenu([
      { name: 'g', view_path: '', uri: '/g', icon: '', hidden: false, public: false, children: [] },
    ])
    expect(viewsByUri.size).toBe(0)
  })
})