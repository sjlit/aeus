<script setup lang="ts">
import { ArrowRight, Document } from '@element-plus/icons-vue'
import { resolveIcon } from '@/utils/icons'
import { formatRelativeTime } from '@/utils/time'

export interface RecentVisitItem {
  path: string
  title: string
  icon: string
  section: string
  visitedAt: number
  query?: Record<string, string>
}

defineProps<{
  items: RecentVisitItem[]
  /** 由页面统一维护的分钟级 now,避免每行各自 setInterval。 */
  now: number
}>()

defineEmits<{
  clear: []
  browse: []
}>()
</script>

<template>
  <section class="surface-card recent-card" aria-labelledby="recent-title">
    <header class="panel-head">
      <div>
        <h2 id="recent-title" class="panel-title">继续工作</h2>
        <p class="panel-sub">按最近访问排序,不是标签页顺序</p>
      </div>
      <button v-if="items.length" class="text-btn" type="button" @click="$emit('clear')">
        清空
      </button>
    </header>

    <ul v-if="items.length" class="recent-list">
      <li v-for="item in items" :key="item.path">
        <router-link
          class="recent-item"
          :to="item.query ? { path: item.path, query: item.query } : item.path"
        >
          <span class="recent-icon" aria-hidden="true">
            <el-icon>
              <component :is="resolveIcon(item.icon) || Document" />
            </el-icon>
          </span>
          <span class="recent-main">
            <span class="recent-title">{{ item.title }}</span>
            <span v-if="item.section" class="recent-section">{{ item.section }}</span>
          </span>
          <span class="recent-time">{{ formatRelativeTime(item.visitedAt, now) }}</span>
          <el-icon class="recent-arrow" aria-hidden="true"><ArrowRight /></el-icon>
        </router-link>
      </li>
    </ul>

    <div v-else class="empty-state">
      <span class="empty-icon" aria-hidden="true">
        <el-icon><Document /></el-icon>
      </span>
      <p class="empty-title">还没有访问记录</p>
      <p class="empty-desc">打开任意菜单后,这里会自动记住最近去过的页面。</p>
      <button class="text-btn text-btn--strong" type="button" @click="$emit('browse')">
        搜索页面
      </button>
    </div>
  </section>
</template>

<style scoped>
.recent-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
  padding: 20px 22px;
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
  color: var(--acc-mint-deep);
}

.text-btn--strong {
  margin-top: 14px;
  color: var(--acc-mint-deep);
  font-weight: 600;
}

.recent-list {
  margin: 14px 0 0;
  padding: 0;
  list-style: none;
}

.recent-list li + li {
  border-top: 1px solid var(--line-soft);
}

.recent-item {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 58px;
  padding: 8px 10px;
  margin: 0 -10px;
  border-radius: 10px;
  color: inherit;
  text-decoration: none;
  transition: background-color 0.16s ease;
}

.recent-item:hover {
  background: var(--menu-active-tint-soft);
}

.recent-item:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: -2px;
}

.recent-icon {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  background: var(--surface-soft);
  color: var(--acc-mint-deep);
  font-size: var(--fs-md);
}

.recent-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.recent-title {
  overflow: hidden;
  font-size: var(--fs-base);
  font-weight: 500;
  color: var(--ink);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-section {
  overflow: hidden;
  font-size: var(--fs-xs);
  color: var(--ink-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-time {
  margin-left: auto;
  flex: 0 0 auto;
  font-size: var(--fs-xs);
  color: var(--ink-3);
  font-variant-numeric: tabular-nums;
}

.recent-arrow {
  flex: 0 0 auto;
  color: var(--ink-3);
  font-size: var(--fs-sm);
  opacity: 0;
  transform: translateX(-2px);
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.recent-item:hover .recent-arrow {
  opacity: 1;
  transform: translateX(0);
}

.empty-state {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 220px;
  padding: 18px;
  text-align: center;
}

.empty-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  background: var(--surface-soft);
  color: var(--acc-mint-deep);
  font-size: 22px;
}

.empty-title {
  margin: 14px 0 0;
  font-size: var(--fs-base);
  font-weight: 600;
  color: var(--ink);
}

.empty-desc {
  max-width: 320px;
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  line-height: 1.55;
  color: var(--ink-3);
  /* 窄容器下两行时均匀分配,避免第二行只剩一两个字。 */
  text-wrap: balance;
}

@media (max-width: 640px) {
  .recent-card {
    padding: 18px 16px;
  }

  .recent-time {
    display: none;
  }

  .recent-arrow {
    opacity: 1;
  }
}
</style>
