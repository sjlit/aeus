# AEUS Web

Vue 3 + Element Plus 管理后台骨架。侧边栏与业务路由完全由 admin 的
`GET /user/menus` 驱动 —— **菜单 uri 即客户端路由,view_path 决定视图组件**。

> ↩ 父模块文档：[`admin/README.md`](../README.md)。本文件只覆盖前端日常开发命令与内部约定；**前后端契约清单**（侧栏/CRUD 页面/角色授权/租户切换各依赖哪些后端端点）写在主 README 的 [前端管理台（admin/web）](../README.md#前端管理台adminweb) 章节里——后端改 RBAC 端点或 SchemaViewer 模板前先去那里同步确认。

## 启动

```bash
# 1. 后端(mock, 固定 :8080, 种子账号 admin / Admin123)
#    注意:mock 位于 admin/cmd/mock,从 admin/web 进入是 ../cmd/mock
cd ../cmd/mock && go run .

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

- 信封:所有响应 `{code, message, data}`;业务响应 HTTP 200,中间件错误(401/403/404/429 等)使用标准状态码,前端同时按 `code` 和 401 处理。
- 认证失败码 `4001/4002/4006` 触发静默刷新;传输层 HTTP 403 + `4005` 也按会话失效处理;HTTP 200 的业务 `4005`(如旧密码错)只 toast。
- `refresh()` 不覆盖 profile;后端 refresh token **会轮换**,前端把 access/refresh 都持久化到 localStorage(`aeus.access` / `aeus.refresh`),刷新页面后仍可静默续期。对 XSS 更敏感的场景应改为 HttpOnly refresh cookie。
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

> 完整列表与契约细节见主 README 的 [前端管理台（admin/web）](../README.md#前端管理台adminweb)。改动这五类端点的请求 / 响应形状前，先看一眼主 README 的 [接口权限校验（RBAC）](../README.md#接口权限校验rbac) 章节——自助路由（profile / menu / password 等）由 checker 的内置公开清单放行；新增的管理端点默认 fail-closed，必须进入 `sys_permissions` 目录并由角色授权。`WithCheckerAllowlist` 只是显式 bypass，不是“登记后更严格”。