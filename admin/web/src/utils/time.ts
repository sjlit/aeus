/** 工作台时间展示:相对时间与更新时间。输入统一用毫秒时间戳。 */

/**
 * 绝对日期。跨年时带年份,同年只写月-日。
 * 全站统一走这里,避免同一屏里出现 `2025-11-25` 和 `08-28` 两种写法。
 */
export function formatDate(timestamp: number, now = Date.now()): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '—'
  const date = new Date(timestamp)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  if (date.getFullYear() === new Date(now).getFullYear()) return `${month}-${day}`
  return `${date.getFullYear()}-${month}-${day}`
}

export function formatRelativeTime(timestamp: number, now = Date.now()): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '—'
  const diff = Math.max(0, now - timestamp)
  const minute = 60_000
  const hour = 60 * minute
  const day = 24 * hour

  if (diff < minute) return '刚刚'
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`
  if (diff < 2 * day) return '昨天'
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`

  return formatDate(timestamp, now)
}

/** 概览数据的“更新于”:今天只显示时间,跨天显示月-日 时间。 */
export function formatUpdatedAt(timestamp: number, now = Date.now()): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '—'
  const date = new Date(timestamp)
  const nowDate = new Date(now)
  const time = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
  if (
    date.getFullYear() === nowDate.getFullYear() &&
    date.getMonth() === nowDate.getMonth() &&
    date.getDate() === nowDate.getDate()
  ) {
    return time
  }
  return `${formatDate(timestamp, now)} ${time}`
}
