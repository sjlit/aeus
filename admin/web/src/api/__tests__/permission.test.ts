import { describe, expect, it } from 'vitest'
import { parseApiUri } from '../permission'

describe('parseApiUri', () => {
  it('拆 POST /system/sys_user 为 /system/sys_user', () => {
    expect(parseApiUri('POST /system/sys_user')).toBe('/system/sys_user')
  })

  it('拆 GET /system/sys_user/detail/:id', () => {
    expect(parseApiUri('GET /system/sys_user/detail/:id')).toBe(
      '/system/sys_user/detail/:id',
    )
  })

  it('method 后多个空格:只切第一个,保留后续空格作为 uri 一部分', () => {
    expect(parseApiUri('POST /foo bar')).toBe('/foo bar')
  })

  it('无空格 → 空串(损坏数据,UI 应跳过)', () => {
    expect(parseApiUri('malformed')).toBe('')
  })

  it('空串 → 空串', () => {
    expect(parseApiUri('')).toBe('')
  })
})