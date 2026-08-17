# 5. RoleService

角色域的所有接口。下拉、授权查询/替换、菜单预览。2026-08-10 起把原 `PermissionService.ListRolePermissions` 与 `MenuService.ListVisibleMenusByRole` 全部吸收到 `/role/*`,单一域。

## 5.1 注册方式

```go
pb.RegisterRoleServiceRouter(httpSrv, service.NewRoleService(
    service.WithRoleServiceDB(db),
    // 可选:
    service.WithRoleServiceCache(myRedisCache),
))
```

| Functional Option | 说明 |
|---|---|
| `WithRoleServiceDB(db)` | GORM 句柄;nil 时所有方法返回 `1003 Unavailable`(`role service has no database`) |
| `WithRoleServiceCache(c)` | 共享 `cache.Cache` 后端;nil 走默认 |

| 数据 | 缓存键 | 失效信号 | TTL |
|---|---|---|---|
| `(tenant, role)` 授权(菜单 + API 码) | `role:permissions:<tenant>:<role>` | `SUM(id) ON sys_role_permissions` | 30 秒 |
| 当前租户全部角色下拉 | `role:options:<tenant>` | `MAX(updated_at) ON sys_roles` | 1 分钟 |

## 5.2 端点

### GET /role/options

返回当前租户的全部角色下拉项,按 `key` 升序。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "items": [
      { "value": "admin",    "label": "系统管理员" },
      { "value": "operator", "label": "操作员" }
    ]
  }
}
```

| 字段 | 说明 |
|---|---|
| `value` | `Role.Key`(机器标识,前端原样回填 `User.RoleKey`) |
| `label` | `Role.Name`(显示名) |

### GET /role/permissions

读取某角色的授权(菜单 Component 列表 + API 权限码列表)。

**请求**(`/role/permissions?role={role}[&type={0|1|2}]`):

| Query | 类型 | 取值 | 默认 | 含义 |
|---|---|---|---|---|
| `role` | string | 1-60 字符 | (必填) | 角色 Key |
| `type` | int | 0 / 1 / 2 | `0`(UNSPECIFIED) | `0` = 返回 `{menus, apis}` 合读;`1` = MENU(只 `menus`,`apis=[]`);`2` = API(只 `apis`,`menus=[]`) |

> `BUTTON`(3) / `DATA_SCOPE`(4) 在此端点直接返回空结构;它们由 `PermissionService.ListCatalog` 暴露。

**响应 200**(`type=0` 完整示例):

```json
{
  "code": 0,
  "message": "",
  "data": {
    "menus": ["SystemSysUser", "SystemSysRole"],
    "apis": ["POST /system/sys_user", "PUT /system/sys_user/:id"]
  }
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `menus` | string[] | `Menu.Component` 列表(由 `RolePermission` 中 `type=menu` 的 `data` 去重而来) |
| `apis` | string[] | `Permission.Data` 列表(由 `RolePermission` 中 `type=permission` 的 `data` 去重而来) |

### PUT /role/permissions

整体替换授权(原子操作:先删后插)。只接受菜单 Component + API 权限码,不做"按钮/数据范围"分配(后两类不在中间表)。

**请求**(`/role/permissions` body):

```json
{
  "role": "operator",
  "menus": ["SystemSysUser", "SystemSysMenu"],
  "apis": ["GET /system/sys_users", "GET /system/sys_user/detail/:id"]
}
```

| 字段 | 校验 |
|---|---|
| `role` | 1-60 字符;角色必须存在;`IsSuper=true` **拒绝修改**(返回 `4003` `super admin role permissions are managed automatically and cannot be modified`) |
| `menus` | 每项 ≤ 1024 条,每条 1-120 字符;所有 Component 必须存在于 `sys_menus`,否则 `1001 missing menu components [...]` |
| `apis` | 每项 ≤ 1024 条,每条 1-60 字符;所有 Data 必须存在于 `sys_permissions`,否则 `1001 missing permission datas [...]` |

**事务语义**:

1. `tx.Where("`key` = ?", role).First(&role)` — 角色存在性 + IsSuper 检查;
2. `RolePermission.ValidateMenuData` / `ValidatePermissionData` — 引用存在性;
4. `Role.ReplacePermissions(tx, ctx, role, menus, apis)` — 单事务内 `Delete + Create`。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": { "role": "operator", "affected": 5 }
}
```

`affected = len(menus) + len(apis)`。

**失败码**:

| Code | Message | 触发 |
|---|---|---|
| `4004` | — | role 不存在 |
| `4003` | `super admin role permissions are managed automatically and cannot be modified` | IsSuper 角色 |
| `1001` | `missing menu components [...]` | menu Component 不存在 |
| `1001` | `missing permission datas [...]` | permission Data 不存在 |

### GET /role/menus

读取某角色的可见菜单(扁平),用于"管理员预览"——和 `/user/menus` 同样的形状,但参数化在 role 上,不取调用者身份。

**请求**(`/role/menus?role={role}`):

| Query | 校验 |
|---|---|
| `role` | 1-60 字符 |

**响应 200**:同 `/user/menus` 的 `data` 形状(`MenuEntry` 数组 + `total_count`)。

> 此端点不走缓存(每次直接 DB 查询);管理员预览的访问频次远低于当前用户查自己菜单。

## 5.3 PermissionType 枚举

与 `permission.proto` 共享:

| 值 | 常量 | 含义 |
|---|---|---|
| `0` | `PERMISSION_TYPE_UNSPECIFIED` | 不过滤 |
| `1` | `PERMISSION_TYPE_MENU` | 仅菜单 |
| `2` | `PERMISSION_TYPE_API` | 仅接口 |
| `3` | `PERMISSION_TYPE_BUTTON` | 按钮(目录侧,不在此返回) |
| `4` | `PERMISSION_TYPE_DATA_SCOPE` | 数据范围(目录侧,不在此返回) |

> 枚举 query 参数**只能传数字**(proto 枚举 `MapFormWithKind` 不接受字符串形式);字符串 `?type=menu` 会得到绑定错误,详见 [overview.md §URL 形态约定](./overview.md#url-形态约定)。