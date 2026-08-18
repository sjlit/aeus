/**
 * Element Plus 图标解析(侧边栏菜单 / 多标签共用)
 * 后端 menu.icon 是短串(user / sys-role / Monitor);组件名是 PascalCase。
 * 直接命中优先,再做 kebab/snake → PascalCase 的归一;都没有就 null,
 * 模板里再决定要不要画图标。Map 缓存避免每个节点每次渲染都重做字符串归一。
 */
import type { Component } from 'vue'
import * as ElIcons from '@element-plus/icons-vue'

const ICONS = ElIcons as Record<string, Component>

function toPascal(name: string): string {
  return name
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join('')
}

const iconCache = new Map<string, Component | null>()

export function resolveIcon(name: string | undefined): Component | null {
  if (!name) return null
  let icon = iconCache.get(name)
  if (icon === undefined) {
    icon = ICONS[name] ?? ICONS[toPascal(name)] ?? null
    iconCache.set(name, icon)
  }
  return icon
}
