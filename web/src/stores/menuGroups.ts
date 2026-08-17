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

/** 所有非空 uri,去重、保持出现顺序(深度优先)。 */
export function collectMenuUris(nodes: MenuNode[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  const walk = (ns: MenuNode[]) => {
    for (const n of ns) {
      if (n.uri && !seen.has(n.uri)) {
        seen.add(n.uri)
        out.push(n.uri)
      }
      if (n.children?.length) walk(n.children)
    }
  }
  walk(nodes)
  return out
}

/** 按 uri 找节点标题(深度优先);找不到返回 uri 本身。 */
export function titleForUri(uri: string, nodes: MenuNode[]): string {
  for (const n of nodes) {
    if (n.uri === uri) return n.title
    if (n.children?.length) {
      const t = titleForUri(uri, n.children)
      if (t !== uri) return t
    }
  }
  return uri
}