import { describe, expect, it } from 'vitest'
import { resolveBrand } from '../brand'

describe('resolveBrand', () => {
  it('未配置时保留默认 Board / D', () => {
    expect(resolveBrand()).toEqual({ name: 'Board', mark: 'D' })
    expect(resolveBrand(null)).toEqual({ name: 'Board', mark: 'D' })
  })

  it('自定义名称时用首字符作为 mark', () => {
    expect(resolveBrand({ appName: 'Acme' })).toEqual({ name: 'Acme', mark: 'A' })
    expect(resolveBrand({ appName: '商城' })).toEqual({ name: '商城', mark: '商' })
  })

  it('appMark 优先于名称派生', () => {
    expect(resolveBrand({ appName: 'Acme', appMark: 'z' })).toEqual({ name: 'Acme', mark: 'Z' })
  })

  it('空白或非法值回退到默认值', () => {
    expect(resolveBrand({ appName: '  ', appMark: '  ' })).toEqual({ name: 'Board', mark: 'D' })
    expect(resolveBrand({ appName: 123 as unknown as string })).toEqual({ name: 'Board', mark: 'D' })
  })
})
