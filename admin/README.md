# AEUS admin — 通用后台管理模块

**项目地址**: github.com/sjlit/aeus/admin

admin 是 AEUS 框架的通用后台管理领域模块，基于 [rest/v3](https://github.com/sjlit/rest) + GORM，为应用提供即开即用的系统管理能力：

- **通用 REST CRUD**：用户、角色、部门、菜单、权限、角色-权限中间表、审计日志、登录日志 8 个模型自动注册为 REST 资源（列表/搜索、详情、创建、更新、删除、导出、OpenAPI 文档）；
- **JWT 认证**：登录 / 刷新 / 登出（`AuthService`），支持 Token 吊销与登录前后钩子；
- **RBAC 接口鉴权**：`NewPermissionChecker` 对已收录进 `sys_permissions` 的 HTTP 路由按角色校验权限，未收录路由默认放行（详见[接口权限校验](#接口权限校验rbac)）；
- **用户业务 RPC**（`UserService`）：个人资料、改密、管理员重置密码、头像、可见菜单、权限码；
- **租户隔离**：通过 GORM 回调自动为所有租户模型追加 `tenant_id` 过滤与回填，业务代码零侵入。

## 特性

- **声明式 CRUD**：模型上的 `scenarios` / `rule` / `enum` / `format` / `live` 标签直接驱动接口的字段可见性、校验规则与前端表单渲染
- **自动密码哈希**：`models.User` 在 `BeforeCreate` / `BeforeUpdate` 中对密码做 bcrypt 哈希（对已哈希值幂等）
- **密码策略**：8-32 位字母+数字基线（`CheckPasswordPolicy`），所有写密码路径经 GORM 钩子单点收口
- **级联清理**：删除菜单/角色时自动清理 `sys_role_permissions` 中的关联权限；角色 Key 变更自动同步权限引用
- **统一响应格式**：内置 responder 输出 `{code, message, data}` envelope（成功 `code=0`），也可用 `WithResponder` 替换
- **可选的 OpenAPI**：`WithOpenAPI(true)` 后每个资源暴露 `openapi.json`
- **可插拔认证**：`AuthService` 始终由应用自行 `pb.RegisterAuthServiceRouter(...)` 注册，secret 必须来自运行时通道（见[设计约定](#设计约定)）
- **自动菜单注册**：模型实现 `MenuProvider` 时，`Setup` / `RegisterModel` 会按 `MenuEntry()` 自动生成 `sys_menus` 行（`Component` / `Uri` / `ViewPath` 在缺失时由模块名 + 表名推导，可显式覆盖）
- **自动权限目录**：每个挂载模型按 `rest.ScenarioProvider` 声明的场景集生成 `sys_permissions` 行（`Data` 形如 `"<METHOD> <URI>"`），重复启动幂等
- **接口权限执行**：`NewPermissionChecker` 把 HTTP 请求的 `"METHOD <路由模板>"` 与权限目录、角色授权逐一比对（详见[接口权限校验](#接口权限校验rbac)）
- **公开 `RegisterModel`**：应用自定义模型可与内置模型走同一条迁移+菜单+权限注册路径
- **一键引导**：`admin.Seed(db, user, pass)` 幂等收敛到引导状态：超管角色（`key="admin"`, `is_super=true`）+ admin 用户 + 全量授权，每次启动自动补齐新增的菜单/权限

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

	// 1. 装配 admin Server：租户回调 + 8 个 CRUD 资源
	httpSrv := ghttp.New()
	s := admin.New(
		admin.WithDB(db),
		admin.WithOpenAPI(true),
		admin.WithRouter(httpSrv),
	)

	// 2. JWT 校验中间件:/auth/* 放行,其余接口校验 token 与角色权限
	const secret = "change-me"
	httpSrv.Use(mwauth.JWT(
		func(*jwt.Token) (any, error) { return []byte(secret), nil },
		mwauth.WithClaims(auth.Claims{}), // 解析进 *auth.Claims(uid / role / tenant_id)
		// 对 sys_permissions 已收录的 HTTP 路由执行角色权限校验
		mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
		mwauth.WithAllow("/auth/login", "/auth/refresh-token"),
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
- 默认租户隔离即已生效：JWT 中间件写入 `claims.TenantID`，GORM 回调自动按该租户过滤。

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
//    不必是 *transport/http.Server),Setup 内部完成 schema 迁移与 8 个
//    CRUD 资源注册。
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
> **接口鉴权依赖 JWT 中间件**：P0 捷径省略了中间件装配（token 校验与权限执行都在 `mwauth.JWT` 内完成），生产装配请按[快速开始](#快速开始)补上 `mwauth.JWT(...)`，其中 `WithPermissionChecker(admin.NewPermissionChecker(db))` 是 RBAC 生效的前提。

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

业务码分流(前端按 `body.code`,不依赖 HTTP status):

| Code | HTTP | 语义 | 前端处理 |
|------|------|------|---------|
| 0 | 200 | OK | 成功 |
| 4002 | 401 | TokenExpired | refresh + 重试 |
| 4003 | 403 | PermissionDenied | 提示无权限（角色未授权该接口）/ 跳登录页 |
| 4004 | 404 | NotFound | 业务提示 |
| 4005 | 403 | AccessDenied | 跳登录页 |
| 1001 | 400 | Invalid | toast 后端 message |

## 认证与会话

### JWT Claims

`auth.Claims` 内嵌 `jwt.RegisteredClaims`（标准 `exp` / `nbf` / `iat`），自定义字段：

| 字段 | JSON | 说明 |
|------|------|------|
| `UID` | `uid` | 用户工号 |
| `Role` | `uro` | 角色编码 |
| `TenantID` | `tid` | 租户 ID |
| `TokenType` | `token_type` | 令牌类型：`access` / `refresh`（RefreshToken 只接受 `refresh`） |

处理器内通过 `auth.ClaimsFromContext(ctx)` 读取；`auth.TenantIDFromContext(ctx)` 返回租户 ID（无 claims 时为空串）。

### AuthService

| 选项 | 说明 |
|------|------|
| `WithAuthServiceDB(db)` | GORM 句柄（必填） |
| `WithAuthSecret(secret)` | JWT 签名密钥（必填，登录/刷新/登出依赖） |
| `WithTokenExpireSeconds(n)` | access token 有效期，默认 **2h**（非正数按默认处理） |
| `WithTokenStore(store)` | 吊销存储（`Put`/`Del`）；nil 表示无状态模式，logout 不吊销 |
| `WithBeforeLogin(fn)` | 登录前置钩子：校验通过后、凭据校验前执行，返回 error 则短路登录 |
| `WithAfterLogin(fn)` | 登录成功钩子：仅作通知，返回值忽略 |
| `WithLoginLogger(fn)` | 登录审计：成功与失败都回调（IP/UA/Username + 成功时的 access token），**同步**执行；panic 透传到 Login 调用方，建议 recorder 自己 `defer recover()` |

- refresh token 固定 **48h**，由 `RefreshToken` 换取新的 access token；
- `Login` 在凭据校验前不依赖 JWT claims，天然跨租户查找用户。
- 失败路径上的 `Uid`/`TenantID` 留空以避免泄漏用户存在性；同 username 错密码时仍会带上 `Uid` 用于审计聚合。

### 登录安全

**状态校验**：`Login` 与 `RefreshToken` 在凭据验证通过后都会复查用户与角色状态：

| 条件 | 结果 |
|------|------|
| 用户 `status=disabled` | `4003 PermissionDenied`（`user is disabled`） |
| 角色 `status=disabled` | `4003 PermissionDenied`（`role is disabled`） |
| 角色行不存在 | Login：`4003`（`role not found`）；RefreshToken：`4005 AccessDenied` |

- 状态检查在**密码/令牌验证之后**执行，不扩大用户名枚举面（消息与 `tenant is disabled` 同一先例）；
- `/auth/*` 在 JWT allowlist 上、ctx 无 claims，因此 Login / RefreshToken 的状态查询**显式过滤 `tenant_id`**，不依赖 GORM 租户回调；
- 已签发的 access token 到期前仍有效（JWT 无状态）；禁用账户的 48h refresh 续期窗口随 `RefreshToken` 复查而关闭。

**密码策略**（`models.CheckPasswordPolicy`）：8-32 位、仅字母数字、必须同时含字母和数字。所有写密码路径（REST 创建、改密、重置密码）经 `User` 的 `BeforeCreate` / `BeforeUpdate` 钩子单点收口，违规返回 `1001 Invalid`；`User.Password` 的 `rule` 标签（`^[A-Za-z0-9]{8,32}$`）负责 REST 层长度+字符集校验与前端表单渲染（RE2 无 lookahead，字母+数字组合由钩子兜底）；`ChangePassword` 另要求新密码与旧密码不同。已哈希值（`$2` 前缀）与空密码自动跳过策略，幂等语义与 `hashPassword` 一致。

> **待办**：暴力破解防护（登录限流 / 账户锁定）尚未实现——`WithLoginLogger` 已预留失败尝试钩子，可供未来的限流决策消费。

### UserService

| 选项 | 说明 |
|------|------|
| `WithUserServiceDB(db)` | GORM 句柄（必填） |
| `WithUserServiceAdminRole(role)` | 可执行 `ResetPassword` 的角色编码，默认 `admin` |

所有 RPC 均从 JWT claims 读取调用者身份，请求体不携带目标用户（`ResetPassword` 除外）。

### 接口权限校验（RBAC）

`admin.NewPermissionChecker(db)`（`permission.go`，db 为 nil 时 panic，与其它 Service 构造器一致）返回 JWT 中间件的 `PermissionCheckerFunc`，对**已收录进 `sys_permissions` 目录**的 HTTP 路由执行角色权限校验：

```go
mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
```

**执行时机**：checker 在 JWT 中间件 **token 验证通过之后、claims 写入 ctx 之前**运行（`middleware/auth/jwt.go`）——它以参数形式接收解析出的 claims，**不能**依赖 `auth.ClaimsFromContext`；后续 handler 与 GORM 租户回调才从 ctx 读 claims。

**请求标识**：`metadata.RequestMethod + " " + metadata.RequestPath`。transport/http 写入的 `RequestPath` 是 gin 的 **FullPath 路由模板**（如 `PUT /system/sys_user/:id`），与 `permissionCode` 生成 `sys_permissions.data` 的 `"<METHOD> <URI>"` 格式同构，二者精确匹配。

**判定流程**（仅 `RequestProtocol == "http"` 时生效，CLI / 其它协议直接放行）：

| 步骤 | 条件 | 结果 |
|------|------|------|
| 1. 目录查询 | `sys_permissions` 存在 `type=api` 且 `data` 匹配的行 | 进入步骤 2；查无此行为**放行**（fail-open） |
| 2. 授权查询 | `sys_role_permissions` 存在 `role_key + tenant_id + type=permission + data` 匹配的行 | **放行** |
| 3. 否则 | 目录内但角色无授权 | **拒绝**：业务码 `4003 PermissionDenied` |

**设计要点**：

- **fail-open 默认放行**：未收录路由（业务 RPC：`/user/menus`、`/role/options` 等）不受影响，由服务自身逻辑（如 `ResetPassword` 的 admin 角色检查）或应用中间件控制；
- **租户隔离显式化**：授权查询显式过滤 `tenant_id`——checker 运行时 claims 尚未进入 ctx，GORM 租户回调不会自动过滤，不显式传会导致跨租户串权；
- **fail-closed 接线保护**：claims 不是 `*auth.Claims` 时直接拒绝（`4005 AccessDenied`），把中间件接线错误暴露在请求上而不是静默放行；
- **边界**：`type=button` / `type=data_scope` 的权限不参与接口鉴权；`/auth/*` 在 `WithAllow` 列表上直接短路，不进 checker；
- **超管授权窗口**：角色授权由 `admin.Seed` 每次启动补齐（以调用时刻的全局目录为准），新模型注册后需先 `Setup` 后 `Seed`，见[P0 一键集成](#p0-一键集成搭配-dashboardweb)的时序约定。

测试覆盖见 `permission_test.go`：已授权放行 / 未授权 4003 / 未收录路由放行 / 非 http 跳过 / 跨租户不串权 / claims 类型不符 4005。

## 租户隔离

### 机制

`admin.Server.Setup` 在启动时通过 `installTenantScope` 安装 4 个 GORM 回调（注册名 `aeus:tenant:*`）：

- **Query / Update / Delete**：在 SQL 生成前追加 `WHERE tenant_id = ?`，与业务 WHERE 合入同一条语句；
- **Create**：在业务 `BeforeCreate` 钩子**之后**执行，为空白的 `TenantID` 回填 resolver 返回的租户 ID（显式设置的值优先）。

回调按**列名**（`tenant_id`）而非 struct 嵌入判断，因此不依赖 admin 的 `models` 包——应用自建的租户模型同样自动生效。是否带 `tenant_id` 列的结果按 `*schema.Schema` 缓存，热路径上只有一次 map Load。

### Resolver 契约

`middleware.Resolver` 是 `func(ctx context.Context) string`，每次 DB 操作调用一次：

- 返回**非空**租户 ID：读写删除自动限定该租户，创建自动回填；
- 返回 **""**：完全不做租户过滤（跨租户工具、后台任务、`AuthService.Login` 的出口）。

Resolver **必须廉价且不可 panic**——JWT 缺失时返回 `""` 即可，认证决策由中间件负责。

### 默认值与覆盖

默认 resolver 是 `middleware.FromClaimsResolver`：直接读取 `auth.Claims.TenantID`（由 JWT 中间件写入 ctx），因此开箱即用的装配（`admin.New` + JWT 中间件 + `Setup`）即实现端到端租户隔离，**无需额外中间件**。

覆盖示例（优先信任 `X-Tenant-Id` 请求头，兜底 JWT 租户，适用于超管工具）：

```go
admin.WithTenantResolver(func(ctx context.Context) string {
	if h := metadata.Get(ctx, "X-Tenant-Id"); h != "" {
		return h
	}
	return middleware.FromClaimsResolver(ctx)
})
```

> 注意：上述示例会盲目信任请求头，请在上游（如仅限超管的路由中间件）限制该请求头的来源。

### 边界情况

- **`Menu` / `Permission` / `Tenant` 无租户**：这三个模型没有 `tenant_id` 列，全局共享，GORM 回调自动跳过；
- **级联清理不回绕**：`purgePermissions` 使用 `NewDB + SkipHooks` 的独立会话，避免删除权限时再次触发租户回调与递归钩子；
- **rest/v3 侧同步**：注册资源时把 resolver 注入 `ResourceConfig.TenantResolve`，Search / Export 查询路径自动带 `tenant_id` 过滤——**仅对含 `tenant_id` 列的模型注入**（rest/v3 无条件追加该过滤，注入给全局模型会产生非法 SQL）。

## 数据模型

| 模型 | 表名 | 租户 | 说明 |
|------|------|------|------|
| `Tenant` | `sys_tenants` | ❌ | 租户实体：`id`（char(60) 字符串主键，即各表 `tenant_id` 引用的值；创建时留空由 `BeforeCreate` 生成 uuid）、`name`、`status`（`enabled` / `disabled`，登录时校验）。全局可见，无 `tenant_id` 列 |
| `User` | `sys_users` | ✅ | 用户：`uid`（工号，唯一）、`username`、`role_key`、`dept_id`、bcrypt `password`、`avatar`、`status`、`email`、`gender`、`description` |
| `Role` | `sys_roles` | ✅ | 角色：`name` + `key`（机器标识，唯一）+ `builtin` + `is_super`（超管：授权由 Seed 自动管理，禁手动修改）。`key` 变更自动同步 `sys_role_permissions`；删除自动清理关联权限 |
| `Menu` | `sys_menus` | ❌ | 菜单树：`parent`（父级菜单 Component，全局共享）、`name`（唯一，标题）、`component`（唯一，标识）、`uri`、`hidden` / `public`。删除自动清理 `sys_role_permissions` 中 `type=menu` 的行 |
| `Department` | `sys_departments` | ✅ | 部门树：`parent_id` + `name` |
| `Permission` | `sys_permissions` | ❌ | 全局权限目录：`type`（`api` 接口 / `button` 按钮 / `data_scope` 数据范围）+ `data`（权限标识，全租户共享）+ `description` |
| `RolePermission` | `sys_role_permissions` | ✅ | 角色-权限中间表：`role_key`（角色 Key）+ `type`（`menu` 菜单 / `permission` 权限）+ `data`（菜单 Component / 权限标识），按租户生效 |
| `Audit` | `sys_audits` | ✅ | 审计日志：`uid`、`action`、`module`、`table`、`data` |
| `LoginLog` | `sys_login_logs` | ✅ | 登录日志：`uid`、`ip`、`browser`、`os`、`platform`、`access_token`、`user_agent` |

所有模型在 `Setup` 时由 rest/v3 自动 `AutoMigrate`，无需手动迁移。模型上的 `scenarios`、`rule`、`enum` 等标签驱动 rest/v3 的字段可见性与校验（如 `User.Uid` 的 `rule:"required;unique;regexp:^[a-zA-Z0-9]{3,8}$"`）。

## Migrations

Schema is governed entirely by GORM `AutoMigrate` during `Setup` —
models registered in `Server.getModels()` create tables and add columns
on every startup.  There is no hand-written SQL migration layer in this
module (the project has not shipped, so destructive re-creates are
acceptable over preserving historical data).

## API 一览

### URL 形态约定

admin 模块所有业务 RPC（`AuthService` / `UserService` / `RoleService` / `PermissionService` / `MenuService`）遵循同一规则：

| HTTP 方法 | 参数位置 | 说明 |
|---|---|---|
| **GET** | query string | 资源定位 + 过滤参数全部走 `?key=value`，由 transport/http `ctx.Bind` 从 query 串绑定到 proto 字段 |
| **PUT** / **POST** | request body（`body: "*"`） | 资源标识 + 写入字段全部在 JSON body，客户端 `Content-Type: application/json` |

根因：gin v1.12 只认 `:`/`*` 通配，`{role}` 路径段永不匹配；protoc-gen-go-aeus 把 `google.api.http` 注解原样抄进 gin 路由，而 `?` 在 gin 里被当作字面量注册成死路由。所以 proto 注解只写路径（如 `get: "/role/permissions"`），wire URL 中的 query 串 / body 在 README 与代码注释中显式记录。

后续新增 RPC 沿用同一约定即可。`rest/v3` 通用 CRUD 路径（`/system/sys_user/{id}` 等 gin `:id` 形式）由 rest/v3 自己生成，不在本约定范围内。

#### 命名约定（2026-08-10 起生效）

- **URL 根 = 资源域，与实现 Service 一一对应**：`/auth` → AuthService（会话）、`/user` → UserService（当前用户自助）、`/role` → RoleService（角色管理）、`/menu` → MenuService（菜单管理）、`/permission` → PermissionService（全局权限目录）、`/tenant` → TenantService（租户管理）
- **复数 = 集合端点**；同一域下动作由 HTTP method + 子路径表达
- **"角色的东西"归 `/role/*`**（options / menus / permissions）；**"自己的东西"归 `/user/*`**（profile / menus / permissions / password / avatar）
- **路径 ≥ 2 级**；下拉数据（`{value, label}` 形状）用 `options`，树形数据用 `tree`，面包屑用 `breadcrumb`；集合 / 列表读用 `List*`，单资源 / 固定结构读用 `Get*`
- **模块级命名空间由挂载点提供**：admin 模块设计为挂载在网关之下（如反向代理 `/admin/*` → 本服务），模块内部保持短路径；应用直接暴露时可自行在网关层加前缀，业务 RPC 路径无需改动

### 通用 CRUD（由 rest/v3 生成，ModuleName = `system`）

资源名由**表名**派生（singular / plural），以 `User`（表 `sys_users`）为例：

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/system/sys_users` | 列表 / 搜索 |
| `GET` | `/system/sys_user/detail/:id` | 详情 |
| `POST` | `/system/sys_user` | 创建 |
| `PUT` | `/system/sys_user/:id` | 更新 |
| `DELETE` | `/system/sys_user/:id` | 删除 |
| `GET` | `/system/sys_user/export` | 导出 |
| `GET` | `/system/sys_user/openapi.json` | OpenAPI 文档（需 `WithOpenAPI(true)`） |

全部资源一览：

| 资源 | 创建 / 详情 / 更新 / 删除 / 导出 | 列表 / 搜索 |
|------|------|------|
| `User` | `/system/sys_user` | `/system/sys_users` |
| `Role` | `/system/sys_role` | `/system/sys_roles` |
| `Menu` | `/system/sys_menu` | `/system/sys_menuses` |
| `Department` | `/system/sys_department` | `/system/sys_departments` |
| `Permission` | `/system/sys_permission` | `/system/sys_permissions` |
| `RolePermission` | `/system/sys_role_permission` | `/system/sys_role_permissions` |
| `Audit` | `/system/sys_audit` | `/system/sys_audits` |
| `LoginLog` | `/system/sys_login_log` | `/system/sys_login_logs` |

> 注意 `Menu` 的复数是 `sys_menuses`：复数是 rest/v3 对表名的机械转换（`inflector.Pluralize`），不做语义化处理。

### AuthService

| 方法 | 路径 | 请求体 | 说明 |
|------|------|--------|------|
| `POST` | `/auth/login` | `{username, password}` | 登录，返回 `{uid, username, expires, access_token, refresh_token}` |
| `POST` | `/auth/refresh-token` | `{refresh_token}` | 用 refresh token 换取新 access token |
| `POST` | `/auth/logout` | `{access_token}` | 登出（配置 TokenStore 时同步吊销） |

### UserService

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/user/profile` | 当前用户资料 |
| `PATCH` | `/user/profile` | 更新资料（username / email / gender / description） |
| `POST` | `/user/change-password` | 修改自己的密码 `{old_password, new_password}` |
| `POST` | `/user/reset-password` | 管理员重置他人密码 `{target_uid, new_password}`（需 `admin` 角色） |
| `POST` | `/user/set-avatar` | 设置头像 URL `{avatar_url}`（≤1024 字符） |
| `GET` | `/user/menus` | 当前角色可见菜单（扁平列表，前端按 `parent` 重建树） |
| `GET` | `/user/permissions` | 当前角色的 api 权限码列表 |

### RoleService / PermissionService / MenuService

角色-权限-菜单管理 RPC（`pb/role.proto`、`pb/permission.proto`、`pb/menu.proto`）。这些服务与 `AuthService` 一样需应用自行装配：

```go
pb.RegisterRoleServiceRouter(httpSrv, service.NewRoleService(service.WithRoleServiceDB(db)))
pb.RegisterPermissionServiceRouter(httpSrv, service.NewPermissionService(service.WithPermissionServiceDB(db)))
pb.RegisterMenuServiceRouter(httpSrv, service.NewMenuService(service.WithMenuServiceDB(db)))
```

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/role/options` | 角色下拉项 `[{value: key, label: name}]`（按 key 升序） |
| `GET` | `/role/permissions?role=…[&type=…]` | 某角色的权限（详见下方 type 语义） |
| `PUT` | `/role/permissions` | 整体替换某角色权限，body: `{"role":"admin","menus":[菜单 Component...],"apis":[权限码...]}` |
| `GET` | `/role/menus?role=…` | 某角色可见菜单（扁平，`name` = 菜单标题） |
| `GET` | `/permission/catalog?type=…` | 全局目录权限码（可选 `?type=` 过滤） |
| `GET` | `/menu/tree` | 完整菜单树（节点 `name` = 菜单 **Component**，非标题） |
| `GET` | `/menu/options` | 层级下拉项（`value` = 菜单 **Component**，可直接作为 `parent` 提交） |
| `GET` | `/menu/breadcrumb?id=…` | Component 链到根的路径（面包屑） |

> **枚举 query 参数**：`PermissionType`（`type`）是 proto 枚举，**只能传数字**（`0/1/2/3/4` = `PERMISSION_TYPE_UNSPECIFIED/MENU/API/BUTTON/DATA_SCOPE`）；字符串形式 `?type=menu` 无法通过 `MapFormWithTag` 绑定，会得到绑定错误。`role` / `id` 传字符串 / 数字字符串即可。详见[URL 形态约定](#url-形态约定)。
>
> **`GET /role/permissions` 的 type 过滤语义**：`type` 缺省（`UNSPECIFIED`）返回 `{menus, apis}` 合读（角色编辑页一屏展示）；`type=1`（MENU）只返菜单 Component、`apis` 为空；`type=2`（API）只返权限码、`menus` 为空。原 `GET /permission/role` 已于 2026-08-10 合并至本端点。
>
> **`GET /permission/catalog` 的 type 过滤语义**：菜单已不在全局目录中，因此 `type=1`（MENU）**返回全部目录权限码**（退化为 "all"）；要过滤 api 请用 `type=2`（API）。

### TenantService

租户管理 RPC（`pb/tenant.proto`），与其它 Service 一样需应用自行装配：

```go
pb.RegisterTenantServiceRouter(httpSrv, service.NewTenantService(service.WithTenantServiceDB(db)))
```

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/tenant/options` | 全量租户下拉项 `[{id, name}]`（按 name 升序，含 disabled 租户——状态只控制登录，不控制展示） |
| `GET` | `/tenant/detail?id=…` | 按 id 查租户信息 `{id, name, status, created_at}`，不存在返回 404 |

租户 CRUD 走 rest/v3 通用路径 `/system/sys_tenants`（同 User/Role 形态）。

> **⚠️ 访问控制（应用方必须处理）**：`Tenant` 模型**没有 `tenant_id` 列**，租户隔离回调对其天然跳过，因此 `/system/sys_tenants` 与 `/tenant/*` 对**任意通过 JWT 中间件的用户可见**。admin 模块不做鉴权决策，请应用方给这些路由挂 super-admin 专属中间件，例如（IsSuper 检查与 `UserService.ResetPassword` 同款，见 `service/user.go`）：
>
> ```go
> // 按路径前缀拦截租户管理路由,仅允许 IsSuper 角色访问
> func superAdminOnly(db *gorm.DB) middleware.Middleware {
>     return func(next middleware.Handler) middleware.Handler {
>         return func(ctx context.Context) error {
>             path, _ := metadata.Get(ctx, metadata.RequestPath)
>             if !strings.HasPrefix(path, "/system/sys_tenants") &&
>                 !strings.HasPrefix(path, "/tenant/") {
>                 return next(ctx)
>             }
>             claims, ok := auth.ClaimsFromContext(ctx)
>             if !ok {
>                 return errs.ErrAccessDenied
>             }
>             var role models.Role
>             if err := db.WithContext(ctx).Where("key = ?", claims.Role).
>                 First(&role).Error; err != nil || !role.IsSuper {
>                 return errs.ErrAccessDenied
>             }
>             return next(ctx)
>         }
>     }
> }
> // 与 JWT 中间件一起挂载(顺序:JWT 在前,claims 已写入 ctx)
> http.Use(mw.JWT(/* ... */), superAdminOnly(db))
> ```

`AuthService.Login` 内置租户状态校验：`sys_tenants` 中 `status=disabled` 的租户一律拒绝登录（`PermissionDenied`）；表不存在（旧库）或查无该租户行（历史孤儿 uuid）时放行，由 Seed 在启动时收敛出实体行。

## 响应格式

默认 responder 统一输出 `{code, message, data}` envelope；业务结果由 `body.code` 决定：

```json
// 成功（code = errors.OK = 0）
{"code": 0, "message": "", "data": { ... }}

// 业务失败（message 字段直接给前端 toast 文案）
{"code": 4003, "message": "permission denied"}
```

业务码定义见 [`pkg/errs/const.go`](pkg/errs/const.go)（`OK/Invalid/NotFound/PermissionDenied/AccessDenied/TokenExpired` …）。前端拦截器按 `body.code` 分流（不依赖 HTTP status），分流表见下方"业务码分流"。可通过 `admin.WithResponder(...)` 替换为应用自己的 responder。

> HTTP 状态码约定：成功路径固定 `200`；失败路径由 `transport/http` 的 error interceptor 根据 `*errors.Error.HTTPStatus()` 映射为 `400/401/403/404/500/504/503`，**业务码才是客户端分流的依据**，HTTP status 仅用于网关/浏览器层。

## 设计约定

### AuthService 注册路径

`Server.Setup` **不**自动注册 `AuthService`。应用始终自行装配：

```go
pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret(os.Getenv("JWT_SECRET")),
))
```

这样生产应用保留对 `AuthSecret`、`TokenStore` 与 TTL 的完整控制权，admin 不会把任何密钥默认值烘焙进二进制。`cmd/mock` 是一个完整示例：见 `admin/cmd/mock/service.go` Init。

### 租户隔离是数据层契约

隔离由 GORM 回调层实现，不是 HTTP 中间件——不依赖路由结构、调用链中是否经过中间件，只依赖操作 ctx 中是否有 claims。`AuthService.Login`（无 claims）天然运行在未隔离模式，而任何带 claims 的请求天然被隔离。

## 目录结构

```
admin/
├── auth/              # JWT Claims 类型与 ctx 提取（ClaimsFromContext / TenantIDFromContext）
├── middleware/        # Resolver 契约与默认实现 FromClaimsResolver
├── models/            # GORM 模型（8 个）+ MenuSpec / MenuProvider / 级联清理钩子
├── pb/                # proto 定义与生成代码（auth / user / role / permission / menu）
│   ├── auth.proto         # AuthService 定义
│   ├── user.proto         # UserService 定义
│   ├── role.proto         # RoleService 定义
│   ├── permission.proto   # PermissionService 定义
│   ├── menu.proto         # MenuService 定义
│   └── *_http.pb.go       # 生成的路由注册代码
├── service/           # AuthService / UserService / RoleService / PermissionService / MenuService 业务实现
├── third_party/       # protoc 依赖的 google api / validate proto
├── cmd/mock/          # demo 启动器（ScopeContext + dev-only secret/seed defaults）
├── docs/              # 设计文档、INTEGRATION-TODO、设计约定 spec
├── server.go          # Server 装配：租户回调安装 + 资源注册 + 可选 AuthService
├── tenant_scope.go    # GORM 租户回调（aeus:tenant:*）
├── menu_derive.go     # 注册时按 ModuleName+TableName 推导 sys_menus 行
├── permission_derive.go # 注册时按 ScenarioProvider 推导 sys_permissions 行
├── permission.go      # NewPermissionChecker:对已收录路由执行角色权限校验(JWT 中间件钩子)
├── option.go          # Functional options
├── responder.go       # 默认 envelope（{code,message,data}）
├── seed.go            # admin.Seed 收敛引导（超管角色/用户 + 全量授权补齐）
├── types.go           # 错误定义（ErrHttpRequired / ErrDBRequired）
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

- `admin_test.go` — `Server.Setup` 资源注册、迁移回调、Responder 集成
- `tenant_scope_test.go` — GORM 租户回调（Query/Update/Delete/Create/回填）
- `menu_derive_test.go` — `MenuSpec` 推导、Component/Uri/ViewPath 覆盖规则、孤儿 parent 校验
- `permission_derive_test.go` — `permissionDataPattern` 正则、`ensurePermissionRows` 幂等
- `permission_test.go` — `NewPermissionChecker`：已授权放行 / 未授权 4003 / 未收录路由放行 / 非 http 跳过 / 跨租户不串权 / claims 类型不符 4005
- `seed_test.go` — `admin.Seed` 幂等性、全量授权、启动补齐/自愈、参数校验、Seed+Login 联动
- `responder_test.go` — envelope 解包、wrapped error 链透传、`{code,message,data}` 形状
- `admin_e2e_test.go` — `Setup` + 自动注册 AuthService 的端到端 HTTP 调用
- `login_log_test.go` — `recordLogin` 成功/失败各分支
- `service/` 下的 `*_test.go` — AuthService 全流程（登录/刷新/登出/钩子/无状态模式）、UserService RPC、RoleService / PermissionService / MenuService 的 HTTP 集成测试（含 `live:"type:dropdown;url:/role/options"` 注解解析）
