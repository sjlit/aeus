<script setup lang="ts">
// 与 router/index.ts 里这条静态路由的 meta.componentName (= deriveComponentName
// '@/views/dashboard/Index.vue') 对齐。keep-alive :include=tabs.cachedViews
// 要求稳定可匹配的 name,异步组件包装层一旦丢失 name 就会让缓存失效。
defineOptions({ name: 'DashboardIndex' })

import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Search } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { useTabsStore } from '@/stores/tabs'
import { useUiStore } from '@/stores/ui'
import { usePageTitle } from '@/composables/usePageTitle'
import { resolveIcon } from '@/utils/icons'

const title = usePageTitle('工作台')
const route = useRoute()
const auth = useAuthStore()
const menu = useMenuStore()
const tabs = useTabsStore()
const ui = useUiStore()

/** 按时间段问候。 */
const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
})

/** 实时时钟(mono 字体,秒级跳动)。 */
const now = ref(new Date())
let timer: number | undefined
onMounted(() => {
  timer = window.setInterval(() => { now.value = new Date() }, 1000)
})
onUnmounted(() => { if (timer !== undefined) window.clearInterval(timer) })

const clock = computed(() =>
  now.value.toLocaleTimeString('zh-CN', { hour12: false }),
)
const dateLine = computed(() =>
  new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'long',
  }).format(now.value),
)

/**
 * 快捷导航:按所属节(菜单一级分组)聚合全部可路由节点。
 * 直接消费 flatIndex + sectionsByUri 两个现成快照,不依赖具体业务模块,
 * 菜单变化时本页自动跟着变 —— 这是「通用」的关键。
 */
interface NavItem {
  uri: string
  title: string
  icon: string
}

interface NavGroup {
  name: string
  items: NavItem[]
}

const navGroups = computed<NavGroup[]>(() => {
  const bySection = new Map<string, NavItem[]>()
  const idx = menu.flatIndex
  for (const uri of idx.uris) {
    const sec = menu.sectionsByUri.get(uri) || '其他'
    let items = bySection.get(sec)
    if (!items) {
      items = []
      bySection.set(sec, items)
    }
    items.push({
      uri,
      title: idx.titlesByUri.get(uri) ?? uri,
      icon: idx.iconsByUri.get(uri) ?? '',
    })
  }
  return [...bySection.entries()]
    .map(([name, items]) => ({ name, items }))
})

/** 最近访问:当前打开的业务标签页(排除落地页自身与不可关的常驻页)。 */
const recentTabs = computed(() =>
  tabs.tabs.filter(t => t.closable && t.path !== route.path).slice(0, 8),
)

/** 个人资料页入口:按 view_path 含 profile 反查 uri;菜单里没有就隐藏入口。 */
const profileUri = computed(() => {
  for (const [uri, view] of menu.flatIndex.viewsByUri) {
    if (view.includes('profile')) return uri
  }
  return ''
})

/** 租户名只在登录瞬态(LoginResponse)里有,刷新后拿不到就隐藏该行。 */
const tenantName = computed(() => {
  const c = auth.currentUser as Partial<{ tenant_name: string }> | null
  return c?.tenant_name ?? ''
})
</script>

<template>
  <div class="dash">
    <!-- 欢迎区:问候 + 全局搜索 + 时钟 -->
    <section class="hero glass">
      <div class="hero-left">
        <h1 class="greet">
          {{ greeting }}，<span class="text-gradient">{{ auth.displayName }}</span>
        </h1>
        <p class="date-line">{{ dateLine }}</p>
      </div>

      <div class="hero-right">
        <button class="search-pill" type="button" @click="ui.openPalette()">
          <el-icon><Search /></el-icon>
          <span>搜索菜单</span>
          <kbd>Ctrl K</kbd>
        </button>
        <span class="clock">{{ clock }}</span>
      </div>
    </section>

    <div class="bento">
      <!-- 账号卡 -->
      <section class="glass tile tile-account">
        <div class="account">
          <img
            v-if="auth.userProfile?.avatar"
            class="avatar"
            :src="auth.userProfile.avatar"
            alt=""
          >
          <span v-else class="avatar avatar-fallback">{{ auth.initials }}</span>
          <div class="account-meta">
            <p class="account-name">{{ auth.displayName }}</p>
            <p class="account-sub">{{ auth.userProfile?.email || auth.userProfile?.role || '—' }}</p>
          </div>
        </div>
        <router-link
          v-if="profileUri"
          :to="profileUri"
          class="account-link"
        >
          编辑个人资料 →
        </router-link>
      </section>

      <!-- 工作区信息卡 -->
      <section class="glass tile tile-info">
        <h2 class="tile-title">WORKSPACE</h2>
        <dl class="info-rows">
          <div v-if="tenantName" class="info-row">
            <dt>租户</dt>
            <dd>{{ tenantName }}</dd>
          </div>
          <div class="info-row">
            <dt>角色</dt>
            <dd>{{ auth.userProfile?.role || '—' }}</dd>
          </div>
          <div class="info-row">
            <dt>菜单</dt>
            <dd>{{ menu.flatIndex.uris.length }} 个页面</dd>
          </div>
        </dl>
      </section>

      <!-- 最近访问 -->
      <section v-if="recentTabs.length > 0" class="glass tile tile-recent">
        <h2 class="tile-title">RECENT</h2>
        <nav class="recent-list">
          <router-link
            v-for="t in recentTabs.slice(0, 5)"
            :key="t.path"
            :to="{ path: t.path, query: t.query }"
            class="recent-item"
          >
            <el-icon v-if="resolveIcon(t.icon ?? '')" class="recent-icon">
              <component :is="resolveIcon(t.icon ?? '')" />
            </el-icon>
            <span>{{ t.title }}</span>
          </router-link>
        </nav>
      </section>

      <!-- 快捷导航:整幅宽卡,按节分组排布 -->
      <section class="glass tile tile-nav">
        <h2 class="tile-title">QUICK NAV</h2>
        <div v-if="navGroups.length > 0" class="nav-groups">
          <div
            v-for="g in navGroups"
            :key="g.name"
            class="nav-group"
          >
            <h3 class="nav-group-title">{{ g.name }}</h3>
            <nav class="nav-list">
              <router-link
                v-for="item in g.items"
                :key="item.uri"
                :to="item.uri"
                class="nav-item"
              >
                <el-icon v-if="resolveIcon(item.icon)" class="nav-icon">
                  <component :is="resolveIcon(item.icon)" />
                </el-icon>
                <span>{{ item.title }}</span>
              </router-link>
            </nav>
          </div>
        </div>
        <p v-else class="nav-empty">菜单为空,请联系管理员分配权限。</p>
      </section>
    </div>
  </div>
</template>

<style scoped lang="scss">
.dash {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  min-height: 100%;
}

/* ---- 欢迎区 ---- */
.hero {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 24px 28px;
}

.greet {
  font-family: var(--display);
  font-size: clamp(22px, 2.4vw, 30px);
  font-weight: 700;
  line-height: 1.3;
}

.date-line {
  margin-top: 8px;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.08em;
  color: var(--ink-2);
}

.hero-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.search-pill {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 9px 16px;
  border: 1px solid rgba(30, 90, 90, 0.14);
  border-radius: var(--r-pill);
  background: rgba(255, 255, 255, 0.55);
  color: var(--ink-2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;

  kbd {
    padding: 1px 7px;
    border-radius: 6px;
    border: 1px solid rgba(30, 90, 90, 0.18);
    background: rgba(255, 255, 255, 0.7);
    font-family: var(--mono);
    font-size: 11px;
    color: var(--ink-3);
  }

  &:hover {
    border-color: var(--acc-mint);
    color: var(--acc-mint-deeper);
    box-shadow: var(--ring-soft);
  }
}

.clock {
  font-family: var(--mono);
  font-size: 15px;
  font-weight: 600;
  color: var(--acc-mint-deep);
  font-variant-numeric: tabular-nums;
}

/* ---- 主区网格 ---- */
/* ---- Bento 网格 ----
 * 上排三张小卡(账号 / 工作区 / 最近访问),下面一张整幅宽的快捷导航,
 * 无论菜单多少都能铺满,不会出现大片留白。 */
.bento {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  align-items: stretch;
}

.tile {
  padding: 20px 22px;
}

.tile-title {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ink-3);
}

.tile-nav {
  grid-column: 1 / -1;
}

/* 工作区信息行:点线引导 */
.info-rows {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.info-row {
  display: flex;
  align-items: baseline;
  gap: 8px;

  dt {
    font-family: var(--mono);
    font-size: 11px;
    letter-spacing: 0.08em;
    color: var(--ink-2);
    flex-shrink: 0;
  }

  dd {
    flex: 1;
    display: flex;
    align-items: baseline;
    gap: 8px;
    font-size: 13px;
    color: var(--ink);
    font-weight: 500;

    &::before {
      content: '';
      order: -1;
      flex: 1;
      border-bottom: 1px dotted rgba(30, 90, 90, 0.25);
      transform: translateY(-3px);
      min-width: 12px;
    }
  }
}

.nav-groups {
  margin-top: 14px;
  display: flex;
  flex-wrap: wrap;
  gap: 24px 48px;
}

.nav-group {
  min-width: 150px;
}

.nav-group-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--acc-mint-deeper);
}

.nav-list {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  margin: 0 -10px;
  border-radius: var(--r-sm);
  color: var(--ink-2);
  font-size: 13px;
  text-decoration: none;
  transition: all 0.15s ease;

  .nav-icon {
    color: var(--acc-mint-deep);
    font-size: 15px;
  }

  &:hover {
    background: var(--menu-active-tint);
    color: var(--acc-mint-deeper);
    transform: translateX(2px);
  }
}

/* ---- 账号卡 ---- */
.account {
  display: flex;
  align-items: center;
  gap: 14px;
}

.avatar {
  width: 46px;
  height: 46px;
  border-radius: 50%;
  object-fit: cover;
  flex-shrink: 0;
}

.avatar-fallback {
  display: grid;
  place-items: center;
  background: linear-gradient(135deg, var(--acc-mint), var(--acc-lilac));
  color: #fff;
  font-family: var(--display);
  font-weight: 700;
  font-size: 16px;
}

.account-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--ink);
}

.account-sub {
  margin-top: 3px;
  font-size: 12px;
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 170px;
}

.account-link {
  display: inline-block;
  margin-top: 16px;
  font-size: 12px;
  color: var(--acc-mint-deep);
  text-decoration: none;

  &:hover {
    color: var(--acc-mint-deeper);
    text-decoration: underline;
  }
}

.recent-list {
  margin-top: 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.recent-item {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  padding: 7px 10px;
  margin: 0 -10px;
  border-radius: var(--r-sm);
  color: var(--ink-2);
  font-size: 13px;
  text-decoration: none;
  transition: all 0.15s ease;

  .recent-icon {
    font-size: 14px;
    color: var(--ink-3);
  }

  &:hover {
    background: var(--menu-active-tint-soft);
    color: var(--acc-mint-deeper);
  }
}

@media (max-width: 900px) {
  .bento {
    grid-template-columns: 1fr;
  }

  .search-pill kbd {
    display: none;
  }
}
</style>
