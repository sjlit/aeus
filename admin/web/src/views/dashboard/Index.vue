<script setup lang="ts">
// 与 router/index.ts 里这条静态路由的 meta.componentName 对齐。
defineOptions({ name: 'DashboardIndex' })

import { computed, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import {
  Key,
  Refresh,
  Search,
  Tickets,
  User,
  UserFilled,
  WarningFilled,
} from '@element-plus/icons-vue'
import type { Component } from 'vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import PageShell from '@/components/layout/PageShell.vue'
import ActivityFeed from './components/ActivityFeed.vue'
import LoginTrend from './components/LoginTrend.vue'
import QuickAccessDialog from './components/QuickAccessDialog.vue'
import QuickActions from './components/QuickActions.vue'
import RecentVisits from './components/RecentVisits.vue'
import StatCard from './components/StatCard.vue'
import { DASHBOARD_PATH } from '@/router'
import { usePageTitle } from '@/composables/usePageTitle'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { useUiStore } from '@/stores/ui'
import { useWorkspaceStore, type WorkspaceLink } from '@/stores/workspace'
import { fetchDashboardOverview, type DashboardOverview } from '@/api/dashboard'
import { menuIconName } from '@/utils/icons'
import { formatRelativeTime, formatUpdatedAt } from '@/utils/time'

interface KpiCard {
  id: string
  label: string
  value: number
  hint: string
  icon: Component
  tone?: 'default' | 'up' | 'down'
  to: string
}

const title = usePageTitle('工作台')
const router = useRouter()
const auth = useAuthStore()
const menu = useMenuStore()
const ui = useUiStore()
const workspace = useWorkspaceStore()

const overview = ref<DashboardOverview | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const overviewError = ref(false)
const loadedAt = ref(0)
const quickDialogOpen = ref(false)

// 日期/问候/相对时间共用一个分钟级时钟;页面被 keep-alive 缓存时停掉。
const now = ref(new Date())
let clockTimer: number | undefined

function startClock(): void {
  stopClock()
  now.value = new Date()
  clockTimer = window.setInterval(() => {
    now.value = new Date()
  }, 60_000)
}

function stopClock(): void {
  if (clockTimer !== undefined) {
    window.clearInterval(clockTimer)
    clockTimer = undefined
  }
}

onMounted(() => {
  startClock()
  void loadOverview()
})
onActivated(() => {
  startClock()
  if (loadedAt.value > 0 && Date.now() - loadedAt.value > 60_000) {
    void loadOverview(true)
  }
})
onDeactivated(stopClock)
onUnmounted(stopClock)

async function loadOverview(silent = false): Promise<void> {
  if (!silent) loading.value = true
  refreshing.value = true
  try {
    overview.value = await fetchDashboardOverview()
    loadedAt.value = Date.now()
    overviewError.value = false
  } catch {
    // 概览失败不能拖垮启动台:继续展示最近访问/常用入口。
    overviewError.value = true
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

const isAdmin = computed(() => overview.value?.scope === 'admin')
const counts = computed(() => overview.value?.counts)
const loginSummary = computed(() => overview.value?.logins)
const auditSummary = computed(() => overview.value?.audits)
const trend = computed(() => loginSummary.value?.trend ?? [])
const recentAudits = computed(() => auditSummary.value?.recent ?? [])
const auditsToday = computed(() => auditSummary.value?.today ?? 0)
const todayLogins = computed(() => loginSummary.value?.today ?? 0)
const yesterdayLogins = computed(() => loginSummary.value?.yesterday ?? 0)

const greeting = computed(() => {
  const hour = now.value.getHours()
  if (hour < 6) return '夜深了'
  if (hour < 12) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})

const dateLine = computed(() =>
  new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    weekday: 'long',
  }).format(now.value),
)

const displayName = computed(() =>
  auth.displayName && auth.displayName !== '—' ? auth.displayName : '',
)

const subtitle = computed(
  () => `${greeting.value}${displayName.value ? `，${displayName.value}` : ''} · ${dateLine.value}`,
)

const menuLinks = computed<WorkspaceLink[]>(() => {
  const index = menu.flatIndex
  return index.uris.map((uri) => ({
    path: uri,
    title: index.titlesByUri.get(uri) ?? uri,
    icon: menuIconName(
      index.iconsByUri.get(uri),
      index.componentsByUri.get(uri),
    ),
    section: menu.groupTitlesByUri.get(uri) ?? '',
  }))
})

const menuByPath = computed(() => {
  const map = new Map<string, WorkspaceLink>()
  for (const item of menuLinks.value) map.set(item.path, item)
  return map
})

const uriByComponent = computed(() => {
  const map = new Map<string, string>()
  for (const [uri, component] of menu.flatIndex.componentsByUri) {
    map.set(component, uri)
  }
  return map
})

function isKnownRoute(path: string): boolean {
  return router.resolve(path).matched.some((record) => record.name !== 'not-found')
}

const recentVisits = computed(() =>
  workspace.visits
    .filter((item) => item.path !== DASHBOARD_PATH && isKnownRoute(item.path))
    .slice(0, 6)
    .map((item) => {
      const fresh = menuByPath.value.get(item.path)
      if (!fresh) return item
      return {
        ...item,
        title: fresh.title || item.title,
        icon: item.icon || fresh.icon,
        section: fresh.section || item.section,
      }
    }),
)

const favoriteItems = computed(() =>
  workspace.favorites
    .map((item) => menuByPath.value.get(item.path))
    .filter((item): item is WorkspaceLink => Boolean(item))
    .slice(0, 6),
)

const frequentItems = computed(() => {
  const stats = new Map<string, { item: WorkspaceLink; count: number; last: number }>()
  for (const visit of workspace.visits) {
    const item = menuByPath.value.get(visit.path)
    if (!item) continue
    const current = stats.get(visit.path)
    if (current) {
      current.count++
      current.last = Math.max(current.last, visit.visitedAt)
    } else {
      stats.set(visit.path, { item, count: 1, last: visit.visitedAt })
    }
  }
  return [...stats.values()]
    .sort((a, b) => b.count - a.count || b.last - a.last)
    .map((entry) => entry.item)
    .slice(0, 6)
})

const recommendedItems = computed(() => {
  const preferred = [
    'SystemSysUsers',
    'SystemSysRoles',
    'SystemSysMenus',
    'SystemSysAudits',
    'SystemSysLoginLogs',
    'SystemSysPermissions',
    'SystemSysTenants',
  ]
  const byComponent = new Map<string, WorkspaceLink>()
  for (const item of menuLinks.value) {
    const component = menu.flatIndex.componentsByUri.get(item.path)
    if (component) byComponent.set(component, item)
  }
  const picked = preferred
    .map((component) => byComponent.get(component))
    .filter((item): item is WorkspaceLink => Boolean(item))
  return picked.length > 0 ? picked.slice(0, 6) : menuLinks.value.slice(0, 6)
})

const quickItems = computed(() => {
  if (favoriteItems.value.length > 0) return favoriteItems.value
  if (frequentItems.value.length > 0) return frequentItems.value
  return recommendedItems.value
})

const quickMode = computed<'favorites' | 'frequent' | 'recommended'>(() => {
  if (favoriteItems.value.length > 0) return 'favorites'
  if (frequentItems.value.length > 0) return 'frequent'
  return 'recommended'
})

const kpis = computed<KpiCard[]>(() => {
  if (!isAdmin.value || !counts.value) return []
  const delta = todayLogins.value - yesterdayLogins.value
  const latestAudit = recentAudits.value[0]
  return [
    {
      id: 'users',
      label: '用户',
      value: counts.value.users,
      hint: '当前租户账号',
      icon: User,
      to: uriByComponent.value.get('SystemSysUsers') ?? '',
    },
    {
      id: 'roles',
      label: '角色',
      value: counts.value.roles,
      hint: '角色总数',
      icon: UserFilled,
      to: uriByComponent.value.get('SystemSysRoles') ?? '',
    },
    {
      id: 'logins',
      label: '今日登录',
      value: todayLogins.value,
      hint: delta === 0 ? '与昨日持平' : `较昨日 ${delta > 0 ? '+' : ''}${delta}`,
      icon: Key,
      tone: delta > 0 ? 'up' : delta < 0 ? 'down' : 'default',
      to: uriByComponent.value.get('SystemSysLoginLogs') ?? '',
    },
    {
      id: 'audits',
      label: '今日操作',
      value: auditsToday.value,
      hint: latestAudit
        ? `最近 ${formatRelativeTime(latestAudit.created_at * 1000, now.value.getTime())}`
        : '暂无操作',
      icon: Tickets,
      to: uriByComponent.value.get('SystemSysAudits') ?? '',
    },
  ]
})

const overviewMeta = computed(() => {
  const value = counts.value
  if (!value) return ''
  const items = [
    `部门 ${value.departments}`,
    `菜单 ${value.menus}`,
    `权限 ${value.permissions}`,
  ]
  if (value.tenants) items.push(`租户 ${value.tenants}`)
  return items.join(' · ')
})

const updatedText = computed(() => {
  const generated = overview.value?.generated_at
  const timestamp = generated ? generated * 1000 : loadedAt.value
  return formatUpdatedAt(timestamp, now.value.getTime())
})

async function clearVisits(): Promise<void> {
  try {
    await ElMessageBox.confirm(
      '清空后无法恢复,确定清空最近访问吗?',
      '清空最近访问',
      {
        type: 'warning',
        confirmButtonText: '清空',
        cancelButtonText: '取消',
      },
    )
    workspace.clearVisits()
  } catch {
    // 用户取消
  }
}

function browseAll(): void {
  ui.openPalette()
}

function viewAllAudits(): void {
  const uri = uriByComponent.value.get('SystemSysAudits')
  if (uri) {
    void router.push(uri)
    return
  }
  ui.openPalette()
}
</script>

<template>
  <PageShell class="dashboard-shell">
    <PageHeader :title="title" :subtitle="subtitle">
      <template #actions>
        <el-button :icon="Search" @click="browseAll">搜索页面</el-button>
        <el-button :icon="Refresh" :loading="refreshing" @click="loadOverview()">
          刷新
        </el-button>
      </template>
    </PageHeader>

    <div v-if="overviewError" class="overview-error" role="status">
      <el-icon aria-hidden="true"><WarningFilled /></el-icon>
      <span>系统概览暂时不可用,启动台仍可正常使用。</span>
      <button type="button" class="error-retry" @click="loadOverview()">重试</button>
    </div>

    <section v-if="isAdmin" class="overview" aria-label="系统概览">
      <div class="stat-grid">
        <StatCard
          v-for="kpi in kpis"
          :key="kpi.id"
          :label="kpi.label"
          :value="kpi.value"
          :hint="kpi.hint"
          :icon="kpi.icon"
          :tone="kpi.tone"
          :to="kpi.to"
          :loading="loading && !overview"
        />
      </div>
      <p class="overview-meta">
        <span>{{ overviewMeta }}</span>
        <span>更新于 {{ updatedText }}</span>
      </p>
    </section>

    <div class="workspace-grid">
      <RecentVisits
        :items="recentVisits"
        :now="now.getTime()"
        @clear="clearVisits"
        @browse="browseAll"
      />
      <QuickActions
        :items="quickItems"
        :mode="quickMode"
        @manage="quickDialogOpen = true"
        @browse="browseAll"
      />
    </div>

    <section v-if="isAdmin" class="insight-grid" aria-label="系统动态">
      <LoginTrend
        :points="trend"
        :today="todayLogins"
        :yesterday="yesterdayLogins"
      />
      <ActivityFeed
        :items="recentAudits"
        :today="auditsToday"
        :now="now.getTime()"
        @view-all="viewAllAudits"
      />
    </section>

    <QuickAccessDialog v-model="quickDialogOpen" />
  </PageShell>
</template>

<style scoped>
.dashboard-shell {
  width: 100%;
  max-width: 1560px;
  margin: 0 auto;
}

.overview-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border: 1px solid rgba(154, 101, 22, 0.18);
  border-radius: var(--r-sm);
  background: rgba(212, 160, 74, 0.08);
  color: var(--el-color-warning);
  font-size: var(--fs-sm);
}

.error-retry {
  margin-left: auto;
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: var(--el-color-warning);
  font-family: var(--body);
  font-size: var(--fs-xs);
  font-weight: 600;
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.overview {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.overview-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 2px;
  font-size: var(--fs-xs);
  color: var(--ink-3);
}

.workspace-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr);
  gap: 16px;
  align-items: stretch;
}

.insight-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr);
  gap: 16px;
  align-items: start;
}

@media (max-width: 1180px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .workspace-grid,
  .insight-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .overview-meta {
    align-items: flex-start;
    flex-direction: column;
    gap: 3px;
  }
}
</style>
