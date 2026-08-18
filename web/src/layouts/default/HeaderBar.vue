<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Expand, Fold, Search } from '@element-plus/icons-vue'
import { useUiStore } from '../../stores/ui'
import UserMenu from './UserMenu.vue'

const ui = useUiStore()
const route = useRoute()

interface Crumb {
    label: string
    path: string
}

const crumbs = computed<Crumb[]>(() =>
    route.matched
        .filter((r) => r.meta?.title)
        .map((r) => ({ label: r.meta.title as string, path: r.path })),
)
</script>

<template>
    <div class="toolbar">
        <div class="cluster left">
            <button class="icon-btn" type="button" :aria-label="ui.sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
                :title="ui.sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'" @click="ui.toggleSidebar()">
                <el-icon>
                    <component :is="ui.sidebarCollapsed ? Expand : Fold" />
                </el-icon>
            </button>
            <nav v-if="crumbs.length" class="crumbs" aria-label="Breadcrumb">
                <template v-for="(c, i) in crumbs" :key="c.path">
                    <router-link v-if="i < crumbs.length - 1" :to="c.path" class="crumb crumb-link">
                        {{ c.label }}
                    </router-link>
                    <span v-else class="crumb crumb-current">
                        {{ c.label }}
                    </span>
                    <span v-if="i < crumbs.length - 1" class="crumb-sep">/</span>
                </template>
            </nav>
        </div>

        <div class="searchbox">
            <el-icon class="s-icon">
                <Search />
            </el-icon>
            <input class="s-input" type="text" placeholder="搜索…" aria-label="搜索" />
        </div>

        <div class="cluster right">
            <UserMenu />
        </div>
    </div>
</template>

<style scoped>
.toolbar {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 16px;
    height: 52px;
    padding: 0 14px;
}

.cluster {
    display: flex;
    align-items: center;
    min-width: 0;
}

.left {
    gap: 10px;
}

.right {
    gap: 8px;
}

/* ── icon-btn(折叠) ── */
.icon-btn {
    width: 32px;
    height: 32px;
    flex-shrink: 0;
    border: none;
    background: var(--glass-soft);
    border-radius: 8px;
    display: grid;
    place-items: center;
    cursor: pointer;
    color: var(--ink-2);
    font-size: 15px;
    padding: 0;
    font-family: inherit;
    transition: background 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

.icon-btn:hover {
    background: var(--acc-mint);
    color: white;
    transform: scale(1.05);
}

.icon-btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--acc-mint) inset;
}

/* ── 面包屑 ── */
.crumbs {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    font-family: var(--mono);
    font-size: 12px;
    letter-spacing: 0.02em;
}

.crumb {
    flex-shrink: 0;
    transition: color 0.14s;
}

.crumb-link {
    color: var(--ink-2);
    text-decoration: none;
}

.crumb-link:hover {
    color: var(--acc-mint);
}

.crumb-current {
    color: var(--ink);
    font-weight: 600;
}

.crumb-sep {
    color: var(--ink-3);
    user-select: none;
}

/* ── 搜索框 ── */
.searchbox {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 7px 10px 7px 14px;
    background: var(--glass-soft);
    border: 1px solid var(--glass-border);
    border-radius: var(--r-pill);
    min-width: 0;
    max-width: 460px;
    width: 100%;
    justify-self: center;
    color: var(--ink-2);
    font-family: inherit;
    transition: background 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.searchbox:focus-within {
    background: white;
    border-color: var(--acc-mint);
    box-shadow: var(--ring);
}

.searchbox:hover {
    background: white;
    border-color: var(--acc-mint);
    box-shadow: var(--ring-soft);
    transform: translateY(-1px);
}

.s-icon {
    font-size: 14px;
    color: var(--ink-3);
    flex-shrink: 0;
}

.s-input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    font-family: inherit;
    font-size: 12px;
    color: var(--ink);
    letter-spacing: 0.02em;
}

.s-input::placeholder {
    color: var(--ink-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

/* ── 响应式 ── */
@media (max-width: 980px) {
    .toolbar {
        gap: 10px;
        padding: 0 10px;
    }

    /* 窄屏只留当前页,链路/分隔符藏掉(见模板注释) */
    .crumbs .crumb-link,
    .crumbs .crumb-sep {
        display: none;
    }
}

@media (max-width: 720px) {
    .searchbox {
        padding: 8px;
        min-width: 38px;
        justify-content: center;
    }

    .s-input {
        display: none;
    }
}
</style>
