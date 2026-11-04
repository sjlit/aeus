# 认证与会话


## JWT Claims

`auth.Claims` 内嵌 `jwt.RegisteredClaims`（标准 `exp` / `nbf` / `iat`），自定义字段：

| 字段 | JSON | 说明 |
|------|------|------|
| `UID` | `uid` | 用户工号 |
| `Role` | `uro` | 角色编码 |
| `TenantID` | `tid` | 租户 ID |
| `TokenType` | `token_type` | 令牌类型：`access` / `refresh`（RefreshToken 只接受 `refresh`） |

处理器内通过 `auth.ClaimsFromContext(ctx)` 读取；`auth.TenantIDFromContext(ctx)` 返回租户 ID（无 claims 时为空串）。

## AuthService

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

## 登录安全

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

## 吊销链路（TokenStore + RevocationValidator）

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
    mwauth.WithAllow("/auth/login", "/auth/refresh-token", "/auth/logout"),
))
```

- 无法解析的 token（坏签名 / 过期）在 validator 处**放行透传**，由中间件自己的 parse 给出精确错误码；旧 token（无 jti / token_type claim）跳过检查保持兼容；
- 默认内存黑名单容量 65536 条、到期惰性清理；多实例部署务必注入共享 store。

> **登录限流**：`admin/cmd/mock` 默认已在 `/auth/login` 前挂 `middleware/auth.RateLimit`（token bucket，按客户端 IP，5 次突发 / 60 秒补 1 个），命中返回 `4010 TooManyAttempts`（HTTP 429）+ `Retry-After` header。登录前的 `{user}` 占位符解析不到 claims，因此不要在这里做 per-account 限流；账号维度应放到登录 handler 内或 JWT 之后的已认证路由。详见 [`docs/api/auth.md` §3.7](../api/auth.md)。**未实现**：`User.LockedUntil` 字段（失败 N 次锁账号）—— 与 token bucket 正交，需叠加在 `AuthService.Login` 状态检查之后。

## UserService

| 选项 | 说明 |
|------|------|
| `WithUserServiceDB(db)` | GORM 句柄（必填） |
| `WithUserServiceAdminRole(role)` | 可执行 `ResetPassword` 的角色编码，默认 `admin` |

所有 RPC 均从 JWT claims 读取调用者身份，请求体不携带目标用户（`ResetPassword` 除外）。
