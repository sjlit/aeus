# 4. UserService

当前用户自助接口。从 JWT claims 读取调用者身份,请求体**不携带**目标用户(`ResetPassword` 除外)。

## 4.1 注册方式

```go
pb.RegisterUserServiceRouter(httpSrv, service.NewUserService(
    service.WithUserServiceDB(db),
    // 可选:
    service.WithUserServiceCache(myRedisCache), // 共享 cache.Cache 后端
))
```

| Functional Option | 说明 |
|---|---|
| `WithUserServiceDB(db)` | GORM 句柄(必填) |
| `WithUserServiceCache(c)` | 共享 `cache.Cache` 后端;nil 走 dbcache 默认内存缓存 |

**缓存**:`UserService` 内部为 `(tenant, role) → grants` 维护 1 分钟 TTL 的 read-through cache,失效信号是 `SUM(id) ON sys_role_permissions`。role 授权变化后至多 1 分钟生效。

## 4.2 端点

### GET /user/profile

读取调用者资料。

**请求**:无 body。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "uid": "alice",
    "username": "alice",
    "email": "alice@example.com",
    "gender": "F",
    "description": "",
    "avatar": "https://...",
    "role": "admin",
    "dept_id": 1
  }
}
```

| 字段 | 类型 | 来源 |
|---|---|---|
| `uid` | string | `User.UID` |
| `username` | string | `User.Username` |
| `email` | string | `User.Email`(可能为空) |
| `gender` | string | `User.Gender`(自由文本,如 `M`/`F`/`U`,不枚举) |
| `description` | string | `User.Description` |
| `avatar` | string | `User.Avatar`(`SetAvatarByURL` 设置;空表示无) |
| `role` | string | `User.RoleKey`(Role.Key,机器标识) |
| `dept_id` | int64 | `User.DeptID` |

**失败码**:`4005`(无 claims) / `4004`(用户不存在)/ `1001`(内部错误)。

### PATCH /user/profile

部分更新调用者资料。

**请求**(`/user/profile`):

```json
{ "username": "alice2", "email": "alice2@...", "gender": "F", "description": "" }
```

| 字段 | 空值语义 |
|---|---|
| `username` / `email` / `gender` | 空字符串 = 保持原值(不覆盖) |
| `description` | 任何非空 = 覆盖;空字符串 = **清空**(字段注释中显式标注) |

> `description` 字段采用"始终应用"约定,允许前端显式清空;其余字段采用"非空才覆盖"约定。

**响应 200**:同 `/user/profile` 的 `data` 形状(更新后值)。

**失败码**:同 `/user/profile`。

### POST /user/change-password

**请求**(`/user/change-password`):

```json
{ "old_password": "...", "new_password": "..." }
```

**校验**:

- `old_password` 必须匹配当前 bcrypt hash → 否则 `4005 invalid username or password`。
- `new_password` 与 `old_password` 不能相同 → 否则 `1001 new password must differ from the old one`。
- `new_password` 必须满足密码策略(8-32、字母数字必含其一) → 否则 `1001`(`password must be 5-32 characters` / `password may only contain letters and digits` / `password must contain both letters and digits`)。

策略由 `BeforeUpdate` 钩子单点收口,前端 `User.Password` 的 `rule:"regexp:^[A-Za-z0-9]{8,32}$"` 只覆盖长度+字符集。

**响应 200**:`{ "code": 0, "message": "", "data": {} }`(空对象)。

### POST /user/reset-password

**仅超管**(角色 `is_super=true`)可调用,重置**他人**密码。

**请求**(`/user/reset-password`):

```json
{ "target_uid": "bob", "new_password": "..." }
```

**校验**:

- 调用者角色必须 `is_super=true`(每次重查 DB,即时降权立刻生效)→ 否则 `4005`。
- `target_uid` 必须存在 → 否则 `4004`。
- `new_password` 走 `BeforeUpdate` 策略校验(同上)。

**响应 200**:

```json
{ "code": 0, "message": "", "data": { "target_uid": "bob" } }
```

### POST /user/set-avatar

设置调用者的头像 URL(≤ 1024 字符)。

**请求**(`/user/set-avatar`):

```json
{ "avatar_url": "https://cdn.example.com/avatars/alice.png" }
```

**校验**:

- URL 必须非空且 `len(url) ≤ 1024` → 否则 `1001`。
- 空白字符会被 `TrimSpace` 后再存。

**实现细节**:不走 `User.Save`,而是单条 `UPDATE users SET avatar=? WHERE uid=?`,`RowsAffected == 0` 映射 `4004`(`ErrNotFound`)。

**响应 200**:

```json
{ "code": 0, "message": "", "data": { "avatar_url": "https://..." } }
```

### GET /user/menus

当前角色可见菜单(扁平列表;前端按 `parent` 重建父子关系)。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "total_count": 6,
    "menus": [
      { "name": "用户管理", "component": "SystemSysUser", "uri": "/system/sys-users", "parent": "", "icon": "user", "public": false, "hidden": false, "view_path": "@/views/system/sys-user/Index.vue" },
      { "name": "角色管理", "component": "SystemSysRole", "uri": "/system/sys-roles", "parent": "", "icon": "role", "public": false, "hidden": false, "view_path": "@/views/system/sys-role/Index.vue" }
    ]
  }
}
```

| 字段 | 说明 |
|---|---|
| `name` | 菜单标题(Menu.Name) |
| `component` | 组件标识(Menu.Component,唯一) |
| `uri` | 路由路径 |
| `parent` | 父级菜单的 `name`,空 = 顶级 |
| `icon` | 前端图标 hint |
| `public` | true = 无需认证可见 |
| `hidden` | true = 在侧栏隐藏但存在 |
| `view_path` | 自动派生的 SPA 视图路径,如 `@/views/system/sys-user/Index.vue`;空 = 走前端静态映射表兜底 |

**派生规则**(在 `ServerSetup` 时一次性写入 `sys_menus`):

- `view_path` 由 `module + table` 推导为 `@/views/<module>/<singular-table>/Index.vue`(单数化来自 rest/v3 内置 inflector)。
- `uri` 由 `module + table` 推导为 `/<module-dash>/<table-dash>`(`_` → `-`)。
- `component` 由 `ModuleName + TableName` PascalCase 拼接。

### GET /user/permissions

当前角色被授予的 API 权限码(`Permission.Data`)。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "total_count": 12,
    "permissions": [
      "POST /system/sys_user",
      "PUT /system/sys_user/:id",
      "DELETE /system/sys_user/:id",
      "GET /system/sys_users",
      "..."
    ]
  }
}
```

权限码格式:`"<METHOD> <URI>"`,与 `Permission.Data` 字段、`NewPermissionChecker` 缓存键同构。前端用其做按钮级 `v-auth` 守卫。

> `BUTTON` / `DATA_SCOPE` 类型权限**不**返回在这里(它们不参与接口执行,只走角色编辑页的分配)。

## 4.3 缓存策略

| 数据 | 缓存键 | 失效信号 | TTL |
|---|---|---|---|
| 当前用户 `(tenant, role)` 的菜单 / 权限码集合 | `user:visible_menus:<tenant>:<role>` / `user:perm_codes:<tenant>:<role>` | `SUM(id) ON sys_role_permissions` | 1 分钟 |

应用升级注册新模型后,下一次 Seed 会补全 `sys_role_permissions`,缓存最长 1 分钟生效。

## 4.4 错误码

| 场景 | Code | Message |
|---|---|---|
| ctx 无 JWT claims | `4005` | — |
| 用户不存在 | `4004` | — |
| ChangePassword 旧密码错 | `4005` | `invalid username or password` |
| ChangePassword 新旧密码相同 | `1001` | `new password must differ from the old one` |
| ChangePassword 违反策略 | `1001` | `password must be 5-32 characters` 等 |
| ResetPassword 非超管 | `4005` | — |
| ResetPassword 目标不存在 | `4004` | — |
| SetAvatarByURL URL 越界 | `1001` | — |