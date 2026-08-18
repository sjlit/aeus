import type { MenuNode } from '../types'

/** 兜底分组名:在 SECTION_DEFINITIONS 里写一个 `{ name: 此常量, components: [] }`
 *  的条目即可把它声明为 catcher,吸纳所有未被显式列出的顶级节点。
 *  catcher 在侧栏的渲染位置由它在数组里的声明顺序决定(用户完全控制)。 */
export const DEFAULT_SECTION_NAME = '其它'

/** 侧栏分组的固定定义。前端拥有分组布局,后端只下发 parent/children
 *  树形结构。新增/调整分组、改组标题、改组归属、调整分组顺序,都只动此常量。
 *
 *  - `name`: 侧栏节标题。
 *  - `components`: 该节包含的一级菜单 component 列表(精确匹配,
 *    大小写敏感)。匹配到的一级节点放进 `section.items`,组内顺序由
 *    后端 `MenuEntry.Sort` 决定,前端不再排序。
 *  - `components: []`:catcher 节,吸纳所有未被显式列出的顶级节点;
 *    同一份 definitions 里只有第一个 catcher 真正吸纳,后续空 catcher
 *    被过滤掉。声明顺序 = 渲染顺序;声明在末尾 → 渲染在底部。
 *
 *  后端的 section container(parent='' + 无 uri)在树形结构里仍然存在,
 *  见 menuGroups.isSectionContainer / shouldRenderAsSubMenu。 */
export interface SectionDefinition {
  /** 显示标题。 */
  name: string
  /** 一级菜单节点的 component 名,精确匹配。空数组表示此 def 为 catcher。 */
  components: string[]
}

export const SECTION_DEFINITIONS: SectionDefinition[] = [
  { name: '基础能力', components: ['SystemUserCenter', 'SystemLogs', 'SystemSettings'] },
  // 兜底:catcher,声明在末尾 → 渲染在侧栏底部。想挪到顶部/中间,改顺序即可;
  // 想完全去掉"其它"分组,删掉这一行,未列入的顶级节点会被静默丢弃。
  { name: DEFAULT_SECTION_NAME, components: [] },
]

/** SectionDefinition 引用的 component 是否存在于给定树里(便于启动期检查配置漂移)。 */
export function findMissingComponents(
  definitions: SectionDefinition[],
  tree: MenuNode[],
): string[] {
  const known = new Set(tree.map((n) => n.component))
  const missing: string[] = []
  for (const def of definitions) {
    for (const comp of def.components) {
      if (!known.has(comp)) missing.push(comp)
    }
  }
  return missing
}
