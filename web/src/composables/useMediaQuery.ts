/**
 * useMediaQuery · 响应式媒体查询组合式函数
 * 返回 Ref<boolean>,mounted 时初始化,媒体查询变化时自动更新。
 * (复制自参考项目,HeaderTabStrip 移动端下拉需要)
 */
import { onMounted, onBeforeUnmount, shallowRef, type Ref } from 'vue'

export function useMediaQuery(query: string): Ref<boolean> {
  const matches = shallowRef(false)
  let mql: MediaQueryList | null = null

  onMounted(() => {
    mql = window.matchMedia(query)
    matches.value = mql.matches
    const handler = (e: MediaQueryListEvent) => {
      matches.value = e.matches
    }
    mql.addEventListener('change', handler)
    onBeforeUnmount(() => {
      mql?.removeEventListener('change', handler)
    })
  })

  return matches
}
