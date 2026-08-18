<script setup lang="ts">
/**
 * SidebarContent · 侧栏菜单的纯内容部分(品牌 + 导航树)
 * 同时被桌面 <el-aside> 与移动端 <el-drawer> 包裹使用,
 * 因此自身不持有 aside 的玻璃外框/drawer 的模态,只渲染 chrome 内部。
 * 外层样式 .menu-chrome 写在全局 app.scss,内层 .brand/.nav-scroll/.nav-menu
 * 通过该类选择器自动在两种容器里生效。
 */
import { computed, ref, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { useMenuStore } from '@/stores/menu'
import { useUiStore } from '@/stores/ui'
import type { MenuNode } from '@/types'
import { resolveIcon } from '@/utils/icons'

const menu = useMenuStore()
const ui = useUiStore()
const route = useRoute()

function subIndex(node: Pick<MenuNode, 'uri' | 'view_path' | 'name'>): string {
    return node.uri || node.view_path || node.name
}

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

const labelOnly = computed(() =>
    new Map(menu.sections.map((s) => [s.name, s.items.filter((n) => !n.uri && !n.children?.length)])),
)

const menuRef = ref<{ open?: (index: string) => void } | null>(null)

watch(defaultOpeneds, async (openeds) => {
    await nextTick()
    for (const idx of openeds) menuRef.value?.open?.(idx)
})
</script>

<template>
    <div class="brand text-gradient"><span class="brand-name">aeus</span></div>
    <div class="nav-scroll">
        <div v-for="section in menu.sections" :key="section.name" class="nav-section">
            <div class="nav-section-title">{{ section.name }}</div>
            <div v-for="node in labelOnly.get(section.name) ?? []" :key="`label-${node.view_path ?? node.name}`"
                class="nav-group-label">
                {{ node.name }}
            </div>
            <el-menu ref="menuRef" class="nav-menu" :default-active="route.path"
                :default-openeds="defaultOpeneds" :router="true" :collapse="ui.sidebarCollapsed"
                background-color="transparent" text-color="var(--sidebar-ink)" active-text-color="var(--ink)">
                <template v-for="node in section.items" :key="node.uri || node.view_path || node.name">
                    <el-sub-menu v-if="node.children && node.children.length" :index="subIndex(node)"
                        popper-class="sidebar-pop">
                        <template #title>
                            <el-icon v-if="resolveIcon(node.icon)">
                                <component :is="resolveIcon(node.icon)" />
                            </el-icon>
                            <span>{{ node.name }}</span>
                        </template>
                        <el-menu-item v-for="child in node.children" :key="child.uri" :index="child.uri">
                            <el-icon v-if="resolveIcon(child.icon)">
                                <component :is="resolveIcon(child.icon)" />
                            </el-icon>
                            <span>{{ child.name }}</span>
                        </el-menu-item>
                    </el-sub-menu>
                    <el-menu-item v-else-if="node.uri" :index="node.uri">
                        <el-icon v-if="resolveIcon(node.icon)">
                            <component :is="resolveIcon(node.icon)" />
                        </el-icon>
                        <span>{{ node.name }}</span>
                    </el-menu-item>
                </template>
            </el-menu>
        </div>
    </div>
</template>