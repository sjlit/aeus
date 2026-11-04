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

/** 可路由页面总数(状态条读数)。 */
const pageCount = computed(() => menu.flatIndex.uris.length)

/** 最近访问:当前打开的业务标签页(排除落地页自身与不可关的常驻页)。 */
const recentTabs = computed(() =>
  tabs.tabs.filter(t => t.closable && t.path !== route.path).slice(0, 6),
)

/** 个人资料页入口:按 view_path 含 profile 反查 uri;菜单里没有就隐藏入口。 */
const profileUri = computed(() => {
  for (const [uri, view] of menu.flatIndex.viewsByUri) {
    if (view.includes('profile')) return uri
  }
  return ''
})

/** 租户名只在登录瞬态(LoginResponse)里有,刷新后拿不到就隐藏该格。 */
const tenantName = computed(() => {
  const c = auth.currentUser as Partial<{ tenant_name: string }> | null
  return c?.tenant_name ?? ''
})
</script>

<template>
  <div class="dash">
    <!-- DECK · 状态甲板:问候 + 搜索/时钟,下方一行会话状态读数 -->
    <section class="deck glass rise-in" data-i="0">
      <span class="corner c-tl"></span>
      <span class="corner c-tr"></span>
      <span class="corner c-bl"></span>
      <span class="corner c-br"></span>

      <div class="deck-main">
        <div class="deck-left">
          <h1 class="greet">
            {{ greeting }},<span class="text-gradient">{{ auth.displayName }}</span>
          </h1>
          <p class="date-line">{{ dateLine }}</p>
        </div>

        <div class="deck-right">
          <button class="search-pill" type="button" @click="ui.openPalette()">
            <el-icon><Search /></el-icon>
            <span>搜索菜单</span>
            <kbd>Ctrl K</kbd>
          </button>
          <span class="clock">{{ clock }}</span>
        </div>
      </div>

      <div class="deck-status">
        <span class="status-cell">
          <span class="dot-attention dot-attention--ok"></span>
          系统正常
        </span>
        <span v-if="tenantName" class="status-cell">
          <span class="status-key">租户</span>{{ tenantName }}
        </span>
        <span class="status-cell">
          <span class="status-key">菜单</span>{{ navGroups.length }} 组 · {{ pageCount }} 页
        </span>
        <span class="status-cell">
          <span class="status-key">页签</span>{{ tabs.tabs.length }} 个打开
        </span>
      </div>
    </section>

    <div class="grid">
      <!-- NAVIGATION · 快捷导航:按菜单节分组,芯片网格 -->
      <section class="glass panel panel-nav rise-in" data-i="1">
        <h2 class="panel-title"><span class="title-bar"></span>NAVIGATION</h2>
        <div v-if="navGroups.length > 0" class="nav-groups">
          <div
            v-for="g in navGroups"
            :key="g.name"
            class="nav-group"
          >
            <h3 class="nav-group-title">
              {{ g.name }}<span class="nav-count">{{ g.items.length }}</span>
            </h3>
            <nav class="nav-chips">
              <router-link
                v-for="item in g.items"
                :key="item.uri"
                :to="item.uri"
                class="nav-chip"
              >
                <el-icon v-if="resolveIcon(item.icon)" class="chip-icon">
                  <component :is="resolveIcon(item.icon)" />
                </el-icon>
                <span>{{ item.title }}</span>
              </router-link>
            </nav>
          </div>
        </div>
        <p v-else class="nav-empty">菜单为空,请联系管理员分配权限。</p>
      </section>

      <div class="side">
        <!-- OPERATOR · 当前操作员档案 -->
        <section class="glass panel rise-in" data-i="2">
          <h2 class="panel-title"><span class="title-bar"></span>OPERATOR</h2>
          <div class="operator">
            <img
              v-if="auth.userProfile?.avatar"
              class="avatar"
              :src="auth.userProfile.avatar"
              alt=""
            >
            <span v-else class="avatar avatar-fallback">{{ auth.initials }}</span>
            <div class="operator-meta">
              <p class="operator-name">{{ auth.displayName }}</p>
              <p class="operator-sub">{{ auth.userProfile?.email || '—' }}</p>
            </div>
          </div>
          <dl class="operator-rows">
            <div class="op-row">
              <dt>角色</dt>
              <dd>{{ auth.userProfile?.role || '—' }}</dd>
            </div>
            <div class="op-row">
              <dt>UID</dt>
              <dd class="op-mono">{{ auth.currentUser?.uid || '—' }}</dd>
            </div>
          </dl>
          <router-link
            v-if="profileUri"
            :to="profileUri"
            class="profile-link"
          >
            编辑个人资料 →
          </router-link>
        </section>

        <!-- RECENT · 最近访问;没有打开的页签时整卡隐藏 -->
        <section v-if="recentTabs.length > 0" class="glass panel rise-in" data-i="3">
          <h2 class="panel-title"><span class="title-bar"></span>RECENT</h2>
          <nav class="recent-list">
            <router-link
              v-for="t in recentTabs"
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
      </div>
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

/* ---- DECK · 状态甲板 ----
 * 四角 HUD 括号 + 渐隐网格纹理铺科技底色;
 * 上区问候/时钟,下区一行会话状态读数(mono 分隔)。 */
.deck {
  position: relative;
  padding: 26px 28px 0;
  overflow: hidden;
}

/* 细网格纹理:薄荷细线 28px 栅格,向下渐隐;
 * 颜色取 --acc-mint 的 rgb 值,透明度压到 ~0.055 只做底纹不抢戏 */
.deck::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(108, 197, 168, 0.055) 1px, transparent 1px),
    linear-gradient(90deg, rgba(108, 197, 168, 0.055) 1px, transparent 1px);
  background-size: 28px 28px;
  -webkit-mask-image: linear-gradient(to bottom, #000, transparent 80%);
  mask-image: linear-gradient(to bottom, #000, transparent 80%);
  pointer-events: none;
}

/* 四角括号:1px 薄荷描边的 L 形,内缩 10px 落在玻璃卡圆角之内 */
.corner {
  position: absolute;
  width: 14px;
  height: 14px;
  border: 1px solid var(--acc-mint);
  opacity: 0.5;
  pointer-events: none;
}

.c-tl { top: 10px; left: 10px; border-right: none; border-bottom: none; }
.c-tr { top: 10px; right: 10px; border-left: none; border-bottom: none; }
.c-bl { bottom: 10px; left: 10px; border-right: none; border-top: none; }
.c-br { bottom: 10px; right: 10px; border-left: none; border-top: none; }

.deck-main {
  position: relative;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
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

.deck-right {
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
  transition:
    border-color 0.15s ease,
    color 0.15s ease,
    box-shadow 0.15s ease,
    transform 0.1s ease;

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

  &:active {
    transform: translateY(1px);
  }
}

.clock {
  font-family: var(--mono);
  font-size: 16px;
  font-weight: 600;
  color: var(--acc-mint-deep);
  font-variant-numeric: tabular-nums;
}

/* 状态条:点线分隔的会话读数行 */
.deck-status {
  position: relative;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  margin-top: 22px;
  padding: 12px 0;
  border-top: 1px dashed rgba(30, 90, 90, 0.15);
}

.status-cell {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  font-family: var(--mono);
  font-size: 12px;
  font-weight: 500;
  color: var(--ink);

  &:first-child { padding-left: 0; }
  &:not(:last-child) { border-right: 1px solid rgba(30, 90, 90, 0.1); }

  .status-key {
    font-size: 11px;
    font-weight: 400;
    letter-spacing: 0.04em;
    color: var(--ink-2);
  }
}

/* ---- 主区网格:左导航(2 份) / 右列档案+最近(1 份) ---- */
.grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.side {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.panel {
  padding: 20px 22px;
}

/* 面板标题:渐变竖条 + mono 大写字距 */
.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.14em;
  color: var(--ink-3);
}

.title-bar {
  width: 3px;
  height: 12px;
  border-radius: 2px;
  background: linear-gradient(180deg, var(--acc-mint), var(--acc-lilac));
  flex-shrink: 0;
}

/* ---- NAVIGATION ---- */
.nav-groups {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.nav-group-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--acc-mint-deeper);
}

.nav-count {
  padding: 0 7px;
  border-radius: var(--r-pill);
  border: 1px solid rgba(30, 90, 90, 0.12);
  background: var(--glass-soft);
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 500;
  line-height: 16px;
  color: var(--ink-2);
}

.nav-chips {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.nav-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid rgba(30, 90, 90, 0.1);
  border-radius: var(--r-sm);
  background: var(--glass-soft);
  color: var(--ink-2);
  font-size: 13px;
  text-decoration: none;
  transition: all 0.15s ease;

  .chip-icon {
    font-size: 14px;
    color: var(--acc-mint-deep);
    flex-shrink: 0;
  }

  &:hover {
    background: var(--menu-active-tint);
    border-color: rgba(108, 197, 168, 0.4);
    color: var(--acc-mint-deeper);
    transform: translateY(-1px);
    box-shadow: 0 4px 12px var(--shadow-soft);

    .chip-icon {
      color: var(--acc-mint-deeper);
    }
  }

  &:active {
    transform: translateY(0);
  }
}

.nav-empty {
  margin-top: 14px;
  font-size: 13px;
  color: var(--ink-3);
}

/* ---- OPERATOR ---- */
.operator {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 14px;
}

.avatar {
  width: 46px;
  height: 46px;
  border-radius: var(--r-sm);
  object-fit: cover;
  flex-shrink: 0;
  box-shadow: 0 0 0 1px var(--glass-border), 0 4px 12px var(--shadow-soft);
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

.operator-meta {
  min-width: 0;
}

.operator-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--ink);
}

.operator-sub {
  margin-top: 3px;
  font-size: 12px;
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.operator-rows {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px dashed rgba(30, 90, 90, 0.12);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.op-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;

  dt {
    font-family: var(--mono);
    font-size: 11px;
    letter-spacing: 0.06em;
    color: var(--ink-2);
    flex-shrink: 0;
  }

  dd {
    font-size: 12.5px;
    font-weight: 500;
    color: var(--ink);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: right;
  }

  .op-mono {
    font-family: var(--mono);
    font-size: 11.5px;
  }
}

.profile-link {
  display: inline-block;
  margin-top: 14px;
  font-size: 12px;
  color: var(--acc-mint-deep);
  text-decoration: none;

  &:hover {
    color: var(--acc-mint-deeper);
    text-decoration: underline;
  }
}

/* ---- RECENT ---- */
.recent-list {
  margin-top: 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.recent-item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 10px;
  margin: 0 -10px;
  border-radius: var(--r-sm);
  color: var(--ink-2);
  font-size: 13px;
  text-decoration: none;
  transition: all 0.15s ease;

  .recent-icon {
    font-size: 14px;
    color: var(--ink-3);
    flex-shrink: 0;
  }

  &:hover {
    background: var(--menu-active-tint-soft);
    color: var(--acc-mint-deeper);

    .recent-icon {
      color: var(--acc-mint-deep);
    }
  }
}

/* ---- 响应式 ---- */
@media (max-width: 960px) {
  .grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .search-pill kbd {
    display: none;
  }

  .status-cell {
    padding: 0 12px;
  }
}
</style>
