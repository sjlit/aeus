import { describe, expect, it } from 'vitest'
import {
  extractApiUri,
  groupPermissionsByMenu,
  type PermissionGroup,
} from '../groupPermissions'
import type { PermissionItem } from '@/api/permission'

function perm(data: string, id = 1): PermissionItem {
  return { id, type: 'api', data, description: '' }
}

describe('extractApiUri', () => {
  it('拆出首个空格后的 URI', () => {
    expect(extractApiUri('POST /system/sys_user')).toBe('/system/sys_user')
  })

  it('多个空格保留后续部分', () => {
    expect(extractApiUri('POST /foo bar')).toBe('/foo bar')
  })

  it('无空格 → 空串', () => {
    expect(extractApiUri('malformed')).toBe('')
  })

  it('空串 → 空串', () => {
    expect(extractApiUri('')).toBe('')
  })
})

describe('groupPermissionsByMenu', () => {
  const menus = [
    { uri: '/system/sys_audit', name: '审计' },
    { uri: '/system/sys_user', name: '用户' },
  ]

  it('同一 uri 多个 scenario → 同一组', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit'),
      perm('PUT /system/sys_audit'),
      perm('DELETE /system/sys_audit'),
      perm('GET /system/sys_audit'),
    ]
    const got = groupPermissionsByMenu(items, menus)
    expect(got).toEqual([
      { key: '/system/sys_audit', title: '审计', items },
    ])
  })

  it('导出类的细粒度 URI 进「其他」组(无独立 menu)', () => {
    const items: PermissionItem[] = [
      perm('GET /system/sys_audit/export'),
    ]
    const got = groupPermissionsByMenu(items, menus)
    expect(got[0]).toMatchObject({ key: '__other__', title: '其他' })
  })

  it('多个 module → 按 zh-Hans 标题升序', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_user'),
      perm('POST /system/sys_audit'),
    ]
    const got = groupPermissionsByMenu(items, menus)
    expect(got.map((g: PermissionGroup) => g.title)).toEqual(['审计', '用户'])
  })

  it('menu 缺失的 permission 进「其他」组,放在最后', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit'),
      perm('POST /custom/rpc'),
    ]
    const got = groupPermissionsByMenu(items, menus)
    expect(got).toHaveLength(2)
    expect(got[1]).toMatchObject({ key: '__other__', title: '其他' })
    expect(got[1]!.items).toHaveLength(1)
  })

  it('全无 menu 命中 → 只返回「其他」组', () => {
    const items: PermissionItem[] = [perm('POST /custom/a'), perm('POST /custom/b')]
    const got = groupPermissionsByMenu(items, menus)
    expect(got).toHaveLength(1)
    expect(got[0]!.title).toBe('其他')
  })

  it('损坏 data(indexOf<0) → 跳过', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit'),
      perm('malformed'),
    ]
    const got = groupPermissionsByMenu(items, menus)
    expect(got[0]!.items).toHaveLength(1)
  })

  it('组内 items 保留输入顺序', () => {
    const items: PermissionItem[] = [
      perm('GET /system/sys_audit', 1),
      perm('POST /system/sys_audit', 2),
      perm('DELETE /system/sys_audit', 3),
    ]
    const got = groupPermissionsByMenu(items, menus)
    expect(got[0]!.items.map((i) => i.id)).toEqual([1, 2, 3])
  })

  it('空入参 → 空数组', () => {
    expect(groupPermissionsByMenu([], menus)).toEqual([])
  })
})