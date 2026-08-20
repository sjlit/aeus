import { describe, expect, it } from 'vitest'
import { groupPermissions, type PermissionGroup } from '../groupPermissions'
import type { PermissionItem } from '@/api/permission'

function perm(data: string, group = '', id = 1): PermissionItem {
  return { id, type: 'api', data, description: '', group }
}

describe('groupPermissions', () => {
  it('同一 group 多 scenario → 同一组', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit', '审计'),
      perm('PUT /system/sys_audit', '审计'),
      perm('DELETE /system/sys_audit', '审计'),
      perm('GET /system/sys_audit', '审计'),
      perm('GET /system/sys_audit/export', '审计'),
    ]
    const got = groupPermissions(items)
    expect(got).toEqual([
      { key: '审计', title: '审计', items },
    ])
  })

  it('多个 group → 按 zh-Hans 标题升序', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_user', '用户'),
      perm('POST /system/sys_audit', '审计'),
    ]
    const got = groupPermissions(items)
    expect(got.map((g: PermissionGroup) => g.title)).toEqual(['审计', '用户'])
  })

  it('空 group → 「未分组」组,放末尾', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit', '审计'),
      perm('POST /custom/rpc', ''),
    ]
    const got = groupPermissions(items)
    expect(got).toHaveLength(2)
    expect(got[1]).toMatchObject({ key: '__untitled__', title: '未分组' })
  })

  it('全空 group → 只返回「未分组」组', () => {
    const items: PermissionItem[] = [perm('a'), perm('b')]
    const got = groupPermissions(items)
    expect(got).toHaveLength(1)
    expect(got[0]!.title).toBe('未分组')
  })

  it('group 只含空白 → 「未分组」', () => {
    const items: PermissionItem[] = [perm('a', '   ')]
    const got = groupPermissions(items)
    expect(got[0]!.title).toBe('未分组')
  })

  it('组内 items 保留输入顺序', () => {
    const items: PermissionItem[] = [
      perm('GET /x', '审计', 1),
      perm('POST /x', '审计', 2),
      perm('DELETE /x', '审计', 3),
    ]
    const got = groupPermissions(items)
    expect(got[0]!.items.map((i) => i.id)).toEqual([1, 2, 3])
  })

  it('空入参 → 空数组', () => {
    expect(groupPermissions([])).toEqual([])
  })
})