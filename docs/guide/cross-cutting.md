# 横切能力


## 中间件

#### 通用 middleware 包

```go
import "github.com/sjlit/aeus/middleware"

type Handler = middleware.Handler      // func(ctx context.Context) error
type Middleware = middleware.Middleware // func(Handler) Handler

// 多个 middleware 串联
chain := middleware.Chain(mw1, mw2, mw3)

// 显式短路
return middleware.Abort(errs.ErrAccessDenied)

// CLI 等无内置指标的 server 用 RequestMetrics 补齐指标
svr.Use(middleware.RequestMetrics(recorder))
```

#### JWT（`middleware/auth`）

```go
import (
    "github.com/golang-jwt/jwt/v5"
    "github.com/sjlit/aeus/middleware/auth"
)

keyFunc := func(*jwt.Token) (any, error) { return []byte(secret), nil }

jwtMW := auth.JWT(keyFunc,
    auth.WithAllow("/auth/login", "/auth/refresh-token"), // 精确/"*"/"<prefix>*"
    auth.WithClaims(MyClaims{}),
    auth.WithPermissionChecker(func(ctx context.Context, c jwt.Claims) error {
        // 业务权限校验
        return nil
    }),
)

// handler 中读取 claims
claims, ok := auth.FromContext(ctx)
```

#### RateLimit（token-bucket，4010）

```go
limiter, _ := auth.NewMemoryLimiter(5, 1.0/60) // 5 个令牌，每秒补 1/60 个
svr.Use(auth.RateLimit(auth.RateLimitOpts{
    Path:     "/auth/login",
    Keys:     []string{"ip:{ip}", "user:{user}"},  // 支持 {ip}/{user}/{tenant}/{path}
    Capacity: 5,
    Refill:   1.0 / 60,
    Limiter:  limiter, // 默认 NewMemoryLimiter(per-process)，多实例需换 Redis 后端
}))
```

超过限流时返回 `errs.CodeTooManyAttempts (4010)`（HTTP 429），并通过 metadata TeeWriter 写入 `Retry-After` 响应头。详细决策、占位符语义、多实例部署影响见 [`middleware/auth/README.md`](../../middleware/auth/README.md)。


## 可观测性

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/infra/telemetry/otel"
    "github.com/sjlit/aeus/infra/telemetry/prometheus"
)

// Provider 进程级
provider, shutdownProvider, err := otel.NewProvider("my-service",
    otel.WithEndpoint("otel-collector:4317"),
    otel.WithSampler(sdktrace.TraceIDRatioBased(0.1)),
)
if err != nil { log.Fatal(err) }
defer shutdownProvider()

// Tracer 服务级
tracer, shutdownTracer, err := otel.NewTracer("my-service", provider)
if err != nil { log.Fatal(err) }
defer shutdownTracer()

recorder := prometheus.NewRecorder(nil) // nil → prometheus.DefaultRegisterer

svc := aeus.New(
    aeus.WithName("my-service"),
    aeus.WithTracer(tracer),
    aeus.WithMetrics(recorder),
    aeus.WithServer(http.New(http.WithEnableMetrics(true))),
)
svc.Run()
```

挂上之后所有 server 会自动：

- 为每个请求创建 span，kind = `SpanKindServer`，HTTP/gRPC/CLI 一致；
- 写入 trace context（`traceparent` 等）到 metadata；
- 记录 `aeus_request_duration_seconds{protocol,path,status}` 与 `aeus_request_total{protocol,path,status}`（path 优先取路由模板，避免高基数）；
- 注册中心心跳 `aeus_registry_heartbeat_total{status}`；
- logger 自动追加 `trace_id` / `span_id` 字段。

`infra/telemetry` 提供的 carrier：`HTTPHeaderCarrier{Header http.Header}`、`MetadataCarrier{MD *metadata.Metadata}`，分别适配 `http.Header` 与 `metadata.Metadata`，在外部调用与跨进程传播时使用。

`prometheus.MetricsHandler()` 返回 `http.Handler`（即 `promhttp.Handler()`），可挂到任意 server。


## Logger

```go
import "github.com/sjlit/aeus/infra/logger"

// 用 slog 直接构造
lg := logger.New(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})))

// 或包级默认（slog text handler 写到 stdout）
logger.Info(ctx, "service started")
logger.Infof(ctx, "service %s started on %s", name, addr)

// 链路追踪字段自动注入
ctx = metadata.NewContext(ctx, metadata.New().Set("trace_id", "abc"))
logger.Info(ctx, "msg") // 会自动追加 trace_id=abc
```

`Logger` 接口：`Debug / Debugf / Info / Infof / Warn / Warnf / Error / Errorf` + `With(args...)`。包级 `Default()` 返回进程级单例，**所有 logger 调用都会从 ctx 读 metadata 并自动追加 `trace_id` / `span_id`**。


## 错误码（pkg/errs）

```go
import "github.com/sjlit/aeus/pkg/errs"

// 业务码
var CodeTooManyAttempts errs.Code = 4010

// 构造
err := errs.New(errs.CodeNotFound, "user not found")
err := errs.Newf(errs.CodeInvalid, "field %s missing", "email")
err := errs.Wrap(errs.CodeInternal, ioErr)

// 断言 + 映射
if errs.IsCode(err, errs.CodeTooManyAttempts) {
    ae := err.(*errs.Error)
    httpStatus := ae.HTTPStatus() // 4010 → 429
    grpcStatus := ae.GRPCStatus() // 4010 → codes.ResourceExhausted(8)
}
```

`Code` 范围：1000s 协议 / 参数、2000s 时序、3000s 限流 / 配额、4000s 鉴权（4010 `TooManyAttempts`）、5000s 网络、6000s 数据 / 存储、8000s 外部依赖。`(*Error)` 实现 `error`、`Unwrap`、`HTTPStatus`、`GRPCStatus`、`Code`；`errs.Is` / `errs.IsCode` 是常用的判定助手。


## Metadata

```go
import "github.com/sjlit/aeus/metadata"

md := metadata.New()
md.Set("X-Tenant", "acme")        // key 自动 lowercase
val, ok := md.Get("x-tenant")     // 一致地按 lowercase 取

ctx = metadata.NewContext(ctx, md)
md2 := metadata.FromContext(ctx)  // 没有时返回 New()，不会 nil-deref
metadata.Set(ctx, "x-uid", "u-1") // 返回带 metadata 的新 ctx
```

框架在 HTTP / gRPC / CLI server 入口自动 tee 出 / 写入框架级 metadata（`X-AEUS-Request-ID`、`-Path`、`-Method`、`-Protocol`、`-ClientIP`），并写回响应 header。


## pkg/bytepool / pkg/netutil

```go
buf := bytepool.GetBytes(4096)
defer bytepool.PutBytes(buf)

b := bytepool.GetBuffer()
defer bytepool.PutBuffer(b)

// 当 listener 监听 0.0.0.0/:0 时，拿到可注册的对外 IP
addr := netutil.EffectiveAddr(":0", listener) // e.g. 192.168.1.10:54321
ip := netutil.LocalIP()                       // 第一个非 loopback IPv4
```

---
