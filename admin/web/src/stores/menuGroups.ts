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

/** 一级菜单容器:无 uri 且有 children,后端把它插在 sys_menus 里用作
 *  分组标题。store.load 拿到平铺数据 buildTree 后,容器成为根节点,
 *  它的 name 直接作为侧栏节标题。 */
export function isSectionContainer(node: MenuNode): boolean {
  return !node.uri && (node.children?.length ?? 0) > 0
}

/** 把 /user/menus 的平铺结果(每行带 parent 字段)构造成嵌套树:
 *  - parent=="" 的节点是根(section container 或孤立顶级菜单);
 *  - 其他节点按 parent.component 挂到对应根的 children 数组里。
 *  parent 引用不存在 component、或者自指(component==parent)的节点
 *  被升为根(后者会引发自引用循环,直接挂进去会让 children 数组
 *  持有自身,后续 walk 进入无限递归)。与后端 Menu.BuildTree 的
 *  孤儿处理一致。输入数组的顺序决定根/同级兄弟的展示顺序——sort
 *  在后端 MenuEntry.Sort 上完成,这里不再排序。 */
export function buildTree(flat: MenuNode[]): MenuNode[] {
  if (flat.length === 0) return []
  // 先克隆一份,避免污染 store 缓存里的扁平数据;children 在下方填充。
  const nodes = flat.map((n) => ({ ...n, children: [] as MenuNode[] }))
  const byComponent = new Map<string, MenuNode>()
  for (const n of nodes) {
    byComponent.set(n.component, n)
  }
  const roots: MenuNode[] = []
  for (const n of nodes) {
    if (!n.parent || n.parent === n.component || !byComponent.has(n.parent)) {
      // parent=="" / 自指 / 指向不存在的 component,都作为根。
      roots.push(n)
    } else {
      byComponent.get(n.parent)!.children.push(n)
    }
  }
  return roots
}

/** 根节点按节分组:
 *  - section container(uri 空 + 有 children)用自身 name 作节标题;
 *  - 其他根节点继续按 uri 首段分组('system' / 'general')。
 *  首次出现顺序保留。 */
export function groupBySection(tree: MenuNode[]): Section[] {
  const map = new Map<string, MenuNode[]>()
  for (const root of tree) {
    const key = isSectionContainer(root) ? root.name : uriSegment(root.uri)
    if (!map.has(key)) map.set(key, [])
    map.get(key)!.push(root)
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
  /** uri → 服务端下发的视图路径(view_path);路由注册时按此查表。 */
  viewsByUri: Map<string, string>
}

/** 单次深度优先遍历,同时产出 uri 列表、uri → 标题/图标/view_path 映射。
 *  标题来自 n.name(后端 MenuNode 用 name 作显示名);view_path 由服务端下发,
 *  客户端不做推导。 */
export function flattenMenu(nodes: MenuNode[]): MenuFlat {
  const uris: string[] = []
  const titlesByUri = new Map<string, string>()
  const iconsByUri = new Map<string, string>()
  const viewsByUri = new Map<string, string>()
  const seen = new Set<string>()
  const walk = (ns: MenuNode[]) => {
    for (const n of ns) {
      if (n.uri && !seen.has(n.uri)) {
        seen.add(n.uri)
        uris.push(n.uri)
        titlesByUri.set(n.uri, n.name)
        iconsByUri.set(n.uri, n.icon)
        if (n.view_path) viewsByUri.set(n.uri, n.view_path)
      }
      if (n.children?.length) walk(n.children)
    }
  }
  walk(nodes)
  return { uris, titlesByUri, iconsByUri, viewsByUri }
}