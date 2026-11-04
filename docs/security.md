# 安全上线清单

AEUS 的默认值面向本地开发。把服务暴露到生产网络前，至少检查以下项目。

## HTTP transport

| 项目 | 建议 |
|---|---|
| TLS | 使用 `WithCertFile` / `WithKeyFile`，或在反向代理终止 TLS |
| pprof | 不要依赖 `AEUS_DEBUG` 暴露；仅在需要时用 `WithDebugAddr("127.0.0.1:6060")` 开独立 loopback 监听 |
| CORS | 显式配置 `WithCORS(CORSConfig{AllowOrigins: ...})`；不要用 `*` 搭配 credentials |
| 监听地址 | 用 `WithAddress` 显式指定；`HTTP_PORT` 仅用于本地开发 |
| 健康检查 | `/health`、`/ready` 只暴露状态，不要塞入敏感信息 |

## JWT / RBAC

- 替换 `JWT_SECRET`，不要使用示例值 `change-me`。
- 登录、刷新、登出链路会轮换 refresh token；客户端不要把 token 写入日志。
- `admin.NewPermissionChecker` 默认 fail-closed：未收录进 `sys_permissions`
  的业务路由会被拒绝。新增 REST/RPC 后运行 Seed 或迁移以补齐目录。
- `TenantService` 的 RPC 在服务层强制要求 super 角色；`/system/sys_tenant*`
  仍受 `sys_permissions` 目录控制，建议只把写权限授予 super 角色。

## 数据库与 Seed

- 生产环境设置 `AUTO_MIGRATE=false`，先用 `make migrate` 显式建表/迁移。
- 多实例部署设置 `SEED_ON_START=false`，把 `make seed` 作为一次性 Job。
- 首次登录后立即修改默认 `ADMIN_PASSWORD`。
- `WithAutoMigrate(false)` 仍会同步 `sys_schemas` 元数据（资源字段渲染依赖它），
  但不会创建或修改业务表。

## 管理台

- 生产构建使用同源部署：Go binary 托管 `web/dist`，`VITE_API_BASE` 留空。
- 前后端分离时设置 `CORS_ORIGINS` 为管理台的确切 origin。
- 管理台默认使用 localStorage 保存 access/refresh token；对 XSS 敏感的场景应改为
  HttpOnly refresh cookie，并重新评估 CORS credentials 配置。

更完整的 admin 迁移说明见 [security-upgrade.md](../admin/docs/api/security-upgrade.md)。
