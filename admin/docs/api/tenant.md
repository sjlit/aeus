# 8. TenantService

租户域的读端点。租户的写操作走 REST CRUD `/system/sys_tenant`。

> ⚠️ **访问控制**:`Tenant` 模型**没有 `tenant_id` 列**(它是所有 tenant_id 列的源头,无法自指),GORM 租户回调对其天然不生效。`/tenant/*` 两个 RPC 已在服务层强制要求 super 角色；`/system/sys_tenant*` 仍走 `sys_permissions` 目录，默认 fail-closed，建议只授给 super 角色。详见 §8.4。

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

## 8.4 访问控制

`TenantService.ListTenantOptions` / `TenantService.Tenant` 已内置 super 检查：从 ctx 读取 `*auth.Claims`，再按 `role.key` **实时**查库，只有 `Role.IsSuper=true` 才放行，否则返回 `4005 AccessDenied`。因此 `/tenant/options` 与 `/tenant/detail` 即使被误授给普通角色，也不会泄露全局租户列表。

通用 REST 的 `/system/sys_tenant*` 仍走 `sys_permissions` 目录（fail-closed）：

- 超管角色由 `Seed` 自动获得授权；
- 其他角色必须显式授权；
- 建议只把 `sys_tenant*` 的写权限授给 super 角色，因为租户表没有 `tenant_id` 列，GORM 回调无法为它做行级隔离。

**不要用 allowlist 代替访问控制**——allowlist 只是绕过权限执行器，不会真正鉴权。租户 RPC 的 super 检查是服务层强制约束，不依赖应用方再挂中间件。

## 8.5 登录时的租户状态校验

`AuthService.Login` 内置:

| 条件 | 结果 |
|---|---|
| `sys_tenants.status=disabled` | `4003 tenant is disabled`,**Login 拒绝** |
| `sys_tenants` 表不存在(旧库) | 放行(由 Seed 在启动时收敛出实体行) |
| 租户行缺失(孤儿 uuid) | 放行(同上) |

`RefreshToken` 不会重查租户状态,只重查用户 + 角色。被禁租户下 RefreshToken 会继续颁发新 access token——这与禁用账户强制下线(`4003 user is disabled`)在 RefreshToken 路径上的语义保持一致;若需"租户禁用立即吊销所有会话",应用方需结合 `TokenStore` 实现。