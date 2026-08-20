import type { PermissionItem } from '@/api/permission'

export interface PermissionGroup {
  /** 唯一 key:group 字段,或 '未分组'(空 group 兜底)。 */
  key: string
  /** 展示标题:group 字段原值;空 → '未分组'。 */
  title: string
  items: PermissionItem[]
}

const UNTITLED_KEY = '__untitled__'
const UNTITLED_TITLE = '未分组'

/**
 * 按 PermissionItem.group 聚合(后端 sys_permissions.group 列,AutoMigrate
 * 由 derive.go 从 MenuEntry().Name 写入)。组内保留输入顺序;组间按
 * zh-Hans 标题升序;空 group 归入 '未分组',放末尾。
 *
 * 故意只读 inputs,不修改;不为聚合键做大小写归一化——后端字段是
 * operator-visible 的展示文本,大小写敏感更可预测。
 */
export function groupPermissions(perms: PermissionItem[]): PermissionGroup[] {
  const groups = new Map<string, PermissionGroup>()
  for (const p of perms) {
    const raw = p.group.trim()
    const title = raw || UNTITLED_TITLE
    const key = raw || UNTITLED_KEY
    let g = groups.get(key)
    if (!g) {
      g = { key, title, items: [] }
      groups.set(key, g)
    }
    g.items.push(p)
  }
  const named = [...groups.entries()]
    .filter(([k]) => k !== UNTITLED_KEY)
    .map(([, g]) => g)
    .sort((a, b) => a.title.localeCompare(b.title, 'zh-Hans-CN'))
  const untitled = groups.get(UNTITLED_KEY)
  return untitled ? [...named, untitled] : named
}