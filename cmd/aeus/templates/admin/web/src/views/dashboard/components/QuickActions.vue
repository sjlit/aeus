<script setup lang="ts">
import { computed } from 'vue'
import { Grid, Search, Setting } from '@element-plus/icons-vue'
import { resolveIcon } from '@/utils/icons'
import type { WorkspaceLink } from '@/stores/workspace'

const props = defineProps<{
  items: WorkspaceLink[]
  /** favorites=用户固定;frequent=按访问频次;recommended=权限内推荐 */
  mode: 'favorites' | 'frequent' | 'recommended'
}>()

defineEmits<{
  manage: []
  browse: []
}>()

const subtitle = computed(() => {
  if (props.mode === 'favorites') return '你固定的入口'
  if (props.mode === 'frequent') return '根据最近访问推荐'
  return '按当前权限推荐'
})
</script>

<template>
  <section class="surface-card quick-card" aria-labelledby="quick-title">
    <header class="panel-head">
      <div>
        <h2 id="quick-title" class="panel-title">常用入口</h2>
        <p class="panel-sub">{{ subtitle }}</p>
      </div>
      <button class="text-btn" type="button" @click="$emit('manage')">
        <el-icon><Setting /></el-icon>
        管理
      </button>
    </header>

    <div v-if="items.length" class="quick-grid">
      <router-link
        v-for="item in items"
        :key="item.path"
        class="quick-item"
        :to="item.path"
        :title="item.title"
      >
        <span class="quick-icon" aria-hidden="true">
          <el-icon>
            <component :is="resolveIcon(item.icon) || Grid" />
          </el-icon>
        </span>
        <span class="quick-main">
          <span class="quick-title">{{ item.title }}</span>
          <span v-if="item.section" class="quick-section">{{ item.section }}</span>
        </span>
      </router-link>
    </div>

    <div v-else class="quick-empty">
      <p>还没有固定入口</p>
      <button class="text-btn text-btn--strong" type="button" @click="$emit('manage')">
        添加常用页面
      </button>
    </div>

    <footer class="quick-footer">
      <button class="footer-btn" type="button" @click="$emit('browse')">
        <el-icon><Search /></el-icon>
        浏览全部页面
      </button>
    </footer>
  </section>
</template>

<style scoped>
.quick-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
  padding: 20px 22px 0;
}

.text-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: var(--ink-3);
  font-family: var(--body);
  font-size: var(--fs-xs);
  cursor: pointer;
}

.text-btn:hover {
  color: var(--acc-mint-deep);
}

.text-btn--strong {
  margin-top: 8px;
  color: var(--acc-mint-deep);
  font-weight: 600;
}

.quick-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 14px;
}

.quick-item {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  min-height: 62px;
  padding: 10px;
  border: 1px solid var(--line-soft);
  border-radius: 10px;
  background: var(--surface);
  color: inherit;
  text-decoration: none;
  transition: border-color 0.16s ease, background-color 0.16s ease;
}

.quick-item:hover {
  border-color: rgba(47, 125, 99, 0.24);
  background: var(--menu-active-tint-soft);
}

.quick-item:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 1px;
}

.quick-icon {
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

.quick-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.quick-title {
  overflow: hidden;
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-section {
  overflow: hidden;
  font-size: var(--fs-xs);
  color: var(--ink-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 120px;
  margin-top: 12px;
  border: 1px dashed var(--line);
  border-radius: 10px;
  color: var(--ink-3);
  font-size: var(--fs-sm);
}

.quick-empty p {
  margin: 0;
}

.quick-footer {
  display: flex;
  justify-content: center;
  margin: auto -22px 0;
  padding-top: 14px;
  border-top: 1px solid var(--line-soft);
}

.footer-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  justify-content: center;
  padding: 11px 12px;
  border: 0;
  border-radius: 0 0 var(--r-md) var(--r-md);
  background: transparent;
  color: var(--ink-3);
  font-family: var(--body);
  font-size: var(--fs-xs);
  cursor: pointer;
}

.footer-btn:hover {
  background: var(--menu-active-tint-soft);
  color: var(--acc-mint-deep);
}

@media (max-width: 480px) {
  .quick-card {
    padding: 18px 16px 0;
  }

  .quick-grid {
    grid-template-columns: 1fr;
  }

  .quick-footer {
    margin-left: -16px;
    margin-right: -16px;
  }
}
</style>
