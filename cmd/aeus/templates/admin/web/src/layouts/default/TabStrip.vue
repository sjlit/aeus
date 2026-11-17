<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ArrowLeft, ArrowRight, Back, CircleClose, Close, Refresh, Right } from '@element-plus/icons-vue'
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
const contextMenuRef = ref<HTMLElement | null>(null)
const contextMenuPosition = ref({ x: 0, y: 0 })
let closeContextMenuOnOutsideClick: (() => void) | null = null
// 键盘打开时记录锚点,关闭后把焦点还回去
let contextMenuAnchor: HTMLElement | null = null
let contextMenuRestoreFocus = false

// 读屏播报:标签切换/关闭等操作的结果
const announcement = ref('')
let announceSeq = 0
function announce(message: string) {
  const seq = ++announceSeq
  announcement.value = ''
  nextTick(() => {
    // 连续操作时只应用最后一条,避免排队的旧回调覆盖新消息、造成重复播报
    if (seq === announceSeq) announcement.value = message
  })
}

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
        case 'Delete':
        case 'Backspace': {
            e.preventDefault()
            const current = list[currentIdx]
            if (!current?.closable) return
            closeTab(current)
            nextTick(() => focusActiveTab())
            return
        }
        case 'F10':
            // 只有 Shift+F10 才接管;普通 F10 直接放行,不拦截浏览器默认行为
            if (e.shiftKey) {
                e.preventDefault()
                const current = list[currentIdx]
                if (current) openContextMenuForKeyboard(current, target)
            }
            return
        case 'ContextMenu':
            e.preventDefault()
            {
                const current = list[currentIdx]
                if (current) openContextMenuForKeyboard(current, target)
            }
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
    announce(`已切换到 ${tab.title}`)
}

function closeTab(tab: Tab) {
    if (!tab.closable) return
    tabsStore.removeTab(tab.path)
    announce(`已关闭 ${tab.title}`)
    if (route.path === tab.path) {
        const newActive = tabsStore.tabs.find(t => t.path === tabsStore.activeTab)
        if (newActive) {
            router.push({ path: newActive.path, query: newActive.query })
        }
    }
}

function tabNodeAt(index: number): HTMLElement | undefined {
    return tabsContainer.value?.querySelectorAll<HTMLElement>('.tab')[index]
}

function focusActiveTab() {
    const idx = tabsStore.tabs.findIndex(t => t.path === tabsStore.activeTab)
    if (idx >= 0) tabNodeAt(idx)?.focus()
}

/** 菜单 / 命令面板跳转后,让激活标签回到标签栏可视区。 */
function scrollActiveIntoView() {
    const idx = tabsStore.tabs.findIndex(t => t.path === tabsStore.activeTab)
    if (idx < 0) return
    const node = tabNodeAt(idx)
    const container = tabsContainer.value
    if (!node || !container) return
    // offsetLeft 的参考系可能是 topbar(定位祖先),用 rect 转成容器内坐标。
    const nodeRect = node.getBoundingClientRect()
    const containerRect = container.getBoundingClientRect()
    const left = nodeRect.left - containerRect.left + container.scrollLeft
    const right = left + nodeRect.width
    if (left < container.scrollLeft) {
        container.scrollTo({ left: Math.max(0, left - 8), behavior: 'smooth' })
    } else if (right > container.scrollLeft + container.clientWidth) {
        container.scrollTo({ left: right - container.clientWidth + 8, behavior: 'smooth' })
    }
    nextTick(checkScroll)
}

function closeContextMenu() {
    contextMenuVisible.value = false
    contextMenuTab.value = null
    if (closeContextMenuOnOutsideClick) {
        document.removeEventListener('click', closeContextMenuOnOutsideClick)
        closeContextMenuOnOutsideClick = null
    }
    // 键盘打开的菜单:关闭后把焦点还给原来的标签。
    // 必须 nextTick 后再判断 —— "关闭当前/全部"会移除锚点标签,而 DOM 更新是异步的:
    // 此刻 isConnected 可能仍为 true,直接 focus 会落在一个即将消失的元素上。
    // 锚点不在了就回退到当前激活标签(此时 store 已更新)。
    const anchor = contextMenuAnchor
    if (contextMenuRestoreFocus) {
        nextTick(() => {
            if (anchor?.isConnected) anchor.focus()
            else focusActiveTab()
        })
    }
    contextMenuRestoreFocus = false
    contextMenuAnchor = null
}

/** 在指定坐标打开菜单;键盘触发时记录锚点以便关闭后归还焦点。 */
function openContextMenuAt(x: number, y: number, tab: Tab, anchor?: HTMLElement) {
    contextMenuTab.value = tab
    // 估算菜单尺寸,靠近视口右下角时向内收,避免右键菜单被裁掉。
    const menuWidth = 184
    const menuHeight = 260
    contextMenuPosition.value = {
        x: Math.max(8, Math.min(x, window.innerWidth - menuWidth - 8)),
        y: Math.max(8, Math.min(y, window.innerHeight - menuHeight - 8)),
    }
    contextMenuAnchor = anchor ?? null
    contextMenuRestoreFocus = Boolean(anchor)
    contextMenuVisible.value = true
    nextTick(() => {
        // 键盘用户从第一个菜单项开始;Esc 可关闭。
        contextMenuRef.value?.querySelector<HTMLElement>('.menu-item')?.focus()
        closeContextMenuOnOutsideClick = closeContextMenu
        document.addEventListener('click', closeContextMenuOnOutsideClick)
    })
}

function openContextMenu(e: MouseEvent, tab: Tab) {
    e.preventDefault()
    openContextMenuAt(e.clientX, e.clientY, tab)
}

/** Shift+F10 / ContextMenu 键:以标签左下角为锚点打开同一套菜单。 */
function openContextMenuForKeyboard(tab: Tab, anchor: HTMLElement) {
    const rect = anchor.getBoundingClientRect()
    openContextMenuAt(rect.left, rect.bottom, tab, anchor)
}

/** 菜单内 ↑↓ 循环移动焦点;Esc 由模板上的处理器关闭。 */
function onContextMenuKeydown(e: KeyboardEvent) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    const items = Array.from(contextMenuRef.value?.querySelectorAll<HTMLElement>('.menu-item') ?? [])
    if (!items.length) return
    const current = items.indexOf(document.activeElement as HTMLElement)
    const next = e.key === 'ArrowDown'
        ? (current + 1) % items.length
        : (current - 1 + items.length) % items.length
    items[next]?.focus()
}

function onContextMenuAction(action: string) {
    const tab = contextMenuTab.value
    if (!tab) return
    switch (action) {
        case 'refresh':
            tabsStore.refreshTab(tab.path)
            announce(`已刷新 ${tab.title}`)
            break
        case 'close':
            closeTab(tab)
            break
        case 'closeOthers':
            tabsStore.removeOtherTabs(tab.path)
            if (route.path !== tab.path) {
                router.push({ path: tab.path, query: tab.query })
            }
            announce('已关闭其他标签页')
            break
        case 'closeLeft':
            tabsStore.removeLeftTabs(tab.path)
            announce('已关闭左侧标签页')
            break
        case 'closeRight':
            tabsStore.removeRightTabs(tab.path)
            announce('已关闭右侧标签页')
            break
        case 'closeAll':
            tabsStore.removeAllTabs()
            announce('已关闭全部标签页')
            const homeTab = tabsStore.tabs[0]
            if (homeTab) {
                router.push({ path: homeTab.path, query: homeTab.query })
            }
            break
    }
    closeContextMenu()
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
    nextTick(scrollActiveIntoView)
    window.addEventListener('resize', checkScroll)
})
onBeforeUnmount(() => {
    window.removeEventListener('resize', checkScroll)
    if (closeContextMenuOnOutsideClick) {
        document.removeEventListener('click', closeContextMenuOnOutsideClick)
        closeContextMenuOnOutsideClick = null
    }
})
watch(
    () => [tabsStore.tabs.length, tabsStore.activeTab],
    () => nextTick(() => {
        checkScroll()
        scrollActiveIntoView()
    }),
)
</script>

<template>
    <span class="sr-only" role="status" aria-live="polite">{{ announcement }}</span>
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
            <div v-if="contextMenuVisible" ref="contextMenuRef" class="context-menu" role="menu" :style="{
                left: contextMenuPosition.x + 'px',
                top: contextMenuPosition.y + 'px',
            }" @keydown.esc.stop.prevent="closeContextMenu" @keydown="onContextMenuKeydown">
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
/* 仅供读屏软件播报,视觉上隐藏但保留在可访问性树中 */
.sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
}

.tabs-row {
    display: flex;
    align-items: center;
    height: 48px;
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
    padding: 7px 12px;
    border-radius: 8px;
    cursor: pointer;
    color: var(--ink-2);
    font-size: var(--fs-sm);
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
    font-size: var(--fs-sm);
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
    width: 18px;
    height: 18px;
    border: none;
    background: transparent;
    border-radius: 4px;
    cursor: pointer;
    color: var(--ink-3);
    font-size: var(--fs-xs);
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
    font-size: var(--fs-sm);
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
    backdrop-filter: blur(16px) saturate(160%);
    -webkit-backdrop-filter: blur(16px) saturate(160%);
    border: 1px solid var(--glass-border);
    border-radius: var(--r-md);
    box-shadow: var(--overlay-shadow);
    padding: 4px;
    min-width: 160px;
}

.menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    font-size: var(--fs-xs);
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
    font-size: var(--fs-base);
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

    /* 移动端下拉:与放大后的顶栏(60px)匹配,EP 默认 32px 太矮 */
    --el-select-input-height: 40px;
    --el-select-input-font-size: var(--fs-base);
}

.option-label {
    display: flex;
    align-items: center;
    gap: 8px;
}

.option-icon {
    font-size: var(--fs-base);
}

/* ── 响应式 ── */
@media (max-width: 720px) {
    .tab {
        max-width: 160px;
    }
}
</style>
