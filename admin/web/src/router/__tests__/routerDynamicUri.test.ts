import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { router } from '../index'

// 仅覆盖「菜单 URI 含动态段」这一行为:
// /system/sys_role/perm/:roleKey 是注册进路由表的 pattern,
// 用户实际访问 /system/sys_role/perm/admin 必须能命中该路由,
// 而不是被守卫踢回 '/'(再跳到 flatIndex.uris[0] 即 /system/sys-users)。
//
// 不能用真实的 createWebHashHistory,本测试用 createMemoryHistory 重新
// 搭一个最小的等价路由树,只关心「动态段 pattern 与实参路径的匹配」。

describe('路由守卫:动态段 pattern', () => {
  it('未注入菜单时 resolve 实参路径必然无 menu: 命中', () => {
    // router 是模块级单例,加载时还没注册任何 menu: 路由,
    // 所以 matched 必为空 —— 用以钉死 hasRoute 不可用的语义。
    const resolved = router.resolve({
      path: '/system/sys_role/perm/admin',
      matched: [],
    } as never)
    const hit = resolved.matched.some(
      (r) => typeof r.name === 'string' && r.name.startsWith('menu:'),
    )
    expect(hit).toBe(false)
  })

  it('动态段路由注册后,实参路径 resolve 能命中该 route', () => {
    // 复刻 router/index.ts 的 registerMenuRoutes 行为:加一条带占位符的路由。
    router.addRoute('default', {
      path: '/system/sys_role/perm/:roleKey',
      name: 'menu:/system/sys_role/perm/:roleKey',
      component: { template: '<div />' },
      meta: { title: '权限配置' },
    })
    const resolved = router.resolve('/system/sys_role/perm/admin')
    expect(
      resolved.matched.some(
        (r) => typeof r.name === 'string' && r.name.startsWith('menu:'),
      ),
    ).toBe(true)
    expect(resolved.params.roleKey).toBe('admin')
  })

  it('createMemoryHistory + hasRoute 对动态段永远 false(钉死老 buggy 行为)', () => {
    // 用全新的内存 router 验证旧逻辑为什么会误判:
    // hasRoute 按 name 精确匹配,实参路径显然不等于 pattern 路径。
    const fresh = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: '/',
          name: 'root',
          component: { template: '<div />' },
          children: [
            {
              path: 'system/sys_role/perm/:roleKey',
              name: 'menu:/system/sys_role/perm/:roleKey',
              component: { template: '<div />' },
            },
          ],
        },
      ],
    })
    expect(fresh.hasRoute('menu:/system/sys_role/perm/:roleKey')).toBe(true)
    // hasRoute 按 name 全等匹配;admin 是参数值,不是 name —— 永远 false。
    expect(fresh.hasRoute('menu:/system/sys_role/perm/admin')).toBe(false)
  })
})