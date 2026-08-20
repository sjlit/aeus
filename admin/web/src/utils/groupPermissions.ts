import type { PermissionItem } from '@/api/permission'

export interface PermissionGroup {
  /** 唯一 key:uri 或 `__other__`(无 menu 命中的兜底)。 */
  key: string
  /** 展示标题:命中 menu 时用 menu.name;否则用 uri 或 "其他"。 */
  title: string
  items: PermissionItem[]
}

const OTHER_KEY = '__other__'
const OTHER_TITLE = '其他'

/** 把 'POST /system/sys_audit' 拆出 URI。'POST ' 后无空格 = 损坏数据,返回 ''。 */
export function extractApiUri(data: string): string {
  const i = data.indexOf(' ')
  return i < 0 ? '' : data.slice(i + 1)
}

interface MenuRef {
  uri: string
  name: string
}

/**
 * 把 PermissionItem[] 按 module+table(经 menu 映射)聚合:
 * - 每个 permission 的 uri 用 menu.uri 查 menu.name,作为 group title;
 * - menu 缺失的 permission 全部归到 `__other__` 组;
 * - 损坏 data(indexOf(' ') < 0)跳过;
 * - 组内 items 保持输入顺序;
 * - 组按 title 升序;`__other__` 永远最后。
 *
 * 故意只读 inputs,不修改;menuRefs 允许只填 uri+name 两个字段,
 * 避免外部依赖整个 MenuTreeNode 形状,便于测试。
 */
export function groupPermissionsByMenu(
  perms: PermissionItem[],
  menuRefs: MenuRef[],
): PermissionGroup[] {
  const titleByUri = new Map<string, string>()
  for (const m of menuRefs) {
    if (m.uri) titleByUri.set(m.uri, m.name)
  }
  const groups = new Map<string, PermissionGroup>()
  let otherItems: PermissionItem[] = []
  for (const p of perms) {
    const uri = extractApiUri(p.data)
    if (!uri) continue
    const title = titleByUri.get(uri)
    if (!title) {
      otherItems.push(p)
      continue
    }
    let g = groups.get(uri)
    if (!g) {
      g = { key: uri, title, items: [] }
      groups.set(uri, g)
    }
    g.items.push(p)
  }
  if (otherItems.length > 0) {
    groups.set(OTHER_KEY, { key: OTHER_KEY, title: OTHER_TITLE, items: otherItems })
  }
  const named = [...groups.entries()]
    .filter(([k]) => k !== OTHER_KEY)
    .map(([, g]) => g)
    .sort((a, b) => a.title.localeCompare(b.title, 'zh-Hans-CN'))
  const other = groups.get(OTHER_KEY)
  return other ? [...named, other] : named
}