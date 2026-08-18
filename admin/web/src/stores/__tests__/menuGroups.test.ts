import { describe, expect, it } from 'vitest'
import {
  buildTree,
  flattenMenu,
  groupBySection,
  isSectionContainer,
  uriSegment,
} from '../menuGroups'
import type { MenuNode } from '../../types'

/** 构造一个 MenuNode,children 默认空数组、parent 默认空、其它默认空串/false。 */
function node(p: Partial<MenuNode> & { name: string; component: string }): MenuNode {
  return {
    name: p.name,
    component: p.component,
    parent: p.parent ?? '',
    view_path: p.view_path ?? '',
    uri: p.uri ?? '',
    icon: p.icon ?? '',
    hidden: p.hidden ?? false,
    public: p.public ?? false,
    children: p.children ?? [],
  }
}

const flat = [
  // section containers (parent="", uri="", 无 view_path)
  node({ name: '用户中心', component: 'SystemUserCenter', icon: 'UserFilled' }),
  node({ name: '日志记录', component: 'SystemLogs', icon: 'Tickets' }),
  node({ name: '系统设置', component: 'SystemSettings', icon: 'Tools' }),
  // 用户中心 children
  node({
    name: '用户管理',
    component: 'SystemSysUsers',
    parent: 'SystemUserCenter',
    uri: '/system/sys-users',
    view_path: '@/views/system/sys_user/Index.vue',
  }),
  node({
    name: '角色管理',
    component: 'SystemSysRoles',
    parent: 'SystemUserCenter',
    uri: '/system/sys-roles',
    view_path: '@/views/system/sys_role/Index.vue',
  }),
  // 日志记录 children
  node({
    name: '操作日志',
    component: 'SystemSysAudits',
    parent: 'SystemLogs',
    uri: '/system/sys-audits',
    view_path: '@/views/system/sys_audit/Index.vue',
  }),
  node({
    name: '登录日志',
    component: 'SystemSysLoginLogs',
    parent: 'SystemLogs',
    uri: '/system/sys-login-logs',
    view_path: '@/views/system/sys_login_log/Index.vue',
  }),
  // 系统设置 children
  node({
    name: '菜单管理',
    component: 'SystemSysMenus',
    parent: 'SystemSettings',
    uri: '/system/sys-menus',
    view_path: '@/views/system/sys_menu/Index.vue',
  }),
]

const tree = buildTree(flat)

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

describe('isSectionContainer', () => {
  it('uri 空 + 有 children → true', () => {
    expect(isSectionContainer(node({
      name: 'x', component: 'X', children: [node({ name: 'c', component: 'C' })],
    }))).toBe(true)
  })
  it('uri 非空 + 有 children → false(普通 sub-menu)', () => {
    expect(isSectionContainer(node({
      name: 'x', component: 'X', uri: '/x', children: [node({ name: 'c', component: 'C' })],
    }))).toBe(false)
  })
  it('uri 空 + 无 children → false(纯标签)', () => {
    expect(isSectionContainer(node({ name: 'x', component: 'X' }))).toBe(false)
  })
})

describe('buildTree', () => {
  it('空数组 → 空根', () => {
    expect(buildTree([])).toEqual([])
  })

  it('parent=="" 的节点成为根', () => {
    const roots = buildTree(flat)
    expect(roots.map((r) => r.component)).toEqual([
      'SystemUserCenter', 'SystemLogs', 'SystemSettings',
    ])
  })

  it('子节点按 parent 嵌套进对应根的 children', () => {
    const roots = buildTree(flat)
    expect(roots[0].children.map((c) => c.component)).toEqual([
      'SystemSysUsers', 'SystemSysRoles',
    ])
    expect(roots[1].children.map((c) => c.component)).toEqual([
      'SystemSysAudits', 'SystemSysLoginLogs',
    ])
    expect(roots[2].children.map((c) => c.component)).toEqual([
      'SystemSysMenus',
    ])
  })

  it('不修改输入数组的 children(避免污染 store 缓存的原始数据)', () => {
    buildTree(flat)
    for (const n of flat) {
      expect(n.children).toEqual([])
    }
  })

  it('parent 指向不存在 component 的孤儿被升为根', () => {
    const orphan = node({
      name: '孤儿', component: 'X', parent: 'NonExistent',
      uri: '/x',
    })
    const roots = buildTree([orphan])
    expect(roots).toHaveLength(1)
    expect(roots[0].component).toBe('X')
  })

  it('不引入意外的引用循环(节点是自己的 parent)', () => {
    const cycle = node({
      name: 'cycle', component: 'Cycle', parent: 'Cycle',
      uri: '/cycle',
    })
    const roots = buildTree([cycle])
    // parent 指向自身 → 找不到 parent,升为根
    expect(roots).toHaveLength(1)
    expect(roots[0].component).toBe('Cycle')
  })
})

describe('groupBySection', () => {
  it('section container 用自身 Name 作节标题,普通根按 uri 首段', () => {
    const secs = groupBySection(tree)
    // 用户中心 / 日志记录 / 系统设置 三个 section(各自包含一个根节点);
    // 没有 system / general 之类,因为所有可见根都已挂到 section 下。
    expect(secs.map((s) => s.name)).toEqual(['用户中心', '日志记录', '系统设置'])
    expect(secs[0].items.map((n) => n.component)).toEqual(['SystemUserCenter'])
    expect(secs[1].items.map((n) => n.component)).toEqual(['SystemLogs'])
    expect(secs[2].items.map((n) => n.component)).toEqual(['SystemSettings'])
  })

  it('混入一个不属于任何 section 的孤立根 → 落到 uri 首段对应的节', () => {
    const standalone = node({
      name: '概览', component: 'Overview', uri: '/workspace/overview',
      view_path: '@/views/workspace/overview/Index.vue',
    })
    const mixed = buildTree([...flat, standalone])
    const secs = groupBySection(mixed)
    // 三个 section + workspace(由孤立根的 uri 首段决定)
    expect(secs.map((s) => s.name)).toEqual(['用户中心', '日志记录', '系统设置', 'workspace'])
    expect(secs[3].items.map((n) => n.name)).toEqual(['概览'])
  })
})

describe('flattenMenu', () => {
  it('深度优先收集全部非空 uri(含子节点),去重保序', () => {
    expect(flattenMenu(tree).uris).toEqual([
      '/system/sys-users',
      '/system/sys-roles',
      '/system/sys-audits',
      '/system/sys-login-logs',
      '/system/sys-menus',
    ])
  })

  it('同时产出 uri → 标题映射', () => {
    const { titlesByUri } = flattenMenu(tree)
    expect(titlesByUri.get('/system/sys-roles')).toBe('角色管理')
    expect(titlesByUri.get('/system/sys-login-logs')).toBe('登录日志')
    expect(titlesByUri.has('/system/nope')).toBe(false)
  })

  it('同时产出 uri → 图标映射', () => {
    const { iconsByUri } = flattenMenu(tree)
    expect(iconsByUri.get('/system/sys-roles')).toBe('')
    expect(iconsByUri.has('/system/nope')).toBe(false)
  })

  it('同时产出 uri → view_path 映射(原样保留 @/ 前缀)', () => {
    const { viewsByUri } = flattenMenu(tree)
    expect(viewsByUri.get('/system/sys-users')).toBe('@/views/system/sys_user/Index.vue')
    expect(viewsByUri.get('/system/sys-menus')).toBe('@/views/system/sys_menu/Index.vue')
    expect(viewsByUri.has('/system/nope')).toBe(false)
  })

  it('section container 的空 uri 不会进入映射', () => {
    const { uris, viewsByUri } = flattenMenu(tree)
    for (const sec of tree) {
      expect(uris).not.toContain(sec.uri)
      expect(viewsByUri.has(sec.uri)).toBe(false)
    }
  })
})