# middleware/auth — JWT 校验与限流中间件

通用 transport 层中间件集合,无 transport 依赖(可挂 gin / gRPC / CLI)。admin 在 `cmd/mock` 里把它俩组合使用,生产应用可照搬。

## 构成

| 文件 | 暴露 API |
|---|---|
| `jwt.go` | `JWT(keyFunc, opts...)` —— Bearer token 校验、claims 解析、permission checker 接入 |
| `rate_limit.go` | `RateLimit(opts)` —— token bucket 限流,4010 拒绝 |
| `rate_limit_memory.go` | `NewMemoryLimiter(capacity, refill)` —— 默认 per-process 后端 |

## RateLimit

### 用途

挡在任何需要节流的路径前,典型用例是 `/auth/login` 暴力破解防护,也适合 `/api/*` 高频接口的全局 RPS 限制。

### 算法

经典 token bucket:

- **冷启动**:每个 key 起始 `Capacity` 个 token(突发上限)
- **稳态**:每秒补 `Refill` 个
- **判定**:`Allow` 时刷新当前 token 数,扣 1 个;扣不动则 `retryAfter = (1 - deficit) / refill`(告诉客户端多久后再试)

### 配置

```go
type RateLimitOpts struct {
    Path     string    // 路径前缀,沿用 WithAllow 的语法(精确 / "*" / "<prefix>*")
    Keys     []string  // key 模板,支持 {ip} {user} {tenant} {path} 占位符
    Capacity int       // 桶容量(突发),>0
    Refill   float64   // 每秒补几个,>0
    Limiter  Limiter   // 存储后端,nil 用 NewMemoryLimiter
}
```

### 关键决策

| 决策 | 取值 | 理由 |
|---|---|---|
| 短路语义 | 任一 key 耗尽即拒 | IP+账号双桶防"一个撞线全放过" |
| 挂载位置 | `RateLimit → JWT → handler` | 在 bcrypt 慢路径之前拦截 |
| `{user}` 解析 | 读 `jwt.MapClaims["uid"]`,fallback `["sub"]` | admin `*auth.Claims` 是 struct,不被解析;按账号限流需把 RateLimit 放 JWT 之后 |
| 失败关闭 | Limiter 报错 → 500 | fail-open 等于不限流 |
| 业务码 | 新增 `4010 TooManyAttempts`(不复用 3001) | 前端精确分流 |
| Retry-After | 走 metadata header("Retry-After" key) | transport/http 把它写进响应 |

### 占位符语义

| 占位符 | 取值来源 | JWT 之前 | JWT 之后 |
|---|---|---|---|
| `{ip}` | `metadata.RequestClientIP` | ✓ | ✓ |
| `{user}` | `jwt.MapClaims["uid"]` → `["sub"]` | 空串(匿名桶) | 已解析的 uid |
| `{tenant}` | `jwt.MapClaims["tid"]` | 空串 | 已解析的 tid |
| `{path}` | `metadata.RequestPath` | ✓ | ✓ |

未识别占位符原样保留(误信配置时仍能落到合理 key)。

### 多实例部署

`NewMemoryLimiter` 是 per-process:默认 N 实例 = N× 实际限速。多实例部署应共享后端(Redis);`RateLimitOpts.Limiter` 字段是挂载点。Redis 适配器后续单独发布,本版本不实现。

### 与"账号锁定"的关系

token bucket 只看**速率**不看**结果**,同一个 IP 用对 5 次密码后仍能继续。完整"失败 N 次锁账号"需要叠加 `User.LockedUntil` 字段 + `AuthService.Login` 状态检查(P2 任务,与本中间件正交)。

### 测试

- `rate_limit_test.go` — 9 用例:key 模板、Allow 路径匹配、4010 + Retry-After、Limiter 错误 fail-closed、多 key 短路、panic 校验
- `rate_limit_memory_test.go` — 8 用例:冷启动、补桶、per-key 隔离、容量上限、idle GC、retryAfter 时长、并发、构造参数

## JWT(简表)

详见 [jwt.go](./jwt.go) 行内注释;admin 在 `service.go` 里的完整装配示例:

```go
httpSrv.Use(mw.RateLimit(mw.RateLimitOpts{
    Path: "/auth/login", Keys: []string{"ip:{ip}", "user:{user}"},
    Capacity: 5, Refill: 1.0 / 60,
}))
httpSrv.Use(mw.JWT(
    func(*jwt.Token) (any, error) { return []byte(secret), nil },
    mw.WithClaims(adminAuth.Claims{}),
    mw.WithPermissionChecker(admin.NewPermissionChecker(db)),
    mw.WithAllow("/auth/login", "/auth/refresh-token"),
))
```