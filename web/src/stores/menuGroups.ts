import type { MenuNode } from '../types'

export interface Section {
  name: string
  items: MenuNode[]
}

/** 取 uri 首段('/system/sys-users' → 'system');空 → 'general'。 */
export function uriSegment(uri: string): string {
  const parts = uri.split('/').filter(Boolean)
  return parts[0] ?? 'general'
}

/** 根节点按 uri 首段分组,保持首次出现顺序。 */
export function groupBySection(tree: MenuNode[]): Section[] {
  const map = new Map<string, MenuNode[]>()
  for (const root of tree) {
    const seg = uriSegment(root.uri)
    if (!map.has(seg)) map.set(seg, [])
    map.get(seg)!.push(root)
  }
  return [...map.entries()].map(([name, items]) => ({ name, items }))
}

export interface MenuFlat {
  /** 所有非空 uri,去重、保持出现顺序(深度优先)。 */
  uris: string[]
  /** uri → 标题;路由注册(meta.title)与面包屑共用,一次遍历得到。 */
  titlesByUri: Map<string, string>
  /** uri → 图标短串;多标签页的标签图标用,与 uris 同一次遍历得到。 */
  iconsByUri: Map<string, string>
}

/** 单次深度优先遍历,同时产出 uri 列表、uri → 标题与 uri → 图标映射(替代原先两个独立 DFS)。 */
export function flattenMenu(nodes: MenuNode[]): MenuFlat {
  const uris: string[] = []
  const titlesByUri = new Map<string, string>()
  const iconsByUri = new Map<string, string>()
  const seen = new Set<string>()
  const walk = (ns: MenuNode[]) => {
    for (const n of ns) {
      if (n.uri && !seen.has(n.uri)) {
        seen.add(n.uri)
        uris.push(n.uri)
        titlesByUri.set(n.uri, n.title)
        iconsByUri.set(n.uri, n.icon)
      }
      if (n.children?.length) walk(n.children)
    }
  }
  walk(nodes)
  return { uris, titlesByUri, iconsByUri }
}