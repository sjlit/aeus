<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ArrowLeft, ArrowRight, Back, CircleClose, Close, FolderRemove, Refresh, Right } from '@element-plus/icons-vue'
import { useTabsStore, type Tab } from '@/stores/tabs'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { resolveIcon } from '@/utils/icons'

const router = useRouter()
const route = useRoute()
const tabsStore = useTabsStore()
const isMobile = useMediaQuery('(max-width: 768px)')

// 标签栏滚动控制
const tabsContainer = ref<HTMLElement | null>(null)
const showLeftArrow = ref(false)
const showRightArrow = ref(false)

// 右键菜单
const contextMenuVisible = ref(false)
const contextMenuTab = ref<Tab | null>(null)
const contextMenuPosition = ref({ x: 0, y: 0 })

// 拖拽状态
const dragTab = ref<Tab | null>(null)
const dragOverTab = ref<Tab | null>(null)

function onTabsListKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    if (!target || !target.classList.contains('tab')) return
    const idxStr = target.dataset.tabIdx
    if (idxStr === undefined) return
    const currentIdx = Number(idxStr)
    if (Number.isNaN(currentIdx) || currentIdx < 0) return

    const list = tabsStore.tabs
    let nextIdx: number | null = null

    switch (e.key) {
        case 'ArrowLeft':
            nextIdx = currentIdx > 0 ? currentIdx - 1 : list.length - 1
            break
        case 'ArrowRight':
            nextIdx = currentIdx < list.length - 1 ? currentIdx + 1 : 0
            break
        case 'Home':
            nextIdx = 0
            break
        case 'End':
            nextIdx = list.length - 1
            break
        case 'Enter':
        case ' ':
            e.preventDefault()
            if (list[currentIdx]) switchTab(list[currentIdx]!)
            return
        default:
            return
    }

    e.preventDefault()
    if (nextIdx === null) return
    const nextTab = list[nextIdx]
    if (!nextTab) return
    switchTab(nextTab)
    nextTick(() => {
        const nodes = tabsContainer.value?.querySelectorAll<HTMLElement>('.tab')
        const el = nodes?.[nextIdx!]
        el?.focus()
        el?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    })
}

function switchTab(tab: Tab) {
    tabsStore.setActiveTab(tab.path)
    router.push({ path: tab.path, query: tab.query })
}

function closeTab(tab: Tab) {
    if (!tab.closable) return
    tabsStore.removeTab(tab.path)
    if (route.path === tab.path) {
        const newActive = tabsStore.tabs.find(t => t.path === tabsStore.activeTab)
        if (newActive) {
            router.push({ path: newActive.path, query: newActive.query })
        }
    }
}

function openContextMenu(e: MouseEvent, tab: Tab) {
    e.preventDefault()
    contextMenuTab.value = tab
    contextMenuPosition.value = { x: e.clientX, y: e.clientY }
    contextMenuVisible.value = true
    const closeMenu = () => {
        contextMenuVisible.value = false
        document.removeEventListener('click', closeMenu)
    }
    nextTick(() => {
        document.addEventListener('click', closeMenu)
    })
}

function onContextMenuAction(action: string) {
    const tab = contextMenuTab.value
    if (!tab) return
    switch (action) {
        case 'refresh':
            tabsStore.refreshTab(tab.path)
            break
        case 'close':
            closeTab(tab)
            break
        case 'closeOthers':
            tabsStore.removeOtherTabs(tab.path)
            if (route.path !== tab.path) {
                router.push({ path: tab.path, query: tab.query })
            }
            break
        case 'closeLeft':
            tabsStore.removeLeftTabs(tab.path)
            break
        case 'closeRight':
            tabsStore.removeRightTabs(tab.path)
            break
        case 'closeAll':
            tabsStore.removeAllTabs()
            const homeTab = tabsStore.tabs[0]
            if (homeTab) {
                router.push({ path: homeTab.path, query: homeTab.query })
            }
            break
    }
    contextMenuVisible.value = false
}

function onDragStart(e: DragEvent, tab: Tab) {
    dragTab.value = tab
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function onDragOver(e: DragEvent, tab: Tab) {
    e.preventDefault()
    dragOverTab.value = tab
}
function onDrop(e: DragEvent, targetTab: Tab) {
    e.preventDefault()
    if (!dragTab.value || dragTab.value.path === targetTab.path) return
    const fromIdx = tabsStore.tabs.findIndex(t => t.path === dragTab.value!.path)
    const toIdx = tabsStore.tabs.findIndex(t => t.path === targetTab.path)
    if (fromIdx === -1 || toIdx === -1) return
    const [removed] = tabsStore.tabs.splice(fromIdx, 1)
    if (removed) tabsStore.tabs.splice(toIdx, 0, removed)
    dragTab.value = null
    dragOverTab.value = null
}
function onDragEnd() {
    dragTab.value = null
    dragOverTab.value = null
}

function checkScroll() {
    const container = tabsContainer.value
    if (!container) return
    showLeftArrow.value = container.scrollLeft > 0
    showRightArrow.value = container.scrollLeft + container.clientWidth < container.scrollWidth - 10
}
function scrollLeft() { tabsContainer.value?.scrollBy({ left: -200, behavior: 'smooth' }) }
function scrollRight() { tabsContainer.value?.scrollBy({ left: 200, behavior: 'smooth' }) }

onMounted(() => {
    checkScroll()
    window.addEventListener('resize', checkScroll)
})
onBeforeUnmount(() => {
    window.removeEventListener('resize', checkScroll)
})
watch(() => tabsStore.tabs.length, () => nextTick(checkScroll))
</script>

<template>
    <div v-if="isMobile" class="tabs-mobile">
        <el-select :model-value="tabsStore.activeTab" size="small" class="tab-select" @update:model-value="(val: string) => {
            const tab = tabsStore.tabs.find(t => t.path === val)
            if (tab) switchTab(tab)
        }">
            <el-option v-for="tab in tabsStore.tabs" :key="tab.path" :label="tab.title" :value="tab.path">
                <span class="option-label">
                    <el-icon v-if="tab.icon" class="option-icon">
                        <component :is="resolveIcon(tab.icon)" />
                    </el-icon>
                    {{ tab.title }}
                </span>
            </el-option>
        </el-select>
    </div>

    <div v-else class="tabs-row">
        <button v-if="showLeftArrow" class="scroll-arrow scroll-left" aria-label="向左滚动" title="向左滚动"
            @click="scrollLeft">
            <el-icon>
                <ArrowLeft />
            </el-icon>
        </button>
        <div ref="tabsContainer" class="tabs-list" role="tablist" aria-label="已打开的标签页" @scroll="checkScroll"
            @keydown="onTabsListKeydown">
            <div v-for="(tab, idx) in tabsStore.tabs" :key="tab.path" class="tab" :class="{
                active: tab.path === tabsStore.activeTab,
                'drag-over': dragOverTab?.path === tab.path,
            }" :data-tab-idx="idx" :data-tab-path="tab.path" role="tab" :aria-selected="tab.path === tabsStore.activeTab"
                :tabindex="tab.path === tabsStore.activeTab ? 0 : -1" :aria-label="tab.title" :draggable="true"
                @click="switchTab(tab)" @contextmenu="openContextMenu($event, tab)"
                @dragstart="onDragStart($event, tab)" @dragover="onDragOver($event, tab)" @drop="onDrop($event, tab)"
                @dragend="onDragEnd">
                <el-icon v-if="tab.icon" class="tab-icon">
                    <component :is="resolveIcon(tab.icon)" />
                </el-icon>
                <span class="tab-title">{{ tab.title }}</span>
                <button v-if="tab.closable" class="tab-close" aria-label="关闭当前" title="关闭当前" tabindex="-1"
                    @click.stop="closeTab(tab)" @mousedown.stop>
                    <el-icon>
                        <Close />
                    </el-icon>
                </button>
            </div>
        </div>
        <button v-if="showRightArrow" class="scroll-arrow scroll-right" aria-label="向右滚动" title="向右滚动"
            @click="scrollRight">
            <el-icon>
                <ArrowRight />
            </el-icon>
        </button>
        <teleport to="body">
            <div v-if="contextMenuVisible" class="context-menu" role="menu" :style="{
                left: contextMenuPosition.x + 'px',
                top: contextMenuPosition.y + 'px',
            }">
                <button class="menu-item" role="menuitem" type="button" @click="onContextMenuAction('refresh')">
                    <el-icon>
                        <Refresh />
                    </el-icon>
                    刷新当前
                </button>
                <button v-if="contextMenuTab?.closable" class="menu-item" role="menuitem" type="button"
                    @click="onContextMenuAction('close')">
                    <el-icon>
                        <Close />
                    </el-icon>
                    关闭当前
                </button>
                <button class="menu-item" role="menuitem" type="button" @click="onContextMenuAction('closeOthers')">
                    <el-icon>
                        <CircleClose />
                    </el-icon>
                    关闭其他
                </button>
                <button class="menu-item" role="menuitem" type="button" @click="onContextMenuAction('closeLeft')">
                    <el-icon>
                        <Back />
                    </el-icon>
                    关闭左侧
                </button>
                <button class="menu-item" role="menuitem" type="button" @click="onContextMenuAction('closeRight')">
                    <el-icon>
                        <Right />
                    </el-icon>
                    关闭右侧
                </button>
                <div class="menu-divider"></div>
                <button class="menu-item" role="menuitem" type="button" @click="onContextMenuAction('closeAll')">
                    <el-icon>
                        <Close />
                    </el-icon>
                    全部关闭
                </button>
            </div>
        </teleport>
    </div>
</template>

<style scoped>
.tabs-row {
    display: flex;
    align-items: center;
    height: 42px;
    padding: 0 8px 0 12px;
    gap: 6px;
}

.tabs-list {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 2px;
    overflow-x: auto;
    scrollbar-width: none;
    -ms-overflow-style: none;
    min-width: 0;
}

.tabs-list::-webkit-scrollbar {
    display: none;
}

.tab {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 6px 10px;
    border-radius: 8px;
    cursor: pointer;
    color: var(--ink-2);
    font-size: 12.5px;
    font-weight: 400;
    letter-spacing: 0.01em;
    white-space: nowrap;
    flex-shrink: 0;
    max-width: 220px;
    background: transparent;
    border: none;
    font-family: inherit;
    transition:
        color 0.18s ease,
        background 0.18s ease,
        box-shadow 0.18s ease;
}

.tab:hover {
    color: var(--ink);
    background: var(--glass-soft);
}

.tab:focus {
    outline: none;
}

.tab:focus-visible {
    outline: none;
    background: var(--glass-soft);
    box-shadow: 0 0 0 2px var(--acc-mint) inset;
}

.tab.active {
    color: var(--ink);
    font-weight: 600;
    background: var(--glass-soft);
}

.tab.active::after {
    content: '';
    position: absolute;
    left: 6px;
    right: 6px;
    bottom: 1px;
    height: 2px;
    border-radius: 2px;
    background: linear-gradient(90deg, var(--acc-mint), var(--acc-lilac));
    box-shadow: 0 0 6px rgba(108, 197, 168, 0.4);
    pointer-events: none;
}

.tab.drag-over {
    background: rgba(108, 197, 168, 0.10);
}

.tab-icon {
    font-size: 13px;
    flex-shrink: 0;
    color: var(--ink-3);
    transition: color 0.18s ease;
}

.tab:hover .tab-icon {
    color: var(--ink-2);
}

.tab.active .tab-icon {
    /* el-icon 是 SVG,background-clip:text 对其无效;直接用菜单激活色 */
    color: var(--menu-active-1);
    filter: drop-shadow(0 0 3px rgba(108, 197, 168, 0.3));
}

.tab-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
}

.tab-close {
    display: grid;
    place-items: center;
    width: 16px;
    height: 16px;
    border: none;
    background: transparent;
    border-radius: 4px;
    cursor: pointer;
    color: var(--ink-3);
    font-size: 11px;
    flex-shrink: 0;
    padding: 0;
    font-family: inherit;
    transition: background 0.14s, color 0.14s, opacity 0.14s;
    opacity: 0;
}

.tab.active .tab-close {
    opacity: 0.85;
}

.tab:hover .tab-close {
    opacity: 0.9;
}

.tab-close:hover {
    background: var(--glass);
    color: var(--ink);
    opacity: 1;
}

.tab-close:focus-visible {
    outline: none;
    background: var(--glass);
    color: var(--ink);
    opacity: 1;
    box-shadow: 0 0 0 1.5px var(--acc-mint) inset;
}

.scroll-arrow {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border: none;
    background: transparent;
    border-radius: 6px;
    cursor: pointer;
    color: var(--ink-2);
    font-size: 13px;
    padding: 0;
    font-family: inherit;
    flex-shrink: 0;
    transition: background 0.16s, color 0.16s;
}

.scroll-arrow:hover {
    background: var(--glass-soft);
    color: var(--ink);
}

.scroll-arrow:focus-visible {
    outline: none;
    background: var(--glass-soft);
    box-shadow: 0 0 0 1.5px var(--acc-mint) inset;
}

.context-menu {
    position: fixed;
    z-index: 9999;
    background: var(--glass-strong);
    backdrop-filter: blur(30px) saturate(160%);
    -webkit-backdrop-filter: blur(30px) saturate(160%);
    border: 1px solid var(--glass-border);
    border-radius: var(--r-md);
    box-shadow: 0 12px 40px rgba(60, 40, 90, 0.18);
    padding: 4px;
    min-width: 160px;
}

.menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    font-size: 12px;
    color: var(--ink-2);
    cursor: pointer;
    border-radius: 6px;
    border: none;
    background: transparent;
    width: 100%;
    text-align: left;
    font-family: inherit;
    transition: background 0.16s, color 0.16s;
}

.menu-item:hover {
    background: var(--glass-soft);
    color: var(--ink);
}

.menu-item:focus-visible {
    outline: none;
    background: var(--glass-soft);
    color: var(--ink);
    box-shadow: 0 0 0 1.5px var(--acc-mint) inset;
}

.menu-item .el-icon {
    font-size: 14px;
}

.menu-divider {
    height: 1px;
    background: var(--glass-border);
    margin: 4px 0;
}

/* ── 移动端 ── */
.tabs-mobile {
    padding: 8px 4px;
}

.tab-select {
    width: 100%;
}

.option-label {
    display: flex;
    align-items: center;
    gap: 8px;
}

.option-icon {
    font-size: 14px;
}

/* ── 响应式 ── */
@media (max-width: 720px) {
    .tab {
        max-width: 160px;
    }
}
</style>
