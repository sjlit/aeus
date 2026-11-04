# 9. 接口权限校验(RBAC)

`admin.NewPermissionChecker(db)` 返回 `mwauth.PermissionCheckerFunc`,挂在 JWT 中间件上,对**已收录进 `sys_permissions` 目录**的 HTTP 路由执行角色权限校验。

## 9.1 接线

```go
import (
    mwauth "github.com/sjlit/aeus/middleware/auth"
    "github.com/sjlit/aeus/admin"
    "github.com/sjlit/aeus/admin/auth"
)

httpSrv.Use(mwauth.JWT(
    func(*jwt.Token) (any, error) { return []byte(secret), nil },
    mwauth.WithClaims(auth.Claims{}), // 解析进 *auth.Claims(uid / role / tenant_id)
    mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
    mwauth.WithAllow("/auth/login", "/auth/refresh-token", "/auth/logout"),
))
```

| 顺序 | 中间件 | 作用 |
|---|---|---|
| 1 | `JWT(...)` | 解析 token,把 claims 写入 ctx |
| 2 | `WithPermissionChecker(...)` | 在 claims 入 ctx **之前**执行(见下) |

## 9.2 执行时机

`NewPermissionChecker` 的签名是 `PermissionCheckerFunc = func(ctx, claims jwt.Claims) error`——它**接收解析出的 claims 作为参数**,不依赖 `auth.ClaimsFromContext`。

执行发生在 token 验证通过之后、`auth.ClaimsFromContext` 可用之前。代码顺序在 `middleware/auth/jwt.go` 中是固定的:

```
1. ParseWithClaims + signature verify
2. Run PermissionChecker(ctx, claims)   ← admin.NewPermissionChecker 在这里跑
3. 把 claims 写入 ctx
4. next.ServeHTTP(ctx)
```

业务 RPC handler 与 GORM 租户回调都从 ctx 读 claims,此时它们已经可用。

## 9.3 请求标识

请求码 = `RequestMethod + " " + RequestPath`。

- `RequestMethod`:来自 `metadata.Get(ctx, metadata.RequestMethod)`,例 `"PUT"`。
- `RequestPath`:transport/http 写入的是 gin 的 **FullPath 路由模板**(例 `"PUT /system/sys_user/:id"`),不是实例化的实际路径。

> wire URL 形式 `"PUT /system/sys_user/:id"` 与 `sys_permissions.data` 写入的 `"PUT /system/sys_user/:id"` 同构,精确匹配。

## 9.4 判定流程

仅 `RequestProtocol == "http"` 时生效,CLI/gRPC/其它协议直接放行。

| 步骤 | 条件 | 结果 |
|---|---|---|
| 1 | **公开路由**:`(method, path)` 命中内置自助清单或 `WithCheckerAllowlist` 任一项 | **放行**(无需查目录) |
| 2 | **目录查询**:`sys_permissions.type=api` 中存在 `data = "<METHOD> <URI>"` 的行 | 进入步骤 3;**查无**则默认**拒绝**;`WithCheckerFailOpen(true)` 时放行 |
| 3 | **授权查询**:`sys_role_permissions` 中存在 `role_key + type = "permission" + tenant_id` 匹配当前调用者 且 `data = code` 的行 | **放行** |
| 4 | 否则 | **拒绝**:`4003 PermissionDenied` |

补充规则:

- 非 HTTP 协议 → 步骤 0 提前放行,所有步骤跳过。
- 基础设施路由(`FullPath` 为空、`/health`、`/ready`、`/metrics`、`/mcp`、`*/openapi.json`)→ 直接放行,不查 catalog;JWT 仍负责认证。否则 fail-closed 会把静态资源/404 误报为 403。
- claims 不是 `*auth.Claims` → 直接 `4005 AccessDenied`(接线错误直接暴露在请求上,避免静默放行)。
- `type=button` / `type=data_scope` 不参与接口鉴权——只走角色编辑页的分配。

## 9.5 默认策略:fail-closed

未收录路由(目录里没有 `data` 匹配行)→ **默认拒绝**,不进入角色授权判断。自助路由(profile / menu / password / avatar)由 `defaultCheckerAllowlist` 放行;admin 的 CRUD 与业务 RPC 由 `derive.go` 写入 catalog,再由 `Seed` 授给超管、由运维按角色授权。

拒绝路径覆盖**目录未收录**和**目录已收录但当前角色无授权**两种情况(业务码 `4003`)。`WithCheckerAllowlist` 是显式 bypass,不是更严格的开关;`WithCheckerFailOpen(true)` 仅用于迁移期恢复历史行为。

> 安全取舍:fail-closed 会让未 catalog 的内部端点被挡掉。这正是目的——新路由不能因为“忘了登记”而默认对外开放。新增路由时应同时补 `sys_permissions` 或显式 allowlist。

## 9.6 缓存

不再做"每次请求一次点查",而是用 `dbcache` 维护两个集合:

| 集合 | 缓存键 | 来源表 | 失效信号 | TTL |
|---|---|---|---|---|
| API 目录 | `perm:catalog` | `sys_permissions(type=api)` | `MAX(updated_at)` | 无(纯依赖信号) |
| 角色授权 | `perm:grant:<tenant_id>:<role_key>` | `sys_role_permissions(type=permission)` | `SUM(id)` | 1 分钟 |

- 失效信号是表级 marker(`dbcache.SqlDependency`),任何对该表的写入都会让缓存下次重新校验。
- 角色授权 TTL=1 分钟是硬兜底,防止同秒 marker 漏检(GRANT+REVOKE 在同一秒净变化为零)。
- 失效信号是全表级(没法做 per-key marker),所以"一个 role 改"会让所有角色的授权缓存下次重读——**过度失效安全,失效不足会漏授权**。

## 9.7 Allowlist

```go
admin.NewPermissionChecker(db,
    admin.WithCheckerAllowlist(
        "*    /internal/*", // 由上游网关或 handler 自行鉴权
    ),
)
```

自助路由（`/user/profile`、`/user/menus`、`/menu/tree` 等）已内置在公开清单，不需要在这里重复登记。

条目语法:`"<METHOD> <pattern>"`(中间一个空格)。

| 部分 | 含义 |
|---|---|
| METHOD | `"*"` 或空 = 任意方法;否则精确匹配 |
| pattern | 精确 / `"*"`(任意) / `"<prefix>*"`(段边界前缀匹配) |

pattern 匹配语义镜像 `middleware/auth.jwt.isAllowed`:

- 精确匹配 → 命中;
- `"*"` → 命中所有;
- `"<prefix>*"` → 前缀匹配,但**只在段边界**(`prefix` 以 `/` 结尾,或 `path` 后续第一个字符是 `/`)。
  例:`"/internal/*"` 不匹配 `"/internal-rogue/foo"`。

**适用场景**:`WithCheckerAllowlist` 是**显式 bypass**——命中后既不查目录也不查角色授权。示例中的 `/internal/*` 必须由上游网关或 handler 自己承担鉴权。`/auth/login` / `/auth/refresh-token` / `/auth/logout` 这类登录前调用由 JWT `WithAllow` 短路,根本不进 checker。其余鉴权责任(例如 `/user/reset-password` 的 super-admin 检查)由各自 handler 内的应用方守卫承担。

## 9.8 缓存配置

```go
admin.NewPermissionChecker(db,
    admin.WithCheckerCache(myRedisCache), // 共享 cache.Cache 后端
)
```

未配置时走默认进程内缓存(单实例 OK,多实例需自实现共享 cache)。

## 9.9 错误码

| 场景 | Code |
|---|---|
| 未授权(目录收录但角色无授权) | `4003 PermissionDenied` |
| claims 类型不符(`*auth.Claims` 缺失) | `4005 AccessDenied` |
| 目录未收录且不在公开清单 | `4003 PermissionDenied`(fail-closed 默认) |
| 目录未收录且 `WithCheckerFailOpen(true)` | (无错误,迁移期兼容) |

## 9.10 Super 角色自动授权

`admin.Seed` 在每次启动时为每个 `IsSuper=true` 的角色补齐"全目录授权"(`grantFullCatalog`),所以超管账号默认拥有全部 `sys_permissions` 条目对应的接口权限,**无需手动维护**。

新模型注册后:先 `Setup`(建目录) → 再 `Seed`(授全量)。如果颠倒,新目录这次不会授权,下次启动会补齐。详见 [seed-options.md §Seed](./seed-options.md)。