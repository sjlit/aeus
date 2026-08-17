import { defineStore } from 'pinia'
import { fetchMenuTree } from '../api/menu'
import { collectMenuUris, groupBySection, type Section } from './menuGroups'
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
    uris(state): string[] {
      return collectMenuUris(state.tree)
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