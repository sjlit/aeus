# 数据模型与迁移

## 数据模型

| 模型 | 表名 | 租户 | 说明 |
|------|------|------|------|
| `Tenant` | `sys_tenants` | ❌ | 租户实体：`id`（char(60) 字符串主键，即各表 `tenant_id` 引用的值；创建时留空由 `BeforeCreate` 生成 uuid）、`name`、`status`（`enabled` / `disabled`，登录时校验）、`created_at` / `updated_at`。全局可见，无 `tenant_id` 列 |
| `User` | `sys_users` | ✅ | 用户：`uid`（工号，唯一，更新只读）、`username`（与 `uid` 共用登录命名空间；Login 没有租户选择器，因此跨租户也全局唯一）、`role_key`（创建/更新可写，但**任何角色变更都要求 super 调用者**）、`dept_id`、bcrypt `password`、`avatar`、`status`（`normal` / `disabled`，登录时校验）、`email`、`gender`（`man` / `woman` / `other`）、`description`。更新/删除当前角色为 super 的用户同样要求 super 调用者 |
| `Role` | `sys_roles` | ✅ | 角色：`name`、`key`（机器标识，唯一，匹配 `^[a-z][a-z0-9_]*$`，更新只读）、`status`（`enabled` / `disabled`；super 角色的状态变更要求 super 调用者）、`builtin`（REST 只读）、`is_super`（REST 只读；由 Seed 管理）、`data_scope`（`all` / `dept` / `self` / `custom`）、`sort`、`created_by`、`description`。`key` 变更自动同步 `sys_role_permissions` 与 `sys_users.role_key`；删除（含软删）自动清理关联权限并将 `sys_users.role_key` 置空；super 角色的删除要求 super 调用者 |
| `Menu` | `sys_menus` | ❌ | 菜单树：`parent`（父级菜单 Component，全局共享）、`name`（唯一，标题）、`component`（唯一，标识，路由 keep-alive 用）、`uri`、`view_path`（自动生成的 Vue 路径 `@/views/<module>/<singular>/Index.vue`）、`icon`、`hidden` / `public`、`sort`、`description`。删除自动清理 `sys_role_permissions` 中 `type=menu` 的行 |
| `Department` | `sys_departments` | ✅ | 部门树：`parent_id`、`name`、`description` |
| `Permission` | `sys_permissions` | ❌ | 全局权限目录：`type`（`api` 接口 / `button` 按钮 / `data_scope` 数据范围）+ `data`（权限标识，全租户共享，形如 `"<METHOD> <URI>"`）+ `description` |
| `RolePermission` | `sys_role_permissions` | ✅ | 角色-权限中间表：`role_key`（角色 Key）+ `type`（`menu` / `permission`）+ `data`（菜单 Component / 权限标识，按 type 决定宽度），按租户生效 |
| `Audit` | `sys_audits` | ✅ | 审计日志：`uid`、`action`（`create` / `update` / `delete`，带颜色）、`module`、`table`、`data`（变更内容，size 10240）。`WithAudit(true)` 开启后由全局钩子自动写入（见「操作审计」一节） |
| `LoginLog` | `sys_login_logs` | ✅ | 登录日志：`uid`、`ip`、`browser`、`os`、`platform`、SHA-256 `access_token`（不入参；audit 用）、`user_agent` |

所有模型默认在 `Setup` 时由 rest/v3 自动 `AutoMigrate`，无需手动迁移。生产环境可用 `admin.WithAutoMigrate(false)` 关闭建表 DDL，先执行显式迁移，再启动服务；`sys_schemas` 元数据仍会在资源注册时同步。模型上的 `scenarios`、`rule`、`enum` 等标签驱动 rest/v3 的字段可见性与校验（如 `User.Uid` 的 `rule:"required;unique;regexp:^[a-zA-Z0-9]{3,8}$"`）。

## Migrations

Schema is governed entirely by GORM `AutoMigrate` during `Setup` —
models registered in `Server.getModels()` create tables and add columns
on every startup.  There is no hand-written SQL migration layer in this
module (the project has not shipped, so destructive re-creates are
acceptable over preserving historical data).

## 应用自定义模型注册

应用通常在 `Setup` 之后追加注册若干自带模型（业务表）。`Server.RegisterModel(model, opts...)` 是公开入口：

```go
if err := s.Setup(ctx); err != nil { /* ... */ }

// 内置模型之外的应用模型：和 Setup 内置走同一条 迁移+菜单+权限 路径
s.RegisterModel(&MyOrder{})
s.RegisterModel(&MyProduct{},
    admin.WithRegisterMenuSpec(models.MenuSpec{Name: "订单管理", Parent: "SystemSettings", Sort: 50}),
    admin.WithRegisterScenarios(schema.ScenarioSearch, schema.ScenarioDetail),
)
```

`RegisterModel` 的契约：

- 必须在 `Setup` **之后**调用——`rest/v3` 需要 `Setup` 安装的 schema 元表和租户回调才能正确布线路由；
- 失败返回 `ErrHTTPRequired` / `ErrDBRequired`，与 `Setup` 同源；
- 自动按 `MenuProvider.MenuEntry()` + `ModuleName/TableName` 推导 `sys_menus` 行（详见[自动菜单注册](../../README.md#特性)）；
- 自动按 `rest.ScenarioProvider`（或缺省 6 个标准场景：create / update / delete / search / detail / export）生成 `sys_permissions` 行；
- 若应用显式配置了 Server 级 `WithVueOutputDir`，自动生成 Vue `Index.vue`（见 [Vue Index.vue 自动生成](vue-generation.md)）。

### per-call 选项

| 选项 | 作用 | 默认行为 |
|------|------|---------|
| `WithRegisterMenuSpec(spec MenuSpec)` | 覆盖模型 `MenuEntry()`；模型未实现 `MenuProvider` 时也可强制生成菜单行（传 `MenuSpec{}` 显式空 spec 仍会跳过） | 走 `MenuProvider.MenuEntry()`，未实现 `MenuProvider` 则跳过 |
| `WithRegisterScenarios(scenarios ...string)` | 覆盖该模型的权限场景集 | 走 `ScenarioProvider`，未实现则用 6 个标准场景；传空 slice 表示**不为该模型生成任何权限行**（典型：写审计类模型，授权在其它地方） |
| `WithRegisterVueOutputDir(dir string)` | 按模型粒度启用 / 禁用 Vue 生成（独立于 Server 级 `WithVueOutputDir`） | 继承 Server 级 `VueOutputDir`；传 `&""` 显式禁用该模型 |

三个选项互相独立，按需组合。`RegisterModel` 自身签名 (`RegisterModel(model)`) 保持不变，所有选项都是新增的——向后兼容。

应用批量注册完一批模型后，可调用 `admin.Server` 未导出的 `validateMenuParentsRef` 检查（或者直接调 `s.Setup` 的等价路径），把孤儿 parent 在请求到达之前**作为 Warn 日志**暴露出来——Setup 不再因此失败。
