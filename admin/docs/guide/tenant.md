# 租户隔离


## 机制

`admin.Server.Setup` 在 `MultiTenant=true` 时通过 `installTenantScope` 安装 4 个 GORM 回调（注册名 `aeus:tenant:*`）；默认单租户模式不安装：

- **Query / Update / Delete**：在 SQL 生成前追加 `WHERE tenant_id = ?`，与业务 WHERE 合入同一条语句；
- **Create**：在业务 `BeforeCreate` 钩子**之后**执行，把 `TenantID` **强制写成** resolver 返回的租户 ID（请求体不能指定别的租户）。
- 未开启多租户时，`User` / `Role` / `RolePermission` / `Department` 的 `BeforeCreate` 会把空 `TenantID` 落到默认租户 `00000000-...`，保证 REST 新建的用户/角色与 `Seed` 的授权在同一个租户命名空间。

回调按**列名**（`tenant_id`）而非 struct 嵌入判断，因此不依赖 admin 的 `models` 包——应用自建的租户模型同样自动生效。是否带 `tenant_id` 列的结果按 `*schema.Schema` 缓存，热路径上只有一次 map Load。

## Resolver 契约

`middleware.Resolver` 是 `func(ctx context.Context) string`，每次 DB 操作调用一次：

- 返回**非空**租户 ID：读写删除自动限定该租户，创建时 `tenant_id` 会被**强制写成 resolver 的值**（请求体不能指定别的租户）；
- 返回 **""**：完全不做租户过滤（跨租户工具、后台任务、`AuthService.Login` 的出口）；
- 需要显式跨租户操作时用 `admin.WithoutTenant(ctx)`，不要依赖“请求体里写 tenant_id”来绕过。

Resolver **必须廉价且不可 panic**——JWT 缺失时返回 `""` 即可，认证决策由中间件负责。

## 默认值与覆盖

租户隔离默认**关闭**（单租户）。调用 `admin.WithMultiTenant(true)` 或 `admin.WithTenantResolver(...)` 后才会安装 GORM 回调；启用且未提供自定义 resolver 时，默认 resolver 是 `middleware.FromClaimsResolver`，直接读取 `auth.Claims.TenantID`（由 JWT 中间件写入 ctx），无需额外中间件。

管理界面也跟随该开关：单租户模式不注册 `Tenant` 业务模型，因此不会有 `/system/sys_tenants` REST 路由、“租户管理”菜单和对应 CRUD 权限；`sys_tenants` 表仍会迁移并由 `Seed` 写入默认租户行，供登录响应展示租户名。启用多租户后这些资源会自动恢复。

覆盖示例（优先信任 `X-Tenant-Id` 请求头，兜底 JWT 租户，适用于超管工具）：

```go
admin.WithTenantResolver(func(ctx context.Context) string {
	if h := metadata.Get(ctx, "X-Tenant-Id"); h != "" {
		return h
	}
	return middleware.FromClaimsResolver(ctx)
})
```

> 注意：上述示例会盲目信任请求头，请在上游（如仅限超管的路由中间件）限制该请求头的来源。

## 边界情况

- **`Menu` / `Permission` / `Tenant` 无租户**：这三个模型没有 `tenant_id` 列，全局共享，GORM 回调自动跳过；
- **级联清理不回绕**：`purgePermissions` 使用 `NewDB + SkipHooks` 的独立会话，避免删除权限时再次触发租户回调与递归钩子；
- **rest/v3 侧同步**：注册资源时把 resolver 注入 `ResourceConfig.TenantResolve`，Search / Export 查询路径自动带 `tenant_id` 过滤——**仅对含 `tenant_id` 列的模型注入**（rest/v3 无条件追加该过滤，注入给全局模型会产生非法 SQL）。
