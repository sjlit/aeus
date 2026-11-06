/**
 * workspace store · 工作台的本地个性化数据。
 *
 * 两部分都只保存在 localStorage:
 * - visits:真实访问历史(按最近访问排序,不是标签页顺序)
 * - favorites:用户手动固定的常用入口
 *
 * 这里不是业务数据源,所以不需要进后端;访问历史被严格限制在 MAX_VISITS 条,
 * favorites 最多 MAX_FAVORITES 条,避免 localStorage 无限增长。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { safeGet, safeRemove, safeSet } from '../utils/storage'

const VISITS_KEY = 'aeus.app.visits'
const FAVORITES_KEY = 'aeus.app.favorites'
const MAX_VISITS = 12
const MAX_FAVORITES = 8

export interface WorkspaceLink {
  path: string
  title: string
  icon: string
  section: string
  query?: Record<string, string>
}

export interface WorkspaceVisit extends WorkspaceLink {
  visitedAt: number
}

function sanitizeLink(value: unknown): WorkspaceLink | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Record<string, unknown>
  if (typeof item.path !== 'string' || !item.path) return null
  if (typeof item.title !== 'string' || !item.title) return null
  const query =
    item.query && typeof item.query === 'object'
      ? Object.fromEntries(
          Object.entries(item.query as Record<string, unknown>)
            .filter(([, v]) => typeof v === 'string')
            .map(([k, v]) => [k, v as string]),
        )
      : undefined
  return {
    path: item.path,
    title: item.title,
    icon: typeof item.icon === 'string' ? item.icon : '',
    section: typeof item.section === 'string' ? item.section : '',
    ...(query && Object.keys(query).length > 0 ? { query } : {}),
  }
}

function sanitizeLinks(raw: unknown): WorkspaceLink[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const out: WorkspaceLink[] = []
  for (const value of raw) {
    const link = sanitizeLink(value)
    if (!link || seen.has(link.path)) continue
    seen.add(link.path)
    out.push(link)
    if (out.length >= MAX_FAVORITES) break
  }
  return out
}

function sanitizeVisits(raw: unknown): WorkspaceVisit[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const out: WorkspaceVisit[] = []
  for (const value of raw) {
    const link = sanitizeLink(value)
    if (!link || seen.has(link.path)) continue
    const visitedAt = (value as Record<string, unknown>).visitedAt
    if (typeof visitedAt !== 'number' || !Number.isFinite(visitedAt)) continue
    seen.add(link.path)
    out.push({ ...link, visitedAt })
    if (out.length >= MAX_VISITS) break
  }
  return out.sort((a, b) => b.visitedAt - a.visitedAt)
}

export const useWorkspaceStore = defineStore('workspace', () => {
  const visits = ref<WorkspaceVisit[]>(sanitizeVisits(safeGet<unknown>(VISITS_KEY, [])))
  const favorites = ref<WorkspaceLink[]>(sanitizeLinks(safeGet<unknown>(FAVORITES_KEY, [])))

  const favoritePaths = computed(() => new Set(favorites.value.map((item) => item.path)))

  function persistVisits(): void {
    if (visits.value.length === 0) {
      safeRemove(VISITS_KEY)
      return
    }
    safeSet(VISITS_KEY, visits.value)
  }

  function persistFavorites(): void {
    if (favorites.value.length === 0) {
      safeRemove(FAVORITES_KEY)
      return
    }
    safeSet(FAVORITES_KEY, favorites.value)
  }

  /** 记录一次真实访问。同一路径会置顶,而不是产生重复项。 */
  function recordVisit(link: WorkspaceLink): void {
    if (!link.path || !link.title) return
    const next: WorkspaceVisit = {
      ...link,
      visitedAt: Date.now(),
    }
    visits.value = [
      next,
      ...visits.value.filter((item) => item.path !== link.path),
    ].slice(0, MAX_VISITS)
    persistVisits()
  }

  function clearVisits(): void {
    visits.value = []
    safeRemove(VISITS_KEY)
  }

  /** 返回 true 表示收藏状态已改变;false 表示达到上限,调用方负责提示。 */
  function toggleFavorite(link: WorkspaceLink): boolean {
    if (favoritePaths.value.has(link.path)) {
      favorites.value = favorites.value.filter((item) => item.path !== link.path)
      persistFavorites()
      return true
    }
    if (favorites.value.length >= MAX_FAVORITES) return false
    favorites.value = [...favorites.value, link]
    persistFavorites()
    return true
  }

  function isFavorite(path: string): boolean {
    return favoritePaths.value.has(path)
  }

  function clearFavorites(): void {
    favorites.value = []
    safeRemove(FAVORITES_KEY)
  }

  /** 登出/换账号时清空个性化数据,避免下一位用户看到上一位的页面标题。 */
  function clear(): void {
    clearVisits()
    clearFavorites()
  }

  return {
    visits,
    favorites,
    favoritePaths,
    recordVisit,
    clearVisits,
    toggleFavorite,
    isFavorite,
    clearFavorites,
    clear,
    maxVisits: MAX_VISITS,
    maxFavorites: MAX_FAVORITES,
  }
})
