import { defineStore } from 'pinia'
import { fetchMenuTree } from '../api/user'
import {
  buildTree,
  flattenMenu,
  groupBySection,
  type MenuFlat,
  type Section,
} from './menuGroups'
import type { MenuNode } from '../types'

const CACHE_TTL_MS = 5 * 60 * 1000

export const useMenuStore = defineStore('menu', {
  state: () => ({
    /** 后端 /user/menus 的原始平铺响应(每行带 parent 字段,children 为空)。
     *  保留原始数据便于排查;导航/分组始终走 tree。 */
    flat: [] as MenuNode[],
    /** buildTree(flat) 的产物,根节点是 section container(parent==="")
     *  与孤儿节点(找不到 parent),children 按 parent.component 嵌套。 */
    tree: [] as MenuNode[],
    loadedAt: 0,
  }),
  getters: {
    sections(state): Section[] {
      return groupBySection(state.tree)
    },
    // 一次遍历同时产出 uris 与 uri→标题;下面的 getter 都从这里取,避免重复 DFS
    flatIndex(state): MenuFlat {
      return flattenMenu(state.tree)
    },
    uris(): string[] {
      return this.flatIndex.uris
    },
    titlesByUri(): Map<string, string> {
      return this.flatIndex.titlesByUri
    },
    iconsByUri(): Map<string, string> {
      return this.flatIndex.iconsByUri
    },
    viewsByUri(): Map<string, string> {
      return this.flatIndex.viewsByUri
    },
    /**
     * uri → 服务端下发的 stable component ID。registerMenuRoutes 把它写到
     * meta.componentName,然后 <keep-alive :include>(= tabs.cachedViews)就能
     * 精确匹配到对应 view 文件 declareOptions({ name })。
     */
    componentsByUri(): Map<string, string> {
      return this.flatIndex.componentsByUri
    },
  },
  actions: {
    async load(force = false) {
      if (!force && this.tree.length > 0 && Date.now() - this.loadedAt < CACHE_TTL_MS) return
      const raw = await fetchMenuTree()
      this.flat = raw
      this.tree = buildTree(raw)
      this.loadedAt = Date.now()
    },
  },
})