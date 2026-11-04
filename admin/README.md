# AEUS admin — 通用后台管理模块

**项目地址**: github.com/sjlit/aeus/admin

admin 是 AEUS 框架的通用后台管理领域模块，基于 [rest/v3](https://github.com/sjlit/rest) + GORM，为应用提供即开即用的系统管理能力：

- **通用 REST CRUD**：用户、角色、部门、菜单、租户 5 个业务模型自动注册为可写 REST 资源（列表/搜索、详情、创建、更新、删除、导出、OpenAPI 文档）；权限目录、角色-权限中间表、审计日志、登录日志 4 个安全/审计模型只读挂载——它们由 `Setup`/`Seed`/service/审计钩子管理，避免通用写路径绕过业务不变量；
- **JWT 认证**：登录 / 刷新 / 登出（`AuthService`），支持 Token 吊销与登录前后钩子；
- **RBAC 接口鉴权**：`NewPermissionChecker` 对已收录进 `sys_permissions` 的 HTTP 路由按角色校验权限，**未收录路由默认拒绝（fail-closed）**；自助路由（profile / menu / password 等）由内置公开清单放行；`WithCheckerAllowlist` 是额外的显式 bypass 清单，不是“更严格”的开关（详见[接口权限校验](docs/guide/rbac.md)）；
- **用户业务 RPC**（`UserService`）：个人资料、改密、管理员重置密码、头像、可见菜单、权限码；
- **租户隔离（可选）**：显式 `WithMultiTenant(true)` / `WithTenantResolver(...)` 后，通过 GORM 回调为所有租户模型追加 `tenant_id` 过滤，并在创建时强制写入 resolver 的租户 ID；默认单租户不安装回调。

## 特性

- **声明式 CRUD**：模型上的 `scenarios` / `rule` / `enum` / `format` / `live` 标签直接驱动接口的字段可见性、校验规则与前端表单渲染
- **自动密码哈希**：`models.User` 在 `BeforeCreate` / `BeforeUpdate` 中对密码做 bcrypt 哈希（对已哈希值幂等）；`LoginLog.AccessToken` 走 SHA-256（高熵随机串无须 bcrypt 慢哈希）
- **密码策略**：8-32 位字母+数字基线（`CheckPasswordPolicy`），所有写密码路径经 GORM 钩子单点收口
- **级联清理**：删除菜单/角色（含软删）时自动清理 `sys_role_permissions` 中的关联权限，并将仍指向该角色的 `sys_users.role_key` 置空（避免悬空引用）；角色 Key 变更自动同步 `sys_role_permissions.role_key` 与 `sys_users.role_key`（避免用户的角色 Key 失同步）
- **统一响应格式**：内置 responder 输出 `{code, message, data}` envelope（成功 `code=0`），也可用 `WithResponder` 替换
- **可选的 OpenAPI**：`WithOpenAPI(true)` 后每个资源暴露 `openapi.json`
- **可插拔认证**：`AuthService` 始终由应用自行 `pb.RegisterAuthServiceRouter(...)` 注册，secret 必须来自运行时通道（见[设计约定](docs/guide/architecture.md#设计约定)）
- **自动菜单注册**：模型实现 `MenuProvider` 时，`Setup` / `RegisterModel` 会按 `MenuEntry()` 自动生成 `sys_menus` 行（`Component` / `Uri` / `ViewPath` 在缺失时由模块名 + 表名推导，可显式覆盖）。三个分区容器菜单（`SystemUserCenter` / `SystemLogs` / `SystemSettings`）由 `EnsureSectionMenus` 在 `Seed` 阶段写入，业务菜单 `Parent` 都引用这三个 Component。`Setup` 末尾 `validateMenuParentsRef` 会扫所有 `sys_menus`，若发现 `Parent` 指向不存在的 Component 只打 `Warn` 而**不**中断启动——`Menu.BuildTree` 会把孤儿行降级为根，导航树仍可工作；运维应通过 `/system/sys_menu` 修正
- **自动权限目录**：每个挂载模型按实际挂载的场景集生成 `sys_permissions` 行（可写模型 6 个 CRUD 场景；安全/审计模型只读 search/detail/export；`Data` 形如 `"<METHOD> <URI>"`），重复启动幂等
- **接口权限执行**：`NewPermissionChecker` 把 HTTP 请求的 `"METHOD <路由模板>"` 与权限目录、角色授权逐一比对，**默认 fail-closed**——未收录路由直接拒绝，自助路由走内置公开清单，目录里有但当前角色无授权同样拒绝；`WithCheckerAllowlist` 用于显式 bypass（详见[接口权限校验](docs/guide/rbac.md)）
- **读穿透缓存**：权限目录与角色授权都走 `admin/dbcache` 的 SqlDependency 版本标记缓存，写授权后下一个请求就生效，无需应用显式失效
- **公开 `RegisterModel`**：应用自定义模型可与内置模型走同一条迁移+菜单+权限注册路径；per-call 选项（`WithRegisterMenuSpec` / `WithRegisterScenarios` / `WithRegisterVueOutputDir`）按需覆盖
- **Vue 视图自动生成**：`WithVueOutputDir` 启用后，每个模型按 `(module, singular)` 自动创建 `Index.vue`（SchemaViewer 占位），已存在的文件跳过不覆盖手写修改
- **一键引导**：`admin.Seed(db, user, pass)` 幂等收敛到引导状态：分区容器菜单 + 默认租户（`id="00000000-..."`）+ 超管角色（`key="admin"`, `is_super=true`, `data_scope="all"`）+ admin 用户 + 全量授权 + per-tenant `sys_schemas` 克隆；详见 [Seed 收敛契约](#seed-的收敛契约)

## 安装

admin 是独立子模块，按需引入：

```bash
go get github.com/sjlit/aeus/admin
```

依赖的框架组件：

```bash
go get github.com/sjlit/aeus/transport/http    # HTTP 服务（Gin）
go get github.com/sjlit/aeus/middleware/auth   # JWT 认证中间件
```

## 快速开始

```go
package main

import (
	"context"
	"log"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/admin/service"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open("admin.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// 1. 装配 admin Server：5 个可写 CRUD 资源 + 4 个只读安全/审计资源
	httpSrv := ghttp.New()
	s := admin.New(
		admin.WithDB(db),
		admin.WithOpenAPI(true),
		admin.WithRouter(httpSrv),
	)

	// 2. JWT 校验中间件:/auth/* 放行,其余接口校验 token、吊销状态与角色权限
	const secret = "change-me"
	tokenStore := service.NewMemoryTokenStore() // 多实例部署换共享实现
	httpSrv.Use(mwauth.JWT(
		func(*jwt.Token) (any, error) { return []byte(secret), nil },
		mwauth.WithClaims(auth.Claims{}), // 解析进 *auth.Claims(uid / role / tenant_id)
		mwauth.WithValidate(service.NewRevocationValidator(tokenStore, secret)), // 吊销 + token_type 门禁
		// 对 sys_permissions 已收录的 HTTP 路由执行角色权限校验
		mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
		// logout 也必须放行：expired access token 无法通过 JWT，
		// 但 handler 会自行校验 body 中的 token 并吊销 refresh token。
		mwauth.WithAllow("/auth/login", "/auth/refresh-token", "/auth/logout"),
	))

	// 3. 注册 CRUD 资源
	if err := s.Setup(context.Background()); err != nil {
		log.Fatal(err)
	}

	// 4. 认证服务需手动装配与注册（见设计约定）
	pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
		service.WithAuthServiceDB(db),
		service.WithAuthSecret(secret),
	))

	// 5. 用户业务 RPC
	pb.RegisterUserServiceRouter(httpSrv, service.NewUserService(
		service.WithUserServiceDB(db),
	))

	if err := httpSrv.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

启动后：

- 认证：`POST /auth/login`（`{"username":"alice","password":"..."}`）返回 `access_token` / `refresh_token`；
- 用返回的 access token 访问其余接口（`Authorization: Bearer <token>`）；
- 默认是**单租户**模式：不会安装 GORM 租户回调，适合快速起步。需要多租户时显式 `admin.WithMultiTenant(true)`（或 `WithTenantResolver(...)`）；启用后 JWT 中间件写入的 `claims.TenantID` 才会驱动 GORM 回调。

## P0 一键集成(搭配 dashboard/web)

admin 提供两条捷径,让 dashboard/web 这种 SPA 前端能跑通最小闭环:

```go
import (
    "context"
    "log"
    "os"

    "github.com/sjlit/aeus/admin"
    "github.com/sjlit/aeus/admin/pb"
    "github.com/sjlit/aeus/admin/service"
    ghttp "github.com/sjlit/aeus/transport/http"
)

// 1. admin.Server 通过 WithRouter 接管 HTTP server(任何 rest.Router 都行,
//    不必是 *transport/http.Server),Setup 内部完成 schema 迁移与 5 个
//    可写 CRUD + 4 个只读资源的注册。
httpSrv := ghttp.New()
s := admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
)
if err := s.Setup(context.Background()); err != nil { log.Fatal(err) }

// 2. AuthService 由应用自行装配:secret 必须从运行时通道取(env / KMS),
//    硬编码会泄露。
pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret(os.Getenv("JWT_SECRET")),
))

// 3. Seed:幂等收敛——确保超管角色/用户存在,并为所有超管角色补齐全量授权。
if err := admin.Seed(db, "admin", "admin123"); err != nil { log.Fatal(err) }

// 4. 启动
log.Fatal(httpSrv.Start(context.Background()))
```

> **Seed 与 Setup 的时序约定**：授权补齐以 Seed 调用时刻的全局目录（`sys_menus` + `sys_permissions`）为准，因此必须先 `Setup`（建目录）后 `Seed`（授全量）。若先调 `Seed`，目录尚空，本次只确保角色 + 用户，授权会留到下一次启动补齐；应用升级注册新模型后，同样靠下一次启动的 Seed 补齐。
>
> **接口鉴权依赖 JWT 中间件**：P0 捷径省略了中间件装配（token 校验与权限执行都在 `mwauth.JWT` 内完成），生产装配请按[快速开始](#快速开始)补上 `mwauth.JWT(...)`，其中 `WithPermissionChecker(admin.NewPermissionChecker(db))` 是 RBAC 生效的前提。PermissionChecker 默认 **fail-closed**——未收录路由自动拒绝；`WithCheckerAllowlist` 是显式 bypass（不是更严格），每个 bypass 都会在 code review 里可见。需要历史 fail-open 行为时用 `WithCheckerFailOpen(true)` 临时过渡。

### Seed 的收敛契约

`admin.Seed(db, user, pass)` 在事务里一次性收敛以下状态，每条都是**幂等**的：

1. **3 个分区容器菜单**（`EnsureSectionMenus`，Section 容器）：`FirstOrCreate` 写入 `SystemUserCenter`（用户中心）、`SystemLogs`（日志记录）、`SystemSettings`（系统设置），Icon 走 Element Plus 命名。后续所有内置模型的 `MenuEntry().Parent` 都引用这三个 Component；分区本身 `Parent=""`（无嵌套）；
2. **默认租户**：`FirstOrCreate` 写入 `id="00000000-0000-0000-0000-000000000000"`、`name="默认租户"`、`status="enabled"`，作为新角色的 tenant_id 与新用户的归属；
3. **超管角色**：`FirstOrCreate` 写入 `Key="admin"`、`Name="系统管理员"`、`Status="enabled"`、`Builtin=true`、`IsSuper=true`、`DataScope="all"`。已存在的 `Builtin && !IsSuper` 内置角色就地 `Update is_super=true`（兼容旧库升级）；非内置同名角色**不动**（运维自建）；
4. **孤儿租户回填**：若超管角色的 `tenant_id` 不等于默认 uuid，再 `FirstOrCreate` 一条 `Name="默认租户"`、`Status="enabled"` 的同名租户，修复历史孤儿 uuid（早于 tenant 模型存在的库）；
5. **超管用户**：`FirstOrCreate` 写入 `UID="admin"`、`Username=user`、`RoleKey="admin"`、`Password=pass`（`BeforeCreate` bcrypt），落到默认租户；
6. **超管角色全量授权**：找到所有 `is_super=true` 的角色，按 `sys_menus.component` 与 `sys_permissions.data` 的差集补齐 `sys_role_permissions`（**add-only**，已存在的 / 已被运维手动删的会自愈，不删除现存的）；
7. **per-tenant `sys_schemas` 克隆（仅多租户）**：`rest/v3` 在 `Setup` 阶段写 `sys_schemas` 时 `tenant_id` 留空（模板态）；启用多租户（`Setup` 已安装租户回调）时，Seed 把模板按 `(module, table, column)` 去重克隆到每个 active tenant，**add-only**（模板消失时保留旧副本；模板变更不覆盖现有副本）。单租户部署只保留全局模板，避免 schema 接口同时返回模板与副本导致字段重复。

> **不要在 `Setup` 之前调 `Seed`**：分区容器（步骤 1）走的是 `db.Create(&models.Menu{})`，需要 `Setup` 先 `AutoMigrate(&models.Menu{})`；授权补齐（步骤 6）需要 Setup 先建好 `sys_menus` 与 `sys_permissions`。否则步骤 1 失败，事务回滚；步骤 6 退化为空（无菜单可授）。
>
> **失败原子性**：整个 Seed 跑在单个 `db.Transaction` 里；任一步失败（如 bcrypt panic）回滚，库不会半 bootstrapped。

登录响应字段(`{code, message, data}` envelope,data 内;`expires` 是 TTL **相对秒**,7200 = 2h):

```json
{
  "code": 0,
  "message": "",
  "data": {
    "uid": "admin",
    "username": "admin",
    "expires": 7200,
    "access_token": "eyJ...",
    "refresh_token": "eyJ...",
    "tenant_id": "d3cf8ac4-...",
    "tenant_name": "d3cf8ac4-..."
  }
}
```

业务码分流(前端以 `body.code` 为主;401 同时触发静默 refresh,其余传输层状态码只做兜底提示):

| Code | HTTP | 语义 | 前端处理 |
|------|------|------|---------|
| 0 | 200 | OK | 成功 |
| 4002 | 401 | TokenExpired | refresh + 重试 |
| 4003 | 403 | PermissionDenied | toast 无权限（角色未授权该接口） |
| 4004 | 404 | NotFound | 业务提示 |
| 4005 | 403 | AccessDenied | 传输层（HTTP 403）视为会话失效；业务错误（HTTP 200，如旧密码错）只 toast |
| 1001 | 400 | Invalid | toast 后端 message |

> `HTTP` 列是 `errs.Error.HTTPStatus()` 的传输层映射；由 admin responder 处理的业务错误实际仍写 HTTP 200，前端以 `body.code` 为主。

## 深入文档

| 文档 | 内容 |
|---|---|
| [认证与会话](docs/guide/auth.md) | JWT Claims、登录安全、refresh 轮换、吊销链路、UserService |
| [接口权限校验（RBAC）](docs/guide/rbac.md) | `NewPermissionChecker`、请求标识、fail-closed 流程 |
| [dbcache](docs/guide/dbcache.md) | 读穿透缓存、版本标记、TTL、后端切换 |
| [租户隔离](docs/guide/tenant.md) | GORM 回调、Resolver 契约、边界情况 |
| [操作审计](docs/guide/audit.md) | 启用方式、机制、覆盖边界 |
| [数据模型与迁移](docs/guide/models.md) | 内置模型、Migrations、自定义模型注册 |
| [Vue Index.vue 自动生成](docs/guide/vue-generation.md) | 启用、生成内容、行为契约 |
| [前端管理台](docs/guide/web.md) | 后端契约、自动生成 vs 手写视图、开发联调 |
| [响应格式与设计约定](docs/guide/architecture.md) | 统一 envelope、AuthService 注册路径、租户隔离契约 |
| [API 参考](docs/api/README.md) | 全部 REST / RPC 端点与请求/响应格式 |
| [安全升级指南](docs/api/security-upgrade.md) | fail-open → fail-closed 迁移 |

Seed 的收敛契约、`AUTO_MIGRATE` / `SEED_ON_START` 等启动配置见上文
「[P0 一键集成](#p0-一键集成搭配-dashboardweb)」。

## 目录结构

```
admin/
├── auth/              # JWT Claims 类型与 ctx 提取（ClaimsFromContext / TenantIDFromContext）
├── middleware/        # Resolver 契约与默认实现 FromClaimsResolver
├── models/            # GORM 模型（9 个）+ MenuSpec / MenuProvider / 级联清理钩子
├── dbcache/           # 读穿透缓存 + SqlDependency 版本标记（PermissionChecker 用）
├── pb/                # proto 定义与生成代码（auth / user / role / permission / menu / tenant）
│   ├── auth.proto         # AuthService 定义
│   ├── user.proto         # UserService 定义
│   ├── role.proto         # RoleService 定义
│   ├── permission.proto   # PermissionService 定义
│   ├── menu.proto         # MenuService 定义
│   ├── tenant.proto       # TenantService 定义
│   └── *_http.pb.go       # 生成的路由注册代码
├── service/           # AuthService / UserService / RoleService / PermissionService / MenuService / TenantService 业务实现
├── third_party/       # protoc 依赖的 google api / validate proto
├── web/               # Vue 3 + Element Plus 管理后台骨架（@sjlit/rest-ui 驱动 SchemaViewer）
├── cmd/mock/          # demo 启动器（ScopeContext + dev-only secret/seed defaults）
├── docs/              # 指南（docs/guide/）与接口契约（docs/api/）
├── server.go          # Server 装配：租户回调安装 + 资源注册 + 端点挂载
├── tenant_scope.go    # GORM 租户回调（aeus:tenant:*）
├── derive.go          # 推导 Component/Uri/ViewPath + ensureMenuRow/ensurePermissionRows
├── permission.go      # NewPermissionChecker:对已收录路由执行角色权限校验(JWT 中间件钩子)
├── register_model_options.go   # RegisterModel per-call 选项（WithRegisterMenuSpec / Scenarios / VueOutputDir）
├── vue_gen.go         # 自动生成 Vue Index.vue（vueTemplate / vuePathForModel / buildVueContent）
├── schema_endpoint.go        # RegisterSchemaEndpoint（GET /schema/:module/:table）
├── modeltypes_endpoint.go    # RegisterModelTypesEndpoint（GET /rest/model-types/:module/:table）
├── modeltiers_endpoint.go    # RegisterModelTiersEndpoint（GET /rest/model-tiers/:module/:table）
├── options.go         # Functional options
├── responder.go       # 默认 envelope（{code,message,data}）
├── seed.go            # admin.Seed 收敛引导 + EnsureSectionMenus 分区容器
├── errors.go          # 错误定义（ErrHTTPRequired / ErrDBRequired / ErrRouterRequired）
└── Makefile           # proto 代码生成
```

## 开发指南

### 重新生成 pb 代码

修改 `pb/*.proto` 后：

```bash
make proto         # 依赖 protoc + protoc-gen-go + protoc-gen-go-aeus + protoc-gen-validate
make proto-clean   # 清理生成文件
```

### 测试

```bash
go test ./...
```

测试覆盖：

- `setup_test.go` / `user_test.go` / `tenant_server_test.go` — `Server.Setup` 资源注册、迁移回调、Responder 集成、端到端 HTTP 调用（登录 + JWT 中间件 + CRUD 资源）
- `security_guards_test.go` — super 角色/用户的目标级保护、角色变更仅 super、UID/Username 跨字段唯一、单租户 REST 创建第二个 admin 后登录
- `tenant_scope_test.go` — GORM 租户回调（Query/Update/Delete/Create/强制租户）
- `permission_test.go` — `NewPermissionChecker`：已授权放行 / 未授权 4003 / 未收录路由默认**拒绝**（fail-closed）/ 自助路由放行 / 基础设施路由放行 / 显式 allowlist bypass / `WithCheckerFailOpen(true)` 兼容 / allowlist 通配段边界 / 非 http 跳过 / 跨租户不串权 / claims 类型不符 4005 / 缓存命中
- `schema_endpoint_test.go` — `RegisterSchemaEndpoint`：路径解析、pre-condition 失败、HTTP 端到端、未知 module/table、Seed 后字段不重复、租户过滤
- `modeloptions_endpoint_test.go` — `RegisterModelTypesEndpoint` + `RegisterModelTiersEndpoint`：路径解析、pre-condition 失败、HTTP 端到端（成功/缺失必填 query/未知 valueType/未知 module/table/路由注册）、`queryModelTypes` / `queryModelTiers` 分派器单元测试
- `register_model_options_test.go` — `RegisterModel` per-call 选项（`WithRegisterMenuSpec` / `WithRegisterScenarios` / `WithRegisterVueOutputDir`）的覆盖语义、显式空 spec / 空 scenarios 的边界
- `vue_gen_test.go` — `buildVueContent` / `vuePathForModel` / `generateVueFile` / `generateVueForResource`：模板占位符替换、路径遍历保护、已存在文件跳过、失败 warn 不中断
- `seed_test.go` / `seed_tenant_test.go` — `admin.Seed` 幂等性、全量授权、启动补齐/自愈、参数校验、Seed+Login 联动、固定 resolver 下的授权租户、跨租户孤儿回填、多租户 `sys_schemas` 克隆 / 单租户不克隆
- `responder_test.go` — envelope 解包、wrapped error 链透传、`{code,message,data}` 形状
- `models/menu_method_test.go` / `models/permission_method_test.go` / `models/role_method_test.go` / `models/tenant_test.go` / `models/user_test.go` / `models/loginlog_test.go` — `Menu.BuildTree` / `Role.BeforeUpdate` Key 同步 + 软删级联 / `Role.AfterDelete` 硬删级联 / `Permission.ListByType` / `Tenant.BeforeCreate` uuid 回填 / `User.ValidatePassword` / `LoginLog.BeforeCreate` token 哈希 等模型方法的单测
- `dbcache/cacher_test.go` / `dbcache/depend_test.go` — 读穿透缓存命中 / miss / reload / 版本标记失配重载 / 宽限窗 / 共享 cache 后端
- `service/auth_test.go` — AuthService 全流程（登录/刷新/登出/钩子/无状态模式）
- `service/user_test.go` / `service/user_cache_test.go` — UserService RPC + 缓存场景
- `service/role_test.go` / `service/role_cache_test.go` — RoleService 端到端（含 `ReplacePermissions` 事务 + 缓存）
- `service/menu_test.go` / `service/menu_cache_test.go` — MenuService `MenuTree` / `MenuOptions` / `MenuBreadcrumb` 集成
- `service/permission_test.go` — PermissionService `ListCatalog` 集成
- `service/tenant_test.go` / `service/tenant_login_test.go` — TenantService + 登录态 tenant 校验
- `service/convert_test.go` / `service/memory_token_store_test.go` — 类型转换、内存 TokenStore
