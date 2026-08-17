# 7. MenuService

菜单管理域的读端点。菜单的写操作走 REST CRUD `/system/sys_menu`(详见 [rest-crud.md](./rest-crud.md))。

## 7.1 注册方式

```go
pb.RegisterMenuServiceRouter(httpSrv, service.NewMenuService(
    service.WithMenuServiceDB(db),
    // 可选:
    service.WithMenuServiceCache(myRedisCache),
))
```

| Functional Option | 说明 |
|---|---|
| `WithMenuServiceDB(db)` | GORM 句柄;nil 时返回 `1003 Unavailable`(`menu service has no database`) |
| `WithMenuServiceCache(c)` | 共享 `cache.Cache`;nil 走默认 |

| 数据 | 缓存键 | 失效信号 | TTL |
|---|---|---|---|
| 全部菜单行(供 MenuTree 用) | `menu:tree` | `SUM(id) ON sys_menus` | 1 分钟 |
| 单菜单到根的 Component 链 | `menu:breadcrumb:<id>` | 同上(全表失效,懒加载) | 1 分钟 |

`SUM(id)` 而不是 `MAX(updated_at)` 因为后者是 Unix 秒级,同秒重命名看不到变更;`SUM(id)` 在每次插入/删除时必变。

## 7.2 端点

### GET /menu/tree

返回完整菜单树(含 `hidden` 与 `public` 标志)。节点 `name` = 菜单 **Component**(机器标识),`title` = 菜单标题(人类可读)。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "items": [
      {
        "name": "system",
        "title": "系统管理",
        "component": "System",
        "uri": "/system",
        "icon": "setting",
        "hidden": false,
        "public": false,
        "children": [
          {
            "name": "system-sys-user",
            "title": "用户管理",
            "component": "SystemSysUser",
            "uri": "/system/sys-users",
            "icon": "user",
            "hidden": false,
            "public": false,
            "children": []
          },
          {
            "name": "system-sys-role",
            "title": "角色管理",
            "component": "SystemSysRole",
            "uri": "/system/sys-roles",
            "icon": "role",
            "hidden": false,
            "public": false,
            "children": []
          }
        ]
      }
    ]
  }
}
```

| 字段 | 说明 |
|---|---|
| `name` | 节点内部标识(Menu.Name,**菜单标题**) |
| `title` | 节点展示标题(Menu.Name,与 `name` 等值;`name` 字段对应 wire 协议,`title` 是冗余显示字段) |
| `component` | Menu.Component;**也是 Menu.Parent 的引用值** |
| `uri` | Menu.Uri(路由路径) |
| `icon` | Menu.Icon |
| `hidden` | Menu.Hidden |
| `public` | Menu.Public |
| `children` | 子节点,递归同形状 |

> 前端在构建菜单树时,优先用 `component` 做 key(去重 / Vue v-for),`title` 做显示。

### GET /menu/options

层级下拉项(级联选择器数据源)。`value` 是 **Menu.Component**,可直接作为 `Menu.Parent` 回填。

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "items": [
      {
        "value": "System",
        "label": "系统管理",
        "parent": "",
        "children": [
          { "value": "SystemSysUser", "label": "用户管理", "parent": "System", "children": [] },
          { "value": "SystemSysRole", "label": "角色管理", "parent": "System", "children": [] }
        ]
      }
    ]
  }
}
```

| 字段 | 说明 |
|---|---|
| `value` | Menu.Component(也是 Menu.Parent 的引用) |
| `label` | Menu.Name(显示名) |
| `parent` | 父级 `value`,空字符串 = 顶级 |
| `children` | 子级,递归同形状 |

由 `rest.ModelTiers(parent, label, value)` 直接产出——`rest/v3` 自带工具。

### GET /menu/breadcrumb

按 id 取菜单到根的 Component 链(面包屑)。

**请求**(`/menu/breadcrumb?id={id}`):

| Query | 类型 | 校验 |
|---|---|---|
| `id` | uint32 | `>= 1`(`(validate.rules).uint32.gte = 1`) |

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "path": ["System", "SystemSysUser"]
  }
}
```

`path` 是从根到目标菜单的 **Component** 链(不是 uri 链,不是 name 链)。**字段名 `path` 保留 wire 稳定性**——`path` 此处是 Component 链,而非 `Menu.Uri`。

**实现细节**(`models/menu.go:PathTo`):

- 沿 `Menu.Parent` 引用向上走直到 `parent == ""`。
- 任一环节 parent 引用了不存在的 Component 或形成环,walk **停止并返回已收集路径**(降级语义,不报错)。
- 行不存在(目标 id 找不到)→ `4004 menu N not found`。

## 7.3 菜单与角色授权

- 角色的菜单可见性存储在 `sys_role_permissions`(`type=menu`、`data=Menu.Component`)。
- 替换整组授权:`PUT /role/permissions`,详见 [role.md](./role.md)。
- 读某角色的可见菜单:`GET /role/menus`(管理员预览)。
- 读当前调用者的可见菜单:`GET /user/menus`。

## 7.4 自动衍生

`Server.Setup` 在每次启动时:

1. 对每个实现 `MenuProvider` 的模型,从 `MenuEntry()` + `ModuleName()` + `TableName()` 推导 `Component`、`Uri`、`ViewPath`,写入 `sys_menus`。
2. `Setup` 末尾校验 `Menu.Parent` 全部指向现有 `Component`,否则报错并阻止启动**("孤儿 Parent")**。
3. 应用自定义模型也走同一路径——只要实现 `MenuProvider` 即可,详见 [seed-options.md §RegisterModel](./seed-options.md)。