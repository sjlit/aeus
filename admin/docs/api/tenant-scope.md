# 10. 租户隔离

admin 模块通过 GORM 回调层实现租户隔离,**不是 HTTP 中间件**——不依赖路由结构、调用链中是否经过中间件,只依赖操作 ctx 中是否有 claims。

## 10.1 机制

`admin.Server.Setup` 在 **`MultiTenant=true`** 时通过 `installTenantScope` 安装 4 个 GORM 回调(注册名 `aeus:tenant:*`);默认单租户模式不安装:

| 回调 | 触发时机 | 行为 |
|---|---|---|
| Query | SELECT 生成前 | 在 WHERE 子句追加 `tenant_id = ?`,与业务 WHERE 合入同一条 SQL |
| Update | UPDATE 生成前 | 同上 |
| Delete | DELETE 生成前 | 同上 |
| Create | 业务 `BeforeCreate` 钩子之后 | 把 `TenantID` **强制写成** resolver 返回的租户 ID(请求体不能指定别的租户) |

未开启多租户时,`User` / `Role` / `RolePermission` / `Department` 的 `BeforeCreate` 会把空 `TenantID` 落到默认租户 `00000000-0000-0000-0000-000000000000`,保证 REST 新建的用户/角色与 `Seed` 的授权处于同一租户。

回调按**列名**(`tenant_id`)而非 struct 嵌入判断,因此:

- 不依赖 admin 的 `models` 包,应用自建的租户模型同样自动生效;
- 是否带 `tenant_id` 列的结果按 `*schema.Schema` 缓存,热路径上只有一次 map Load;
- `ResourceConfig.TenantResolve` 仅在 `MultiTenant=true` 且模型带 `tenant_id` 列时注入,Search / Export 查询路径自动带 `tenant_id` 过滤——**全局模型**(Menu/Permission/Tenant)不带此注入,避免无效 SQL。

## 10.2 Resolver 契约

```go
type Resolver func(ctx context.Context) string
```

每次 DB 操作调用一次,定义在 `admin/middleware/tenant.go`。

**返回非空租户 ID**:

- 读 / 改 / 删自动限定该租户;
- 创建时 `TenantID` 会被**强制写成该 ID**——请求体里指定其他租户无效,避免跨租户写入。

**返回 `""`(空字符串)**:

- 完全不做租户过滤——操作看到所有租户的行。
- 适用场景:超管 / 跨租户工具 / 后台任务 / `AuthService.Login`(登录前不知道)。

需要显式跨租户操作时,用 `admin.WithoutTenant(ctx)` 包一层,而不是在请求体里伪造 `tenant_id`。`Seed` 内部已经使用该 bypass,保证引导数据不会被自定义 resolver 收敛到单个租户。

**必须满足**:

- 廉价(每个 DB 操作都会调一次);
- 不可 panic(JWT 缺失时返回 `""` 即可);
- 认证决策不由 Resolver 负责,中间件才是。

## 10.3 默认实现

`middleware.FromClaimsResolver`:

```go
func FromClaimsResolver(ctx context.Context) string {
    return auth.TenantIDFromContext(ctx) // *auth.Claims.TenantID
}
```

读取 `*auth.Claims.TenantID`(由 JWT 中间件写入 ctx)。租户隔离默认**关闭**(单租户):只有显式 `admin.WithMultiTenant(true)` 或 `admin.WithTenantResolver(...)` 才会安装 GORM 回调。启用多租户且未配置自定义 resolver 时,`newOptions` 才把 `TenantResolver` 设为 `FromClaimsResolver`。单租户同时不注册 `Tenant` REST 模型和“租户管理”菜单；启用多租户后自动恢复。

## 10.4 自定义 Resolver

```go
admin.WithTenantResolver(func(ctx context.Context) string {
    if h := metadata.Get(ctx, "X-Tenant-Id"); h != "" {
        return h
    }
    return middleware.FromClaimsResolver(ctx)
})
```

适用场景:超管工具优先信任 `X-Tenant-Id` header,兜底 JWT 租户。

> ⚠️ 上述示例盲目信任请求头——必须在上游(例如仅限超管的路由中间件)限制该请求头的来源。

## 10.5 边界与全局模型

### 不带 tenant_id 列的模型

| 模型 | 原因 |
|---|---|
| `Menu` | 菜单全局共享,所有租户看到同一棵树 |
| `Permission` | 权限目录全局共享,所有租户共用一份 |
| `Tenant` | 租户表是租户 ID 的源头,无法自指 |

回调会自动跳过这些模型——它们既不会被加 `tenant_id = ?` 过滤,也不会被 Create 回调强制写入租户 ID。

### 级联清理不回绕回租户回调

`models.Role.AfterDelete` / `Menu.AfterDelete` / `RolePermission` 清理等使用:

```go
db := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true})
```

独立的 NewDB + SkipHooks 会话,避免删除权限时再次触发租户回调与递归钩子。

### rest/v3 侧同步

`server.go:registerModel`:

```go
if s.opts.TenantResolver != nil && modelHasTenantIDColumn(resourceDB, model) {
    resolver := s.opts.TenantResolver
    cfg.TenantResolve = func(ctx, _ *http.Request) (string, error) {
        return resolver(ctx), nil
    }
}
```

- 仅当模型**真有 `tenant_id` 列**时注入 `ResourceConfig.TenantResolve`。
- rest/v3 自身会无条件在 Search / Export 上加 `tenant_id = ?`——若不守卫注入,会给 Menu/Permission/Tenant 产生非法 SQL。`modelHasTenantIDColumn` 守卫避免这个 bug。

## 10.6 多租户覆盖示例

### 超管切换租户(仅超管路由)

```go
admin.WithTenantResolver(func(ctx context.Context) string {
    if h := metadata.Get(ctx, "X-Tenant-Id"); h != "" {
        // 入口处的 superAdminOnly 中间件已限制 header 来源
        return h
    }
    return middleware.FromClaimsResolver(ctx)
})
```

### 后台任务(显式跨租户)

```go
// 后台 goroutine,没有 claims
db := adminDB.WithContext(context.Background()) // ctx 无 claims → resolver 返回 "" → 不限租户
db.Find(&rows)
```

### AuthService.Login

天然在未隔离模式运行:

- `/auth/login` 在 JWT allowlist 上,ctx 无 claims → resolver 返回 `""` → 跨租户查找用户(同 username 多租户同名用户时按 `tenant_id` 显式过滤,见 `service/auth.go:259`)。
- 一旦登录成功,JWT 持有 `tid`,后续请求被 resolver 锁定到该租户。

## 10.7 调试与排错

| 症状 | 排查方向 |
|---|---|
| 数据"消失" | 检查 `WithTenantResolver` 是否返回了非预期值;确认 `*auth.Claims.TenantID` 与表中 `tenant_id` 一致 |
| 跨租户看到数据 | 检查中间件是否把 JWT claims 写入 ctx;检查 resolver 是否错误返回 `""` |
| 全局模型报错 "no column tenant_id" | rest/v3 旧版本无条件追加;升级 admin 确保 `modelHasTenantIDColumn` 守卫生效 |
| 创建时 `tenant_id` 空 | resolver 返回了空字符串;Create 回调不会强制写入(空 = 不限租户 ≠ 默认租户) |
| 删除时出现递归 | `purgeRolePermissions` 必须用 `NewDB + SkipHooks` 会话 |