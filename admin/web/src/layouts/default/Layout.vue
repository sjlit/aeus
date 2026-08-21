<script setup lang="ts">
import { watch } from 'vue'
import { useRoute } from 'vue-router'
import { useMenuStore } from '@/stores/menu'
import { useUiStore } from '@/stores/ui'
import { useTabsStore } from '@/stores/tabs'
import { useMediaQuery } from '@/composables/useMediaQuery'
import HeaderBar from './HeaderBar.vue'
import TabStrip from './TabStrip.vue'
import SidebarContent from './SidebarContent.vue'
import CommandPalette from './CommandPalette.vue'

const menu = useMenuStore()
const ui = useUiStore()
const route = useRoute()
const tabs = useTabsStore()
const isMobile = useMediaQuery('(max-width: 768px)')

// 路由切换时关掉抽屉,避免"点了菜单但抽屉还盖在内容上"
watch(
    () => route.fullPath,
    () => {
        if (isMobile.value) ui.closeMobileSidebar()
    },
)

// 从桌面切到移动端,关掉抽屉(避免抽屉开着然后切换布局)
watch(isMobile, (now) => {
    if (!now) ui.closeMobileSidebar()
})
</script>

<template>
    <el-container class="app-main" direction="horizontal">
        <!-- 桌面:内嵌侧栏。移动端整段不渲染(改为下方抽屉) -->
        <el-aside v-if="!isMobile" class="sidebar menu-chrome"
            :class="{ 'is-collapsed': ui.sidebarCollapsed }"
            :width="ui.sidebarCollapsed ? '64px' : '240px'">
            <SidebarContent />
        </el-aside>

        <!-- 移动端:抽屉式菜单。从左侧滑出,宽度封顶 300px,
             小屏(<=375px)再按 80vw 缩,避免在窄手机上完全盖住内容 -->
        <el-drawer v-if="isMobile" v-model="ui.mobileSidebarOpen" direction="ltr" size="min(300px, 80vw)"
            :with-header="false" :modal="true" :append-to-body="true" class="sidebar-drawer">
            <div class="menu-chrome mobile-menu">
                <SidebarContent />
            </div>
        </el-drawer>

        <el-container class="content" direction="vertical">
            <el-header class="topbar" height="auto">
                <HeaderBar />
                <div class="chrome-sep"></div>
                <TabStrip />
            </el-header>

            <el-main>
                <router-view v-slot="{ Component, route: r }">
                    <!-- Transition 包 keep-alive,模式 out-in 让离场先于入场;
                         page-* 过渡四件套在 motion.scss 定义 -->
                    <Transition name="page" mode="out-in">
                        <keep-alive :include="tabs.cachedViews">
                            <component :is="Component" :key="`${r.path}::${tabs.getRefreshToken(r.path)}`" />
                        </keep-alive>
                    </Transition>
                </router-view>
            </el-main>
        </el-container>

        <!-- ⌘K 命令面板:teleport 到 body,全局热键开关 -->
        <CommandPalette />
    </el-container>
</template>