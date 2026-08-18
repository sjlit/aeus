<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { useMenuStore } from '../stores/menu'
import { useUiStore } from '../stores/ui'
import { useTabsStore } from '../stores/tabs'
import type { MenuNode } from '../types'
import HeaderToolbar from '../components/layout/HeaderToolbar.vue'
import HeaderTabStrip from '../components/layout/HeaderTabStrip.vue'
import { usePageTitle } from '../composables/usePageTitle'
// 菜单/多标签共用的图标解析(缓存 + kebab/snake 归一),见 utils/icons.ts
import { resolveIcon } from '../utils/icons'

const menu = useMenuStore()
const ui = useUiStore()
const route = useRoute()
const tabs = useTabsStore()

const pageTitle = usePageTitle()

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

// 纯标题节点(无 uri、无 children)放在 el-menu 外渲染;每节一次过滤,
// 避免模板的 v-for 里每次渲染都重建数组。
const labelOnly = computed(() =>
  new Map(menu.sections.map((s) => [s.name, s.items.filter((n) => !n.uri && !n.children?.length)])),
)

// el-menu 的 defaultOpeneds 只在初始化时生效;深链/router.push 后到二级页面,
// 需要主动调 open() 把父菜单重新撑开。直接 watch 集合本身:
// 父菜单集合没变(同父下换子页)就不做无谓的遍历 + open。
const menuRef = ref<{ open?: (index: string) => void } | null>(null)
watch(defaultOpeneds, async (openeds) => {
  await nextTick()
  for (const idx of openeds) menuRef.value?.open?.(idx)
})
</script>

<template>
  <el-container class="app" direction="horizontal">
    <el-aside class="sidebar" :class="{ 'is-collapsed': ui.sidebarCollapsed }"
      :width="ui.sidebarCollapsed ? '64px' : '240px'">
      <div class="brand text-gradient"><span class="brand-name">aeus</span></div>
      <div class="nav-scroll">
        <div v-for="sec in menu.sections" :key="sec.name" class="nav-section">
          <div class="nav-section-title">{{ sec.name }}</div>
          <!-- 没有 uri 也没有 children 的纯标题节点,放在 el-menu 外面
               (el-menu 内部只接受菜单组件)。 -->
          <div v-for="node in labelOnly.get(sec.name) ?? []" :key="`label-${node.component}`"
            class="nav-group-label">
            {{ node.title }}
          </div>
          <el-menu ref="menuRef" class="nav-menu" :default-active="route.path" :default-openeds="defaultOpeneds"
            :router="true" :collapse="ui.sidebarCollapsed" background-color="transparent"
            text-color="var(--sidebar-ink)" active-text-color="var(--ink)">
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
    </el-aside>

    <el-container class="content" direction="vertical">
      <!-- 同一张玻璃卡片:工具栏 + 分隔线 + 多标签栏(参考项目 UnifiedHeader 形态) -->
      <el-header class="topbar" height="auto">
        <HeaderToolbar />
        <div class="chrome-sep"></div>
        <HeaderTabStrip />
      </el-header>

      <el-main>
        <div class="hero">
          <h1><em class="text-gradient--peach">{{ pageTitle }}</em></h1>
        </div>
        <!-- keep-alive 按路由 meta 白名单缓存页面实例;:key 用 path::token,
             多 route 共享同一 view 时缓存 slot 互不碰撞,refreshTab 靠 token 变化
             强制重挂载。见 stores/tabs.ts 头部注释。 -->
        <router-view v-slot="{ Component, route: r }">
          <keep-alive :include="tabs.cachedViews">
            <component :is="Component" :key="`${r.path}::${tabs.getRefreshToken(r.path)}`" />
          </keep-alive>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>
