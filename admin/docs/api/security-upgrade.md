# 安全默认值升级指南

> 适用版本：`a1e7eae` 及之后。
>
> 目标：把历史上的 fail-open、单租户隐式行为和“所有字段都能改”收敛为可预期的安全默认值。

## 1. 接口鉴权：fail-open → fail-closed

### 变化

`NewPermissionChecker` 现在默认拒绝未收录进 `sys_permissions` 的业务路由。旧版本未收录路由会直接放行。

判定顺序：

1. 非 HTTP 协议 → 放行；
2. 基础设施路由（`/health`、`/ready`、`/metrics`、`/mcp`、`*/openapi.json`、静态文件/404）→ 放行，JWT 仍负责认证；
3. 自助路由（`/user/profile`、`/user/menus`、`/menu/tree` 等）→ 放行；
4. `(METHOD, path)` 在目录中且当前角色有授权 → 放行；
5. 其他 → `4003 PermissionDenied`。

### 迁移步骤

1. 启动一次新版本，让 `derive.go` 把新增业务路由写入 `sys_permissions`；
2. 检查 `Seed` 是否给超管角色补齐授权；
3. 给普通角色分配对应权限；
4. 如仍有第三方路由未登记，优先补 `sys_permissions` 或 `WithCheckerAllowlist`，不要直接打开 fail-open。

临时回退（仅迁移期）：

```go
admin.NewPermissionChecker(db, admin.WithCheckerFailOpen(true))
```

`WithCheckerFailOpen(true)` 会让未收录路由重新放行，只应作为短期过渡。

## 2. 单租户 → 多租户

默认是**单租户**：不安装 GORM 租户回调。`User` / `Role` / `RolePermission` / `Department` 的 `BeforeCreate` 会把空 `tenant_id` 落到 `models.DefaultTenantID`。

开启多租户：

```go
admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
    admin.WithMultiTenant(true),
    // 可选；不传时使用 FromClaimsResolver
    admin.WithTenantResolver(adminmiddleware.FromClaimsResolver),
)
```

注意事项：

- 多租户开启后，Create 回调会把 `tenant_id` **强制写成** resolver 的返回值，请求体不能指定别的租户；
- `Login` 没有租户选择器，同一 `uid` / `username` 出现在多个租户会直接拒绝登录；
- `UID` 和 `Username` 共用登录命名空间，跨字段、跨租户都必须唯一；
- 历史数据的 `tenant_id` 为空时，先用迁移脚本补齐，再开启多租户。

## 3. 只读表

以下表通过通用 REST 只暴露 `search` / `detail` / `export`：

| 表 | 原因 |
|---|---|
| `sys_permissions` | 权限目录由代码派生，手工修改会绕过部署约束 |
| `sys_role_permissions` | 授权关系必须走 `ReplaceRolePermissions` 校验 |
| `sys_audits` | 审计日志只追加 |
| `sys_login_logs` | 登录日志只追加，token 以哈希形式保存 |

前端对应页面已设置为只读；后端即使收到写请求也会拒绝。

## 4. 字段与目标级保护

| 操作 | 规则 |
|---|---|
| 修改 `User.RoleKey` | 任何实际角色变更都要求调用者自身是 super |
| 更新/删除当前角色为 super 的用户 | 要求 super 调用者 |
| 更新/删除 super 角色 | 要求 super 调用者 |
| `POST /user/reset-password` | 要求 super 调用者，且从 DB 实时读取角色 |
| `GET /tenant/options`、`GET /tenant/detail` | 要求 super 调用者 |
| `Role.Key` / `Role.Builtin` / `Role.IsSuper` | REST 更新不可写 |
| `User.UID` | REST 更新不可写 |

非 super 的用户管理员仍然可以修改用户的邮箱、状态、备注等字段，但不能改角色。

## 5. 响应与前端行为

| 场景 | HTTP | 业务码 | 前端 |
|---|---|---|---|
| 业务参数错误（admin responder） | 200 | `1001` 等 | toast |
| 角色无权限 | 403 | `4003` | toast 无权限 |
| 传输层未登录/凭证失效 | 403 | `4005` | 跳登录页 |
| 旧密码错误等业务 `4005` | 200 | `4005` | 只 toast，不跳登录 |
| 限流 | 429 | `4010` | toast + Retry-After |
| 未知 5xx | 5xx | `5000` 等 | 统一 `internal server error` |

refresh token 会轮换，前端必须持久化响应中的新值。`admin/web` 使用 `aeus.access` / `aeus.refresh` 两个 localStorage key。

## 6. 生产建表

`admin.WithAutoMigrate(false)`（脚手架的 `AUTO_MIGRATE=false`）会关闭 Setup 和资源注册阶段的业务表 DDL，但 `sys_schemas` 元数据仍会同步。生产环境应先执行显式迁移（脚手架模板提供 `make migrate`），再启动服务：

```bash
make migrate
AUTO_MIGRATE=false SEED_ON_START=false make run
```

多实例部署时把 `make seed` 作为一次性 Job 执行；不要在每个 Pod 启动时同时 Seed。

## 7. 检查清单

- [ ] 新路由已进入 `sys_permissions`，或明确加入 allowlist；
- [ ] 超管角色 grants 完整；
- [ ] 多租户数据已完成 `tenant_id` 回填；
- [ ] 前端四个只读页面没有写操作入口；
- [ ] 生产环境没有启用 `WithCheckerFailOpen(true)`；
- [ ] 默认管理员密码、JWT secret 已替换。
