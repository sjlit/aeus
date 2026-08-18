import { describe, it, expect } from 'vitest'
import { envelopeCode, envelopeMessage, isAuthFailureCode } from '../envelope'

describe('envelopeCode', () => {
  it('从信封提取 code', () => {
    expect(envelopeCode({ code: 0, message: '', data: 1 })).toBe(0)
    expect(envelopeCode({ code: 4002, message: 'expired' })).toBe(4002)
  })
  it('非对象或非信封返回 -1', () => {
    expect(envelopeCode(null)).toBe(-1)
    expect(envelopeCode('x')).toBe(-1)
    expect(envelopeCode({})).toBe(-1)
  })
})

describe('envelopeMessage', () => {
  it('提取 message,缺失时返回空串', () => {
    expect(envelopeMessage({ code: 4005, message: 'invalid username or password' })).toBe('invalid username or password')
    expect(envelopeMessage(null)).toBe('')
  })
})

describe('isAuthFailureCode', () => {
  it('4001/4002/4006 判定为认证失败', () => {
    expect(isAuthFailureCode(4001)).toBe(true)
    expect(isAuthFailureCode(4002)).toBe(true)
    expect(isAuthFailureCode(4006)).toBe(true)
  })
  it('4003/4005/0 不是认证失败', () => {
    expect(isAuthFailureCode(4003)).toBe(false)
    expect(isAuthFailureCode(4005)).toBe(false)
    expect(isAuthFailureCode(0)).toBe(false)
  })
})