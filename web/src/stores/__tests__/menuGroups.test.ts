import { describe, expect, it } from 'vitest'
import { flattenMenu, groupBySection, uriSegment } from '../menuGroups'
import type { MenuNode } from '../../types'

const tree: MenuNode[] = [
  {
    name: 'SystemSysUsers', title: '用户管理', component: 'SystemSysUsers',
    uri: '/system/sys-users', icon: '', hidden: false, public: false,
    children: [
      {
        name: 'SystemSysUserDetail', title: '用户详情', component: 'SystemSysUserDetail',
        uri: '/system/sys-users/detail', icon: '', hidden: true, public: false, children: [],
      },
    ],
  },
  {
    name: 'SystemSysRoles', title: '角色管理', component: 'SystemSysRoles',
    uri: '/system/sys-roles', icon: '', hidden: false, public: false, children: [],
  },
  {
    name: 'WorkspaceOverview', title: '概览', component: 'WorkspaceOverview',
    uri: '/workspace/overview', icon: '', hidden: false, public: false, children: [],
  },
  {
    name: 'GroupOnly', title: '纯分组', component: 'GroupOnly',
    uri: '', icon: '', hidden: false, public: false,
    children: [
      {
        name: 'Child', title: '子项', component: 'Child',
        uri: '/system/child', icon: '', hidden: false, public: false, children: [],
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
    expect(secs[0].items.map((n) => n.title)).toEqual(['用户管理', '角色管理'])
    expect(secs[2].items.map((n) => n.title)).toEqual(['纯分组'])
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
})