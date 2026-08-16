# admin Tenant 模型与 Service — Design Spec

| 项 | 值 |
|---|---|
| 日期 | 2026-08-13 |
| 决策者 | goelea |
| 范围 | Tenant 模型 + sys_tenants 表 + REST CRUD + TenantService + Login 租户状态校验 + Seed 收敛 |
| 后端模块 | `/mobe/workspace/aeus/admin` (Go · rest/v3 + GORM · JWT) |
| 状态 | 全部决策已拍板,待实施 |
| 父文档 | `admin/docs/INTEGRATION-TODO.md` §4.5(多租户扩展蓝图,本 spec 将其第一条落地) |
| 前端 | 无改动(MVP 单租户,前端仍无切换 UI) |

---

## 1. 目标与非目标

**目标**:

- 新增 `Tenant` 模型(`models/tenant.go`)与 `sys_tenants` 表,注册后自动获得 `/system/sys_tenants` REST CRUD
- 新增 `pb/tenant.proto` + `service/tenant.go`(`TenantService`):`ListTenantOptions` / `GetTenant`
- `AuthService.Login` 集成租户状态校验:`status=disabled` 的租户拒绝登录
- `Seed` 收敛:落 `admin` 租户实体行(与现有 role/user 同 uuid),老库升级自动补齐
- `registerModel` 的 `TenantResolve` 按模型列守卫(见 §3.8)

**非目标(留待后续,蓝图 §4.5 其余条目)**:

- 前端租户切换 UI、`X-Tenant-Id` header 切换
- `sys_user_tenants` 多对多(用户跨租户)
- 登录响应 `tenants: []` 多租户选择
- `ProvisionTenant` 开通引导(创建租户即建 admin 角色/用户)
- 租户配额/计费/自定义配置

---

## 2. 架构总览

```
sys_tenants (id CHAR(60) PK, name, status, created_at/updated_at/deleted_at)
    ▲ tenant_id 引用(无 FK,沿用现状)
    │
sys_users / sys_roles / sys_departments / sys_role_permissions / sys_audits / sys_login_logs

┌─ REST 自动 CRUD: /system/sys_tenants(rest/v3 派生,模型注册即得)─┐
│  Tenant 无 tenant_id 列 → GORM 租户回调自动跳过 → 全局可见        │
└──────────────────────────────────────────────────────────────────┘
┌─ TenantService(tenant.proto)─┐
│  GET /tenant/options         │ 全量租户下拉(id/name,按 name 排序)
│  GET /tenant/detail?id=      │ 租户信息(前端展示用)
└──────────────────────────────┘
┌─ AuthService.Login ─────────────────────────────────────────────┐
│  查 user(跨租户)→ 校验租户状态(disabled 拒绝)→ 签发 JWT(tid)    │
└─────────────────────────────────────────────────────────────────┘
┌─ Seed ──────────────────────────────────────────────────────────┐
│  FirstOrCreate sys_tenants {id: uuid, name: "默认租户", enabled} │
│  → 同 uuid 建 admin role / admin user(现有逻辑)→ grant 目录      │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. 决策逐条锁定

### 3.1 主键:`id` 为 uuid 字符串主键

`id CHAR(60)` 主键,值即各表 `tenant_id` 引用的 uuid。创建时 `id` 留空则由 `BeforeCreate` 钩子生成 `uuid.NewString()`(与 `seed.go` 现有 uuid 生成一致)。`auth.go` 的 `resolveTenantName`(`WHERE id = ?`)无需改动。

### 3.2 字段:仅 `id / name / status` + 时间戳,无 `code`

蓝图 §4.5 原列 `id / code / name / status`,**去掉 `code`**:跨表引用直接指向主键 id,不存在第二个标识符的消费方;`name` 已承担展示职责;少一个 unique 约束与只读语义。将来若需运营商检索 slug,加 nullable unique 列是纯增量改动。

### 3.3 Tenant 不嵌 `TenantModel`

Tenant 无 `tenant_id` 列,`hasTenantIDColumn` 返回 false → GORM 租户回调(查询过滤/create 回填)自动跳过 → `sys_tenants` 全局可见。这正是租户管理的语义:租户自身不能被租户隔离。因不能嵌 `BaseModel`(uint ID 冲突),时间戳与软删字段自带。

### 3.4 Service 范围:REST 自动 CRUD + 两个 RPC

- REST CRUD 由模型注册自动派生(`/system/sys_tenants` 的 create/update/delete/detail/search/export)
- `TenantService`:`ListTenantOptions`(全量下拉)、`GetTenant`(按 id 查,前端展示)
- 不做 `ProvisionTenant`(见 §1 非目标)

### 3.5 Login 租户状态校验

在 Login 找到 user 之后、签发 JWT 之前:

- `sys_tenants` 表不存在(`HasTable` false)→ 放行(旧库兼容)
- 查无该 `tenant_id` 行 → 放行(兼容历史孤儿 uuid)
- `status == "disabled"` → 拒绝,返回 `errs.PermissionDenied`("tenant is disabled")
- `status == "enabled"` → 放行

`resolveTenantName` 的查询与该校验合并为一次读(实现时统一,避免同事务内两次查表)。

### 3.6 Seed 收敛 admin 租户行

现有 role ensure 之前:`FirstOrCreate` sys_tenants 行 `{id: uuid, name: "默认租户", status: "enabled"}`,role/user 复用同一 uuid(现有逻辑保持)。幂等:新库直接建行,老库(已有孤儿 uuid)升级后自动补实体行;登录响应 `tenant_name` 由 uuid 变为"默认租户"。

### 3.7 访问控制:留应用层

admin 模块不内置鉴权(与「JWT 中间件是应用的责任」哲学一致)。`/system/sys_tenants` 与 `TenantService` 路由需由应用方挂 super-admin 中间件保护,README 补充示例(如校验 `claims.IsSuper`)。`TenantService` 的 RPC 不做身份检查(读操作)。

### 3.8 `registerModel` 的 TenantResolve 按模型列守卫

rest/v3 的 Search/Export 在 `TenantResolve` 生效时**无条件**追加 `WHERE tenant_id = ?`(无列存在检查),与 admin 自己的 GORM 回调层(`hasTenantIDColumn` 守卫)不一致。改动:

- `registerModel` 中,仅当模型 schema 含 `tenant_id` 列时才设置 `cfg.TenantResolve`
- 效果 1:Tenant(及 Menu/Permission)的 REST Search/Export 不再生成非法 SQL
- 效果 2:修复现有潜在 bug —— Menu/Permission 的 search 从「SQL 报错」变为「正常返回全量」;有 tenant_id 列的模型行为完全不变

### 3.9 注册进 `getModels()`

`&models.Tenant{}` 加入 `server.go` 的 `getModels()`,自动获得:表迁移、REST 路由、`sys_menus` 行(菜单名「租户管理」)、每场景 `sys_permissions` 行。`MenuEntry()` 仿 `Role` 实现。

---

## 4. 组件设计

### 4.1 `models/tenant.go`

```go
type Tenant struct {
    ID        string          `json:"id" gorm:"primaryKey;column:id;type:char(60)" props:"readonly:update" ...`
    Name      string          `json:"name" gorm:"size:60;column:name" rule:"required" ...`
    Status    string          `json:"status" gorm:"size:20;default:enabled;column:status" enum:"enabled:启用;disabled:禁用" scenarios:"create;update;list;search" ...`
    CreatedAt int64           `json:"created_at" gorm:"column:created_at" scenarios:"view;export" ...`
    UpdatedAt int64           `json:"updated_at" gorm:"index;autoUpdateTime;column:updated_at" scenarios:"view;export" ...`
    DeletedAt gorm.DeletedAt  `json:"deleted_at" gorm:"index"`
}
```

- `BeforeCreate`:ID 为空 → `uuid.NewString()`(hook 与 `User.BeforeCreate` 同模式)
- `TableName()` → `"sys_tenants"`;`ModuleName()` → `"system"`;`MenuEntry()` → `{Name: "租户管理"}`
- tags 沿用现有约定(rule/enum/scenarios/props);`id` 的 `scenarios` 不含 `create`(创建时 UI 不展示,由 BeforeCreate 生成),更新时只读(`props:"readonly:update"`)

### 4.2 `pb/tenant.proto` + `service/tenant.go`

```proto
service TenantService {
  // 全量租户下拉(id/name),按 name 排序。GET /tenant/options
  rpc ListTenantOptions(google.protobuf.Empty) returns (ListTenantOptionsResponse);

  // 按 id 查租户信息。GET /tenant/detail?id={id}
  rpc GetTenant(GetTenantRequest) returns (TenantInfo);
}
message TenantInfo { string id = 1; string name = 2; string status = 3; int64 created_at = 4; }
message ListTenantOptionsResponse { repeated TenantOption items = 1; }
message TenantOption { string id = 1; string name = 2; }
```

- `service/tenant.go` 遵循现有 Options/Option/New 模式(`service/role.go` 同款)
- `ListTenantOptions` 不用 `rest.ModelTypes`(它强制按 `tenant_id` 过滤);直接 `db.Find(&[]models.Tenant{})` —— Tenant 无 tenant_id 列,GORM 回调自然跳过
- `GetTenant` 查无 → `errs.ErrNotFound`
- GET 路由用 query 参数风格(对齐 `/role/permissions?role=xxx`),避免 `/tenant/options` 与路径参数 `{id}` 的歧义

### 4.3 `service/auth.go` — Login 集成

```go
// resolveTenant 一次查询返回 (name, status):
//   表不存在 / 查无行 → ("", tenantID 回退, 放行)
//   status=disabled   → PermissionDenied("tenant is disabled")
//   status=enabled    → (name, 放行)
```

- 调用点:Login 中 user 查询成功后(此时 `user.TenantID` 已知),失败则整体拒绝且不写 LoginLog 成功记录(登录失败记录沿用现有失败路径)
- `resolveTenantName` 现有逻辑并入该查询

### 4.4 `seed.go` — 收敛

事务内、role ensure 之前:

```go
tenant := &models.Tenant{ID: uuid, Name: "默认租户", Status: "enabled"}
tx.Where("id = ?", tenant.ID).Attrs(*tenant).FirstOrCreate(tenant)
role := &models.Role{TenantModel: ...{TenantID: tenant.ID}, Key: "admin", ...}
// 现有 role/user/grant 逻辑不变,tenant_id 统一用 tenant.ID
```

### 4.5 `server.go` — TenantResolve 守卫

`registerModel` 中:

```go
// 仅对有 tenant_id 列的模型启用 rest/v3 的 TenantResolve;
// 无该列的模型(如 Menu/Permission/Tenant)若仍注入,Search/Export
// 会生成 WHERE tenant_id = ? 非法 SQL。
if s.opts.TenantResolver != nil && modelHasTenantIDColumn(model) {
    cfg.TenantResolve = ...
}
```

- `modelHasTenantIDColumn` 通过 GORM 解析 schema 后复用 `hasTenantIDColumn`(tenant_scope.go 已有)

---

## 5. 数据流

- **登录**:`POST /auth/login` → 查 user(Login RPC 无 JWT,resolver 返回 "" → 天然跨租户)→ `resolveTenant`(§4.3)→ 签发 JWT(含 tid)→ 响应含 `tenant_id/tenant_name`
- **租户 CRUD**:应用层挂 super-admin 中间件的 `/system/sys_tenants` → rest/v3 派生 handlers → 无租户过滤(全局)
- **下拉/详情**:`GET /tenant/options`、`GET /tenant/detail?id=` → 全量(全局)
- **Seed**:启动时收敛 `sys_tenants` 行 + role/user/grant(现有路径)

## 6. 错误处理

- `GetTenant` 查无 → `errs.ErrNotFound`
- Login 遇 disabled 租户 → `errs.Format(errs.PermissionDenied, "tenant is disabled")`(HTTP 映射沿用 pkg/errs 语义表)
- `ListTenantOptions`/`GetTenant` 的 DB 错误原样上抛,由既有 HTTP wrapper 处理

## 7. 测试清单

- `models`:BeforeCreate 空 ID 生成 uuid、显式 ID 不被覆盖;tags 场景齐全
- `service/tenant_test.go`:ListTenantOptions 返回全量且排序;GetTenant 命中 / 404
- `service/auth_test.go`:disabled 租户拒绝登录;表不存在放行;查无行放行;enabled 正常;登录响应 tenant_name 为真名
- `seed_test.go`:新库收敛后 `sys_tenants` 有 admin 行且 id 与 role/user 的 tenant_id 一致;老库(孤儿 uuid)升级补齐;两次运行幂等
- `server`/集成:`sys_tenants` 注册后 Search/Export 可用且不过滤;Menu 的 search 不再报错(回归 §3.8);tenant-scoped 模型搜索仍带 `tenant_id` 过滤

## 8. 影响与风险

- **Menu/Permission 搜索行为变化**(§3.8):从 SQL 报错变为正常返回全量。属于 bug 修复,但若外部依赖旧报错行为(不太可能)需知悉。
- **老库升级**:孤儿 uuid 的租户行在下次 Seed 自动补齐;其他非 admin 租户(若有)仍无实体行,登录放行(`resolveTenant` 兼容)。
- **`resolveTenantName`**:表与行齐备后,登录响应 `tenant_name` 由 uuid 变为"默认租户";前端只展示,无逻辑依赖。
- **访问控制**(§3.7):若应用方不挂中间件,`/system/sys_tenants` 对任意登录用户可见。README 需显著标注。
