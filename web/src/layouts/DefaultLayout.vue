<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useMenuStore } from '../stores/menu'

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
const crumb = computed(() => (route.path === '/' ? 'Home' : route.path.slice(1)))

const actions = [
  { tip: '新建(占位)', icon: '+' },
  { tip: '收藏(占位)', icon: '*' },
  { tip: '帮助(占位)', icon: '?' },
]

function go(uri: string) {
  if (uri) router.push(uri)
}

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
  <div class="app">
    <aside class="sidebar">
      <div class="brand">aeus</div>

      <div class="nav-scroll">
        <template v-for="sec in menu.sections" :key="sec.name">
          <nav class="nav-section">
            <div class="title">{{ sec.name }}</div>
            <template v-for="node in sec.items" :key="node.component">
              <a v-if="node.uri" :class="{ on: route.path === node.uri }" @click="go(node.uri)">
                <span class="ic">{{ node.name.slice(0, 1).toUpperCase() }}</span>
                <span>{{ node.title }}</span>
              </a>
              <div v-else class="group-label">{{ node.title }}</div>
            </template>
          </nav>
        </template>
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
    </aside>

    <main class="main">
      <div class="topbar">
        <div class="search">
          <span style="color: var(--ink-2)">Q</span>
          <input placeholder="Search nodes, users, logs..." />
        </div>
        <div class="actions">
          <el-tooltip v-for="a in actions" :key="a.icon" :content="a.tip" placement="bottom">
            <div class="icon-btn">{{ a.icon }}</div>
          </el-tooltip>
        </div>
      </div>

      <div class="hero">
        <div class="kicker"><span class="dot"></span>Live · AP-SOUTHEAST-1</div>
        <div class="crumb">Workspace / {{ crumb }}</div>
        <h1><em>{{ pageTitle }}</em></h1>
      </div>

      <router-view />
    </main>
  </div>
</template>
