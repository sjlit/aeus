import { describe, expect, it } from 'vitest'
import { setsEqual } from '../setEqual'

describe('setsEqual', () => {
  it('两个相同 size 的 Set,元素一致 → true', () => {
    expect(setsEqual(new Set(['a', 'b']), new Set(['b', 'a']))).toBe(true)
  })

  it('size 不同 → false', () => {
    expect(setsEqual(new Set(['a']), new Set(['a', 'b']))).toBe(false)
  })

  it('size 一致但元素不同 → false', () => {
    expect(setsEqual(new Set(['a', 'b']), new Set(['a', 'c']))).toBe(false)
  })

  it('两个空 Set → true', () => {
    expect(setsEqual(new Set(), new Set())).toBe(true)
  })

  it('空 Set vs 非空 → false', () => {
    expect(setsEqual(new Set(), new Set(['a']))).toBe(false)
  })

  it('支持任意类型(generic)', () => {
    expect(setsEqual(new Set([1, 2]), new Set([2, 1]))).toBe(true)
    expect(setsEqual<number>(new Set([1, 2]), new Set([1]))).toBe(false)
  })
})