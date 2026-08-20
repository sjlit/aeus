import { describe, expect, it } from 'vitest'
import {
  groupKey,
  groupPermissions,
  groupPermissionsAsTree,
  isGroupKey,
  type ApiTreeNode,
} from '../groupPermissions'
import type { PermissionItem } from '@/api/permission'

function perm(data: string, group = '', id = 1): PermissionItem {
  return { id, type: 'api', data, description: '', group }
}

describe('groupKey / isGroupKey', () => {
  it('合成 key 前缀稳定,不与真 permission.data 冲突', () => {
    expect(groupKey('审计')).toBe('__group:审计')
    expect(isGroupKey('__group:审计')).toBe(true)
  })

  it('真 permission.data 不被识别为 group', () => {
    expect(isGroupKey('POST /system/sys_user')).toBe(false)
    expect(isGroupKey('__group_审计')).toBe(false) // 不同前缀
    expect(isGroupKey('')).toBe(false)
  })
})

describe('groupPermissions', () => {
  it('同一 group 多 scenario → 同一组', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit', '审计'),
      perm('PUT /system/sys_audit', '审计'),
      perm('GET /system/sys_audit', '审计'),
      perm('GET /system/sys_audit/export', '审计'),
    ]
    const got = groupPermissions(items)
    expect(got).toEqual([{ key: '审计', title: '审计', items }])
  })

  it('多个 group → 按 zh-Hans 标题升序', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_user', '用户'),
      perm('POST /system/sys_audit', '审计'),
    ]
    expect(groupPermissions(items).map((g) => g.title)).toEqual(['审计', '用户'])
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
    const got = groupPermissions([perm('a'), perm('b')])
    expect(got).toHaveLength(1)
    expect(got[0]!.title).toBe('未分组')
  })

  it('group 只含空白 → 「未分组」', () => {
    expect(groupPermissions([perm('a', '   ')])[0]!.title).toBe('未分组')
  })

  it('组内 items 保留输入顺序', () => {
    const items: PermissionItem[] = [
      perm('GET /x', '审计', 1),
      perm('POST /x', '审计', 2),
      perm('DELETE /x', '审计', 3),
    ]
    expect(groupPermissions(items)[0]!.items.map((i) => i.id)).toEqual([1, 2, 3])
  })

  it('空入参 → 空数组', () => {
    expect(groupPermissions([])).toEqual([])
  })
})

describe('groupPermissionsAsTree', () => {
  it('每个 group 是一个父节点,叶子 id == permission.data', () => {
    const items: PermissionItem[] = [
      perm('POST /system/sys_audit', '审计'),
      perm('GET /system/sys_audit', '审计'),
      perm('POST /system/sys_user', '用户'),
    ]
    const tree = groupPermissionsAsTree(items)
    // 标题升序:审计 → 用户
    expect(tree).toHaveLength(2)
    expect(tree[0]).toMatchObject({ id: '__group:审计', label: '审计' })
    expect(tree[0]!.children!.map((c) => c.id)).toEqual([
      'POST /system/sys_audit',
      'GET /system/sys_audit',
    ])
    expect(tree[0]!.children![0]!.data).toBe('POST /system/sys_audit')
    expect(tree[1]).toMatchObject({ id: '__group:用户', label: '用户' })
  })

  it('叶子携带 data 字段,提交保存时直接读', () => {
    const tree = groupPermissionsAsTree([perm('GET /x', 'A')])
    expect(tree[0]!.children![0]!.data).toBe('GET /x')
  })

  it('空 group 归入「未分组」', () => {
    const tree = groupPermissionsAsTree([perm('GET /x', '')])
    expect(tree[0]!.id).toBe('__group:未分组')
  })

  it('空入参 → 空数组', () => {
    expect(groupPermissionsAsTree([])).toEqual<ApiTreeNode[]>([])
  })
})