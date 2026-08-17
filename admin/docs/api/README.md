# admin 模块 — 接口文档索引

本目录是 `github.com/sjlit/aeus/admin` 模块的接口参考,按"协议→服务"分层组织。`admin/README.md` 是入门指南(安装、快速开始、示例代码),本文档面向需要调用、接入或扩展 HTTP/RPC 接口的开发者。

## 阅读顺序

| # | 文件 | 内容 |
|---|---|---|
| 1 | [overview.md](./overview.md) | 模块形态、响应外壳、错误码、JWT Claims、URL 约定、模块命名、认证与租户隔离机制 |
| 2 | [rest-crud.md](./rest-crud.md) | rest/v3 自动生成的 8 个资源 CRUD + schema 元数据端点 |
| 3 | [auth.md](./auth.md) | `AuthService` — `/auth/login`、`/auth/refresh-token`、`/auth/logout`、登录钩子、TokenStore |
| 4 | [user.md](./user.md) | `UserService` — 当前用户自助接口(资料、改密、头像、可见菜单、权限码) |
| 5 | [role.md](./role.md) | `RoleService` — 角色下拉、权限查询/替换、角色菜单预览 |
| 6 | [permission.md](./permission.md) | `PermissionService` — 全局权限目录 |
| 7 | [menu.md](./menu.md) | `MenuService` — 菜单树、级联下拉、面包屑 |
| 8 | [tenant.md](./tenant.md) | `TenantService` — 租户下拉、租户详情(含访问控制提示) |
| 9 | [rbac.md](./rbac.md) | `NewPermissionChecker` — JWT 中间件上挂接的接口权限执行 |
| 10 | [tenant-scope.md](./tenant-scope.md) | GORM 租户回调、`WithTenantResolver` 契约、跨租户边界 |
| 11 | [seed-options.md](./seed-options.md) | `admin.Seed` 引导、`admin.New` 与各 Service 的 Functional Options |

## 服务清单

| 服务 | 注册方式 | 来源文件 |
|---|---|---|
| REST 资源(8 个) | `admin.New(...).Setup(ctx)` 自动挂到 `WithRouter` 指定的 router | `server.go:registerModel` |
| `AuthService` | `pb.RegisterAuthServiceRouter(router, service.NewAuthService(...))` | `pb/auth.proto`、`service/auth.go` |
| `UserService` | `pb.RegisterUserServiceRouter(router, service.NewUserService(...))` | `pb/user.proto`、`service/user.go` |
| `RoleService` | `pb.RegisterRoleServiceRouter(router, service.NewRoleService(...))` | `pb/role.proto`、`service/role.go` |
| `PermissionService` | `pb.RegisterPermissionServiceRouter(router, service.NewPermissionService(...))` | `pb/permission.proto`、`service/permission.go` |
| `MenuService` | `pb.RegisterMenuServiceRouter(router, service.NewMenuService(...))` | `pb/menu.proto`、`service/menu.go` |
| `TenantService` | `pb.RegisterTenantServiceRouter(router, service.NewTenantService(...))` | `pb/tenant.proto`、`service/tenant.go` |
| Schema 元数据 | `admin.New(...).Setup(ctx)` 自动挂 `/schema/:module/:table` | `schema_endpoint.go` |
| 权限执行 | `mwauth.WithPermissionChecker(admin.NewPermissionChecker(db))` | `permission.go` |

> `AuthService` 故意不挂在 `Setup` 里——这是设计约定,见 [overview.md §AuthService 注册路径](./overview.md#authservice-注册路径)。

## 路径根约定

```
/auth/*         AuthService    会话生命周期
/user/*         UserService    当前用户自助
/role/*         RoleService    角色域(下拉、授权、菜单预览)
/permission/*   PermissionService  全局权限目录
/menu/*         MenuService    菜单管理读端点
/tenant/*       TenantService   租户管理读端点
/system/*       rest/v3 自动资源  CRUD + schema 元数据
/schema/*       admin           schema 元数据
```

## 配套文档

- [`../INTEGRATION-TODO.md`](../INTEGRATION-TODO.md) — admin ↔ dashboard/web 集成 TODO(契约、待办、决策日志)
- [`../../README.md`](../../README.md) — 入门、装配示例、设计原则
- `pkg/errs/const.go` — 业务码常量定义