<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Document, Search, Star, StarFilled } from '@element-plus/icons-vue'
import { useMenuStore } from '@/stores/menu'
import { useWorkspaceStore, type WorkspaceLink } from '@/stores/workspace'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { menuIconName, resolveIcon } from '@/utils/icons'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const menu = useMenuStore()
const workspace = useWorkspaceStore()
const isMobile = useMediaQuery('(max-width: 640px)')
const query = ref('')

watch(
  () => props.modelValue,
  (open) => {
    if (open) query.value = ''
  },
)

const allItems = computed<WorkspaceLink[]>(() => {
  const index = menu.flatIndex
  // 动态段(如 /system/sys_role/perm/:roleKey)需要实际参数才能打开,
  // 不能作为固定入口;它们仍可通过侧栏进入。
  return index.uris
    .filter((uri) => !uri.includes(':'))
    .map((uri) => ({
      path: uri,
      title: index.titlesByUri.get(uri) ?? uri,
      icon: menuIconName(
        index.iconsByUri.get(uri),
        index.componentsByUri.get(uri),
      ),
      section: menu.groupTitlesByUri.get(uri) ?? '',
    }))
})

const filteredItems = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  if (!keyword) return allItems.value
  return allItems.value.filter(
    (item) =>
      item.title.toLowerCase().includes(keyword) ||
      item.path.toLowerCase().includes(keyword) ||
      item.section.toLowerCase().includes(keyword),
  )
})

function close(): void {
  emit('update:modelValue', false)
}

function toggle(item: WorkspaceLink): void {
  const changed = workspace.toggleFavorite(item)
  if (!changed) {
    ElMessage.warning(`最多固定 ${workspace.maxFavorites} 个入口`)
  }
}

function clearFavorites(): void {
  workspace.clearFavorites()
}
</script>

<template>
  <el-dialog
    :model-value="props.modelValue"
    :width="isMobile ? '92%' : '520px'"
    title="管理常用入口"
    append-to-body
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p class="dialog-desc">固定的入口会显示在工作台右侧,最多 {{ workspace.maxFavorites }} 个。</p>

    <el-input
      v-model="query"
      class="dialog-search"
      placeholder="搜索页面名称、路径或分组"
      clearable
      :prefix-icon="Search"
    />

    <div class="quick-pick-list">
      <button
        v-for="item in filteredItems"
        :key="item.path"
        class="quick-pick"
        :class="{ 'is-favorite': workspace.isFavorite(item.path) }"
        type="button"
        :aria-pressed="workspace.isFavorite(item.path)"
        @click="toggle(item)"
      >
        <span class="quick-pick-icon" aria-hidden="true">
          <el-icon>
            <component :is="resolveIcon(item.icon) || Document" />
          </el-icon>
        </span>
        <span class="quick-pick-main">
          <span class="quick-pick-title">{{ item.title }}</span>
          <span class="quick-pick-section">{{ item.section }} · {{ item.path }}</span>
        </span>
        <span class="quick-pick-action" aria-hidden="true">
          <el-icon>
            <StarFilled v-if="workspace.isFavorite(item.path)" />
            <Star v-else />
          </el-icon>
        </span>
      </button>

      <p v-if="filteredItems.length === 0" class="dialog-empty">没有匹配的页面</p>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <button
          v-if="workspace.favorites.length > 0"
          class="text-btn"
          type="button"
          @click="clearFavorites"
        >
          清空固定
        </button>
        <span class="dialog-count">已固定 {{ workspace.favorites.length }}/{{ workspace.maxFavorites }}</span>
        <el-button type="primary" @click="close">完成</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-desc {
  margin: 0 0 12px;
  color: var(--ink-3);
  font-size: var(--fs-sm);
  line-height: 1.5;
}

.dialog-search {
  margin-bottom: 12px;
}

.quick-pick-list {
  max-height: min(420px, 52vh);
  overflow-y: auto;
  padding-right: 2px;
}

.quick-pick {
  display: flex;
  align-items: center;
  gap: 11px;
  width: 100%;
  padding: 9px 10px;
  border: 1px solid transparent;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  font-family: var(--body);
  text-align: left;
  cursor: pointer;
}

.quick-pick:hover {
  background: var(--menu-active-tint-soft);
}

.quick-pick.is-favorite {
  border-color: rgba(47, 125, 99, 0.18);
  background: var(--menu-active-tint-soft);
}

.quick-pick:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: -1px;
}

.quick-pick-icon {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--surface-soft);
  color: var(--acc-mint-deep);
  font-size: 15px;
}

.quick-pick-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.quick-pick-title {
  overflow: hidden;
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-pick-section {
  overflow: hidden;
  font-size: var(--fs-xs);
  color: var(--ink-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-pick-action {
  margin-left: auto;
  flex: 0 0 auto;
  color: var(--ink-3);
  font-size: var(--fs-md);
}

.quick-pick.is-favorite .quick-pick-action {
  color: var(--el-color-warning);
}

.dialog-empty {
  padding: 28px 0;
  color: var(--ink-3);
  font-size: var(--fs-sm);
  text-align: center;
}

.dialog-footer {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.text-btn {
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: var(--ink-3);
  font-family: var(--body);
  font-size: var(--fs-xs);
  cursor: pointer;
}

.text-btn:hover {
  color: var(--el-color-danger);
}

.dialog-count {
  margin-left: auto;
  color: var(--ink-3);
  font-size: var(--fs-xs);
}
</style>
