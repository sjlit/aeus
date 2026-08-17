<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useMenuStore } from '../stores/menu'
import { titleForUri } from '../stores/menuGroups'
import { registerMenuRoutes } from '../router'

const auth = useAuthStore()
const menu = useMenuStore()
const route = useRoute()
const router = useRouter()

const query = ref('')

const avatar = computed(() => auth.profile?.username?.slice(0, 2).toUpperCase() ?? '?')
const pageTitle = computed(() => titleForUri(route.path, menu.tree))
const crumb = computed(() => (route.path === '/' ? 'Home' : route.path.slice(1)))

onMounted(async () => {
  await menu.load()
  await registerMenuRoutes()
})

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
            <div class="name">{{ auth.userProfile?.username ?? auth.profile?.username ?? '—' }}</div>
            <div class="role">{{ auth.profile?.tenant_name ?? auth.userProfile?.role ?? '—' }}</div>
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
          <input v-model="query" placeholder="Search nodes, users, logs..." />
        </div>
        <div class="actions">
          <el-tooltip content="新建(占位)" placement="bottom"><div class="icon-btn">+</div></el-tooltip>
          <el-tooltip content="收藏(占位)" placement="bottom"><div class="icon-btn">*</div></el-tooltip>
          <el-tooltip content="帮助(占位)" placement="bottom"><div class="icon-btn">?</div></el-tooltip>
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
