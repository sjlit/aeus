<script setup lang="ts">
import { computed } from 'vue'
import { Tickets } from '@element-plus/icons-vue'
import type { DashboardAuditItem } from '@/api/dashboard'
import { formatRelativeTime } from '@/utils/time'

const props = defineProps<{
  items: DashboardAuditItem[]
  today: number
  now: number
}>()

defineEmits<{
  viewAll: []
}>()

const ACTION_LABELS: Record<string, string> = {
  create: '新建',
  update: '更新',
  delete: '删除',
}

const TABLE_LABELS: Record<string, string> = {
  sys_users: '用户',
  sys_roles: '角色',
  sys_menus: '菜单',
  sys_departments: '部门',
  sys_permissions: '权限',
  sys_role_permissions: '角色授权',
  sys_tenants: '租户',
  sys_audits: '操作日志',
  sys_login_logs: '登录日志',
}

function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? action
}

function targetLabel(table: string): string {
  return TABLE_LABELS[table] ?? table
}

const hasItems = computed(() => props.items.length > 0)
</script>

<template>
  <section class="surface-card activity-card" aria-labelledby="activity-title">
    <header class="panel-head">
      <div>
        <h2 id="activity-title" class="panel-title">最近操作</h2>
        <p class="panel-sub">今日 {{ today }} 次变更</p>
      </div>
      <button class="text-btn" type="button" @click="$emit('viewAll')">
        查看全部
      </button>
    </header>

    <ul v-if="hasItems" class="activity-list">
      <li v-for="item in items" :key="item.id" class="activity-item">
        <span
          class="activity-dot"
          :class="`activity-dot--${item.action}`"
          aria-hidden="true"
        ></span>
        <div class="activity-main">
          <p class="activity-text">
            <span class="activity-actor">{{ item.uid || '系统' }}</span>
            {{ actionLabel(item.action) }}了
            <span class="activity-target">{{ targetLabel(item.table) }}</span>
          </p>
          <p class="activity-meta">
            {{ item.module }}/{{ item.table }}
          </p>
        </div>
        <time class="activity-time" :datetime="new Date(item.created_at * 1000).toISOString()">
          {{ formatRelativeTime(item.created_at * 1000, now) }}
        </time>
      </li>
    </ul>

    <div v-else class="activity-empty">
      <span class="activity-empty-icon" aria-hidden="true">
        <el-icon><Tickets /></el-icon>
      </span>
      <p>暂无操作记录</p>
      <span>对数据的增删改会记录在这里。</span>
    </div>
  </section>
</template>

<style scoped>
.activity-card {
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

.activity-list {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
}

.activity-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 0;
}

.activity-item + .activity-item {
  border-top: 1px solid var(--line-soft);
}

.activity-dot {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  margin-top: 6px;
  border-radius: 50%;
  background: var(--el-color-info);
}

.activity-dot--create {
  background: var(--el-color-success);
}

.activity-dot--update {
  background: var(--el-color-warning);
}

.activity-dot--delete {
  background: var(--el-color-danger);
}

.activity-main {
  min-width: 0;
}

.activity-text {
  margin: 0;
  font-size: var(--fs-sm);
  line-height: 1.45;
  color: var(--ink-2);
}

.activity-actor {
  font-weight: 600;
  color: var(--ink);
}

.activity-target {
  color: var(--ink);
  font-weight: 500;
}

.activity-meta {
  margin: 3px 0 0;
  overflow: hidden;
  font-family: var(--mono);
  font-size: var(--fs-2xs);
  color: var(--ink-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.activity-time {
  flex: 0 0 auto;
  margin-left: auto;
  font-size: var(--fs-xs);
  color: var(--ink-3);
  font-variant-numeric: tabular-nums;
}

.activity-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 176px;
  text-align: center;
}

.activity-empty-icon {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 13px;
  background: var(--surface-soft);
  color: var(--acc-mint-deep);
  font-size: 21px;
}

.activity-empty p {
  margin: 12px 0 0;
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink);
}

.activity-empty span {
  margin-top: 4px;
  font-size: var(--fs-xs);
  color: var(--ink-3);
}

@media (max-width: 640px) {
  .activity-card {
    padding: 18px 16px;
  }

  .activity-time {
    display: none;
  }
}
</style>
