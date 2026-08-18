<!--
  UserMenu · 用户头像 + 下拉菜单(迁移自旧项目,去掉 i18n 与「个人中心」项)
  - 头像:用户名前两个字符,渐变底
  - 下拉:退出登录(新项目暂无个人中心页)
  - 登出走 auth.logout + 跳 /login,与旧项目行为一致
-->
<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowDown, SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const router = useRouter()

// profile(LoginResponse)与 userProfile(GET /user/profile)都可能先到,
// 优先级与迁移前 DefaultLayout 中的一致。
const username = computed(() => auth.userProfile?.username ?? auth.profile?.username ?? '')
const displayName = computed(() => username.value || '—')
const avatar = computed(() => username.value.slice(0, 2).toUpperCase() || '?')

async function onLogout() {
  await auth.logout()
  // 跳转到登录页;router 守卫看到 accessToken=null 会放过。
  await router.push({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })
}

function handleCommand(cmd: string) {
  if (cmd === 'logout') void onLogout()
}
</script>

<template>
  <el-dropdown trigger="click" @command="handleCommand">
    <button class="user-menu-trigger" type="button" :title="displayName">
      <span class="um-avatar">{{ avatar }}</span>
      <span class="um-name">{{ displayName }}</span>
      <el-icon class="um-arrow"><ArrowDown /></el-icon>
    </button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="logout" divided>
          <el-icon><SwitchButton /></el-icon>
          退出登录
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<style scoped>
.user-menu-trigger {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px 4px 4px;
  background: var(--glass-soft);
  border: 1px solid var(--glass-border);
  border-radius: var(--r-pill);
  cursor: pointer;
  transition: all 0.18s;
  font-family: inherit;
  color: var(--ink);
}
.user-menu-trigger:hover {
  background: white;
  border-color: var(--acc-mint);
  box-shadow: 0 2px 10px var(--shadow-soft);
}
.um-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--acc-peach), var(--acc-lilac));
  display: grid;
  place-items: center;
  color: white;
  font-weight: 600;
  font-size: 10px;
  flex-shrink: 0;
}
.um-name {
  font-size: 12px;
  font-weight: 600;
}
.um-arrow {
  font-size: 10px;
  color: var(--ink-3);
}
</style>
