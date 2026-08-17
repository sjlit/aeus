# 2. REST 通用 CRUD

由 `rest/v3` 根据注册的 GORM 模型自动生成。每个模型固定暴露 7 个端点(列表 / 详情 / 创建 / 更新 / 删除 / 导出 / OpenAPI),加上 1 个独立的 schema 元数据端点。

## 2.1 资源路径

所有资源统一以模块名 `system` 挂载。资源单复数由 rest/v3 自动从表名派生。

| 资源 | 表 | 创建 / 详情 / 更新 / 删除 / 导出 | 列表 / 搜索 | OpenAPI |
|---|---|---|---|---|
| `User` | `sys_users` | `/system/sys_user` | `/system/sys_users` | `/system/sys_user/openapi.json` |
| `Role` | `sys_roles` | `/system/sys_role` | `/system/sys_roles` | `/system/sys_role/openapi.json` |
| `Menu` | `sys_menus` | `/system/sys_menu` | `/system/sys_menuses` | `/system/sys_menu/openapi.json` |
| `Department` | `sys_departments` | `/system/sys_department` | `/system/sys_departments` | `/system/sys_department/openapi.json` |
| `Permission` | `sys_permissions` | `/system/sys_permission` | `/system/sys_permissions` | `/system/sys_permission/openapi.json` |
| `RolePermission` | `sys_role_permissions` | `/system/sys_role_permission` | `/system/sys_role_permissions` | `/system/sys_role_permission/openapi.json` |
| `Audit` | `sys_audits` | `/system/sys_audit` | `/system/sys_audits` | `/system/sys_audit/openapi.json` |
| `LoginLog` | `sys_login_logs` | `/system/sys_login_log` | `/system/sys_login_logs` | `/system/sys_login_log/openapi.json` |
| `Tenant` | `sys_tenants` | `/system/sys_tenant` | `/system/sys_tenants` | `/system/sys_tenant/openapi.json` |

> `Menu` 的复数是 `sys_menuses`(机械转换,非语义化)。

## 2.2 端点模板

以 `User`(表 `sys_users`)为例,所有资源统一遵循:

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/system/sys_users` | 列表 / 搜索;支持 `?page=&page_size=&order=&sort=` + 模型 `props:"match:exactly"` 字段的过滤 |
| `GET` | `/system/sys_user/detail/:id` | 详情(单资源全字段视图) |
| `POST` | `/system/sys_user` | 创建;body 为 JSON,带 rest/v3 的 `Validate()` 与 `scenarios:"create"` 校验 |
| `PUT` | `/system/sys_user/:id` | 整体更新;body 为 JSON |
| `DELETE` | `/system/sys_user/:id` | 删除(软删) |
| `GET` | `/system/sys_user/export` | 导出(CSV/Excel,具体格式由 formatter 决定) |
| `GET` | `/system/sys_user/openapi.json` | 该资源的 OpenAPI 3.0 文档(需 `WithOpenAPI(true)`) |

## 2.3 列表 / 搜索查询参数

rest/v3 的 `Search` 操作接受以下 query 参数(全部可选):

| 参数 | 含义 |
|---|---|
| `page` | 页码(1-based,默认 1) |
| `page_size` | 每页条数(默认 20,具体上限由 rest/v3 配置) |
| `order` | 排序方向:`asc` / `desc` |
| `sort` | 排序字段(GORM 列名) |
| 模型字段名 | 精确或模糊匹配,匹配策略取决于该字段的 `props:"match:exactly"` 标签;不在 query 中的字段不出现在 WHERE |

例:

```http
GET /system/sys_users?page=1&page_size=20&role_key=admin&status=enabled
```

## 2.4 字段可见性:scenarios

每个字段的 `scenarios` 标签决定它在哪些 REST 场景下出现。常见值:

| Scenario | 出现场景 |
|---|---|
| `list` | 列表响应 |
| `view` / `detail` | 详情响应 |
| `create` | 创建请求体 |
| `update` | 更新请求体 |
| `search` | 列表过滤条件 |
| `export` | 导出 CSV 列 |

**重要示例**:

- `User.Password` 的 `scenarios:"create"` — 只在创建请求中出现;列表 / 详情 / 导出中自动隐藏,前端无需手动 `Omit`。
- `User.RoleKey` 的 `live:"type:dropdown;url:/role/options"` — 前端表单自动渲染为下拉,数据源 `/role/options`。
- `User.DeptID` 的 `live:"type:dropdown;url:/department/labels"` — 同上。
- `Role.Key` 的 `props:"readonly:update"` — 创建后 Key 不可改(由 rest/v3 在更新路径上拒绝)。
- `Permission.Type` 的 `enum:"api:接口;button:按钮;data_scope:数据范围"` — 写入值必须是枚举集合。

## 2.5 校验:rule + validate

字段 `rule` 标签驱动 rest/v3 的输入校验(RE2 正则,无 lookahead):

| 字段 | rule |
|---|---|
| `User.UID` | `required;unique;regexp:^[a-zA-Z0-9]{3,8}$` |
| `User.Username` | `required` |
| `User.RoleKey` | `required` |
| `User.Password` | `required;regexp:^[A-Za-z0-9]{8,32}$`(REST 层只校验长度+字符集) |
| `User.Email` | — |
| `Role.Key` | `required;regexp:^[a-z][a-z0-9_]*$` |
| `Role.Name` | `required` |
| `Menu.Name` | `required` |
| `Menu.Component` | `required;unique` |
| `Menu.Uri` | `required` |

复杂约束(例如 "字母+数字组合")在 GORM 钩子里兜底(`models.User.BeforeCreate` / `BeforeUpdate`),不通过 RE2 校验,返回 `1001 Invalid`。

## 2.6 自动注册的菜单与权限目录

`admin.Server.Setup` 在注册模型时:

1. **自动建菜单行**:若模型实现 `MenuProvider`,根据 `MenuEntry()` + 模块名 + 表名推导出 `Component`/`Uri`/`ViewPath`,写入 `sys_menus`。
2. **自动建权限目录行**:对每个场景(create/update/delete/search/detail/export)写一行 `sys_permissions`,`Data` 形如 `"<METHOD> <URI>"`(例:`"POST /system/sys_user"`)。
3. **校验孤儿 Parent**:`Setup` 末尾扫描 `sys_menus`,凡 `Parent` 引用不存在 `Component` 的全部报错。

注册期间错误立即失败,不会以"半成品"状态启动。

## 2.7 应用自定义模型

应用可在 `Setup(ctx)` 后,通过 `Server.RegisterModel(model)` 挂载自己的 GORM 模型——前提是模型实现:

- `gorm.Tabler`(提供 `TableName() string`)
- `rest.ModuleNamer`(提供 `ModuleName() string`,默认 `"system"`)
- 可选 `rest.ScenarioProvider`(声明该资源暴露的场景集;默认 6 个)
- 可选 `models.MenuProvider`(提供 `MenuEntry() MenuSpec` 用于自动建菜单)

详见 [seed-options.md §RegisterModel](./seed-options.md)。

## 2.8 Schema 元数据端点

```
GET /schema/:module/:table
```

返回 rest/v3 的 schema 元数据,供前端 `@nobla/rest-ui` 渲染自动 CRUD 页:

```json
{
  "code": 0,
  "message": "",
  "data": [
    { "name": "uid", "type": "string", "rule": "required;unique;regexp:^[a-zA-Z0-9]{3,8}$", "scenarios": [...], ... },
    ...
  ]
}
```

**约束**:
- 必须挂在 JWT 之后,无 allowlist 短路。
- 路径参数 `:module`、`:table` 必须非空,否则 `1001 Invalid`。
- 表名未注册 → `4004 NotFound`。
- HTTP 状态始终 `200`,业务码由 `body.code` 决定。

注册入口(`schema_endpoint.go`):

```go
const SchemaEndpointPath = "/schema/:module/:table"
func RegisterSchemaEndpoint(opts *Options) (http.HandlerFunc, error)
```

由 `Server.Setup` 在内置模型注册完毕后自动调用,应用一般无需手动调。