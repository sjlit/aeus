<script setup lang="ts">
// 与 router/index.ts 里菜单路由 meta.componentName (= deriveComponentName('@/views/system/sys_menu/Index.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'SystemSysMenuses' })

import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import IconPicker from '@/components/widgets/IconPicker.vue'
import { resolveIcon } from '../../../utils/icons'
import { buildTree } from '../../../stores/menuGroups'
import { usePageTitle } from '@/composables/usePageTitle'
import {
  createMenu,
  deleteMenu,
  fetchMenuAll,
  fetchMenuOptions,
  updateMenu,
  type MenuItem,
  type MenuOptionNode,
} from '../../../api/menu'

// 页面标题跟随菜单(route.meta.title 由路由注册时从菜单写入),
// 避免 el-table 页面没有 schema 兜底。
const title = usePageTitle('sys_menus')

// ----- 状态 ------------------------------------------------------------

interface MenuRow extends MenuItem {
  /** 仅运行时使用:前端 buildTree 构造的 children,el-table tree-props 读它。 */
  children: MenuRow[]
}

const loading = ref(false)
const tree = ref<MenuRow[]>([])
/** /menu/options 返回的级联选项直接作为 ParentOption 用——shape 完全一致
 *  (value/label/parent/children),不需要再 copy 一遍。 */
type ParentOption = MenuOptionNode

/** 把 buildTree 过程中产生的 MenuRow 提交时回退成扁平 MenuItem;
 *  children 是运行时构造,不需要。 */
type FlatMenu = Omit<MenuRow, 'children'>

/** 空表单默认值集中在一处:openCreate/重置场景共用,新增字段时只改这里。 */
const EMPTY_FORM: FlatMenu = {
  id: 0,
  parent: '',
  name: '',
  component: '',
  uri: '/',
  view_path: '',
  icon: '',
  hidden: false,
  public: false,
  sort: 0,
  description: '',
  created_at: 0,
  updated_at: 0,
}

const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const formRef = ref()
const submitting = ref(false)

const form = reactive<FlatMenu>({ ...EMPTY_FORM })

const formRules = {
  name: [{ required: true, message: '请输入菜单标题', trigger: 'blur' }],
  component: [{ required: true, message: '请输入组件名称', trigger: 'blur' }],
  uri: [{ required: true, message: '请输入路由', trigger: 'blur' }],
}

function resetForm(): void {
  Object.assign(form, EMPTY_FORM)
}

// ----- 数据加载 ---------------------------------------------------------

/** MenuItem[] 走 stores/menuGroups.buildTree 的同一份实现:同样的 component/
 *  parent 约束、同样的孤儿处理、同样的 O(n) Map 查找。原来这里手写的 O(n²)
 *  inner-loop 是多余,删掉。返回 (MenuItem & {children: MenuItem[]})[],
 *  cast 后带 MenuRow 给 el-table 用,children 字段已经在泛型签名里。 */
async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await fetchMenuAll()
    tree.value = buildTree<MenuItem>(data.items ?? []) as unknown as MenuRow[]
  } catch {
    // http 拦截器已 toast;保留空树,用户可重新刷新。
    tree.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)

// ----- 父级选择器 -------------------------------------------------------

const parentOptions = ref<ParentOption[]>([])

/** 加载一次 MenuOptionNode(创建/编辑共用);命中本地缓存就跳过。 */
async function loadOptions(): Promise<ParentOption[]> {
  if (parentOptions.value.length > 0) return parentOptions.value
  const data = await fetchMenuOptions()
  parentOptions.value = data.items ?? []
  return parentOptions.value
}

// ----- 操作 ------------------------------------------------------------

/** 打开新建对话框。parent 是可选的;为"在某个节点下挂子菜单"提供
 *  上下文,触发按钮的 row 会被传入。 */
async function openCreate(parentRow?: MenuRow) {
  dialogMode.value = 'create'
  await loadOptions()
  resetForm()
  form.parent = parentRow?.component ?? ''
  dialogVisible.value = true
}

async function openEdit(row: MenuRow) {
  dialogMode.value = 'edit'
  await loadOptions()
  Object.assign(form, row)
  dialogVisible.value = true
}

async function onSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    const payload = {
      parent: form.parent,
      name: form.name,
      component: form.component,
      uri: form.uri,
      view_path: form.view_path,
      icon: form.icon,
      hidden: form.hidden,
      public: form.public,
      sort: form.sort,
      description: form.description,
    }
    if (dialogMode.value === 'create') {
      await createMenu(payload)
      ElMessage.success('菜单已创建')
    } else {
      await updateMenu(form.id, payload)
      ElMessage.success('菜单已更新')
    }
    dialogVisible.value = false
    await load()
  } catch {
    // http 拦截器已 toast
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: MenuRow) {
  try {
    await ElMessageBox.confirm(
      `确认删除菜单 “${row.name}” 吗？子菜单将一并删除。`,
      '删除菜单',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    await deleteMenu(row.id)
    ElMessage.success('菜单已删除')
    await load()
  } catch {
    // http 拦截器已 toast
  }
}
</script>

<template>
  <div class="menu-page">
    <header class="menu-page__header">
      <h2>{{ title }}</h2>
      <div class="menu-page__actions">
        <el-button :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" @click="openCreate()">新增菜单</el-button>
      </div>
    </header>

    <el-table v-loading="loading" :data="tree" row-key="id"
      :tree-props="{ children: 'children', hasChildren: 'children.length > 0' }" :default-expand-all="true" border
      class="menu-table">
      <el-table-column prop="name" label="菜单标题" min-width="180" />
      <el-table-column prop="component" label="组件" min-width="180" />
      <el-table-column prop="uri" label="路由" min-width="160" />
      <el-table-column prop="view_path" label="视图路径" min-width="220" show-overflow-tooltip />
      <el-table-column prop="icon" label="图标" min-width="120">
        <template #default="{ row }">
          <span v-if="resolveIcon(row.icon)" class="menu-row-icon">
            <el-icon>
              <component :is="resolveIcon(row.icon)" />
            </el-icon>
            <span class="menu-row-icon__name">{{ row.icon }}</span>
          </span>
          <span v-else class="dim">{{ row.icon || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="sort" label="排序" width="80" align="center" />
      <el-table-column label="公开" width="80" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.public" type="success" size="small">是</el-tag>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="隐藏" width="80" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.hidden" type="info" size="small">是</el-tag>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="备注" min-width="160" show-overflow-tooltip />
      <el-table-column label="操作" width="220" fixed="right" align="center">
        <template #default="{ row }">
          <!-- el-table slot 的 row 推为 DefaultRow(=Record<string, any>);
               实际数据是 MenuRow[](:data="tree"),这里 inline 收敛到 MenuRow,
               避免把函数签名放宽到 Record<string, any>。 -->
          <el-button size="small" type="primary" link @click="openCreate(row as MenuRow)">新增子菜单</el-button>
          <el-button size="small" type="primary" link @click="openEdit(row as MenuRow)">编辑</el-button>
          <el-button size="small" type="danger" link @click="onDelete(row as MenuRow)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" draggable :title="dialogMode === 'create' ? '新增菜单' : '编辑菜单'" width="640px"
      :close-on-click-modal="false" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="formRules" class="schema-form" label-width="96px"
        label-position="right">
        <el-form-item label="父级菜单" prop="parent">
          <el-cascader v-model="form.parent" :options="parentOptions" :props="{
            value: 'value',
            label: 'label',
            children: 'children',
            checkStrictly: true,
            emitPath: false,
          }" clearable placeholder="留空表示顶级菜单" class="menu-form__parent" />
        </el-form-item>
        <el-form-item label="菜单标题" prop="name">
          <el-input v-model="form.name" placeholder="例如 用户管理" />
        </el-form-item>
        <el-form-item label="组件名称" prop="component">
          <el-input v-model="form.component" placeholder="PascalCase,与路由 keep-alive 名称对齐" />
        </el-form-item>
        <el-form-item label="路由" prop="uri">
          <el-input v-model="form.uri" placeholder="/system/sys-menus" />
        </el-form-item>
        <el-form-item label="视图路径" prop="view_path">
          <el-input v-model="form.view_path" placeholder="@/views/system/sys_menus/Index.vue" />
        </el-form-item>
        <el-form-item label="图标" prop="icon">
          <IconPicker v-model="form.icon" placeholder="选择 element-plus 图标" />
        </el-form-item>
        <el-form-item label="排序" prop="sort">
          <el-input-number v-model="form.sort" :min="0" :step="10" />
        </el-form-item>
        <el-form-item label="备注" prop="description">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="可见性">
          <el-checkbox v-model="form.hidden">隐藏</el-checkbox>
          <el-checkbox v-model="form.public">公开</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">
          {{ dialogMode === 'create' ? '创建' : '保存' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.menu-page {
  padding: 16px;
}

.menu-page__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.menu-page__header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.menu-page__actions {
  display: flex;
  gap: 8px;
}

.menu-table {
  width: 100%;
}

.menu-form__parent {
  width: 100%;
}

.dim {
  color: var(--el-text-color-placeholder);
}

.menu-row-icon {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.menu-row-icon__name {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
