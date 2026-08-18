<script setup lang="ts">
import { computed, ref, watch, nextTick, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { useMenuStore } from '../stores/menu'
import { useUiStore } from '../stores/ui'
import * as ElIcons from '@element-plus/icons-vue'
import type { MenuNode } from '../types'
import HeaderToolbar from '../components/layout/HeaderToolbar.vue'

const menu = useMenuStore()
const ui = useUiStore()
const route = useRoute()

// 标题由 registerMenuRoutes 写入 meta.title,与 PlaceholderView 取法保持一致。
const pageTitle = computed(() => (route.meta.title as string) ?? route.path)

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
</script>

<template>
  <el-container class="app" direction="horizontal">
    <el-aside class="sidebar" :class="{ 'is-collapsed': ui.sidebarCollapsed }"
      :width="ui.sidebarCollapsed ? '64px' : '240px'">
      <div class="brand"><span class="brand-name">aeus</span></div>
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
      <el-header class="topbar" height="auto">
        <HeaderToolbar />
      </el-header>

      <el-main>
        <div class="hero">
          <h1><em>{{ pageTitle }}</em></h1>
        </div>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>
