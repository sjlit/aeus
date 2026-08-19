# AEUS Web

Vue 3 + Element Plus 管理后台骨架。侧边栏与业务路由完全由 admin 的
`GET /user/menus` 驱动 —— **菜单 uri 即客户端路由,view_path 决定视图组件**。

> ↩ 父模块文档：[`admin/README.md`](../README.md)。本文件只覆盖前端日常开发命令与内部约定；**前后端契约清单**（侧栏/CRUD 页面/角色授权/租户切换各依赖哪些后端端点）写在主 README 的 [前端管理台（admin/web）](../README.md#前端管理台adminweb) 章节里——后端改 RBAC 端点或 SchemaViewer 模板前先去那里同步确认。

## 启动

```bash
# 1. 后端(mock, 固定 :8080, 种子账号 admin / Admin123)
cd ../../cmd/mock && go run .

# 2. 前端(dev, :5173, /api 代理到 :8080)
npm install
npm run dev
```

打开 http://localhost:5173 ，用 `admin / Admin123` 登录。

## 命令

| 命令 | 作用 |
|------|------|
| `npm run dev` | 开发服务器(:5173) |
| `npm run typecheck` | vue-tsc 类型检查 |
| `npm test` | vitest 单测(信封/菜单分组纯函数、view_path 规范化、auth store) |
| `npm run build` | 类型检查 + 构建到 dist/ |

## 约定

- 信封:所有响应 `{code, message, data}`,HTTP 恒 200;只看 `code`。
- 认证失败码 `4001/4002/4006` 触发静默刷新;`4003/4005` 只 toast。
- `refresh()` 不覆盖 profile;refresh token 不轮换。
- 路由:Hash 模式;业务路由从 MenuTree 动态注册,uri 即路径,view_path 决定视图文件。
- 视觉:`src/styles/style.scss` 提供共享 token + 玻璃拟态工具类;
  `src/styles/app.scss` 桥接 Element Plus 主题并承载管理台布局。`app.scss` 顶部用 `@use './style'` 引入共享样式。

## 关键端点速查

前端实际只依赖后端**五类**端点（按使用频率）：

| 用途 | 端点 |
|------|------|
| 登录 / 刷新 / 登出 | `/auth/login` · `/auth/refresh-token` · `/auth/logout` |
| 当前用户自助 | `/user/profile` · `/user/change-password` · `/user/menus` · `/user/permissions` |
| CRUD 渲染（SchemaViewer 自动驱动） | `/schema/:module/:table` · `/rest/model-types/:module/:table` · `/rest/model-tiers/:module/:table` |
| 角色授权编辑 | `/role/options` · `/role/permissions` · `/role/menus` · `/permission/catalog` · `/menu/tree` |
| 租户切换（多租户模式下） | `/tenant/options` · `/tenant/detail` |

> 完整列表与契约细节见主 README 的 [前端管理台（admin/web）](../README.md#前端管理台adminweb)。改动这五类端点的请求 / 响应形状前，先看一眼主 README 的 [接口权限校验（RBAC）](../README.md#接口权限校验rbac) 章节——`admin/web` 不在 PermissionChecker 目录里，**走 `WithCheckerAllowlist` 显式放行**；若新增端点忘了登记 allowlist，会被 fail-closed 默认拒绝。