# 8. TenantService

租户域的读端点。租户的写操作走 REST CRUD `/system/sys_tenant`。

> ⚠️ **访问控制必须由应用方处理**:`Tenant` 模型**没有 `tenant_id` 列**(它是所有 tenant_id 列的源头,无法自指),所以 GORM 租户回调对其天然不生效——`/system/sys_tenants` 与 `/tenant/*` 对任意通过 JWT 中间件的用户可见。admin 模块不做鉴权决策,应用必须自行挂超管专属中间件。详见 §8.4。

## 8.1 注册方式

```go
pb.RegisterTenantServiceRouter(httpSrv, service.NewTenantService(
    service.WithTenantServiceDB(db),
))
```

| Functional Option | 说明 |
|---|---|
| `WithTenantServiceDB(db)` | GORM 句柄 |

无缓存(租户表行数小、变化少)。

## 8.2 端点

### GET /tenant/options

全量租户下拉项(按 `name` 升序),**含 disabled 租户**——disabled 只控制登录,不控制展示。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "items": [
      { "id": "tenant-uuid-1", "name": "默认租户" },
      { "id": "tenant-uuid-2", "name": "演示租户" },
      { "id": "tenant-uuid-3", "name": "已停用租户" }
    ]
  }
}
```

| 字段 | 说明 |
|---|---|
| `id` | `Tenant.ID`(char(60),与 `sys_users.tenant_id` 等列引用值相同) |
| `name` | `Tenant.Name`(显示名) |

### GET /tenant/detail

按 id 取单个租户的详细信息。

**请求**(`/tenant/detail?id={id}`):

| Query | 类型 | 校验 |
|---|---|---|
| `id` | string | 1-60 字符(`(validate.rules).string.min_len=1, max_len=60`) |

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "id": "tenant-uuid-1",
    "name": "默认租户",
    "status": "enabled",
    "created_at": 1700000000
  }
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 同上 |
| `name` | string | 租户名 |
| `status` | string | `enabled` / `disabled` |
| `created_at` | int64 | Unix 秒(`int64`,不是 ISO 字符串) |

**失败码**:租户不存在 → `4004 NotFound`。

## 8.3 Tenant 模型字段

```go
type Tenant struct {
    ID        string         `gorm:"primaryKey;type:char(60)"` // 主键 = uuid 字符串
    Name      string         `gorm:"size:60"`                   // 显示名
    Status    string         `gorm:"default:enabled"`           // enabled/disabled
    CreatedAt int64          `gorm:"column:created_at"`         // Unix 秒
    UpdatedAt int64          `gorm:"autoUpdateTime"`            // Unix 秒
    DeletedAt gorm.DeletedAt                                   // 软删
}
```

- **主键**:字符串 char(60),由 `BeforeCreate` 钩子在缺省时填入新 uuid,保证 `sys_users.tenant_id` 等外键引用稳定。
- **不带 `tenant_id` 列**(`TenantModel` 没嵌入),GORM 租户回调自动跳过,查询天然跨租户。
- **嵌入 `BaseModel`**:uint 自增 ID 不存在,主键由 `gorm:"primaryKey"` 直接落在 `ID` 上。

## 8.4 ⚠️ 应用方必须加访问控制

```go
// 推荐实现:按路径前缀拦截,仅允许 IsSuper 角色访问
func superAdminOnly(db *gorm.DB) middleware.Middleware {
    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context) error {
            path, _ := metadata.Get(ctx, metadata.RequestPath)
            if !strings.HasPrefix(path, "/system/sys_tenants") &&
                !strings.HasPrefix(path, "/tenant/") {
                return next(ctx)  // 非租户路由放行
            }
            claims, ok := auth.ClaimsFromContext(ctx)
            if !ok {
                return errs.ErrAccessDenied
            }
            var role models.Role
            if err := db.WithContext(ctx).Where("key = ?", claims.Role).
                First(&role).Error; err != nil || !role.IsSuper {
                return errs.ErrAccessDenied
            }
            return next(ctx)
        }
    }
}

// 挂载顺序:JWT 在前(写入 claims) → superAdminOnly 在后
httpSrv.Use(
    mwauth.JWT(/* ... */),
    superAdminOnly(db),
)
```

也可通过 `NewPermissionChecker` 的 `WithCheckerAllowlist` 把租户路由从执行器短路:

```go
mwauth.WithPermissionChecker(
    admin.NewPermissionChecker(db,
        admin.WithCheckerAllowlist(
            "GET /tenant/options",
            "GET /tenant/detail",
            // CRUD 路由按 catalog 自动放行(IsSuper 角色自动拥有全目录授权)
        ),
    ),
)
```

但**仅靠 allowlist 不够**——allowlist 只是绕过权限执行器,不会真正鉴权;`superAdminOnly` 这类显式守卫才是真正的访问控制层。

## 8.5 登录时的租户状态校验

`AuthService.Login` 内置:

| 条件 | 结果 |
|---|---|
| `sys_tenants.status=disabled` | `4003 tenant is disabled`,**Login 拒绝** |
| `sys_tenants` 表不存在(旧库) | 放行(由 Seed 在启动时收敛出实体行) |
| 租户行缺失(孤儿 uuid) | 放行(同上) |

`RefreshToken` 不会重查租户状态,只重查用户 + 角色。被禁租户下 RefreshToken 会继续颁发新 access token——这与禁用账户强制下线(`4003 user is disabled`)在 RefreshToken 路径上的语义保持一致;若需"租户禁用立即吊销所有会话",应用方需结合 `TokenStore` 实现。