# 1. 概述

`admin` 是 AEUS 框架的通用后台管理模块,基于 rest/v3 + GORM,提供即开即用的系统管理能力。本节覆盖所有接口共享的协议层约定。

## 1.1 模块形态

| 维度 | 取值 |
|---|---|
| 包名 | `github.com/sjlit/aeus/admin` |
| 数据访问 | GORM(任意 driver:SQLite/MySQL/PostgreSQL) |
| HTTP 框架 | rest/v3(协议无关,默认由 `transport/http.Server` 接入) |
| 认证 | JWT(由应用方接入 `middleware/auth.JWT`) |
| 租户隔离 | GORM 回调层(`aeus:tenant:*`) |
| 协议 | HTTP/1.1 + JSON,业务 RPC 由 proto 生成 |
| 响应外壳 | `{code, message, data}` envelope,业务码见下 |

## 1.2 响应外壳

`admin/responder.go` 提供默认 responder,所有路由(REST 资源 + 业务 RPC)统一输出:

```json
// 成功(code = 0)
{ "code": 0, "message": "", "data": { ... } }

// 业务失败(message 直接给前端 toast)
{ "code": 4003, "message": "permission denied" }
```

**关键点**:

- admin responder 处理的业务成功/失败固定 HTTP `200`;`transport/http` error interceptor 处理的中间件错误(JWT/限流/404/panic)使用 `400/401/403/404/429/500/503/504`。两条路径共用同一个 envelope,前端以 `body.code` 为主、同时兼容 401 刷新逻辑。
- `data` 字段在失败路径上为 `null`(JSON `omitempty`)。
- 错误信息通过 `errors.As(err, &e)` 沿 `Unwrap` 链找回 `*errs.Error`,保留真实业务码;普通 `error` 退化到 `1001 Invalid`。
- 可通过 `admin.WithResponder(...)` 整体替换为应用自定义 responder。

## 1.3 业务码

业务码定义于 `pkg/errs/const.go`。前端按 `body.code` 分流:

| Code | 常量 | HTTP Status | 语义 | 前端处理 |
|---|---|---|---|---|
| `0` | `CodeOK` | 200 | 成功 | 成功 |
| `1001` | `CodeInvalid` | 400 | 参数/业务校验失败 | toast 后端 `message` |
| `4002` | `CodeTokenExpired` | 401 | Token 过期 | refresh + 重试原请求 |
| `4003` | `CodePermissionDenied` | 403 | 无权限(角色未授权该接口) | toast 无权限 |
| `4004` | `CodeNotFound` | 404 | 资源不存在 | 业务提示 |
| `4005` | `CodeAccessDenied` | 403 | 传输层未登录/凭证无效；业务层也复用该码（旧密码错等，HTTP 200） | HTTP 403 跳登录页；HTTP 200 只 toast |
| `4010` | `CodeTooManyAttempts` | 429 | 登录/操作过于频繁(见 [`auth.md` §3.7](./auth.md)) | 读取 `Retry-After` 退避后重试 |
| `1003` | `CodeUnavailable` | 503 | 服务不可用(无 DB 等) | 业务提示 |

> **HTTP 列的含义**:admin responder 处理业务错误时仍写 HTTP `200`;上表的 HTTP Status 是同一业务码经 `transport/http` error interceptor / `errs.Error.HTTPStatus()` 处理时的标准映射(JWT、限流、404、panic 等)。前端以 `body.code` 为主、401 触发 refresh。
>
> **变更提醒**:`pkg/errs/error.go` 的 `TokenExpired` 自 2026-08-10 起映射到 HTTP `401`(原先映射 500 是 bug)。

## 1.4 JWT Claims

`admin/auth/claims.go` 定义 `*auth.Claims`,由 `middleware/auth.JWT(...).WithClaims(Claims{})` 在 ctx 中存放。处理器内通过 `auth.ClaimsFromContext(ctx)` 读取,`auth.TenantIDFromContext(ctx)` 单独取租户 ID。

| Go 字段 | JSON | 来源 | 用途 |
|---|---|---|---|
| `UID` | `uid` | `User.UID` | 当前用户工号,供 handler 直接定位调用者 |
| `Role` | `uro` | `User.RoleKey` | 角色 Key(机器标识,非显示名) |
| `TenantID` | `tid` | `User.TenantID` | 租户 ID,GORM 回调据此隔离 |
| `TokenType` | `token_type` | `"access"` / `"refresh"` | `AuthService.RefreshToken` 只接受 `refresh` |
| `jwt.RegisteredClaims` | — | jwt v5 | `exp` / `nbf` / `iat` / `jti`(UUID) |

签名:`HS256`,secret 由应用方通过 `service.WithAuthSecret(...)` 注入。

## 1.5 认证与会话

### AuthService 注册路径

**`Server.Setup` 不会自动注册 `AuthService`**——它由应用自行装配并注册:

```go
pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret(os.Getenv("JWT_SECRET")),  // 必须从运行时通道取
))
```

这样生产应用保留对 `AuthSecret`、`TokenStore` 与 TTL 的完整控制权,admin 不会把任何密钥默认值烘焙进二进制。

### Token TTL

| Token | 默认 TTL | 配置 |
|---|---|---|
| access | 2h(7200 秒) | `service.WithTokenExpireSeconds(n)`(≤0 按默认) |
| refresh | 48h(172800 秒) | 固定,不可配 |

### 状态校验

`Login` 与 `RefreshToken` 在凭据校验通过后都会复查用户与角色状态:

| 条件 | Login | RefreshToken |
|---|---|---|
| 用户 `status=disabled` | `4003` (`user is disabled`) | `4003` (`user is disabled`) |
| 角色 `status=disabled` | `4003` (`role is disabled`) | `4003` (`role is disabled`) |
| 角色行不存在 | `4003` (`role not found`) | `4005` (AccessDenied) |
| 租户 `status=disabled` | `4003` (`tenant is disabled`) | 重新发 token,但不影响会话 |

## 1.6 URL 形态约定

admin 模块所有业务 RPC 遵循同一规则:

| HTTP 方法 | 参数位置 | 说明 |
|---|---|---|
| **GET** | query string | 资源定位 + 过滤参数全部走 `?key=value`,由 transport/http `ctx.Bind` 从 query 串绑定到 proto 字段 |
| **PUT** / **POST** / **PATCH** | request body(`body: "*"`) | 资源标识 + 写入字段全部在 JSON body,客户端 `Content-Type: application/json` |

**根因**:gin v1.12 只认 `:`/`*` 通配,`{role}` 路径段永不匹配;protoc-gen-go-aeus 把 `google.api.http` 注解原样抄进 gin 路由,而 `?` 在 gin 里被当作字面量注册成死路由。所以 proto 注解只写路径(如 `get: "/role/permissions"`),wire URL 中的 query 串 / body 在 README 与代码注释中显式记录。

后续新增 RPC 沿用同一约定即可。`rest/v3` 通用 CRUD 路径(`/system/sys_user/{id}` 等 gin `:id` 形式)由 rest/v3 自己生成,不归本约定。

### 命名约定(2026-08-10 起生效)

- **URL 根 = 资源域,与实现 Service 一一对应**:`/auth` → AuthService、`/user` → UserService、`/role` → RoleService、`/menu` → MenuService、`/permission` → PermissionService、`/tenant` → TenantService
- **复数 = 集合端点**;同一域下动作由 HTTP method + 子路径表达
- **"角色的东西"归 `/role/*`**(options / menus / permissions);**"自己的东西"归 `/user/*`**(profile / menus / permissions / password / avatar)
- **路径 ≥ 2 级**;下拉数据(`{value, label}` 形状)用 `options`,树形数据用 `tree`,面包屑用 `breadcrumb`;集合 / 列表读用 `List*`,单资源 / 固定结构读用 `Get*`
- **模块级命名空间由挂载点提供**:admin 设计为挂载在网关之下(如反向代理 `/admin/*` → 本服务),模块内部保持短路径;应用直接暴露时可自行在网关层加前缀,业务 RPC 路径无需改动

## 1.7 通用 CRUD 资源

资源名由**表名**派生(singular / plural),所有资源统一挂载在 `/system/` 下(`ModuleName = "system"`):

| 资源 | 表名 | 写 (create/update/delete) | 读 (detail/list/export) |
|---|---|---|---|
| `User` | `sys_users` | ✅ `/system/sys_user` | `/system/sys_users` |
| `Role` | `sys_roles` | ✅ `/system/sys_role` | `/system/sys_roles` |
| `Menu` | `sys_menus` | ✅ `/system/sys_menu` | `/system/sys_menuses` |
| `Department` | `sys_departments` | ✅ `/system/sys_department` | `/system/sys_departments` |
| `Tenant` | `sys_tenants` | ✅ `/system/sys_tenant` | `/system/sys_tenants` |
| `Permission` | `sys_permissions` | ❌ 只读（由 `Setup`/`Seed` 管理） | `/system/sys_permission` |
| `RolePermission` | `sys_role_permissions` | ❌ 只读（授权走 `/role/permissions`） | `/system/sys_role_permission` |
| `Audit` | `sys_audits` | ❌ 只读（由审计钩子写入） | `/system/sys_audit` |
| `LoginLog` | `sys_login_logs` | ❌ 只读（由登录流程写入） | `/system/sys_login_log` |

> 注意 `Menu` 的复数是 `sys_menuses`:复数是 rest/v3 对表名的机械转换(`inflector.Pluralize`),不做语义化处理。

可写资源的标准 7 个端点详见 [rest-crud.md](./rest-crud.md)；只读资源只挂载 detail / list / export。安全/审计表不暴露通用写路径，避免绕过 service 层不变量。

## 1.8 数据模型一览

| 模型 | 表 | 租户 | 主键 | 关键字段 |
|---|---|---|---|---|
| `Tenant` | `sys_tenants` | ❌ | `string`(char(60)) | `id`、`name`、`status`、`created_at`、`updated_at`、`deleted_at` |
| `User` | `sys_users` | ✅ | `BaseModel`(uint) | `uid`、`username`、`role_key`、`dept_id`、`password`(bcrypt)、`status`、`avatar`、`email`、`gender`、`description` |
| `Role` | `sys_roles` | ✅ | `BaseModel`(uint) | `key`(唯一机器标识)、`name`、`builtin`、`is_super`、`data_scope`、`status`、`sort` |
| `Menu` | `sys_menus` | ❌ | `BaseModel`(uint) | `parent`、`name`、`component`(唯一)、`uri`、`view_path`、`hidden`、`public` |
| `Department` | `sys_departments` | ✅ | `BaseModel`(uint) | `parent_id`、`name`、`description` |
| `Permission` | `sys_permissions` | ❌ | `BaseModel`(uint) | `type`(api/button/data_scope)、`data`(权限码)、`description` |
| `RolePermission` | `sys_role_permissions` | ✅ | `BaseModel`(uint) | `role_key`、`type`(menu/permission)、`data`(Menu.Component 或 Permission.Data) |
| `Audit` | `sys_audits` | ✅ | `BaseModel`(uint) | `uid`、`action`、`module`、`table`、`data` |
| `LoginLog` | `sys_login_logs` | ✅ | `BaseModel`(uint) | `uid`、`ip`、`browser`、`os`、`platform`、`access_token`(SHA-256 哈希)、`user_agent` |

**租户列**:6/9 模型(User/Role/Department/RolePermission/Audit/LoginLog)带 `tenant_id`;Menu/Permission/Tenant 全局共享。

所有模型在 `Setup` 时由 rest/v3 自动 `AutoMigrate`,无需手动迁移。模型上的 `scenarios`、`rule`、`enum` 等标签驱动 rest/v3 的字段可见性与校验。

## 1.9 业务码对照速查

| 出现场景 | Code | 消息示例 |
|---|---|---|
| 成功 | `0` | `""` |
| Login 用户名/密码错 | `4005` | `invalid username or password` |
| Login 用户被禁 | `4003` | `user is disabled` |
| Login 角色被禁 | `4003` | `role is disabled` |
| Login 角色行不存在 | `4003` | `role not found` |
| Login 租户被禁 | `4003` | `tenant is disabled` |
| RefreshToken access token 顶替 refresh | `4005` | — |
| ChangePassword 旧密码错 | `4005` | `invalid username or password` |
| ChangePassword 新旧密码相同 | `1001` | `new password must differ from the old one` |
| ChangePassword 违反密码策略 | `1001` | `password must be 5-32 characters` / `password may only contain letters and digits` / `password must contain both letters and digits` |
| ResetPassword 非超管 | `4005` | — |
| ResetPassword 目标用户不存在 | `4004` | — |
| SetAvatarByURL URL 越界 | `1001` | — |
| ReplaceRolePermissions 超管角色被改 | `4003` | `super admin role permissions are managed automatically and cannot be modified` |
| ReplaceRolePermissions menu/permission 不存在 | `1001` | `missing menu components [...]` / `missing permission datas [...]` |
| PermissionChecker 未授权 | `4003` | `permission denied` |
| PermissionChecker claims 类型不符 | `4005` | — |
| TenantService.Tenant 未找到 | `4004` | — |
| MenuBreadcrumb 未找到 | `4004` | `menu N not found` |