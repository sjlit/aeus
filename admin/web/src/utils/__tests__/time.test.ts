import { describe, expect, it } from 'vitest'
import { formatRelativeTime, formatUpdatedAt } from '../time'

const NOW = new Date(2026, 10, 6, 14, 32, 0).getTime()

describe('time utils', () => {
  it('formatRelativeTime 覆盖刚刚/分钟/小时/昨天/跨年日期', () => {
    expect(formatRelativeTime(NOW - 10_000, NOW)).toBe('刚刚')
    expect(formatRelativeTime(NOW - 5 * 60_000, NOW)).toBe('5 分钟前')
    expect(formatRelativeTime(NOW - 3 * 60 * 60_000, NOW)).toBe('3 小时前')
    expect(formatRelativeTime(NOW - 30 * 60 * 60_000, NOW)).toBe('昨天')
    expect(formatRelativeTime(NOW - 3 * 24 * 60 * 60_000, NOW)).toBe('3 天前')
    expect(formatRelativeTime(new Date(2025, 0, 2).getTime(), NOW)).toBe('2025-01-02')
    expect(formatRelativeTime(0, NOW)).toBe('—')
  })

  it('formatUpdatedAt 今天只显示时间,跨天带日期', () => {
    expect(formatUpdatedAt(new Date(2026, 10, 6, 9, 5).getTime(), NOW)).toBe('09:05')
    expect(formatUpdatedAt(new Date(2026, 10, 5, 21, 8).getTime(), NOW)).toBe('11-05 21:08')
    expect(formatUpdatedAt(0, NOW)).toBe('—')
  })
})
