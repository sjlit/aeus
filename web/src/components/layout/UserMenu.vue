<!--
  UserMenu · 用户头像 + 下拉菜单(迁移自旧项目,去掉 i18n 与「个人中心」项)
  - 头像:用户名前两个字符,渐变底
  - 下拉:退出登录(新项目暂无个人中心页)
  - 登出走 auth.logout + 跳 /login,与旧项目行为一致
-->
<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowDown, SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '../../stores/auth'
import { goToLogin } from '../../router'

const auth = useAuthStore()
const router = useRouter()

// 显示名/头像(用户名前两字符)由 auth store 的 getter 统一提供
async function onLogout() {
  await auth.logout()
  // 跳转到登录页;router 守卫看到 accessToken=null 会放过。
  await router.push(goToLogin(router.currentRoute.value.fullPath))
}

function handleCommand(cmd: string) {
  if (cmd === 'logout') void onLogout()
}
</script>

<template>
  <el-dropdown trigger="click" @command="handleCommand">
    <button class="user-menu-trigger" type="button" :title="auth.displayName">
      <span class="um-avatar">{{ auth.initials }}</span>
      <span class="um-name">{{ auth.displayName }}</span>
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
