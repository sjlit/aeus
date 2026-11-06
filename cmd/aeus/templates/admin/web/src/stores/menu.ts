import { defineStore } from 'pinia'
import { fetchMenuTree } from '../api/user'
import {
  buildTree,
  flattenMenu,
  groupBySection,
  groupTitlesByUri,
  sectionize,
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
    /** 节驱动的分组(配置定义见 menuSections.ts)。CommandPalette 按节分
     *  组渲染时直接走 sectionsByUri,不再各自 DFS 一遍。 */
    sections(state): Section[] {
      return groupBySection(state.tree)
    },
    /** 一次遍历产出 uris 与 uri→标题/图标/view_path/component 等映射。
     *  调用方都把整份 flatIndex 当成快照用,不再各自派生同名 getter。 */
    flatIndex(state): MenuFlat {
      return flattenMenu(state.tree)
    },
    /** uri → 所属节名;由 sectionize(sections) 按节 DFS 产出,与扁平
     *  索引共享"seen / 出现顺序"的语义。 */
    sectionsByUri(): Map<string, string> {
      return sectionize(this.sections)
    },
    /** uri → 最近一级容器标题(用户中心/日志记录/系统设置…)。
     *  工作台的“继续工作/常用入口”用它显示更具体的归属;
     *  没有容器归属的孤儿节点回退到顶层节名。 */
    groupTitlesByUri(state): Map<string, string> {
      const out = groupTitlesByUri(state.tree)
      const sections = sectionize(groupBySection(state.tree))
      for (const uri of flattenMenu(state.tree).uris) {
        if (!out.has(uri)) out.set(uri, sections.get(uri) ?? '')
      }
      return out
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
