import { http } from './http'

/** GET /dashboard/overview —— 工作台概览。
 *
 * scope=admin 时返回系统计数、登录趋势与最近操作；scope=personal 时只返回
 * 会话级信息，前端据此隐藏系统概览卡，而不是把 0 当成真实数据展示。
 */
export interface DashboardCounts {
  users: number
  roles: number
  departments: number
  menus: number
  permissions: number
  tenants?: number
}

export interface DashboardTrendPoint {
  date: string
  count: number
}

export interface DashboardLogins {
  today: number
  yesterday: number
  trend: DashboardTrendPoint[]
}

export interface DashboardAuditItem {
  id: number
  uid: string
  action: 'create' | 'update' | 'delete' | string
  module: string
  table: string
  created_at: number
}

export interface DashboardAudits {
  today: number
  recent: DashboardAuditItem[]
}

export interface DashboardOverview {
  scope: 'admin' | 'personal'
  counts?: DashboardCounts
  logins?: DashboardLogins
  audits?: DashboardAudits
  generated_at: number
}

export function fetchDashboardOverview(): Promise<DashboardOverview> {
  return http.get<DashboardOverview>('/dashboard/overview')
}
