import { computed } from 'vue'
import { useRoute } from 'vue-router'

/** 页面标题:registerMenuRoutes 把菜单标题写入 route.meta.title,
 *  未命中回退到路径。空 fallback 表示沿用路径作兜底(原默认行为)。 */
export function usePageTitle(fallback: string = '') {
  const route = useRoute()
  return computed(() => (route.meta.title as string) ?? (fallback || route.path))
}
