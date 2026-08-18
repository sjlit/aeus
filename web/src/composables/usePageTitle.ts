import { computed } from 'vue'
import { useRoute } from 'vue-router'

/** 页面标题:registerMenuRoutes 把菜单标题写入 route.meta.title,未命中回退到路径。 */
export function usePageTitle() {
  const route = useRoute()
  return computed(() => (route.meta.title as string) ?? route.path)
}
