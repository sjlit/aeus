/**
 * Element Plus 图标解析(侧边栏菜单 / 多标签共用)
 * 后端 menu.icon 是短串(user / sys-role / Monitor);组件名是 PascalCase。
 * 直接命中优先,再做 kebab/snake → PascalCase 的归一;都没有就 null,
 * 模板里再决定要不要画图标。Map 缓存避免每个节点每次渲染都重做字符串归一。
 *
 * 白名单 + 兜底:
 *  - 这里显式具名 import 的图标会被 rollup tree-shake 保留,
 *    不在白名单里的图标不会被打进 main bundle。
 *  - IconPicker(系统菜单编辑页)需要全集,它自己用 namespace import
 *    放在 sys_menu 路由的懒加载 chunk 里,不污染主 bundle。
 *  - 后端种子数据用了 UserFilled / Tickets / Tools(见 admin/seed.go),
 *    加上模板里直接用到的图标 + 一些常用图标组成这份白名单。
 */
import type { Component } from 'vue'
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  Avatar,
  Back,
  Bell,
  BellFilled,
  Calendar,
  ChatLineRound,
  Check,
  CircleCheck,
  CircleCheckFilled,
  CircleClose,
  CircleCloseFilled,
  Close,
  Compass,
  Connection,
  DataAnalysis,
  DataBoard,
  DataLine,
  Delete,
  Document,
  DocumentCopy,
  Download,
  Edit,
  Expand,
  Filter,
  Folder,
  FolderAdd,
  FolderChecked,
  FolderRemove,
  FullScreen,
  Histogram,
  HomeFilled,
  InfoFilled,
  Key,
  List,
  Lock,
  Menu,
  Message,
  Monitor,
  Moon,
  MoreFilled,
  Notification,
  OfficeBuilding,
  PieChart,
  Plus,
  Postcard,
  Promotion,
  QuestionFilled,
  Refresh,
  Right,
  Search,
  Setting,
  Share,
  Star,
  Sunny,
  SwitchButton,
  Tickets,
  Tools,
  TrendCharts,
  Trophy,
  Upload,
  User,
  UserFilled,
  View,
  WarningFilled,
  WarnTriangleFilled,
  ZoomIn,
  ZoomOut,
} from '@element-plus/icons-vue'

/** raw / kebab / snake → PascalCase 归一。导出供其他需要相同规则的组件复用
 *  (如 IconPicker),保证改一处全网同步。 */
export function toPascal(name: string): string {
  return name
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join('')
}

/** 白名单图标表 —— 只包含这里具名 import 的图标。
 *  未列出的图标返回 null,模板里通过 v-if 跳过渲染。 */
const ICONS: Record<string, Component> = {
  ArrowDown, ArrowLeft, ArrowRight, Avatar, Back, Bell, BellFilled,
  Calendar, ChatLineRound, Check, CircleCheck, CircleCheckFilled,
  CircleClose, CircleCloseFilled, Close, Compass, Connection,
  DataAnalysis, DataBoard, DataLine, Delete, Document, DocumentCopy,
  Download, Edit, Expand, Filter, Folder, FolderAdd, FolderChecked,
  FolderRemove, FullScreen, Histogram, HomeFilled, InfoFilled, Key,
  List, Lock, Menu, Message, Monitor, Moon, MoreFilled, Notification,
  OfficeBuilding, PieChart, Plus, Postcard, Promotion, QuestionFilled,
  Refresh, Right, Search, Setting, Share, Star, Sunny, SwitchButton,
  Tickets, Tools, TrendCharts, Trophy, Upload, User, UserFilled,
  View, WarningFilled, WarnTriangleFilled, ZoomIn, ZoomOut,
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
