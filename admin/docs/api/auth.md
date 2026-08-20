# 3. AuthService

会话生命周期:`Login` → `RefreshToken` → `Logout`。由应用自行装配并 `pb.RegisterAuthServiceRouter(router, ...)` 注册,**不在 `Server.Setup` 中自动注册**。

## 3.1 注册方式

```go
pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret(os.Getenv("JWT_SECRET")), // 必须从运行时通道取
    // 可选:
    service.WithTokenExpireSeconds(2 * 60 * 60),
    service.WithTokenStore(myRedisStore),
    service.WithBeforeLogin(func(ctx, req) error { return nil }),
    service.WithAfterLogin(func(ctx, req, res) {}),
    service.WithLoginLogger(func(ctx, info) { ... }),
))
```

| Functional Option | 说明 |
|---|---|
| `WithAuthServiceDB(db)` | GORM 句柄(必填,缺则构造时 panic) |
| `WithAuthSecret(secret)` | JWT 签名密钥(必填,空值 panic;从 env / KMS 取) |
| `WithTokenExpireSeconds(n)` | access token TTL,默认 `7200`(2h);≤0 按默认 |
| `WithTokenStore(store)` | 吊销存储;nil 默认 `NewMemoryTokenStore()`,不持久化 |
| `WithBeforeLogin(fn)` | 登录前置钩子(凭据校验前);返回 error 短路 Login |
| `WithAfterLogin(fn)` | 登录成功钩子,通知用,返回值忽略 |
| `WithLoginLogger(fn)` | 登录审计回调(成功+失败);IP/UA/Username + 成功时的 access token;**同步**执行,panic 透传到 Login 调用方 |

### TokenStore 接口

```go
type TokenStore interface {
    Put(ctx context.Context, token string, ttl int64) error
    Del(ctx context.Context, token string) error
}
```

默认实现 `service.NewMemoryTokenStore()`(进程内 map)。生产建议自实现 Redis 版。

### LoginLogFunc 回调载荷

```go
type LoginLogInfo struct {
    Success     bool
    UID         string
    TenantID    string
    Username    string
    IP          string   // 来自 metadata.RequestClientIP
    UserAgent   string   // 来自 "User-Agent"
    AccessToken string   // 仅成功路径非空
}

type LoginLogFunc func(ctx context.Context, info LoginLogInfo)
```

## 3.2 端点

### POST /auth/login

**请求**(`/auth/login`):

```json
{ "username": "admin", "password": "plain-text" }
```

> username 同时匹配 `User.UID` 或 `User.Username`(OR 查询)。

**响应 200**:

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
    "tenant_name": "默认租户"
  }
}
```

| 字段 | 说明 |
|---|---|
| `uid` / `username` | 用户标识与登录名 |
| `expires` | access token **TTL 相对秒**(`7200` = 2h),前端 `Date.now() + expires * 1000` 算绝对到期时间 |
| `access_token` / `refresh_token` | JWT,签名 HS256 |
| `tenant_id` / `tenant_name` | 当前租户标识与展示名;表不存在时 `tenant_name = tenant_id` 兜底 |

**失败码**:

| Code | Message | 触发 |
|---|---|---|
| `4005` | `invalid username or password` | 用户不存在 / 密码错(消息统一,不暴露用户名枚举) |
| `4003` | `user is disabled` | 用户 `status=disabled`(在密码校验**之后**) |
| `4003` | `role is disabled` | 角色 `status=disabled` |
| `4003` | `role not found` | 角色行缺失 |
| `4003` | `tenant is disabled` | 租户 `status=disabled` |

### POST /auth/refresh-token

**请求**:

```json
{ "refresh_token": "eyJ..." }
```

**响应 200**:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "uid": "admin",
    "expires": 7200,
    "access_token": "eyJ...",
    "refresh_token": "eyJ..."
  }
}
```

> refresh token **不轮换**——响应中的 `refresh_token` 等于请求中的值。前端继续保留原值。

**校验**:

- token 必须能解析,签名 HS256 合法。
- `token_type` 必须等于 `"refresh"`,否则 `4005`(防止 access token 被冒充 refresh)。
- 重新查 `User.UID + TenantID` → 用户被禁则 `4003`;角色被禁则 `4003`;用户/角色不存在则 `4005`。

### POST /auth/logout

**请求**:

```json
{ "access_token": "eyJ..." }
```

**响应 200**:

```json
{ "code": 0, "message": "", "data": { "uid": "admin" } }
```

**行为**:

- token 必须能解析且签名合法。
- 配置了 `TokenStore` 时调用 `Del(token)` 真正吊销(进程内或 Redis)。
- 未配置 `TokenStore` 时无副作用(无状态模式,logout 仅协议层通知,token 凭自然过期失效)。

**失败码**:签名错误 / token 类型不符 → `4005`。

## 3.3 登录前后钩子

### WithBeforeLogin(ctx, req) error

- 触发时机:`req.Validate()` 之后,凭据查询之前。
- 返回 `error` 短路 Login,客户端得到对应的业务码。
- 典型用途:IP 黑名单、灰度开关、风控前置。

### WithAfterLogin(ctx, req, res)

- 触发时机:`Login` 全部成功,响应即将返回前。
- 返回值忽略(签名上是 fire-and-forget)。
- 典型用途:推送登录事件、下发消息队列。

### WithLoginLogger(ctx, info)

- 触发时机:成功路径在 `AfterLogin` 之前;失败路径贯穿所有失败分支。
- IP 取自 `metadata.RequestClientIP`,UA 取自 `User-Agent` header。
- **同步执行**,延迟会计入 Login 的 RTT;建议自己 `defer recover()` 防止 panic 中断请求。

## 3.4 状态校验时序(防用户名枚举)

`Login` 在凭据查询之前不做任何用户存在性提示——"用户不存在" 与 "密码错" 都返回同一 `4005 invalid username or password`。只有在密码已验证通过之后,才会按 用户 → 角色 → 租户 的顺序把 `disabled` 信息用不同业务码返回(`4003`)。

这样做的代价是合法的"账号被禁"消息是给得到密码的人看的——他们本来就该知道;成本是攻击者无法通过响应码差异枚举用户名。

## 3.5 默认 TTL

```go
const __defaultTokenExpireSeconds int64 = 2 * 60 * 60  // 7200 = 2h
const __refreshTokenTTL          int64 = 48 * 60 * 60  // 172800 = 48h,不可配
```

`WithTokenExpireSeconds(n)` 只影响 access;refresh 固定 48h。

## 3.6 JWT 结构

Header: `{"alg":"HS256","typ":"JWT"}`
Payload:

```json
{
  "uid": "admin",
  "uro": "admin",
  "tid": "d3cf8ac4-...",
  "token_type": "access",
  "jti": "550e8400-...",
  "exp": 1700000000,
  "iat": 1699992800,
  "nbf": 1699992800
}
```

签名:`HMACSHA256(base64url(header) + "." + base64url(payload), secret)`。

## 3.7 登录限流 (`RateLimit`)

admin 包**不**内置登录限流 — 但 `middleware/auth.RateLimit` 是为此而生的通用中间件,`admin/cmd/mock` 默认已经在 `/auth/login` 前挂载,生产装配照搬即可。

### 行为

- 算法:**token bucket**;冷启动 `Capacity` 个突发,稳态每秒补 `Refill` 个。
- 命中:返回 `4010 TooManyAttempts`(HTTP 429)+ `Retry-After: <秒>` header。
- 短路语义:多个 key 任一耗尽即拒(逻辑 AND),例如同时配置 `ip:{ip}` + `user:{user}` 时,IP 桶空就拒绝,user 桶不会被消费。
- 占位符:`{ip}` `{user}` `{tenant}` `{path}`;`{user}` 在 JWT 中间件之前是空串(匿名桶),按账号限流必须把 `RateLimit` 放在 `JWT` **之后**。
- 挂载顺序:**`RateLimit → JWT → handler`** — 在密码校验之前拦截,防止 bcrypt 慢路径被滥用。
- 失败关闭:Limiter 自身报错时直接返回 `500`(不放行)。

### 默认配置(`admin/cmd/mock`)

```go
http.Use(mw.RateLimit(mw.RateLimitOpts{
    Path:     "/auth/login",
    Keys:     []string{"login:ip:{ip}", "login:user:{user}"},
    Capacity: 5,           // 突发 5 次
    Refill:   1.0 / 60,    // 60 秒补 1 个 = 稳态 1 次/分钟
}))
```

### 覆盖范围

| 攻击 | 防护 key |
|---|---|
| 同一 IP 撞库 | `login:ip:{ip}` |
| 同一账号密码字典 | `login:user:{user}`(前提:RateLimit 放在 JWT 之后) |
| 混合攻击 | 两个独立桶任一撞线即拒 |

### 与"账号锁定"的关系

token bucket 只看**速率**不看**结果** — 同一个 IP 用对 5 次密码后仍能继续。完整"失败 N 次锁账号"需要叠加 `User.LockedUntil` 字段 + `AuthService.Login` 状态检查(P2 任务,与本节独立)。

### 跨实例部署

默认 `NewMemoryLimiter` 是 per-process:N 个实例 = N× 实际限速。多实例部署应共享后端(Redis);`RateLimitOpts.Limiter` 字段是挂载点,Redis 适配器后续单独发布。

### 测试覆盖

`middleware/auth/rate_limit_test.go`(9 用例)+ `rate_limit_memory_test.go`(8 用例)覆盖:模板占位、短路、失败关闭、路径前缀不匹配、token bucket 冷启动/补桶/隔离/容量上限/GC/重试时长/并发安全、构造参数校验。