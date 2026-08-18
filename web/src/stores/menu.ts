import { defineStore } from 'pinia'
import { fetchMenuTree } from '../api/menu'
import { flattenMenu, groupBySection, type MenuFlat, type Section } from './menuGroups'
import type { MenuNode } from '../types'

const CACHE_TTL_MS = 5 * 60 * 1000

export const useMenuStore = defineStore('menu', {
  state: () => ({
    tree: [] as MenuNode[],
    loadedAt: 0,
  }),
  getters: {
    sections(state): Section[] {
      return groupBySection(state.tree)
    },
    // 一次遍历同时产出 uris 与 uri→标题;下面的 getter 都从这里取,避免重复 DFS
    flat(state): MenuFlat {
      return flattenMenu(state.tree)
    },
    uris(): string[] {
      return this.flat.uris
    },
    titlesByUri(): Map<string, string> {
      return this.flat.titlesByUri
    },
    iconsByUri(): Map<string, string> {
      return this.flat.iconsByUri
    },
    viewsByUri(): Map<string, string> {
      return this.flat.viewsByUri
    },
  },
  actions: {
    async load(force = false) {
      if (!force && this.tree.length > 0 && Date.now() - this.loadedAt < CACHE_TTL_MS) return
      this.tree = await fetchMenuTree()
      this.loadedAt = Date.now()
    },
  },
})