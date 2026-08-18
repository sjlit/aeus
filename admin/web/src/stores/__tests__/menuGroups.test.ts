import { describe, expect, it } from 'vitest'
import {
  buildTree,
  flattenMenu,
  groupBySection,
  isSectionContainer,
  shouldRenderAsSubMenu,
  uriSegment,
} from '../menuGroups'
import {
  DEFAULT_SECTION_NAME,
  SECTION_DEFINITIONS,
  findMissingComponents,
} from '../menuSections'
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

describe('shouldRenderAsSubMenu', () => {
  it('容器名 ≠ 节名 → true(渲染为 sub-menu 显示容器自身标题)', () => {
    // 多容器归到一节时的典型场景:基础能力 节下包含 用户中心/日志记录/系统设置。
    expect(shouldRenderAsSubMenu(node({ name: '用户中心', component: 'X' }), '基础能力')).toBe(true)
  })
  it('容器名 = 节名 → false(平铺避免重复标题,legacy "1 节 = 1 容器")', () => {
    expect(shouldRenderAsSubMenu(node({ name: '用户中心', component: 'X' }), '用户中心')).toBe(false)
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
  it('按 SECTION_DEFINITIONS 把顶级节点分到对应分组,组名取定义', () => {
    const secs = groupBySection(tree)
    // 默认 SECTION_DEFINITIONS 把三个 section container 归到"基础能力"一节下;
    // 每个一级节点(component)被精确匹配进该节,组名取定义。
    expect(secs.map((s) => s.name)).toEqual(['基础能力'])
    expect(secs[0].items.map((n) => n.component)).toEqual([
      'SystemUserCenter', 'SystemLogs', 'SystemSettings',
    ])
  })

  it('所有顶级节点都被定义覆盖时,不渲染 DEFAULT_SECTION', () => {
    const secs = groupBySection(tree)
    expect(secs.find((s) => s.name === DEFAULT_SECTION_NAME)).toBeUndefined()
  })

  it('未列入 SECTION_DEFINITIONS 的顶级节点被 catcher 吸纳,渲染位置由声明顺序决定', () => {
    const standalone = node({
      name: '概览', component: 'Overview', uri: '/workspace/overview',
      view_path: '@/views/workspace/overview/Index.vue',
    })
    const mixed = buildTree([...flat, standalone])
    const secs = groupBySection(mixed)
    // 默认 SECTION_DEFINITIONS 把 catcher(`components: []`)声明在末尾 →
    // 渲染在侧栏底部,未列入节点(Overview)进入 catcher。
    expect(secs.map((s) => s.name)).toEqual(['基础能力', DEFAULT_SECTION_NAME])
    expect(secs[1].items.map((n) => n.component)).toEqual(['Overview'])
  })

  it('未列入 SECTION_DEFINITIONS 的节点即使有 uri,也不按 uri 首段分(旧行为已废)', () => {
    // 关键回归:旧实现把孤立根按 uriSegment(uri) 分配节(本例会得到 'workspace');
    // 新实现统一进 catcher(若存在),否则静默丢弃。
    const standalone = node({
      name: '概览', component: 'Overview', uri: '/workspace/overview',
      view_path: '@/views/workspace/overview/Index.vue',
    })
    const mixed = buildTree([...flat, standalone])
    const secs = groupBySection(mixed)
    expect(secs.find((s) => s.name === 'workspace')).toBeUndefined()
  })

  it('可传入自定义 definitions 覆盖默认 SECTION_DEFINITIONS', () => {
    // 自定义里没有 catcher,未被列出的顶级节点被静默丢弃。
    const custom = [{ name: '审计', components: ['SystemLogs'] }]
    const secs = groupBySection(tree, custom)
    expect(secs.map((s) => s.name)).toEqual(['审计'])
    expect(secs[0].items.map((n) => n.component)).toEqual(['SystemLogs'])
  })

  it('definitions 引用了树里不存在的 component → 该 def 不渲染空节;未列入节点进 catcher(若有)', () => {
    const custom = [
      { name: '工作台', components: ['NonExistent'] },
      { name: '审计', components: ['SystemLogs'] },
      { name: DEFAULT_SECTION_NAME, components: [] },
    ]
    const secs = groupBySection(tree, custom)
    // 工作台(NonExistent)空 → 过滤;SystemUserCenter/SystemSettings 未列 → 进 catcher。
    expect(secs.map((s) => s.name)).toEqual(['审计', DEFAULT_SECTION_NAME])
    expect(secs.find((s) => s.name === '工作台')).toBeUndefined()
    expect(secs[1].items.map((n) => n.component)).toEqual(['SystemUserCenter', 'SystemSettings'])
  })

  it('catcher 声明在非末尾位置 → 渲染在该位置(用户完全控制顺序)', () => {
    const standalone = node({
      name: '概览', component: 'Overview', uri: '/workspace/overview',
      view_path: '@/views/workspace/overview/Index.vue',
    })
    const mixed = buildTree([...flat, standalone])
    // catcher 挪到开头,渲染在"基础能力"之上
    const custom = [
      { name: DEFAULT_SECTION_NAME, components: [] },
      { name: '基础能力', components: ['SystemUserCenter', 'SystemLogs', 'SystemSettings'] },
    ]
    const secs = groupBySection(mixed, custom)
    expect(secs.map((s) => s.name)).toEqual([DEFAULT_SECTION_NAME, '基础能力'])
    expect(secs[0].items.map((n) => n.component)).toEqual(['Overview'])
  })

  it('多个 catcher → 只第一个真正吸纳,后续空 catcher 被过滤', () => {
    const standalone = node({
      name: '概览', component: 'Overview', uri: '/workspace/overview',
      view_path: '@/views/workspace/overview/Index.vue',
    })
    const mixed = buildTree([...flat, standalone])
    const custom = [
      { name: '第一个 catcher', components: [] },
      { name: '基础能力', components: ['SystemUserCenter', 'SystemLogs', 'SystemSettings'] },
      { name: '第二个 catcher', components: [] },
    ]
    const secs = groupBySection(mixed, custom)
    // 第二个 catcher 没有未列入节点可吸纳(全部都被"第一个 catcher"吸走)→ 空节过滤;
    // 第一个 catcher 出现顺序在前,所以 Overview 进它,基础能力保持原位。
    expect(secs.map((s) => s.name)).toEqual(['第一个 catcher', '基础能力'])
    expect(secs.find((s) => s.name === '第二个 catcher')).toBeUndefined()
    expect(secs[0].items.map((n) => n.component)).toEqual(['Overview'])
  })

  it('没有未列入节点时,catcher 不渲染(空节被过滤)', () => {
    const secs = groupBySection(tree)
    expect(secs.find((s) => s.name === DEFAULT_SECTION_NAME)).toBeUndefined()
  })

  it('无 catcher 的 definitions + 有未列入节点 → 未列入节点被静默丢弃', () => {
    const standalone = node({
      name: '概览', component: 'Overview', uri: '/workspace/overview',
      view_path: '@/views/workspace/overview/Index.vue',
    })
    const mixed = buildTree([...flat, standalone])
    const custom = [{ name: '审计', components: ['SystemLogs'] }]
    const secs = groupBySection(mixed, custom)
    expect(secs.map((s) => s.name)).toEqual(['审计'])
    expect(secs[0].items.map((n) => n.component)).toEqual(['SystemLogs'])
  })
})

describe('findMissingComponents', () => {
  it('definitions 引用了树里没有的 component → 返回缺失列表', () => {
    const missing = findMissingComponents(
      [{ name: '工作台', components: ['NonExistent', 'SystemLogs'] }],
      tree,
    )
    expect(missing).toEqual(['NonExistent'])
  })

  it('全部引用都存在 → 返回空数组', () => {
    expect(findMissingComponents(SECTION_DEFINITIONS, tree)).toEqual([])
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