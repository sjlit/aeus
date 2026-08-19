import { describe, expect, it } from 'vitest'
import { deriveComponentName, normalizeGlobPath } from '../viewPath'

describe('normalizeGlobPath', () => {
  it('@/views/... 转成 ../views/...', () => {
    expect(normalizeGlobPath('@/views/system/sys_user/Index.vue'))
      .toBe('../views/system/sys_user/Index.vue')
  })

  it('/views/... 转成 ../views/...', () => {
    expect(normalizeGlobPath('/views/workspace/overview/Index.vue'))
      .toBe('../views/workspace/overview/Index.vue')
  })

  it('已经是 ../views/... 的相对形式,原样返回', () => {
    expect(normalizeGlobPath('../views/foo/Bar.vue')).toBe('../views/foo/Bar.vue')
    expect(normalizeGlobPath('views/foo/Bar.vue')).toBe('views/foo/Bar.vue')
  })
})

describe('deriveComponentName', () => {
  it('@/views 与 ./views 派生结果一致(都是 views/ 后面的段拼 PascalCase)', () => {
    expect(deriveComponentName('@/views/system/sys_user/Index.vue'))
      .toBe('SystemSysUserIndex')
    expect(deriveComponentName('../views/system/sys_user/Index.vue'))
      .toBe('SystemSysUserIndex')
  })

  it('段内 _ 转 camelCase', () => {
    expect(deriveComponentName('@/views/foo/foo_bar/Detail.vue'))
      .toBe('FooFooBarDetail')
  })

  it('段内 - 转 PascalCase', () => {
    expect(deriveComponentName('@/views/foo-bar/Baz.vue'))
      .toBe('FooBarBaz')
  })

  it('首字母大写保持兼容(Bar 原样 PascalCase)', () => {
    expect(deriveComponentName('@/views/foo/Bar.vue')).toBe('FooBar')
  })

  it('undefined / 空 / 非法路径 返回空串', () => {
    expect(deriveComponentName(undefined)).toBe('')
    expect(deriveComponentName('')).toBe('')
    expect(deriveComponentName('@/notviews/foo.vue')).toBe('')
  })
})