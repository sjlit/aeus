<script setup lang="ts">
// 与 router/index.ts 里菜单路由 meta.componentName (= deriveComponentName('@/views/system/sys_role/Permission.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'SystemSysRolesPermission' })

import { computed, onBeforeUnmount, onMounted, ref, watch, watchEffect } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import type { ElTree } from 'element-plus'
import {
  fetchRolePermissions,
  replaceRolePermissions,
  type RolePermissions,
} from '@/api/role'
import { fetchPermissionList, type PermissionItem } from '@/api/permission'
import { fetchMenuTreeAll, type MenuTreeNode } from '@/api/menu'
import { setsEqual } from '@/utils/setEqual'
import { groupPermissionsByMenu, type PermissionGroup } from '@/utils/groupPermissions'

const route = useRoute()
const router = useRouter()

/** vue-router 对 :roleKey 这类单段动态参数返回 string,无需 array 兜底。
 *  空串表示路由参数缺失,由 mount 钩子统一跳回列表页。 */
const roleKey = computed(() => (route.params.roleKey as string) ?? '')

const activeTab = ref<'menus' | 'apis'>('menus')

/** 当前正在保存的 tab。null 表示都空闲;按钮用 :loading="saving === 'menus'"
 *  判断自身的 spinner,互不干扰。 */
const saving = ref<'menus' | 'apis' | null>(null)

// ── 数据 ───────────────────────────────────────────────
const loading = ref(false)
const menuTree = ref<MenuTreeNode[]>([])
const apiPermissions = ref<PermissionItem[]>([])
/** 角色当前已分配的 menus / apis —— 入参是持久化副本,保存时与本地
 *  勾选合并作为「不变项」随 PUT 一起带走,避免单 tab 保存清空另一侧。 */
const savedMenus = ref<Set<string>>(new Set())
const savedApis = ref<Set<string>>(new Set())

// ── 菜单 tab ───────────────────────────────────────────
const menuTreeRef = ref<InstanceType<typeof ElTree>>()

async function loadAll() {
  if (!roleKey.value) {
    void router.replace('/system/sys_roles')
    return
  }
  loading.value = true
  try {
    // /menu/tree 在设计上与角色无关,所以即便失败也不阻塞已分配授权的渲染
    const [perms, list, treeRes] = await Promise.allSettled([
      fetchRolePermissions(roleKey.value),
      fetchPermissionList('api'),
      fetchMenuTreeAll(),
    ])
    if (perms.status === 'fulfilled') {
      const p: RolePermissions = perms.value
      savedMenus.value = new Set(p.menus)
      savedApis.value = new Set(p.apis)
    }
    if (list.status === 'fulfilled') {
      apiPermissions.value = list.value.items
    }
    if (treeRes.status === 'fulfilled') {
      menuTree.value = treeRes.value.items
    }
  } finally {
    loading.value = false
  }
}
onMounted(loadAll)

// 树实例挂载后,把 savedMenus 回填到 el-tree 的勾选状态。
// 不能用 once:true:[menuTreeRef, menuTree] 二源 watch——
// 两者都"就绪"时 savedApis/savedMenus 还在网络往返中(perms 通常晚于
// /menu/tree 几 ms),快照就空,perms 回来后 watcher 已停。改成「tree
// 实例挂上后,任何 savedMenus 变化都重放」。repeat 不可避免:用户点了
// save 推进 savedMenus → 重新 setCheckedKeys 渲染最新勾选,这是想要
// 的效果(O(tree) 一次遍历,可接受)。
watch([menuTreeRef, savedMenus], ([ref, menus]) => {
  if (!ref || menus.size === 0) return
  ref.setCheckedKeys([...menus], false)
})

async function saveMenus() {
  if (!roleKey.value) return
  const tree = menuTreeRef.value
  if (!tree) return
  // half-checked 一并带走——子勾选代表父节点授权可见,后端 ValidateMenuData
  // 接受所有真实 component。空字符串 component 不可能出现在 el-tree
  // (node-key 必须非空),所以无需 filter 兜底。
  const checked = tree.getCheckedKeys() as string[]
  const half = tree.getHalfCheckedKeys() as string[]
  // 避免 [...checked, ...half] 中间数组——直接两个 for 写入 Set。
  const next = new Set<string>()
  for (const k of checked) next.add(k)
  for (const k of half) next.add(k)
  saving.value = 'menus'
  try {
    await replaceRolePermissions({
      role: roleKey.value,
      menus: [...next],
      apis: [...savedApis.value],
    })
    savedMenus.value = next
    ElMessage.success('菜单已保存')
  } catch {
    // 错误由 http 拦截器 toast;本地状态保持,允许重试。
  } finally {
    saving.value = null
  }
}

function resetMenus() {
  menuTreeRef.value?.setCheckedKeys([...savedMenus.value], false)
}

// ── 权限 tab ───────────────────────────────────────────
const openedGroups = ref<string[]>([])

/** 把 menuTree 拍平成 {uri,name} 引用;只读 uri+name 两个字段,
 *  减少 watcher 对无关字段的依赖。 */
const menuRefs = computed<{ uri: string; name: string }[]>(() => {
  const out: { uri: string; name: string }[] = []
  const walk = (nodes: MenuTreeNode[]) => {
    for (const n of nodes) {
      if (n.uri) out.push({ uri: n.uri, name: n.title })
      if (n.children?.length) walk(n.children)
    }
  }
  walk(menuTree.value)
  return out
})

const apiGroups = computed<PermissionGroup[]>(() =>
  groupPermissionsByMenu(apiPermissions.value, menuRefs.value),
)

/** 每个分组当前勾选中的 permission data。初始化由下方 watchEffect 负责
 *  (数据到达 + 该组尚未被用户编辑时);后续由用户编辑 + saveApis 推进。 */
const groupChecked = ref<Record<string, string[]>>({})

// 数据 / savedApis 变化时初始化 groupChecked(只在该组尚未被用户编辑时)
watchEffect(() => {
  for (const g of apiGroups.value) {
    if (groupChecked.value[g.key]?.length) continue
    const initial: string[] = []
    for (const p of g.items) {
      if (savedApis.value.has(p.data)) initial.push(p.data)
    }
    groupChecked.value[g.key] = initial
  }
})

/** 每个分组上一次「已保存」状态 —— 直接由 savedApis × apiGroups 派生,
 *  不需要手动维护快照:saveApis 推进 savedApis 之后此 computed 自动更新,
 *  dirty diff 与 resetApis 都读它。 */
const savedGroupChecked = computed<Record<string, Set<string>>>(() => {
  const out: Record<string, Set<string>> = {}
  for (const g of apiGroups.value) {
    const s = new Set<string>()
    for (const p of g.items) {
      if (savedApis.value.has(p.data)) s.add(p.data)
    }
    out[g.key] = s
  }
  return out
})

async function saveApis() {
  if (!roleKey.value) return
  const next: string[] = []
  for (const g of apiGroups.value) {
    next.push(...(groupChecked.value[g.key] ?? []))
  }
  saving.value = 'apis'
  try {
    await replaceRolePermissions({
      role: roleKey.value,
      menus: [...savedMenus.value],
      apis: next,
    })
    savedApis.value = new Set(next)
    // savedGroupChecked 由 computed 自动反映新 savedApis,无需手动同步。
    ElMessage.success('权限已保存')
  } catch {
    // 错误由 http 拦截器 toast;本地状态保持,允许重试。
  } finally {
    saving.value = null
  }
}

function resetApis() {
  for (const g of apiGroups.value) {
    const snap = savedGroupChecked.value[g.key]
    groupChecked.value[g.key] = snap ? [...snap] : []
  }
}

// ── dirty 检测 & 离开守卫 ─────────────────────────────
const menusDirty = computed(() => {
  const tree = menuTreeRef.value
  if (!tree) return false
  const checked = new Set<string>([
    ...(tree.getCheckedKeys() as string[]),
    ...(tree.getHalfCheckedKeys() as string[]),
  ])
  return !setsEqual(checked, savedMenus.value)
})

const apisDirty = computed(() => {
  for (const g of apiGroups.value) {
    const snap = savedGroupChecked.value[g.key] ?? new Set<string>()
    const cur = new Set(groupChecked.value[g.key] ?? [])
    if (!setsEqual(snap, cur)) return true
  }
  return false
})

const isDirty = computed(() => menusDirty.value || apisDirty.value)

/** 离开确认:无改动直接放行,有改动弹确认。vue-router 4 的
 *  onBeforeRouteLeave 支持返回 Promise<boolean>,无需旁路状态。 */
async function confirmLeave(): Promise<boolean> {
  if (!isDirty.value) return true
  try {
    await ElMessageBox.confirm('有未保存的改动,确定离开?', '提示', {
      type: 'warning',
      confirmButtonText: '离开',
      cancelButtonText: '留下',
    })
    return true
  } catch {
    return false
  }
}

onBeforeRouteLeave(async () => confirmLeave())

// 兜底:浏览器关闭 / 刷新时也弹确认
function beforeUnload(e: BeforeUnloadEvent) {
  if (isDirty.value) {
    e.preventDefault()
    e.returnValue = ''
  }
}
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>

<template>
  <div class="page role-perm-page" v-loading="loading">
    <header class="page-head glass">
      <div class="head-meta">
        <h2 class="head-title">角色权限配置</h2>
        <p class="head-sub">
          当前角色:
          <code class="role-key">{{ roleKey || '—' }}</code>
        </p>
      </div>
    </header>

    <section class="glass tabs-card">
      <el-tabs v-model="activeTab" class="role-perm-tabs">
        <!-- ── Tab 1:菜单配置 ─────────────────────────── -->
        <el-tab-pane name="menus">
          <template #label>
            <span class="tab-label">菜单配置</span>
          </template>

          <el-tree
            ref="menuTreeRef"
            :data="menuTree"
            show-checkbox
            node-key="component"
            :props="{ label: 'title', children: 'children' }"
            empty-text="暂无可配置菜单"
            class="menu-tree"
          />

          <div class="tab-actions">
            <el-button @click="resetMenus">重置</el-button>
            <el-button type="primary" :loading="saving === 'menus'" @click="saveMenus">
              保存菜单
            </el-button>
          </div>
        </el-tab-pane>

        <!-- ── Tab 2:权限配置 ─────────────────────────── -->
        <el-tab-pane name="apis">
          <template #label>
            <span class="tab-label">权限配置</span>
          </template>

          <el-collapse v-model="openedGroups" v-if="apiGroups.length > 0">
            <el-collapse-item
              v-for="group in apiGroups"
              :key="group.key"
              :name="group.key"
              :title="`${group.title} (${groupChecked[group.key]?.length ?? 0} / ${group.items.length})`"
            >
              <el-checkbox-group v-model="groupChecked[group.key]">
                <el-checkbox
                  v-for="p in group.items"
                  :key="p.data"
                  :value="p.data"
                  class="api-checkbox"
                >
                  <span class="api-data">{{ p.data }}</span>
                  <span class="api-desc">{{ p.description }}</span>
                </el-checkbox>
              </el-checkbox-group>
            </el-collapse-item>
          </el-collapse>
          <el-empty v-else description="暂无可配置权限" />

          <div class="tab-actions">
            <el-button @click="resetApis">重置</el-button>
            <el-button type="primary" :loading="saving === 'apis'" @click="saveApis">
              保存权限
            </el-button>
          </div>
        </el-tab-pane>
      </el-tabs>
    </section>
  </div>
</template>

<style scoped>
.role-perm-page {
  max-width: 1080px;
  margin: 0 auto;
  padding: 8px 4px 24px;
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-head {
  padding: 18px 22px;
  border-radius: var(--r-md);
}

.head-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  font-family: var(--display);
  color: var(--ink);
}

.head-sub {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--ink-2);
}

.role-key {
  font-family: var(--mono);
  font-size: 12px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--surface-2);
  color: var(--ink);
}

.tabs-card {
  padding: 6px 22px 22px;
  border-radius: var(--r-md);
}

.role-perm-tabs {
  --el-tabs-header-height: 48px;
}

.tab-label {
  display: inline-flex;
  align-items: center;
  font-size: 13px;
}

.menu-tree {
  padding: 8px 0;
  max-height: 60vh;
  overflow: auto;
}

.tab-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}

.api-checkbox {
  display: flex;
  width: 100%;
  margin-right: 0;
  margin-bottom: 4px;
}

.api-data {
  font-family: var(--mono);
  font-size: 12px;
  margin-right: 8px;
}

.api-desc {
  color: var(--ink-2);
  font-size: 12px;
}
</style>