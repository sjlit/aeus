import { describe, expect, it } from 'vitest'
import { normalizeGlobPath } from '../viewPath'

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