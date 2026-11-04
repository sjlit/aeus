<script setup lang="ts">
/**
 * CommandPalette · ⌘K / Ctrl+K 全局菜单搜索
 * 数据来自 menu 的扁平索引(flatIndex) + 节归属(sectionsByUri),
 * 与侧栏 / 路由注册共用一次遍历结果,此处不再 DFS;
 * 支持按名称 / 路径 / 分组过滤,上下键选择,Enter 直达路由。
 * 热键监听挂在 window:组件由 Layout 挂载一次,登录后的任何页面都可用。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search } from '@element-plus/icons-vue'
import { useMenuStore } from '@/stores/menu'
import { useUiStore } from '@/stores/ui'
import { resolveIcon } from '@/utils/icons'

interface PaletteItem {
    section: string
    name: string
    uri: string
    icon: string
}

const menu = useMenuStore()
const ui = useUiStore()
const router = useRouter()
const route = useRoute()

const query = ref('')
const activeIdx = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
const listRef = ref<HTMLElement | null>(null)

/** 全部可路由节点的扁平视图,从 menu 的预计算索引直接读,不重复 DFS。 */
const items = computed<PaletteItem[]>(() => {
    const f = menu.flatIndex
    const secByUri = menu.sectionsByUri
    return f.uris.map((uri) => ({
        section: secByUri.get(uri) ?? '',
        name: f.titlesByUri.get(uri) ?? '',
        uri,
        icon: f.iconsByUri.get(uri) ?? '',
    }))
})

const results = computed<PaletteItem[]>(() => {
    const q = query.value.trim().toLowerCase()
    if (!q) return items.value
    return items.value.filter(
        (it) =>
            it.name.toLowerCase().includes(q) ||
            it.uri.toLowerCase().includes(q) ||
            it.section.toLowerCase().includes(q),
    )
})

/** 按节分组渲染;entries 里的 idx 是全局 results 下标,供键盘导航用。 */
const groups = computed(() => {
    const out: { section: string; entries: { item: PaletteItem; idx: number }[] }[] = []
    results.value.forEach((item, idx) => {
        const last = out[out.length - 1]
        if (last && last.section === item.section) {
            last.entries.push({ item, idx })
        } else {
            out.push({ section: item.section, entries: [{ item, idx }] })
        }
    })
    return out
})

watch(results, () => {
    activeIdx.value = 0
})

watch(
    () => ui.paletteOpen,
    (open) => {
        if (!open) return
        query.value = ''
        activeIdx.value = 0
        nextTick(() => inputRef.value?.focus())
    },
)

function select(item: PaletteItem) {
    ui.closePalette()
    if (route.path !== item.uri) void router.push(item.uri)
}

function moveActive(delta: number) {
    const len = results.value.length
    if (!len) return
    activeIdx.value = (activeIdx.value + delta + len) % len
    nextTick(() => {
        listRef.value
            ?.querySelector<HTMLElement>(`[data-idx="${activeIdx.value}"]`)
            ?.scrollIntoView({ block: 'nearest' })
    })
}

function onInputKeydown(e: KeyboardEvent) {
    switch (e.key) {
        case 'ArrowDown':
            e.preventDefault()
            moveActive(1)
            break
        case 'ArrowUp':
            e.preventDefault()
            moveActive(-1)
            break
        case 'Enter': {
            e.preventDefault()
            const item = results.value[activeIdx.value]
            if (item) select(item)
            break
        }
    }
}

/** ⌘K / Ctrl+K 开关;Esc 关闭。Ctrl+Shift+K 是浏览器控制台快捷键,不接管。 */
function onGlobalKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        ui.togglePalette()
        return
    }
    if (e.key === 'Escape' && ui.paletteOpen) ui.closePalette()
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
    <teleport to="body">
        <div v-if="ui.paletteOpen" class="palette-overlay" role="dialog" aria-modal="true" aria-label="搜索菜单"
            @click.self="ui.closePalette()">
            <div class="palette">
                <div class="palette-head">
                    <el-icon class="p-icon">
                        <Search />
                    </el-icon>
                    <input ref="inputRef" v-model="query" class="palette-input" type="text" placeholder="搜索菜单…"
                        aria-label="搜索菜单" @keydown="onInputKeydown" />
                    <kbd class="p-kbd">Esc</kbd>
                </div>

                <div ref="listRef" class="palette-list" role="listbox" aria-label="菜单搜索结果">
                    <template v-if="results.length">
                        <template v-for="g in groups" :key="g.section || '·'">
                            <div v-if="g.section" class="palette-group">{{ g.section }}</div>
                            <div v-for="{ item, idx } in g.entries" :key="item.uri" class="palette-item"
                                :class="{ 'is-active': idx === activeIdx }" :data-idx="idx" role="option"
                                :aria-selected="idx === activeIdx" @click="select(item)" @mouseenter="activeIdx = idx">
                                <el-icon v-if="resolveIcon(item.icon)" class="p-item-icon">
                                    <component :is="resolveIcon(item.icon)" />
                                </el-icon>
                                <span class="p-item-name">{{ item.name }}</span>
                                <span class="p-item-uri">{{ item.uri }}</span>
                            </div>
                        </template>
                    </template>
                    <div v-else class="palette-empty">没有匹配「{{ query }}」的菜单项</div>
                </div>

                <div class="palette-foot">
                    <span><kbd>↑</kbd><kbd>↓</kbd> 选择</span>
                    <span><kbd>Enter</kbd> 打开</span>
                    <span><kbd>Esc</kbd> 关闭</span>
                </div>
            </div>
        </div>
    </teleport>
</template>

<style scoped>
.palette-overlay {
    position: fixed;
    inset: 0;
    z-index: 2100;
    background: rgba(30, 60, 60, 0.28);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    display: grid;
    justify-items: center;
    align-items: start;
    padding: 14vh 16px 16px;
    animation: overlay-in 0.16s ease;
}

.palette {
    width: min(600px, 100%);
    background: var(--glass-strong);
    backdrop-filter: blur(16px) saturate(160%);
    -webkit-backdrop-filter: blur(16px) saturate(160%);
    border: 1px solid var(--glass-border);
    border-radius: var(--r-lg);
    box-shadow: 0 24px 64px rgba(30, 60, 60, 0.28);
    overflow: hidden;
    animation: palette-in 0.16s ease;
}

.palette-head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 14px 16px;
    border-bottom: 1px solid var(--glass-border);
}

.p-icon {
    font-size: 16px;
    color: var(--ink-3);
    flex-shrink: 0;
}

.palette-input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    font-family: inherit;
    font-size: 14px;
    color: var(--ink);
}

.palette-input::placeholder {
    color: var(--ink-3);
}

.p-kbd,
.palette-foot kbd {
    font-family: var(--mono);
    font-size: 10px;
    color: var(--ink-3);
    background: var(--glass-soft);
    border: 1px solid var(--glass-border);
    border-radius: 5px;
    padding: 2px 6px;
    flex-shrink: 0;
}

.palette-list {
    max-height: 46vh;
    overflow-y: auto;
    padding: 6px;
}

.palette-group {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.05em;
    color: var(--ink-3);
    padding: 8px 10px 4px;
}

.palette-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 10px;
    border-radius: 10px;
    cursor: pointer;
    font-size: 13px;
    color: var(--ink-2);
    transition: background 0.14s, color 0.14s;
}

.palette-item.is-active {
    background: var(--glass-soft);
    color: var(--ink);
    font-weight: 600;
}

.p-item-icon {
    font-size: 14px;
    color: var(--ink-3);
    flex-shrink: 0;
}

.palette-item.is-active .p-item-icon {
    color: var(--menu-active-1);
}

.p-item-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.p-item-uri {
    margin-left: auto;
    font-family: var(--mono);
    font-size: 11px;
    color: var(--ink-3);
    opacity: 0.7;
    white-space: nowrap;
}

.palette-empty {
    padding: 28px 16px;
    text-align: center;
    font-size: 13px;
    color: var(--ink-3);
}

.palette-foot {
    display: flex;
    gap: 16px;
    padding: 10px 16px;
    border-top: 1px dashed rgba(30, 90, 90, 0.15);
    font-size: 11px;
    color: var(--ink-3);
}

.palette-foot span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
}

@keyframes overlay-in {
    from {
        opacity: 0;
    }

    to {
        opacity: 1;
    }
}

@keyframes palette-in {
    from {
        opacity: 0;
        transform: translateY(-6px) scale(0.985);
    }

    to {
        opacity: 1;
        transform: none;
    }
}

@media (max-width: 640px) {
    .p-item-uri {
        display: none;
    }
}
</style>
