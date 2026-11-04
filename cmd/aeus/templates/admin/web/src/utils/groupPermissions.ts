import type { PermissionItem } from '@/api/permission'

const UNTITLED_TITLE = '未分组'
const UNTITLED_KEY = '__untitled__'

/** 合成 group 节点的 node-key 前缀。后端 Permission.Data 永远是
 *  'METHOD URI' 格式(必含 1 个空格),不可能以 '__group:' 开头,
 *  所以这个前缀不会与真 permission.data 冲突 —— save 时一过滤就
 *  安全。把 group node-key 集中在一处,避免模板和保存逻辑各自拼
 *  字符串漂移。 */
export const GROUP_KEY_PREFIX = '__group:'

/** 给定 group title 算出虚拟父节点的 node-key。 */
export function groupKey(title: string): string {
  return GROUP_KEY_PREFIX + title
}

/** 判断一个 node-key 是 group 虚拟节点还是真 permission.data。
 *  真 permission.data 永远不含 '__group:' 前缀(derive.go 输出是
 *  '<METHOD> <URI>',METHOD 全大写无下划线)。 */
export function isGroupKey(key: string): boolean {
  return key.startsWith(GROUP_KEY_PREFIX)
}

/** el-tree 的节点形状:group 是父,permission.data 是叶。父节点 id
 *  用合成 key,叶子直接用 permission.data 作为 id,方便
 *  setCheckedKeys/getCheckedKeys/getHalfCheckedKeys 直接产出叶子集合。 */
export interface ApiTreeNode {
  id: string
  label: string
  children?: ApiTreeNode[]
  /** 仅叶子有:真实 permission 行的 data 字段。提交保存时直接读这个。 */
  data?: string
}

export interface PermissionGroup {
  /** 唯一 key:group 字段,或 '__untitled__'(空 group 兜底)。 */
  key: string
  /** 展示标题:group 字段原值;空 → '未分组'。 */
  title: string
  items: PermissionItem[]
}

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

/**
 * 把 groupPermissions 结果拍平成 el-tree 节点数组。每组的第一个节点
 * 是父节点(id = groupKey(title),children = permission leaves),
 * 叶子节点的 id 就是 permission.data —— 直接 setCheckedKeys(savedApis)
 * 就能回填,不需要再做映射。
 *
 * 为什么不直接复用 groupPermissions:这里有"输出形状"和"原始数据"
 * 两套视角,前者必须稳定以便单元测试断言。后端加列时两层都加;
 * 改 group 命名规则时只动 groupPermissions。
 */
export function groupPermissionsAsTree(perms: PermissionItem[]): ApiTreeNode[] {
  return groupPermissions(perms).map((g) => ({
    id: groupKey(g.title),
    label: g.title,
    children: g.items.map((p) => ({
      id: p.data,
      label: p.data,
      data: p.data,
    })),
  }))
}