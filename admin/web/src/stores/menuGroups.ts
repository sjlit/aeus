import type { MenuNode } from '../types'
import {
  SECTION_DEFINITIONS,
  type SectionDefinition,
} from './menuSections'

export interface Section {
  name: string
  items: MenuNode[]
}

/** 一级菜单容器:无 uri 且有 children,后端把它插在 sys_menus 里用作
 *  逻辑父节点(parent="SystemXxx")。store.load 拿到平铺数据 buildTree
 *  后,容器成为根节点,它的 name 在侧栏里作为一级菜单标题出现。 */
export function isSectionContainer(node: MenuNode): boolean {
  return !node.uri && (node.children?.length ?? 0) > 0
}

/** 容器在所属节里的渲染策略。SidebarContent 用它决定是把容器渲染成
 *  el-sub-menu(显示容器自身标题 + 子项),还是把 children 平铺成 el-menu-item。
 *
 *  规则:容器名 ≠ 节名 → sub-menu(呈现一级标题;多容器归到一节时尤其需要);
 *       容器名 = 节名 → 平铺(legacy "1 节 = 1 容器" 布局,避免节标题与
 *       容器标题重复)。
 *
 *  该函数只判断"要不要把容器当成 sub-menu";是否真的渲染为 sub-menu
 *  还要看 isSectionContainer(node) 为真(否则走普通 sub-menu / leaf 分支)。 */
export function shouldRenderAsSubMenu(
  container: MenuNode,
  sectionName: string,
): boolean {
  return container.name !== sectionName
}

/** 把 /user/menus 的平铺结果(每行带 parent 字段)构造成嵌套树:
 *  - parent=="" 的节点是根(section container 或孤立顶级菜单);
 *  - 其他节点按 parent.component 挂到对应根的 children 数组里。
 *  parent 引用不存在 component、或者自指(component==parent)的节点
 *  被升为根(后者会引发自引用循环,直接挂进去会让 children 数组
 *  持有自身,后续 walk 进入无限递归)。与后端 Menu.BuildTree 的
 *  孤儿处理一致。输入数组的顺序决定根/同级兄弟的展示顺序——sort
 *  在后端 MenuEntry.Sort 上完成,这里不再排序。
 *
 *  泛型 + 约束让 /sys_menus 那张 CRUD 表复用同一份实现:它按 MenuItem
 *  的 component/parent 字段走(同结构),不必再各自维护一份 O(N²) 实
 *  现。返回值带 children 字段,call site 可以 narrow 到自己的节点类型。 */
export function buildTree<T extends { component: string; parent: string }>(
  flat: T[],
): (T & { children: T[] })[] {
  if (flat.length === 0) return []
  // 先克隆一份,避免污染 store 缓存里的扁平数据;children 在下方填充。
  const nodes = flat.map((n) => ({ ...n, children: [] as T[] }))
  const byComponent = new Map<string, T & { children: T[] }>()
  for (const n of nodes) {
    byComponent.set(n.component, n)
  }
  const roots: (T & { children: T[] })[] = []
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

/** 根节点按节分组(配置驱动):
 *  - `definitions` 里列出 component 的 def → 进对应分组的 `items`(节名取 `def.name`);
 *  - `definitions` 里 `components: []` 的 def 是 "catcher" 节,吸纳所有未被
 *    显式列出的顶级节点,渲染位置由它在数组里的声明位置决定(用户完全
 *    控制顺序);
 *  - 未声明 catcher 时,未列入的顶级节点被静默丢弃——分组策略完全交给
 *    配置,不再有隐式默认节。
 *  - 输出过滤掉空节(包括没有未列入节点的 catcher、引用了不存在的
 *    component 的 def),保证侧栏不会出现空标题。
 *
 *  多个 catcher 出现时,只有第一个声明位置的最先那个真正吸纳;后续空
 *  catcher 一并被过滤掉(避免重复标题)。
 *
 *  组内顺序保留 buildTree 的顺序,即后端 `MenuEntry.Sort`。 */
export function groupBySection(
  tree: MenuNode[],
  definitions: SectionDefinition[] = SECTION_DEFINITIONS,
): Section[] {
  const byComponent = new Map<string, MenuNode>()
  for (const root of tree) byComponent.set(root.component, root)

  const assigned = new Set<string>()
  const sections: Section[] = []
  let catcherIdx = -1

  for (let i = 0; i < definitions.length; i++) {
    const def = definitions[i]
    const items: MenuNode[] = []
    for (const comp of def.components) {
      const node = byComponent.get(comp)
      if (node) {
        items.push(node)
        assigned.add(comp)
      }
    }
    if (def.components.length === 0 && catcherIdx < 0) catcherIdx = i
    sections.push({ name: def.name, items })
  }

  if (catcherIdx >= 0) {
    const catcher = sections[catcherIdx]
    for (const root of tree) {
      if (!assigned.has(root.component)) catcher.items.push(root)
    }
  }

  // 过滤空节:无未列入节点的 catcher + 引用不存在 component 的 def,都不渲染。
  return sections.filter((s) => s.items.length > 0)
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
  /** uri → 服务端下发的稳定 component ID(后端 MenuEntry.component);
   *  优先作为 <keep-alive :include> 的匹配名(meta.componentName),
   *  这样 view 文件里 defineOptions({ name }) 可以直接对齐服务端语义,
   *  无需再从 view_path 派生。fallback 时再走 deriveComponentName(viewPath)。 */
  componentsByUri: Map<string, string>
}

/** 把 groupBySection 的输出扫一遍,产出 uri → section 的映射。
 *  用于 CommandPalette 这类"按节分组平铺"的视图共享一次遍历,
 *  避免各自再写一份 walk + seen 去重。 */
export function sectionize(
  sections: Section[],
  out: Map<string, string> = new Map(),
): Map<string, string> {
  const seen = new Set<string>()
  const walk = (nodes: MenuNode[], section: string) => {
    for (const n of nodes) {
      if (n.uri && !seen.has(n.uri)) {
        seen.add(n.uri)
        out.set(n.uri, section)
      }
      if (n.children?.length) walk(n.children, section)
    }
  }
  for (const sec of sections) walk(sec.items, sec.name)
  return out
}

/** 单次深度优先遍历,同时产出 uri 列表、uri → 标题/图标/view_path/component 映射。
 *  标题来自 n.name(后端 MenuNode 用 name 作显示名);view_path 与 component 由
 *  服务端下发,客户端不推导。section 信息不在这里产出——交给 sectionize(sections)
 *  走另一条按节 DFS 的路径,与原有 groupBySection 输出对齐。 */
export function flattenMenu(nodes: MenuNode[]): MenuFlat {
  const uris: string[] = []
  const titlesByUri = new Map<string, string>()
  const iconsByUri = new Map<string, string>()
  const viewsByUri = new Map<string, string>()
  const componentsByUri = new Map<string, string>()
  const seen = new Set<string>()
  const walk = (ns: MenuNode[]) => {
    for (const n of ns) {
      if (n.uri && !seen.has(n.uri)) {
        seen.add(n.uri)
        uris.push(n.uri)
        titlesByUri.set(n.uri, n.name)
        iconsByUri.set(n.uri, n.icon)
        if (n.view_path) viewsByUri.set(n.uri, n.view_path)
        if (n.component) componentsByUri.set(n.uri, n.component)
      }
      if (n.children?.length) walk(n.children)
    }
  }
  walk(nodes)
  return { uris, titlesByUri, iconsByUri, viewsByUri, componentsByUri }
}