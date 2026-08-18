/**
 * ui store · 界面状态(侧栏折叠 / 移动端抽屉)
 * 折叠状态持久化到 localStorage,刷新后保持;抽屉态只在当前会话有效。
 * 与 auth store 同为 Options API 风格。
 */
import { defineStore } from 'pinia'
import { safeGetString, safeSetString } from '../utils/storage'

const LS_KEY = 'aeus.sidebar.collapsed'

export const useUiStore = defineStore('ui', {
  state: () => ({
    sidebarCollapsed: safeGetString(LS_KEY) === '1',
    // 移动端抽屉可见性(会话级;从桌面切到移动端时由 Layout 主动关闭)
    mobileSidebarOpen: false,
  }),
  actions: {
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      safeSetString(LS_KEY, this.sidebarCollapsed ? '1' : '0')
    },
    toggleMobileSidebar() {
      this.mobileSidebarOpen = !this.mobileSidebarOpen
    },
    closeMobileSidebar() {
      this.mobileSidebarOpen = false
    },
  },
})