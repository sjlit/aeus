<script setup lang="ts">
import { computed, ref, watch, nextTick, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useMenuStore } from '../stores/menu'
import {
  Plus,
  QuestionFilled,
  Search,
  Star,
} from '@element-plus/icons-vue'
import * as ElIcons from '@element-plus/icons-vue'
import type { MenuNode } from '../types'

const auth = useAuthStore()
const menu = useMenuStore()
const route = useRoute()
const router = useRouter()

// profile(LoginResponse)与 userProfile(GET /user/profile)都可能先到,
// 统一在此处定优先级,模板里不再各写一遍 ?? 链。
const username = computed(() => auth.userProfile?.username ?? auth.profile?.username ?? '')
const displayName = computed(() => username.value || '—')
const displayRole = computed(() => auth.profile?.tenant_name ?? auth.userProfile?.role ?? '—')
const avatar = computed(() => username.value.slice(0, 2).toUpperCase() || '?')

// 标题由 registerMenuRoutes 写入 meta.title,与 PlaceholderView 取法保持一致。
const pageTitle = computed(() => (route.meta.title as string) ?? route.path)

// 面包屑:从 route.matched 自动推导(每段 meta.title 一段)。零配置,
// 中间段是可点击的 router-link,最后一段是当前页(不可点)。响应式下
// 压缩成「… / 当前页」两段,避免窄屏溢出。
interface Crumb { label: string; path: string; last: boolean }
const crumbs = computed<Crumb[]>(() => {
  const list = route.matched
    .filter((r) => r.meta?.title)
    .map((r) => ({ label: r.meta.title as string, path: r.path, last: false }))
  if (list.length) list[list.length - 1]!.last = true
  return list
})
const compactCrumbs = computed<Crumb[]>(() => {
  if (crumbs.value.length <= 2) return crumbs.value
  const last = crumbs.value[crumbs.value.length - 1]!
  return [{ label: '…', path: '', last: false }, last]
})

// 后端 menu.icon 是短串(user / sys-role / Monitor);Element Plus 图标组件名是
// PascalCase。直接命中优先,再做 kebab/snake → PascalCase 的归一;都没有就 null,
// 模板里再决定要不要画图标。
const ICONS = ElIcons as Record<string, Component>
function toPascal(name: string): string {
  return name
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join('')
}
function resolveIcon(name: string | undefined): Component | null {
  if (!name) return null
  return ICONS[name] ?? ICONS[toPascal(name)] ?? null
}

// el-sub-menu 的 index 必须是稳定、唯一的(用于 defaultOpeneds);uri 优先,
// 没有 uri 的文件夹节点就用 component 兜底。el-menu-item 用 uri 作为
// router-link 目标。
function subIndex(node: Pick<MenuNode, 'uri' | 'component'>): string {
  return node.uri || node.component
}

// 当前路径命中的二级菜单父节点(子项 active 时自动展开父)。
const defaultOpeneds = computed<string[]>(() => {
  const path = route.path
  const out: string[] = []
  for (const sec of menu.sections) {
    for (const node of sec.items) {
      if (!node.children?.length) continue
      if (node.children.some((c) => c.uri === path)) out.push(subIndex(node))
    }
  }
  return out
})

// el-menu 的 defaultOpeneds 只在初始化时生效;深链/router.push 后到二级页面,
// 需要主动调 open() 把父菜单重新撑开。
const menuRef = ref<{ open?: (index: string) => void } | null>(null)
watch(
  () => route.path,
  async () => {
    await nextTick()
    for (const idx of defaultOpeneds.value) menuRef.value?.open?.(idx)
  },
)

// 顶部右侧的占位动作:用 Element Plus 图标替代原先的字符。
const actions = [
  { tip: '新建(占位)', icon: Plus },
  { tip: '收藏(占位)', icon: Star },
  { tip: '帮助(占位)', icon: QuestionFilled },
] as const

async function onLogout() {
  await auth.logout()
  // 跳转到登录页;router 守卫看到 accessToken=null 会放过。
  await router.push({ path: '/login', query: { redirect: route.fullPath } })
}

function onCommand(cmd: string) {
  if (cmd === 'logout') void onLogout()
}
</script>

<template>
  <el-container class="app" direction="horizontal">
    <el-aside class="sidebar" width="240px">
      <div class="brand">aeus</div>
      <div class="nav-scroll">
        <div v-for="sec in menu.sections" :key="sec.name" class="nav-section">
          <div class="nav-section-title">{{ sec.name }}</div>
          <!-- 没有 uri 也没有 children 的纯标题节点,放在 el-menu 外面
               (el-menu 内部只接受菜单组件)。 -->
          <div v-for="node in sec.items.filter(
            (n) => !n.uri && !(n.children && n.children.length),
          )" :key="`label-${node.component}`" class="nav-group-label">
            {{ node.title }}
          </div>
          <el-menu ref="menuRef" class="nav-menu" :default-active="route.path" :default-openeds="defaultOpeneds"
            :router="true" background-color="transparent" text-color="var(--sidebar-ink)"
            active-text-color="var(--ink)">
            <template v-for="node in sec.items" :key="node.component">
              <!-- 有 children:二级菜单(文件夹) -->
              <el-sub-menu v-if="node.children && node.children.length" :index="subIndex(node)"
                popper-class="sidebar-pop">
                <template #title>
                  <el-icon v-if="resolveIcon(node.icon)">
                    <component :is="resolveIcon(node.icon)" />
                  </el-icon>
                  <span>{{ node.title }}</span>
                </template>
                <el-menu-item v-for="child in node.children" :key="child.uri" :index="child.uri">
                  <el-icon v-if="resolveIcon(child.icon)">
                    <component :is="resolveIcon(child.icon)" />
                  </el-icon>
                  <span>{{ child.title }}</span>
                </el-menu-item>
              </el-sub-menu>
              <!-- 有 uri 但无 children:叶子菜单 -->
              <el-menu-item v-else-if="node.uri" :index="node.uri">
                <el-icon v-if="resolveIcon(node.icon)">
                  <component :is="resolveIcon(node.icon)" />
                </el-icon>
                <span>{{ node.title }}</span>
              </el-menu-item>
            </template>
          </el-menu>
        </div>
      </div>

      <el-dropdown trigger="click" @command="onCommand">
        <div class="user-pill" role="button" tabindex="0">
          <div class="avatar">{{ avatar }}</div>
          <div class="info">
            <div class="name">{{ displayName }}</div>
            <div class="role">{{ displayRole }}</div>
          </div>
        </div>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="logout">退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </el-aside>

    <el-container class="content" direction="vertical">
      <el-header class="topbar" height="auto">
        <button class="search" type="button" aria-label="Search · ⌘K" title="Search · ⌘K">
          <el-icon class="search-icon">
            <Search />
          </el-icon>
          <span class="search-placeholder">Search nodes, users, logs...</span>
          <kbd class="search-kbd">⌘K</kbd>
        </button>
        <div class="actions">
          <el-tooltip v-for="action in actions" :key="action.tip" :content="action.tip" placement="bottom">
            <div class="icon-btn">
              <el-icon>
                <component :is="action.icon" />
              </el-icon>
            </div>
          </el-tooltip>
        </div>
      </el-header>

      <el-main>
        <div class="hero">
          <!-- 标准面包屑:宽屏完整显示 -->
          <nav v-if="crumbs.length" class="crumbs" aria-label="Breadcrumb">
            <template v-for="(c, i) in crumbs" :key="`w-${c.path}`">
              <router-link v-if="!c.last" :to="c.path" class="crumb crumb-link">{{ c.label }}</router-link>
              <span v-else class="crumb crumb-current">{{ c.label }}</span>
              <span v-if="i < crumbs.length - 1" class="crumb-sep">/</span>
            </template>
          </nav>
          <!-- 紧凑面包屑:窄屏显示 … / 当前页 -->
          <nav v-if="crumbs.length" class="crumbs crumbs-compact" aria-label="Breadcrumb">
            <template v-for="(c, i) in compactCrumbs" :key="`c-${c.path}`">
              <router-link v-if="!c.last" :to="c.path || '#'" class="crumb crumb-link">{{ c.label }}</router-link>
              <span v-else class="crumb crumb-current">{{ c.label }}</span>
              <span v-if="i < compactCrumbs.length - 1" class="crumb-sep">/</span>
            </template>
          </nav>
          <h1><em>{{ pageTitle }}</em></h1>
        </div>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>