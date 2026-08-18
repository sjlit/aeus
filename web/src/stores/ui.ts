/**
 * ui store · 界面状态(侧栏折叠)
 * 持久化到 localStorage,刷新后保持;与 auth store 同为 Options API 风格。
 */
import { defineStore } from 'pinia'

const LS_KEY = 'aeus.sidebar.collapsed'

export const useUiStore = defineStore('ui', {
  state: () => ({
    sidebarCollapsed: localStorage.getItem(LS_KEY) === '1',
  }),
  actions: {
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      localStorage.setItem(LS_KEY, this.sidebarCollapsed ? '1' : '0')
    },
  },
})
