import { describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// 菜单/资料接口 mock:守卫的 ensureMenuRoutes 会真实调用它们,
// 测试环境无后端,给一份最小可用的菜单树(一个 section + 一个叶子)。
vi.mock('../../api/user', () => ({
  fetchMenuTree: vi.fn(async () => [
    {
      name: '系统管理', component: 'System', parent: '', view_path: '',
      uri: '', icon: '', hidden: false, public: false, children: [],
    },
    {
      name: '用户管理', component: 'SystemSysUsers', parent: 'System',
      view_path: '@/views/system/sys_user/Index.vue',
      uri: '/system/sys_users', icon: '', hidden: false, public: false,
      children: [],
    },
  ]),
  fetchProfile: vi.fn(async () => ({
    uid: '1', username: 'admin', email: '', gender: '',
    description: '', avatar: '', role: '', dept_id: 0,
  })),
}))

vi.mock('../../api/auth', () => ({
  login: vi.fn(async () => ({
    uid: '1', username: 'admin', expires: 0,
    access_token: 'at', refresh_token: 'rt',
    tenant_id: '1', tenant_name: 't',
  })),
  logout: vi.fn(async () => ({})),
  refresh: vi.fn(async () => ({ uid: '1', expires: 0, access_token: 'at2' })),
}))

import { router, DASHBOARD_PATH } from '../index'
import { useAuthStore } from '../../stores/auth'

/** 登录后默认落地工作台:钉死 '/' → DASHBOARD_PATH 的守卫行为,
 *  防止未来改动守卫时悄悄退回「菜单第一项落地」。 */
describe('路由守卫:登录后默认进入工作台', () => {
  it('auth.login + replace("/") 应落在 /dashboard', async () => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    // 复刻 LoginView.submit 的导航序列
    await auth.login({ username: 'admin', password: '123456' })
    await router.replace('/')
    expect(router.currentRoute.value.path).toBe(DASHBOARD_PATH)
  })
})
