# 接口权限校验（RBAC）

`admin.NewPermissionChecker(db, opts...)`（`permission.go`，db 为 nil 时 panic，与其它 Service 构造器一致）返回 JWT 中间件的 `PermissionCheckerFunc`，对**已收录进 `sys_permissions` 目录**的 HTTP 路由按角色校验权限：

```go
mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
```

**执行时机**：checker 在 JWT 中间件 **token 验证通过之后、claims 写入 ctx 之前**运行（`middleware/auth/jwt.go`）——它以参数形式接收解析出的 claims，**不能**依赖 `auth.ClaimsFromContext`；后续 handler 与 GORM 租户回调才从 ctx 读 claims。

**请求标识**：`metadata.RequestMethod + " " + metadata.RequestRoute`。transport/http 写入的 `RequestRoute` 是 gin 的 **FullPath 路由模板**（如 `PUT /system/sys_user/:id`），与 `permissionCode` 生成 `sys_permissions.data` 的 `"<METHOD> <URI>"` 格式同构，二者精确匹配。仅有 `RequestPath`、没有 `RequestRoute` 的嵌入方会回退到 `RequestPath`；NoRoute/静态请求的 `RequestRoute` 为空，checker 直接放行。

**判定流程**（仅 `RequestProtocol == "http"` 时生效，CLI / 其它协议直接放行）：

| 步骤 | 条件 | 结果 |
|------|------|------|
| 1. 接线保护 | claims 不是 `*auth.Claims` | **拒绝**：`4005 AccessDenied` |
| 2. 公开路由 | 命中内置自助清单或 `WithCheckerAllowlist` | **放行** |
| 3. 目录查询 | `sys_permissions` 存在 `type=api` 且 `data` 匹配的行 | 进入步骤 4；查无此行默认**拒绝**（fail-closed）；`WithCheckerFailOpen(true)` 时放行 |
| 4. 授权查询 | `sys_role_permissions` 存在 `role_key + tenant_id + type=permission + data` 匹配的行 | **放行** |
| 5. 否则 | 目录内但角色无授权 | **拒绝**：业务码 `4003 PermissionDenied` |

**设计要点**：

- **fail-closed 默认拒绝**：未收录路由（包括新加的业务 RPC）默认拒绝；自助路由（`/user/profile`、`/user/menus`、`/menu/tree` 等）由 `defaultCheckerAllowlist` 放行；admin 的 CRUD 与业务 RPC 在 `derive.go` 中写入 `sys_permissions`，由 `Seed` 授给超管、由运维按角色授权；
- **目标级守卫**：通用 REST 的角色变更、super 角色/用户的 status 更新和删除都要求调用者自身是 super；仅靠路由级 `sys_permissions` 不够，否则拥有用户管理权限的非 super 角色可以禁用/删除超管；
- **`WithCheckerAllowlist` 是 bypass**：命中即跳过目录与角色校验。它不会让检查更严格；新增 bypass 必须能说明“该路由由其他机制保护或本就公开”；
- **基础设施路由**：NoRoute/静态文件（`RequestRoute` 为空）、`/health`、`/ready`、`/metrics`、`/mcp`、`*/openapi.json` 不属于业务 API，checker 直接放行，仍由 JWT 负责认证；否则 fail-closed 会把静态资源/404 误报成 403；JWT 的 `WithAllow("/assets/*")` 等需要放行静态资源本身；
- **租户隔离显式化**：授权查询显式过滤 `tenant_id`——checker 运行时 claims 尚未进入 ctx，GORM 租户回调不会自动过滤，不显式传会导致跨租户串权；
- **接线保护**：claims 不是 `*auth.Claims` 时直接拒绝（`4005 AccessDenied`），把中间件接线错误暴露在请求上而不是静默放行；
- **边界**：`type=button` / `type=data_scope` 的权限不参与接口鉴权；`/auth/*` 在 `WithAllow` 列表上直接短路，不进 checker（`/auth/logout` 也应加入该列表，见下方会话说明）；
- **超管授权窗口**：角色授权由 `admin.Seed` 每次启动补齐（以调用时刻的全局目录为准），新模型注册后需先 `Setup` 后 `Seed`，见[P0 一键集成](../../README.md#p0-一键集成搭配-dashboardweb)的时序约定。

**选项**：

| 选项 | 说明 |
|------|------|
| `WithCheckerAllowlist(entries ...string)` | 显式 **bypass** 清单：命中后跳过 catalog 与角色授权检查。每条 `"<METHOD> <pattern>"`（`METHOD` 为 `*` 或空表示任意方法；`pattern` 支持精确、`*` 通配、`<prefix>*` 段边界前缀）。仅用于“由其他机制保护或本就公开”的路由 |
| `WithCheckerFailOpen(true)` | 临时恢复历史 fail-open 行为：未收录路由放行。只应在迁移期使用，最终目标是全部路由进入 catalog 或显式 allowlist |
| `WithCheckerCache(cache.Cache)` | 注入共享缓存后端，覆盖默认内存后端（默认 `infra/cache/memory`，单进程）。多实例部署换成 Redis 等共享后端，避免每个进程的 catalog 独立陈旧 |

**缓存**：api 目录与每个 `(tenant, role)` 的授权集合都通过 `admin/dbcache` 的读穿透 cacher 加载；catalog 用 `MAX(updated_at)` 作版本标记（无 TTL），grants 用 `SUM(id)` + 1 分钟 TTL（吸收标记盲区）。`dbcache` 的 1 秒宽限窗 + 版本标记组合让授权变更在下一个请求就生效，无需应用显式失效。

测试覆盖见 `permission_test.go`：已授权放行 / 未授权 4003 / 未收录路由默认**拒绝**（fail-closed）/ 自助路由放行 / 基础设施路由放行 / 显式 allowlist bypass / `WithCheckerFailOpen(true)` 兼容 / allowlist 通配段边界 / 非 http 跳过 / 跨租户不串权 / claims 类型不符 4005 / 缓存命中。
