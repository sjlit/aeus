# AEUS admin — 通用后台管理模块

**项目地址**: github.com/sjlit/aeus/admin

admin 是 AEUS 框架的通用后台管理领域模块，基于 [rest/v3](https://github.com/sjlit/rest) + GORM，为应用提供即开即用的系统管理能力：

- **通用 REST CRUD**：用户、角色、部门、菜单、权限、角色-权限中间表、审计日志、登录日志、租户 9 个模型自动注册为 REST 资源（列表/搜索、详情、创建、更新、删除、导出、OpenAPI 文档）；
- **JWT 认证**：登录 / 刷新 / 登出（`AuthService`），支持 Token 吊销与登录前后钩子；
- **RBAC 接口鉴权**：`NewPermissionChecker` 对已收录进 `sys_permissions` 的 HTTP 路由按角色校验权限，**未收录路由默认放行（fail-open）**——拒绝路径只覆盖目录里有 `data` 记录但当前角色无授权的情况；想收紧的子树用 `WithCheckerAllowlist` 显式放行（详见[接口权限校验](#接口权限校验rbac)）；
- **用户业务 RPC**（`UserService`）：个人资料、改密、管理员重置密码、头像、可见菜单、权限码；
- **租户隔离**：通过 GORM 回调自动为所有租户模型追加 `tenant_id` 过滤与回填，业务代码零侵入。

## 特性

- **声明式 CRUD**：模型上的 `scenarios` / `rule` / `enum` / `format` / `live` 标签直接驱动接口的字段可见性、校验规则与前端表单渲染
- **自动密码哈希**：`models.User` 在 `BeforeCreate` / `BeforeUpdate` 中对密码做 bcrypt 哈希（对已哈希值幂等）；`LoginLog.AccessToken` 走 SHA-256（高熵随机串无须 bcrypt 慢哈希）
- **密码策略**：8-32 位字母+数字基线（`CheckPasswordPolicy`），所有写密码路径经 GORM 钩子单点收口
- **级联清理**：删除菜单/角色（含软删）时自动清理 `sys_role_permissions` 中的关联权限；角色 Key 变更自动同步 `sys_role_permissions.role_key` 与 `sys_users.role_key`（避免用户的角色 Key 失同步）
- **统一响应格式**：内置 responder 输出 `{code, message, data}` envelope（成功 `code=0`），也可用 `WithResponder` 替换
- **可选的 OpenAPI**：`WithOpenAPI(true)` 后每个资源暴露 `openapi.json`
- **可插拔认证**：`AuthService` 始终由应用自行 `pb.RegisterAuthServiceRouter(...)` 注册，secret 必须来自运行时通道（见[设计约定](#设计约定)）
- **自动菜单注册**：模型实现 `MenuProvider` 时，`Setup` / `RegisterModel` 会按 `MenuEntry()` 自动生成 `sys_menus` 行（`Component` / `Uri` / `ViewPath` 在缺失时由模块名 + 表名推导，可显式覆盖）。三个分区容器菜单（`SystemUserCenter` / `SystemLogs` / `SystemSettings`）由 `EnsureSectionMenus` 在 `Seed` 阶段写入，业务菜单 `Parent` 都引用这三个 Component。`Setup` 末尾 `validateMenuParentsRef` 会扫所有 `sys_menus`，若发现 `Parent` 指向不存在的 Component 只打 `Warn` 而**不**中断启动——`Menu.BuildTree` 会把孤儿行降级为根，导航树仍可工作；运维应通过 `/system/sys_menu` 修正
- **自动权限目录**：每个挂载模型按 `rest.ScenarioProvider` 声明的场景集生成 `sys_permissions` 行（`Data` 形如 `"<METHOD> <URI>"`），重复启动幂等
- **接口权限执行**：`NewPermissionChecker` 把 HTTP 请求的 `"METHOD <路由模板>"` 与权限目录、角色授权逐一比对，**默认 fail-open**——未收录路由直接放行，目录里有但当前角色无授权才拒绝；想强制目录全覆盖的子树用 `WithCheckerAllowlist` 显式放行（详见[接口权限校验](#接口权限校验rbac)）
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

	// 1. 装配 admin Server：租户回调 + 9 个 CRUD 资源
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
//    不必是 *transport/http.Server),Setup 内部完成 schema 迁移与 9 个
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
> **接口鉴权依赖 JWT 中间件**：P0 捷径省略了中间件装配（token 校验与权限执行都在 `mwauth.JWT` 内完成），生产装配请按[快速开始](#快速开始)补上 `mwauth.JWT(...)`，其中 `WithPermissionChecker(admin.NewPermissionChecker(db))` 是 RBAC 生效的前提。PermissionChecker 默认 **fail-open**——未收录路由自动放行；想强制目录全覆盖（fail-closed）的子树通过 `WithCheckerAllowlist` 显式登记，把每个 bypass 暴露在 code review 里。

### Seed 的收敛契约

`admin.Seed(db, user, pass)` 在事务里一次性收敛以下状态，每条都是**幂等**的：

1. **3 个分区容器菜单**（`EnsureSectionMenus`，Section 容器）：`FirstOrCreate` 写入 `SystemUserCenter`（用户中心）、`SystemLogs`（日志记录）、`SystemSettings`（系统设置），Icon 走 Element Plus 命名。后续所有内置模型的 `MenuEntry().Parent` 都引用这三个 Component；分区本身 `Parent=""`（无嵌套）；
2. **默认租户**：`FirstOrCreate` 写入 `id="00000000-0000-0000-0000-000000000000"`、`name="默认租户"`、`status="enabled"`，作为新角色的 tenant_id 与新用户的归属；
3. **超管角色**：`FirstOrCreate` 写入 `Key="admin"`、`Name="系统管理员"`、`Status="enabled"`、`Builtin=true`、`IsSuper=true`、`DataScope="all"`。已存在的 `Builtin && !IsSuper` 内置角色就地 `Update is_super=true`（兼容旧库升级）；非内置同名角色**不动**（运维自建）；
4. **孤儿租户回填**：若超管角色的 `tenant_id` 不等于默认 uuid，再 `FirstOrCreate` 一条 `Name="默认租户"`、`Status="enabled"` 的同名租户，修复历史孤儿 uuid（早于 tenant 模型存在的库）；
5. **超管用户**：`FirstOrCreate` 写入 `UID="admin"`、`Username=user`、`RoleKey="admin"`、`Password=pass`（`BeforeCreate` bcrypt），落到默认租户；
6. **超管角色全量授权**：找到所有 `is_super=true` 的角色，按 `sys_menus.component` 与 `sys_permissions.data` 的差集补齐 `sys_role_permissions`（**add-only**，已存在的 / 已被运维手动删的会自愈，不删除现存的）；
7. **per-tenant `sys_schemas` 克隆**：`rest/v3` 在 `Setup` 阶段写 `sys_schemas` 时 `tenant_id` 留空（模板态），Seed 把模板按 `(module, table, column)` 去重克隆到每个 active tenant，**add-only**（模板消失时保留旧副本；模板变更不覆盖现有副本）。

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
| `WithTokenStore(store)` | 吊销黑名单（`Revoke(jti, until)` / `Revoked(jti)` / `RevokeIfNotRevoked(...)`，按 JWT `jti` 键）；nil 默认内存实现。**必须**配合 `mwauth.WithValidate(service.NewRevocationValidator(store, secret))` 接入 JWT 中间件才生效（见下文[吊销链路](#吊销链路tokenstore--revocationvalidator)） |
| `WithBeforeLogin(fn)` | 登录前置钩子：校验通过后、凭据校验前执行，返回 error 则短路登录 |
| `WithAfterLogin(fn)` | 登录成功钩子：仅作通知，返回值忽略 |
| `WithLoginLogger(fn)` | 登录审计：成功与失败都回调（IP/UA/Username + 成功时的 access token），**同步**执行；panic 透传到 Login 调用方，建议 recorder 自己 `defer recover()` |

- refresh token 固定 **48h**，`RefreshToken` 每次刷新**轮换**：返回全新 refresh token（新 `jti`），旧 jti 进黑名单——重放旧 refresh token 会被拒绝（`4005 AccessDenied`）；
- 刷新时 access token 从 **DB 最新行**签发（uid / role_key / tenant_id），角色变更后旧 refresh token 换不出旧权限；
- `Login` 在凭据校验前不依赖 JWT claims，天然跨租户查找用户；登录路径不写 TokenStore（黑名单只记显式吊销的 jti）。
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

### 吊销链路（TokenStore + RevocationValidator）

TokenStore 是按 JWT `jti` 键的**黑名单**：只有显式登出 / 轮换才写入条目，登录零写入。要让它真正生效，必须把 `service.RevocationValidator` 接到 JWT 中间件的 `WithValidate` 钩子上——它做两件事：

| 检查 | 结果 |
|------|------|
| `token_type == "refresh"` | 拒绝（`4005 AccessDenied`）——refresh token 只能走 `/auth/refresh-token`，不能当 access token 调 API |
| jti 已被吊销（logout / refresh 轮换） | 拒绝（`4005 AccessDenied`）；store 故障时同样拒绝（fail-closed） |

```go
store := service.NewMemoryTokenStore() // 多实例部署换共享实现(Redis 等)
pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret(secret),
    service.WithTokenStore(store),
))
httpSrv.Use(mwauth.JWT(keyfunc,
    mwauth.WithClaims(auth.Claims{}),
    mwauth.WithValidate(service.NewRevocationValidator(store, secret)), // 吊销 + token_type 门禁
    mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
    mwauth.WithAllow("/auth/login", "/auth/refresh-token"),
))
```

- 无法解析的 token（坏签名 / 过期）在 validator 处**放行透传**，由中间件自己的 parse 给出精确错误码；旧 token（无 jti / token_type claim）跳过检查保持兼容；
- 默认内存黑名单容量 65536 条、到期惰性清理；多实例部署务必注入共享 store。

> **登录限流**：`admin/cmd/mock` 默认已在 `/auth/login` 前挂 `middleware/auth.RateLimit`（token bucket，IP+账号 双 key，5 次突发 / 60 秒补 1 个），命中返回 `4010 TooManyAttempts`（HTTP 429）+ `Retry-After` header。详见 [`docs/api/auth.md` §3.7](docs/api/auth.md)。**未实现**：`User.LockedUntil` 字段（失败 N 次锁账号）—— 与 token bucket 正交，需叠加在 `AuthService.Login` 状态检查之后。

### UserService

| 选项 | 说明 |
|------|------|
| `WithUserServiceDB(db)` | GORM 句柄（必填） |
| `WithUserServiceAdminRole(role)` | 可执行 `ResetPassword` 的角色编码，默认 `admin` |

所有 RPC 均从 JWT claims 读取调用者身份，请求体不携带目标用户（`ResetPassword` 除外）。

### 接口权限校验（RBAC）

`admin.NewPermissionChecker(db, opts...)`（`permission.go`，db 为 nil 时 panic，与其它 Service 构造器一致）返回 JWT 中间件的 `PermissionCheckerFunc`，对**已收录进 `sys_permissions` 目录**的 HTTP 路由按角色校验权限：

```go
mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
```

**执行时机**：checker 在 JWT 中间件 **token 验证通过之后、claims 写入 ctx 之前**运行（`middleware/auth/jwt.go`）——它以参数形式接收解析出的 claims，**不能**依赖 `auth.ClaimsFromContext`；后续 handler 与 GORM 租户回调才从 ctx 读 claims。

**请求标识**：`metadata.RequestMethod + " " + metadata.RequestPath`。transport/http 写入的 `RequestPath` 是 gin 的 **FullPath 路由模板**（如 `PUT /system/sys_user/:id`），与 `permissionCode` 生成 `sys_permissions.data` 的 `"<METHOD> <URI>"` 格式同构，二者精确匹配。

**判定流程**（仅 `RequestProtocol == "http"` 时生效，CLI / 其它协议直接放行）：

| 步骤 | 条件 | 结果 |
|------|------|------|
| 1. 接线保护 | claims 不是 `*auth.Claims` | **拒绝**：`4005 AccessDenied` |
| 2. 显式放行 | `WithCheckerAllowlist` 命中 | **放行** |
| 3. 目录查询 | `sys_permissions` 存在 `type=api` 且 `data` 匹配的行 | 进入步骤 4；查无此行（**未收录即放行**，fail-open） |
| 4. 授权查询 | `sys_role_permissions` 存在 `role_key + tenant_id + type=permission + data` 匹配的行 | **放行** |
| 5. 否则 | 目录内但角色无授权 | **拒绝**：业务码 `4003 PermissionDenied` |

**设计要点**：

- **fail-open 默认放行**：未收录路由（业务 RPC：`/user/menus`、`/role/options`、`/tenant/options` 等）一律放行；想强制目录全覆盖（fail-closed）的子树通过 `WithCheckerAllowlist` 显式声明，把每个 bypass 暴露在 code review 里——未在 allowlist 上的目录收录路由依然按角色授权生效；
- **租户隔离显式化**：授权查询显式过滤 `tenant_id`——checker 运行时 claims 尚未进入 ctx，GORM 租户回调不会自动过滤，不显式传会导致跨租户串权；
- **fail-closed 接线保护**：claims 不是 `*auth.Claims` 时直接拒绝（`4005 AccessDenied`），把中间件接线错误暴露在请求上而不是静默放行；
- **边界**：`type=button` / `type=data_scope` 的权限不参与接口鉴权；`/auth/*` 在 `WithAllow` 列表上直接短路，不进 checker；
- **超管授权窗口**：角色授权由 `admin.Seed` 每次启动补齐（以调用时刻的全局目录为准），新模型注册后需先 `Setup` 后 `Seed`，见[P0 一键集成](#p0-一键集成搭配-dashboardweb)的时序约定。

**选项**：

| 选项 | 说明 |
|------|------|
| `WithCheckerAllowlist(entries ...string)` | 短路跳过 catalog 匹配的命中规则，每条 `"<METHOD> <pattern>"`（`METHOD` 为 `*` 或空表示任意方法；`pattern` 支持精确、`*` 通配、`<prefix>*` 段边界前缀）。默认 fail-open 下，**目录已收录但希望绕过角色授权**的子树才需要登记（如内部监控）；业务 RPC 不再强制放行，因为未收录路由本身就是放行的 |
| `WithCheckerCache(cache.Cache)` | 注入共享缓存后端，覆盖默认内存后端（默认 `infra/cache/memory`，单进程）。多实例部署换成 Redis 等共享后端，避免每个进程的 catalog 独立陈旧 |

**缓存**：api 目录与每个 `(tenant, role)` 的授权集合都通过 `admin/dbcache` 的读穿透 cacher 加载；catalog 用 `MAX(updated_at)` 作版本标记（无 TTL），grants 用 `SUM(id)` + 1 分钟 TTL（吸收标记盲区）。`dbcache` 的 1 秒宽限窗 + 版本标记组合让授权变更在下一个请求就生效，无需应用显式失效。

测试覆盖见 `permission_test.go`：已授权放行 / 未授权 4003 / 未收录路由**放行**（fail-open）/ 显式 allowlist 放行 / allowlist 通配段边界 / 跨目录命中不影响未收录路由 / 非 http 跳过 / 跨租户不串权 / claims 类型不符 4005 / 缓存命中。

## dbcache 读穿透缓存

`admin/dbcache` 提供带依赖版本标记的读穿透缓存，是 `NewPermissionChecker` 内部 cache 层的实现。它解决两个具体问题：

- **避免每个请求一次目录/授权 SQL**：权限校验是热路径上每次请求都要跑的开销，原始实现每次请求两次 SQL（catalog + grant），在 catalog 稳定时几乎是浪费；
- **让授权变更在下一个请求生效**：写一次 `sys_role_permissions`，下一个请求立刻看到，而不是等缓存 TTL 过期。

### 核心 API

```go
c := dbcache.New(db,
    dbcache.WithDependency(dbcache.NewSqlDependency(
        dbcache.WithTable("sys_role_permissions"),
        dbcache.WithColumn("SUM(id)"),
        dbcache.WithCondition("deleted_at IS NULL"),
    )),
    dbcache.WithCacheDuration(time.Minute), // hard TTL
)
set, err := dbcache.Try(c, ctx, "perm:grant:tenant1:admin",
    func(tx *gorm.DB) (map[string]struct{}, error) { /* ... */ })
```

- `Try(c, ctx, key, f)`：命中且未过期则直接返回；否则跑 `f(tx)`（singleflight 保证并发只跑一次），结果存进 cache；
- `CacheDependency.GetValue(ctx, db)` 返回当前源表的一个紧凑版本标记；缓存项写入时记下当时的标记值，下次读取时若与当前值相等就视为未变更直接放行（节省源表扫描）。

### 版本标记策略

| 源表 | 标记列 | 备注 |
|------|--------|------|
| `sys_permissions` | `MAX(updated_at)` | catalog 行可在 admin CRUD 中被就地更新，MAX 跨秒数足够 |
| `sys_role_permissions` | `SUM(id)` | junction 行从不被原地更新（assignment 是 soft delete + re-insert），SUM 自动吃掉软删/重建；自增 id 总比历史大 |

两个标记都是**表全局**的，所以"任何位置一次写"都会让整张表的全部缓存键**惰性失效**——过度失效是安全的，失效不足才会泄露权限。

### 宽限窗与 TTL

- **1 秒宽限窗**：刚写入的缓存在 1 秒内不查 marker，避免写入瞬间的 burst read 每次都重跑 marker 查询；
- **TTL 是硬上限**：版本标记存在同秒级盲区（catalog 的 `MAX(updated_at)` 在秒级，grants 的 SUM 也对同秒级 revoke+grant 漏检），硬 TTL 给一个 bound。catalog 用 0 TTL（标记足够），grants 用 1 分钟 TTL。

### 后端切换

默认 `infra/cache/memory.NewCache()`（单进程内存，`map[string]any`）。多实例部署时通过 `WithCache(cache.Cache)` 切到共享后端（如 Redis）；Redis 后端走 JSON 往返所以不会发生值别名问题，内存后端的值会与缓存项共享底层 slice/map——调用方不能修改返回值。

测试覆盖见 `dbcache/cacher_test.go` 与 `dbcache/depend_test.go`。

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

## 操作审计

### 启用

```go
s := admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
    admin.WithAudit(true),                      // 开启操作审计
    // 可选:追加排除项(内置排除始终生效)
    admin.WithAuditExcludes("app/high_freq_table"),
)
```

### 机制

开启后 `Setup` 在注册任何模型**之前**调用 `installAuditHooks`,向 rest/v3 注册**进程级全局** after-hooks(`RegisterAfterCreate/AfterUpdate/AfterDelete`)。每次 REST 写操作成功后,异步于业务事务地同步追加一行 `sys_audits`:

| 列 | 来源 |
|----|------|
| `uid` | `ResourceConfig.UserResolve`(默认读 JWT claims 的 `UID`;可用 `WithUserResolve` 覆盖) |
| `tenant_id` | `RuntimeScope.TenantID`(与租户隔离同一 resolver) |
| `action` | `create` / `update` / `delete` |
| `module` / `table` | 模型的 `ModuleName()` + `TableName()` |
| `data` | create/update:`[]DiffAttr`(column/label/previous/current)的 JSON;delete:仅 `{"id":<主键>}`(**不序列化整行**,避免把 `User.Password` 等 bcrypt 哈希写进审计表) |

- **全局生效**:由于 rest/v3 在资源构造时快照全局钩子,凡在 `Setup` 之后注册的资源——包括其它模块通过 `NewResourceWithOptions` 注册的应用模型——都自动被审计;
- **排除项**:内置排除 `system/sys_audits`(防递归)与 `system/sys_login_logs`(登录审计有自己的 `WithLoginLogger` 管线),`WithAuditExcludes` 可按 `"<module>/<table>"` 追加;
- **无变更不记录**:update 的 diff 为空(未实际修改)时跳过;
- **best-effort**:审计写入失败仅记 Warn 日志,绝不影响业务响应(after-hook 本就运行在业务事务提交之后、且被库内 `safelog.SafeRun` 包裹);
- **列宽截断**:`data` 按 10240 字节、`uid`/`module`/`table` 按各自列宽做 UTF-8 安全截断。

### 覆盖边界

审计只拦 **REST CRUD 路径**(rest/v3 的 Create/Update/Delete)。service 层裸 gorm 写、定时任务、gRPC 直写不会产生审计行;如需覆盖这些路径,需另行接入 GORM callback 并处理去重。

## 数据模型

| 模型 | 表名 | 租户 | 说明 |
|------|------|------|------|
| `Tenant` | `sys_tenants` | ❌ | 租户实体：`id`（char(60) 字符串主键，即各表 `tenant_id` 引用的值；创建时留空由 `BeforeCreate` 生成 uuid）、`name`、`status`（`enabled` / `disabled`，登录时校验）、`created_at` / `updated_at`。全局可见，无 `tenant_id` 列 |
| `User` | `sys_users` | ✅ | 用户：`uid`（工号，唯一）、`username`、`role_key`、`dept_id`、bcrypt `password`、`avatar`、`status`（`normal` / `disabled`，登录时校验）、`email`、`gender`（`man` / `woman` / `other`）、`description` |
| `Role` | `sys_roles` | ✅ | 角色：`name`、`key`（机器标识，唯一，匹配 `^[a-z][a-z0-9_]*$`）、`status`（`enabled` / `disabled`）、`builtin`、`is_super`（超管：授权由 Seed 自动管理）、`data_scope`（`all` / `dept` / `self` / `custom`）、`sort`、`created_by`、`description`。`key` 变更自动同步 `sys_role_permissions` 与 `sys_users.role_key`；删除（含软删）自动清理关联权限 |
| `Menu` | `sys_menus` | ❌ | 菜单树：`parent`（父级菜单 Component，全局共享）、`name`（唯一，标题）、`component`（唯一，标识，路由 keep-alive 用）、`uri`、`view_path`（自动生成的 Vue 路径 `@/views/<module>/<singular>/Index.vue`）、`icon`、`hidden` / `public`、`sort`、`description`。删除自动清理 `sys_role_permissions` 中 `type=menu` 的行 |
| `Department` | `sys_departments` | ✅ | 部门树：`parent_id`、`name`、`description` |
| `Permission` | `sys_permissions` | ❌ | 全局权限目录：`type`（`api` 接口 / `button` 按钮 / `data_scope` 数据范围）+ `data`（权限标识，全租户共享，形如 `"<METHOD> <URI>"`）+ `description` |
| `RolePermission` | `sys_role_permissions` | ✅ | 角色-权限中间表：`role_key`（角色 Key）+ `type`（`menu` / `permission`）+ `data`（菜单 Component / 权限标识，按 type 决定宽度），按租户生效 |
| `Audit` | `sys_audits` | ✅ | 审计日志：`uid`、`action`（`create` / `update` / `delete`，带颜色）、`module`、`table`、`data`（变更内容，size 10240）。`WithAudit(true)` 开启后由全局钩子自动写入（见「操作审计」一节） |
| `LoginLog` | `sys_login_logs` | ✅ | 登录日志：`uid`、`ip`、`browser`、`os`、`platform`、SHA-256 `access_token`（不入参；audit 用）、`user_agent` |

所有模型在 `Setup` 时由 rest/v3 自动 `AutoMigrate`，无需手动迁移。模型上的 `scenarios`、`rule`、`enum` 等标签驱动 rest/v3 的字段可见性与校验（如 `User.Uid` 的 `rule:"required;unique;regexp:^[a-zA-Z0-9]{3,8}$"`）。

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
- 自动按 `MenuProvider.MenuEntry()` + `ModuleName/TableName` 推导 `sys_menus` 行（详见[自动菜单注册](#特性)）；
- 自动按 `rest.ScenarioProvider`（或缺省 6 个标准场景：create / update / delete / search / detail / export）生成 `sys_permissions` 行；
- 若应用显式配置了 Server 级 `WithVueOutputDir`，自动生成 Vue `Index.vue`（见下一节）。

### per-call 选项

| 选项 | 作用 | 默认行为 |
|------|------|---------|
| `WithRegisterMenuSpec(spec MenuSpec)` | 覆盖模型 `MenuEntry()`；模型未实现 `MenuProvider` 时也可强制生成菜单行（传 `MenuSpec{}` 显式空 spec 仍会跳过） | 走 `MenuProvider.MenuEntry()`，未实现 `MenuProvider` 则跳过 |
| `WithRegisterScenarios(scenarios ...string)` | 覆盖该模型的权限场景集 | 走 `ScenarioProvider`，未实现则用 6 个标准场景；传空 slice 表示**不为该模型生成任何权限行**（典型：写审计类模型，授权在其它地方） |
| `WithRegisterVueOutputDir(dir string)` | 按模型粒度启用 / 禁用 Vue 生成（独立于 Server 级 `WithVueOutputDir`） | 继承 Server 级 `VueOutputDir`；传 `&""` 显式禁用该模型 |

三个选项互相独立，按需组合。`RegisterModel` 自身签名 (`RegisterModel(model)`) 保持不变，所有选项都是新增的——向后兼容。

应用批量注册完一批模型后，可调用 `admin.Server` 未导出的 `validateMenuParentsRef` 检查（或者直接调 `s.Setup` 的等价路径），把孤儿 parent 在请求到达之前**作为 Warn 日志**暴露出来——Setup 不再因此失败。

## Vue Index.vue 自动生成

admin 的 Server 在注册模型时，会按 `(ModuleName, Singular)` 自动写一份 Vue 3 `<script setup>` 模板到 `WithVueOutputDir` 指向的目录——典型的 Vite `views/` 根目录（`<repo>/web/src/views`）。这样新增一个内置模型后，前端不用手写 `Index.vue` 也能跑起 CRUD 页面。

### 启用

```go
s := admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
    admin.WithVueOutputDir("admin/web/src/views"), // 与 web/ 的 Vite alias 对齐
)
```

或对单个模型（`web/` 里已有的 9 个 `sys_*` 视图是手写的，**不要**让生成器覆盖）：

```go
s.RegisterModel(&SomeNewModel{}, admin.WithRegisterVueOutputDir("admin/web/src/views"))  // 启用
s.RegisterModel(&SysUsers{},    admin.WithRegisterVueOutputDir(""))                       // 显式跳过
```

### 生成内容

每个 `Index.vue` 都是同一份模板（`vueTemplate` 常量），占位符替换：

```vue
<script setup lang="ts">
defineOptions({ name: 'SystemSysUsers' })
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { SchemaViewer } from '@sjlit/rest-ui'
const route = useRoute()
const title = computed(() => (route.meta.title as string | undefined) || 'sys_users')
</script>
<template>
  <SchemaViewer module="system" table="sys_users" :title="title" />
</template>
```

文件路径：`{outputDir}/{module-lowercase}/{singular}/Index.vue`（与 `deriveViewPath` 写进 `sys_menus.view_path` 的路径同构）。

### 行为契约

- **幂等**：已存在的文件**直接跳过**（不覆盖手写修改），所以重启 Setup 不会破坏手写的视图；
- **失败 warn 不中断**：文件系统失败（只读挂载、容器内无源码树）只 `Logger.Warn`，**不**回滚已经写入的菜单 / 权限——后者是数据库契约，前端文件是开发期便利；
- **空 ModuleName/Singular 跳过**：与菜单 / 权限的 skip 信号一致，没有派生依据就不写；
- **路径遍历保护**：`vuePathForModel` 拒绝包含 `..` 或路径分隔符的 module/singular，误信源（配置驱动的 TableName 解析器）也写不出 `Index.vue`。

测试覆盖见 `vue_gen_test.go`：`buildVueContent` / `vuePathForModel` / `generateVueFile` / `generateVueForResource`。

## 前端管理台（admin/web）

`admin/web/` 是配套的 Vue 3 + Element Plus SPA，使用 [`@sjlit/rest-ui`](https://github.com/sjlit/rest-ui) 的 `SchemaViewer` 直接驱动整张 CRUD 页面。日常开发命令、目录约定、与 Element Plus 的视觉桥接都写在 [`admin/web/README.md`](web/README.md)；本节只描述**前后端契约**——前端依赖哪些后端端点、每个端点在哪一章定义，便于后端改动时同步检查前端。

### 后端契约清单

| 前端用法 | 后端端点 | 后端定义位置 |
|---------|---------|-------------|
| 任意模型 CRUD 页面（`SchemaViewer` 自动驱动） | `GET /schema/:module/:table`（列元数据）+ `GET /rest/model-types/:module/:table` / `GET /rest/model-tiers/:module/:table`（下拉数据） | [元数据 / 选项查询](#元数据--选项查询由-setup-挂载) |
| 侧边栏（菜单树） | `GET /user/menus` | [UserService](#userservice) |
| 每按钮 guard（前端按钮可见性） | `GET /user/permissions` | [UserService](#userservice) |
| 当前用户右上角菜单 | `GET /user/profile` | [UserService](#userservice) |
| 修改自己密码 | `POST /user/change-password` | [UserService](#userservice) |
| 登录页 | `POST /auth/login` | [AuthService](#authservice) |
| Token 刷新（access token 过期） | `POST /auth/refresh-token` | [AuthService](#authservice) |
| 登出 | `POST /auth/logout` | [AuthService](#authservice) |
| 角色编辑页下拉数据 | `GET /role/options` / `GET /permission/catalog` / `GET /menu/tree` | [RoleService / PermissionService / MenuService](#roleservice--permissionservice--menuservice) |
| 角色授权编辑（按角色 preview） | `GET /role/permissions` / `GET /role/menus` / `PUT /role/permissions` | [RoleService / PermissionService / MenuService](#roleservice--permissionservice--menuservice) |
| 切换租户（下拉） | `GET /tenant/options` / `GET /tenant/detail` | [TenantService](#tenantservice) |

> **契约变更原则**：后端改这五个 RBAC 端点的请求 / 响应形状时，前端 `@sjlit/rest-ui` 与 `admin/web/src/api/*` 都要同步更新；改自动生成的 `Index.vue` 模板（`vueTemplate` 常量在 [`vue_gen.go`](vue_gen.go)）时，前端手写的 9 个 `web/src/views/system/sys_*/Index.vue` 需要逐一对比——前者是契约源，后者是参考实现。

### 自动生成 vs 手写视图

`admin/web/src/views/system/sys_*` 下**目前 9 个模型视图都是手写的**（在 `web/` 引入 SchemaViewer 之前完成；保留是因为这些页面有手写的 #gridview 插槽、`scenarios` 调优与少量 i18n），`WithVueOutputDir` 默认**不会**覆盖它们：

```go
// 这条 RegisterModel 显式告诉生成器：不要碰这个手写视图
s.RegisterModel(&models.User{}, admin.WithRegisterVueOutputDir(""))
```

新增内置或应用模型时，`WithVueOutputDir` 自动生成的是"能跑起来就行"的占位 `SchemaViewer`——首屏跑通后再按需替换为带 `#gridview` 插槽的定制视图。详见 [Vue Index.vue 自动生成](#vue-indexvue-自动生成)。

### 开发联调

```bash
# 1. 启后端（demo 启动器，固定 :8080，种子账号 admin / Admin123）
cd admin/cmd/mock && go run .

# 2. 启前端（:5173，/api 代理到 :8080）
cd admin/web && npm install && npm run dev
```

打开 `http://localhost:5173`，用 `admin / Admin123` 登录。更多命令（`typecheck` / `test` / `build`）与前端内部约定（信封格式、auth 失败码、Hash 路由模式）见 [`admin/web/README.md`](web/README.md)。

## API 一览

### URL 形态约定

admin 模块所有业务 RPC（`AuthService` / `UserService` / `RoleService` / `PermissionService` / `MenuService` / `TenantService`）遵循同一规则：

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
| `Tenant` | `/system/sys_tenant` | `/system/sys_tenants` |

> 注意 `Menu` 的复数是 `sys_menuses`：复数是 rest/v3 对表名的机械转换（`inflector.Pluralize`），不做语义化处理。

### 元数据 / 选项查询（由 Setup 挂载）

`Server.Setup` 在注册完所有内置模型之后，自动在 `s.opts.Router` 上挂载三个 `GET` 端点，给前端 CRUD 渲染与下拉选择器提供元数据查询能力。它们都按 `(module, table)` 定位已注册的模型，无需应用再装配。

| 方法 | 路径 | 必填 query | 可选 query | 说明 |
|------|------|-----------|-----------|------|
| `GET` | `/schema/:module/:table` | — | — | 返回该表的全量列元数据（`[]schema.Schema`），前端按列渲染表单字段 |
| `GET` | `/rest/model-types/:module/:table` | `label`, `value` | `valueType`（默认 `string`；支持 `int`/`int64`/`uint`/`uint64`）、`tenant` | 返回该模型的扁平选项列表 `[{label, value}, ...]`，给下拉框使用 |
| `GET` | `/rest/model-tiers/:module/:table` | `parent`, `label`, `value` | `valueType`（同上）、`tenant` | 返回该模型的层级树 `[{label, value, children:[...]}, ...]`，给树形选择器使用（典型：菜单 `parent` 字段） |

**实现细节**：

- **模型查找**：路径参数 `(module, table)` 通过 `Server.modelsByModuleTable["<module>/<table>"]` 反查模型实例，索引在 `registerModel` 时统一建立——`Setup` 内置循环与外部 `RegisterModel` 两种路径都会自动覆盖。未注册的 `(module, table)` 返回 `4004 NotFound`，方便前端区分 URL 拼写错误与真实 DB 错误。
- **租户隔离**：与 `rest/v3` 通用 CRUD 一致——`tenant` query 参数优先，否则走 `s.opts.TenantResolver(r.Context())`，传 `""` 表示跨租户（与 `rest.ModelTypes` 自身行为对齐）。`Menu` / `Permission` / `Tenant` 等无 `tenant_id` 列的全局模型传 `""` 即可。
- **valueType 分发**：`model-types` / `model-tiers` 端点按 `valueType` 路由到对应的 `rest.ModelTypes[T]` / `rest.ModelTiers[T]` 泛型实例，使前端无需在拼装 payload 时做类型转换（`uint` 字段直接以 JSON 数字形式回传，不会被强制转字符串）。
- **响应格式**：统一走 admin envelope `{"code":0,"message":"","data":[...]}`；HTTP 状态恒为 200，业务码分流（`1001 Invalid` / `4004 NotFound`）由前端按 `body.code` 处理，与 CRUD 端点一致。
- **认证**：必须挂在 JWT 中间件之后（同 CRUD 端点）。`Setup` 不引入新的鉴权层，三个端点的可见性与路由前缀由应用网关决定。

**典型调用**（菜单管理页的"父级菜单"下拉）：

```http
GET /rest/model-tiers/system/sys_menus?parent=parent&label=name&value=component HTTP/1.1
Authorization: Bearer <token>

→ 200 OK
{
  "code": 0,
  "message": "",
  "data": [
    { "label": "系统设置", "value": "SystemSettings", "children": [
        { "label": "菜单管理", "value": "SystemSysMenus", "children": [] },
        ...
    ]},
    ...
  ]
}
```

> **为什么 schema 端点也在这里**：`/schema/:module/:table` 由 `RegisterSchemaEndpoint`（`schema_endpoint.go`）挂载，与本节两个端点共用同一套"Setup 末尾自动注册"的契约——前端 `@nobla/rest-ui` 同时拉取 schema 与下拉数据来驱动整张 CRUD 页面。把三个端点放在同一节便于前端同学一眼看到完整契约。

### AuthService

| 方法 | 路径 | 请求体 | 说明 |
|------|------|--------|------|
| `POST` | `/auth/login` | `{username, password}` | 登录，返回 `{uid, username, expires, access_token, refresh_token}` |
| `POST` | `/auth/refresh-token` | `{refresh_token}` | 换取新 access token + **轮换后的新 refresh token**（旧值立即失效） |
| `POST` | `/auth/logout` | `{access_token?, refresh_token?}`（至少一项） | 登出：按 jti 吊销所给 token，防止被盗 token 在登出后继续可用 |

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
| `GET` | `/role/menus?role=…` | 某角色可见菜单（管理端预览，不限于当前用户角色；扁平，`name` = 菜单标题） |
| `GET` | `/permission/catalog?type=…` | 全局目录权限码（可选 `?type=` 过滤） |
| `GET` | `/menu/tree` | 完整菜单树（节点 `name` = 菜单 **Component**，非标题） |
| `GET` | `/menu/options` | 层级下拉项（`value` = 菜单 **Component**，可直接作为 `parent` 提交） |
| `GET` | `/menu/breadcrumb?id=…` | Component 链到根的路径（面包屑） |

> **枚举 query 参数**：`PermissionType`（`type`）是 proto 枚举，**只能传数字**（`0/1/2/3/4` = `PERMISSION_TYPE_UNSPECIFIED/MENU/API/BUTTON/DATA_SCOPE`）；字符串形式 `?type=menu` 无法通过 `MapFormWithTag` 绑定，会得到绑定错误。`role` / `id` 传字符串 / 数字字符串即可。详见[URL 形态约定](#url-形态约定)。
>
> **`GET /role/permissions` 的 type 过滤语义**：`type` 缺省（`UNSPECIFIED`）返回 `{menus, apis}` 合读（角色编辑页一屏展示）；`type=1`（MENU）只返菜单 Component、`apis` 为空；`type=2`（API）只返权限码、`menus` 为空。原 `GET /permission/role` 已于 2026-08-10 合并至本端点。
>
> **`GET /role/menus` 的归属**：`RoleMenus` 原属 `MenuService.ListVisibleMenusByRole`，2026-08-10 移入 `RoleService`，所有"角色相关"读端点统一归 `/role/*`；前端"按角色预览侧栏"路由固定调用本端点。
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
├── docs/              # 设计文档、INTEGRATION-TODO、设计约定 spec
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
- `tenant_scope_test.go` — GORM 租户回调（Query/Update/Delete/Create/回填）
- `permission_test.go` — `NewPermissionChecker`：已授权放行 / 未授权 4003 / 未收录路由**放行**（fail-open）/ 显式 allowlist 放行 / allowlist 通配段边界 / 跨目录命中不影响未收录路由 / 非 http 跳过 / 跨租户不串权 / claims 类型不符 4005 / 缓存命中
- `schema_endpoint_test.go` — `RegisterSchemaEndpoint`：路径解析、pre-condition 失败、HTTP 端到端、未知 module/table
- `modeloptions_endpoint_test.go` — `RegisterModelTypesEndpoint` + `RegisterModelTiersEndpoint`：路径解析、pre-condition 失败、HTTP 端到端（成功/缺失必填 query/未知 valueType/未知 module/table/路由注册）、`queryModelTypes` / `queryModelTiers` 分派器单元测试
- `register_model_options_test.go` — `RegisterModel` per-call 选项（`WithRegisterMenuSpec` / `WithRegisterScenarios` / `WithRegisterVueOutputDir`）的覆盖语义、显式空 spec / 空 scenarios 的边界
- `vue_gen_test.go` — `buildVueContent` / `vuePathForModel` / `generateVueFile` / `generateVueForResource`：模板占位符替换、路径遍历保护、已存在文件跳过、失败 warn 不中断
- `seed_test.go` / `seed_tenant_test.go` — `admin.Seed` 幂等性、全量授权、启动补齐/自愈、参数校验、Seed+Login 联动、跨租户孤儿回填、per-tenant `sys_schemas` 克隆
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
