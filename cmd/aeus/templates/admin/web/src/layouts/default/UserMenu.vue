<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowDown, SwitchButton, User as UserIcon } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { goToLogin, PROFILE_PATH } from '@/router'

const auth = useAuthStore()
const router = useRouter()

// 显示名/头像(用户名前两字符)由 auth store 的 getter 统一提供
async function onLogout() {
    await auth.logout()
    await router.push(goToLogin(router.currentRoute.value.fullPath))
}

// "个人中心"和"退出登录"走不同分支:前者跳页面,后者登出。
function onCommand(cmd: string) {
    if (cmd === 'profile') {
        // 已在当前页时 router.push 不会重复触发 tab 添加,但 keep-alive
        // 里的 :key 也不变 → 用户体验正常。强制走 push 即可,无需重载。
        void router.push(PROFILE_PATH)
        return
    }
    if (cmd === 'logout') void onLogout()
}
</script>

<template>
    <el-dropdown trigger="click" @command="onCommand">
        <button class="user-menu-trigger" type="button" :title="auth.displayName">
            <span class="um-avatar">{{ auth.initials }}</span>
            <span class="um-name">{{ auth.displayName }}</span>
            <el-icon class="um-arrow">
                <ArrowDown />
            </el-icon>
        </button>
        <template #dropdown>
            <el-dropdown-menu>
                <el-dropdown-item command="profile">
                    <el-icon>
                        <UserIcon />
                    </el-icon>
                    个人中心
                </el-dropdown-item>
                <el-dropdown-item command="logout" divided>
                    <el-icon>
                        <SwitchButton />
                    </el-icon>
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
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--acc-peach), var(--acc-lilac));
    display: grid;
    place-items: center;
    color: white;
    font-weight: 600;
    font-size: 11px;
    flex-shrink: 0;
}

.um-name {
    font-size: 13px;
    font-weight: 600;
}

.um-arrow {
    font-size: 11px;
    color: var(--ink-3);
}
</style>
