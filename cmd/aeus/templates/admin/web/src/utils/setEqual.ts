/** 集合等价判定:size 一致 + 单向元素包含。比 `JSON.stringify` 序列化
 *  再字符串比较快几个数量级,且不依赖元素可序列化。 */
export function setsEqual<T>(a: ReadonlySet<T>, b: ReadonlySet<T>): boolean {
  if (a.size !== b.size) return false
  for (const v of a) if (!b.has(v)) return false
  return true
}