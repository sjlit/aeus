# 6. PermissionService

全局权限目录(`sys_permissions`)的读取。**只读**——目录条目由 admin 自动衍生,管理入口走 rest/v3 通用 CRUD `/system/sys_permissions`。

## 6.1 注册方式

```go
pb.RegisterPermissionServiceRouter(httpSrv, service.NewPermissionService(
    service.WithPermissionServiceDB(db),
))
```

| Functional Option | 说明 |
|---|---|
| `WithPermissionServiceDB(db)` | GORM 句柄 |

无缓存;目录由 `Server.Setup` 一次性写入,运行期不变。

## 6.2 端点

### GET /permission/catalog

读取全局权限目录条目(`Permission.Data`)。

**请求**(`/permission/catalog[?type={0|1|2|3|4}]`):

| Query | 取值 | 默认 | 含义 |
|---|---|---|---|
| `type` | `0`-`4` | `0` | 见 [PermissionType 枚举](./role.md#permissiontype-枚举) |

**语义**:

| type | 行为 |
|---|---|
| `0`(UNSPECIFIED) | 返回全部条目的 `data` |
| `1`(MENU) | **菜单不在全局目录中**,因此这一档返回全部条目(等同于 "all") |
| `2`(API) | 只返回 `type=api` 条目 |
| `3`(BUTTON) | 只返回 `type=button` 条目 |
| `4`(DATA_SCOPE) | 只返回 `type=data_scope` 条目 |

> MENU 档返回全部是为了角色编辑页"勾菜单"时方便——菜单的可选项在 `/menu/options`(参见 [menu.md](./menu.md))。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "items": [
      "POST /system/sys_user",
      "PUT /system/sys_user/:id",
      "DELETE /system/sys_user/:id",
      "GET /system/sys_users",
      "GET /system/sys_user/detail/:id",
      "GET /system/sys_user/export",
      "..."
    ]
  }
}
```

`items` 是 `[]string`,每个值是 `Permission.Data`(`"<METHOD> <URI>"` 形式)。`total_count` 不在此端点出现,前端按 `items.length` 计算。

## 6.3 目录写入策略

`sys_permissions` 不是手工维护的:

1. **自动衍生**:`Server.Setup` 在每个模型注册时按其 `ScenarioProvider` 声明的场景集(create / update / delete / search / detail / export,默认 6 个)生成对应行,`type=api`、`data="<METHOD> <URI>"`,`description` 由资源名自动拼出。
2. **手工插入**:通过 REST CRUD `/system/sys_permission` 创建额外条目(如按钮 / 数据范围);其 `data` 不会与自动衍生的 URI 绑定。
3. **幂等**:`data` 列 `unique` 索引,重复启动不会重复插入。

> 若要新增非 API 类型(button / data_scope),由应用方注册带 `rest.ScenarioProvider` 的模型或在 CRUD 中手工添加;admin 模块本身不提供"建权限"按钮 UI。