# AEUS

**轻量级 Go 微服务脚手架框架**

AEUS 是一个用于快速构建微服务的 Go 脚手架，内置 HTTP（基于 Gin）、gRPC、CLI 多协议服务器，
提供 etcd 服务注册 / 发现、统一缓存抽象、NATS 消息代理、OpenTelemetry 链路追踪、
Prometheus 指标采集、JWT 鉴权与 token-bucket 限流等开箱即用的组件，按需 `go get` 子模块。

**仓库地址**: `github.com/sjlit/aeus`

---

## 核心特性

- **多协议同进程**：HTTP（基于 Gin）、gRPC、CLI 服务器可在同一进程并行启动，由 `aeus.Service` 统一管理生命周期。
- **可替换基础设施接口**：Registry / Cache / Broker / Tracer / MetricsRecorder / Logger 均为薄接口，底层实现（etcd、Redis、NATS、OpenTelemetry、Prometheus、slog）按子模块引入，业务代码不变。
- **链式 HTTP 客户端**：`pkg/httpclient` 提供流式 API，含拦截器、cookie jar、tracer 注入与超时控制。
- **结构化日志**：基于 Go `log/slog`，自动注入 `trace_id` / `span_id`。
- **可观测性**：所有 server 自动为每个请求创建 `SpanKindServer` span、记录请求耗时与总量；HTTP server 可通过 `WithEnableMetrics(true)` 暴露 `/metrics` 端点。
- **JWT + 限流中间件**：`middleware/auth` 提供 `JWT` 校验与基于 token-bucket 的 `RateLimit` 中间件（含 `4010 TooManyAttempts` 业务码、`Retry-After` 响应头）。
- **Proto 扩展**：`pkg/proto/{command,mcp,rest}` 提供 protoc descriptor 扩展，配合 `protoc-gen-go-aeus` 插件生成自定义代码（admin 子模块使用）。
- **可选 pprof 调试**：HTTP transport 在显式 `WithDebugAddr` 时启动独立的 loopback-only pprof 监听器，避免在公网端口暴露调试接口。

---

## 安装

每个子模块拥有独立的 Go module 与版本号，按需引入：

```bash
# 核心（接口 + 内存缓存 + 内存限流器 + JWT + middleware/metadata/errs/httpclient/bytepool/netutil）
go get github.com/sjlit/aeus

# 传输层
go get github.com/sjlit/aeus/transport/http         # Gin
go get github.com/sjlit/aeus/transport/grpc         # grpc-go + etcd resolver
go get github.com/sjlit/aeus/transport/cli          # 自研 TCP CLI 协议

# 基础设施
go get github.com/sjlit/aeus/registry/etcd
go get github.com/sjlit/aeus/infra/cache/redis
go get github.com/sjlit/aeus/infra/broker/nats
go get github.com/sjlit/aeus/infra/telemetry/otel
go get github.com/sjlit/aeus/infra/telemetry/prometheus
```

> `middleware/auth`、`pkg/errs`、`pkg/httpclient`、`pkg/bytepool`、`pkg/netutil`、`metadata`、`infra/logger`、`infra/cache`、`infra/broker` 接口与 `infra/telemetry` 抽象与核心模块同包，无需单独 `go get`。

---

## 设计立场

AEUS 定位为**脚手架框架**，在抽象与务实之间做了明确的取舍：

- **传输层绑定底层引擎，不承诺可替换**：HTTP 传输直接基于 Gin，gRPC 传输直接基于 grpc-go，CLI 为自研实现。`Server` / `Endpointer` / `Traceable` 接口负责的是**生命周期管理与框架集成**（启动、停止、端点上报、可观测性注入），**不是**底层引擎的可替换抽象。引入新引擎（如 Echo、Connect-RPC）意味着重写对应传输模块。
- **基础设施接口可替换**：Registry / Cache / Broker / Logger / Tracer / MetricsRecorder 均为薄接口，底层实现可通过独立子模块替换，不修改业务代码。
- **依赖按需引入**：核心模块只含接口与轻量默认实现（内存缓存、token-bucket 限流器、slog）；重型依赖（Gin、gRPC、etcd、NATS、Redis、OTel、Prometheus）位于独立子模块。
- **端口环境变量约束**：`HTTP_PORT` / `GRPC_PORT` / `CLI_PORT` 是本地开发快捷方式——空字符串时回退到 `:0`（随机端口），但**非空且非数字会直接 panic**（fail-fast，避免无声地绑到错误端口）。生产环境请用 `WithAddress` 显式指定。
- **AEUS_DEBUG 安全**：`AEUS_DEBUG=1` 不会在公网端口暴露 pprof；HTTP transport 仅在显式调用 `WithDebugAddr("127.0.0.1:xxxx")` 时启动独立 loopback pprof 监听器。

---

## 核心概念

| 类型 | 包 | 用途 |
| --- | --- | --- |
| `Service` | `aeus` | 顶层服务容器；调用 `Run()` 启动并阻塞到所有 server 退出 |
| `Application` | `aeus` | 上下文中的只读视图：`ID / Name / Version / Debug / Metadata / Endpoint` |
| `Scope` | `aeus` | 启动期初始化钩子：`Init(ctx) error` |
| `ServiceLoader` | `aeus` | 后台长跑任务：`Init(ctx) + Run(ctx)`，与 server 在同一 `errgroup` 内并行 |
| `Server` | `aeus` | 协议服务器通用接口：`Start(ctx) / Stop(ctx)` |
| `Endpointer` | `aeus` | 可选能力接口：返回对外暴露的端点 URI（用于注册中心） |
| `Traceable` | `aeus` | 可选能力接口：`SetTracer / SetMetrics`，由 `Service` 在 `preStart` 自动注入 |
| `Option` | `aeus` | Functional options：`WithName / WithServer / WithRegistry / WithTracer ...` |
| `Registry` / `Registrar` | `registry` | 服务注册 / 发现抽象 |
| `Cache` / `TypedCache[T]` | `infra/cache` | 缓存接口与泛型包装 |
| `Broker` | `infra/broker` | 消息代理接口 |
| `Tracer` / `Span` / `MetricsRecorder` | `infra/telemetry` | 可观测性接口 |
| `Logger` | `infra/logger` | slog 适配接口 |
| `Metadata` | `metadata` | 请求级 header 在 context 内的传播 |

---

## 快速开始

> 仓库暂未提供 `cmd/aeus` CLI 脚手架（规划中）。在 `main.go` 中直接组装 `Service` 即可：

```go
package main

import (
    "log"

    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/http"
)

func main() {
    svc := aeus.New(
        aeus.WithName("my-service"),
        aeus.WithServer(http.New()),
    )
    if err := svc.Run(); err != nil {
        log.Fatal(err)
    }
}
```

多协议并行：

```go
svc := aeus.New(
    aeus.WithName("my-service"),
    aeus.WithServer(
        http.New(),
        grpc.New(grpc.WithAddress(":9090")),
        cli.New(),
    ),
)
```

> `admin/` 子模块是一套完整的后台 RBAC 服务，演示了如何在真实业务里组合 HTTP/Gin + JWT + 限流 + 资源注册 + 自动建表等所有能力，用法见 [`admin/README.md`](admin/README.md)；在 `admin/` 目录下执行 `make proto` 可重新生成 Proto 代码（依赖 `protoc-gen-go-aeus`）。

---

## 项目结构

```
aeus/
├── app.go                    # Service 生命周期、依赖注入、errgroup 并行启停
├── options.go                # Functional options（WithName / WithServer / WithRegistry ...）
├── types.go                  # Application / Scope / ServiceLoader / Server / Traceable 等接口
├── metadata/                 # 请求元数据 (X-AEUS-*) 在 context 中的存取与传播
├── middleware/               # 通用中间件链
│   ├── middleware.go         # Handler/Middleware/Chain/Abort/RequestMetrics
│   └── auth/                 # JWT 校验 + RateLimit（token-bucket）
│       ├── jwt.go
│       ├── rate_limit.go
│       └── rate_limit_memory.go
├── infra/                    # 可替换基础设施接口 + 实现
│   ├── broker/               # broker.Broker 接口 + nats 子模块
│   │   └── nats/
│   ├── cache/                # cache.Cache/TypedCache 接口 + memory(内置) + redis 子模块
│   │   ├── memory/
│   │   └── redis/
│   ├── telemetry/            # Tracer / MetricsRecorder / Carrier + otel/prometheus 子模块
│   │   ├── otel/
│   │   └── prometheus/
│   └── logger/               # Logger 接口（slog 适配）+ Default()
├── pkg/                      # 跨切面可复用库
│   ├── errs/                 # 业务错误码 + HTTP/gRPC 映射（Code type + sentinel errors）
│   ├── httpclient/           # 链式 HTTP 客户端（拦截器、tracer 注入、cookie、下载）
│   ├── bytepool/             # *bytes.Buffer 与 []byte 分层池
│   ├── netutil/              # EffectiveAddr / LocalIP
│   └── proto/                # protoc descriptor 扩展（command / mcp / rest），配合
│                             #   protoc-gen-go-aeus 插件使用
├── registry/                 # Registrar / Service / Watcher 接口 + etcd 子模块
│   └── etcd/
├── transport/                # 协议服务器
│   ├── http/                 # Gin + MCP + pprof + 静态资源
│   ├── grpc/                 # grpc-go + 内置 registry resolver
│   │   └── resolver/
│   └── cli/                  # 自研 TCP CLI（握手 / 补全 / 命令 / 帧）
├── admin/                    # 独立子模块：完整 RBAC 后台服务示例
├── scripts/                  # release.sh / detect-changed-modules.sh / semver-bump.sh
├── docs/                     # 内部 spec / plan
└── .github/workflows/ci.yml  # 每个子模块的 race 测试 + gofmt + vet
```

| 目录 / 文件 | 描述 |
| --- | --- |
| `app.go` | `Service` 核心，框架入口 |
| `options.go` | Functional options 与 `WithAppContext` 辅助 |
| `types.go` | 框架接口（`Application` / `Scope` / `Server` / `Traceable` ...） |
| `metadata/` | 请求级 header tee-reader/writer + `FromContext` / `NewContext` |
| `middleware/middleware.go` | `Chain`、`Abort` / `IsAbort`、CLI 专用的 `RequestMetrics` |
| `middleware/auth/` | `JWT(keyFunc, opts...)` 与 `RateLimit(RateLimitOpts)`，含 `NewMemoryLimiter` |
| `infra/broker/` | `Broker` 接口（`Init/Publish/Subscribe/Destroy`）+ `Message/Publication/Subscription` |
| `infra/broker/nats/` | NATS 实现，订阅支持 queue group（`SubscriberOptions.Group`） |
| `infra/cache/` | `Cache` 接口 + 泛型 `TypedCache[T]`，提供 `Default()`（内存默认实现） |
| `infra/cache/memory/` | 进程内缓存，含深 clone（避免别名写入污染缓存） |
| `infra/cache/redis/` | `*redis.Client` 适配；前缀默认 `"cache:"`，可由 `WithContext` 注入的 `Application.Name` 覆盖 |
| `infra/telemetry/` | `Tracer / Span / MetricsRecorder` 接口与 `MetadataCarrier / HTTPHeaderCarrier` |
| `infra/telemetry/otel/` | `NewProvider(name, opts...)` 与 `NewTracer(name, provider)` |
| `infra/telemetry/prometheus/` | `NewRecorder(prometheus.Registerer)` + `MetricsHandler()` |
| `infra/logger/` | `Logger` 接口（slog 适配）+ `Default()` |
| `pkg/errs/` | `Code` 类型 + sentinel errors + `HTTPStatus/GRPCStatus` 映射 |
| `pkg/httpclient/` | `Client` 流式 API，含 `New()`（每个实例独立 `*http.Client`，避免共享 `DefaultClient`） |
| `pkg/bytepool/` | `GetBytes(size)/PutBytes` 分层池、`GetBuffer/PutBuffer` |
| `pkg/netutil/` | `EffectiveAddr(addr, listener)`、`LocalIP()` |
| `pkg/proto/` | protoc descriptor 扩展，配合 `protoc-gen-go-aeus` 插件 |
| `registry/` | `Registrar` 接口 + `Service / Watcher` 数据结构 + 全部 options |
| `registry/etcd/` | etcd v3 实现，支持 TTL 续约 + `HealthChecker.Status()` |
| `transport/http/` | Gin 引擎；可选 MCP server（`WithMCP`/`WithMCPServer`...）、TLS、CORS、健康检查、metrics、pprof |
| `transport/grpc/` | `Server` + `Dial(ctx, target, opts...)`（带 registry resolver）+ resolver 子包 |
| `transport/cli/` | 自研 CLI 协议（`Feature = []byte("CLI")`），提供 `Client` 与 `Handle(path, desc, fn)` |
| `admin/` | 独立子模块：完整 RBAC 后台，演示 JWT + 限流 + 自动建表 + REST 资源注册 |
| `scripts/release.sh` | 按模块变更自动打 tag（详见下文「发布」） |
| `.github/workflows/ci.yml` | 矩阵测试：每个子模块跑 `gofmt -l` + `go vet` + `go test -race -count=1 ./...`，并为 etcd/nats 子模块拉起对应容器 |

---

## 开发指南

### 1. HTTP 服务（基于 Gin）

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/http"
)

httpSrv := http.New(
    http.WithAddress(":8080"),
    http.WithCORS(),
    http.WithEnableHealth(true),   // /health, /ready
    http.WithEnableMetrics(true),  // /metrics (Prometheus)
)

httpSrv.GET("/ping", func(ctx *http.Context) error {
    return ctx.Success(map[string]string{"status": "up"})
})

// 原生 http.Handler
httpSrv.Handle("GET", "/raw", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
})

// 静态资源 + 自动 gzip
httpSrv.Webroot("/", true, http.FS(embedFS))

// 中间件链
httpSrv.Use(auth.RateLimit(auth.RateLimitOpts{
    Path: "/auth/login", Keys: []string{"ip:{ip}", "user:{user}"},
    Capacity: 5, Refill: 1.0 / 60,
}))
httpSrv.Use(auth.JWT(keyFunc, auth.WithAllow("/auth/login", "/auth/refresh-token")))

svc := aeus.New(
    aeus.WithName("my-service"),
    aeus.WithServer(httpSrv),
)
svc.Run()
```

可用选项（`transport/http`）：

| Option | 行为 |
| --- | --- |
| `WithAddress(string)` | 监听地址（覆盖 `HTTP_PORT`） |
| `WithNetwork(string)` | 网络类型（默认 `tcp`） |
| `WithCertFile/WithKeyFile` | 启用 TLS |
| `WithCORS()` | 开启 CORS 拦截器 |
| `WithHandler(http.Handler)` | 注入自定义 `http.Handler` |
| `WithGinOptions(...gin.OptionFunc)` | 透传 gin 选项 |
| `WithLogger(logger.Logger)` | 自定义 logger |
| `WithContext(ctx)` | 注入 context |
| `WithDebug(bool)` | 开启调试模式（需配合 `WithDebugAddr` 才暴露 pprof） |
| `WithDebugAddr("127.0.0.1:6060")` | pprof 监听地址；**仅 loopback**，非 loopback 启动会被拒绝 |
| `WithMCP(http.MCPConfig)` | 启用 MCP server（`Path` 缺省 `/mcp`，`Instructions` 映射到 ServerOptions；`Streamable` 回调可微调 `StreamableHTTPOptions`） |
| `WithMCPServer(*mcp.Server)` | 注入预构建的 MCP server（自行注册 Tool/Prompt/Resource） |
| `WithMCPTokenVerifier(v)` | 自定义 bearer token 校验；缺省为对 `MCPConfig.Authorization` 的常量时间比较 |
| `WithEnableHealth(bool)` | 暴露 `/health` + `/ready` |
| `WithEnableMetrics(bool)` | 暴露 `/metrics`（使用 `prometheus.DefaultRegisterer`） |

`Server` 暴露的方法：`Engine() *gin.Engine`、`Mcp() *mcp.Server`、`Endpoint(ctx)`、`Use(...)`、`GET/POST/PUT/HEAD/PATCH/DELETE`、`Handle(method, uri, http.HandlerFunc)`、`Webroot(prefix, autoCompress, fs)`、`SetTracer/SetMetrics`（实现 `Traceable`）。

`HandleFunc` 签名为 `func(ctx *http.Context) error`；通过 `ctx.Success(v)` / `ctx.Error(code, msg)` 写响应，`ctx.Bind(&v)` 解析请求体。

### 2. gRPC 服务与客户端

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/grpc"
)

grpcSrv := grpc.New(grpc.WithAddress(":9090"))
grpcSrv.RegisterService(&yourpb.YourService_ServiceDesc, &yourService{})

svc := aeus.New(
    aeus.WithName("my-service"),
    aeus.WithServer(grpcSrv),
    aeus.WithRegistry(reg), // 让 gRPC client 通过 etcd resolver 发现 endpoint
)

// 客户端
conn, _ := grpc.Dial(ctx, "your-service://my-service",
    grpc.WithRegistry(reg),
    grpc.WithGrpcDialOptions(grpc.WithBlock()),
)
defer conn.Close()
```

gRPC 服务暴露：`RegisterService(*grpc.ServiceDesc, any)`、`Use(...)`、`SetTracer/SetMetrics`、`Endpoint(ctx)`、`Start/Stop`。
gRPC 客户端：`grpc.Dial(ctx, target, opts...)`；`target` 形如 `"<service-name>://<service>"`，由 `transport/grpc/resolver` 通过 `registry` 解析 endpoint。

可用选项：服务侧 `WithNetwork/WithAddress/WithContext/WithLogger`；客户端侧 `WithTLS(*tls.Config)`、`WithRegistry(registry.Registrar)`（`WithRegistrar` 为 deprecated 别名）、`WithGrpcDialOptions(...grpc.DialOption)`。

### 3. CLI 服务

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/cli"
)

cliSrv := cli.New(cli.WithAddress(":7000"))
cliSrv.Handle("/user create", "Create a new user", func(ctx *cli.Context) error {
    name := ctx.Argument(0)
    return ctx.Success(map[string]string{"id": "u-1", "name": name})
})
cliSrv.Use(authMiddleware)

svc := aeus.New(
    aeus.WithName("my-service"),
    aeus.WithServer(cliSrv),
)
svc.Run()
```

> `Handle(pathname, description string, cb HandleFunc)` 三参签名；`/help` 命令会自动注册并列出所有命令。
> 客户端用 `cli.NewClient(ctx, "host:port")`，通过 `client.Execute("user create alice")` 调用。

### 4. 服务注册（etcd）

```go
import (
    "github.com/sjlit/aeus/registry"
    "github.com/sjlit/aeus/registry/etcd"
)

reg := etcd.New()
if err := reg.Init(
    registry.WithAddress("localhost:2379"),
    registry.WithTimeout(5*time.Second),
    registry.WithRegistryContext(etcd.WithContext(context.Background(), &etcd.ClientOptions{
        Username: "root", Password: "***",
    })),
); err != nil {
    log.Fatal(err)
}

// 可选健康检查（实现 HealthChecker 能力接口）
status, err := reg.(registry.HealthChecker).Status(ctx)

svc := aeus.New(
    aeus.WithRegistry(reg),
    aeus.WithRegistrarTimeout(30*time.Second), // TTL + 心跳
)
```

`Registry` 接口：`Name / Init / Register / Deregister / GetService / Watch`。`WithRegistrar` 已被标记 `Deprecated`，请使用 `WithRegistry`。

`registry.Watcher.Next()` 返回实例列表（首次调用或实例变更时返回；否则阻塞直到 ctx 超时）。`registry.HealthChecker` 是可选能力接口，未实现的 registrar 默认视为 healthy。

### 5. 缓存（内存 / Redis / 泛型）

```go
import (
    "github.com/sjlit/aeus/infra/cache"
    "github.com/sjlit/aeus/infra/cache/memory"
)

// 内存缓存
c := memory.NewCache(
    memory.Expiration(5*time.Minute),
    memory.WithLogger(logger.Default()),
)
_ = c.Store(ctx, "key", someValue, cache.NoExpiration)
var out MyType
_ = c.Load(ctx, "key", &out)

// 也可直接走包级助手（默认实例，进程内单例）
_ = cache.Store(ctx, "key", v, cache.NoExpiration)

// 泛型包装
users := cache.NewTypedCache[User](c)
u, _ := users.Load(ctx, "user:1")
```

`Cache` 接口：`Load / Exists / Store / Delete / String`；`Store` 的 `d` 取 `cache.NoExpiration (-1)` 表示永不过期，取 `cache.DefaultExpiration (0)` 走 memory options 的 `Expiration` 默认值。

```go
import (
    goredis "github.com/redis/go-redis/v9"
    "github.com/sjlit/aeus/infra/cache/redis"
)

c := redis.NewCache(
    redis.WithClient(goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})),
    redis.WithPrefix("myapp:"),
    redis.WithContext(aeus.WithAppContext(context.Background(), svc)),
)
```

`redis.NewCache` 会从 ctx 中取 `aeus.Application.Name()` 作为前缀的一部分（最终 key 形如 `<service-name>:<prefix><user-key>`），方便多服务共享一个 Redis。

### 6. 消息代理（NATS）

```go
import (
    "context"

    "github.com/sjlit/aeus/infra/broker"
    "github.com/sjlit/aeus/infra/broker/nats"
)

bkr := nats.New()
initCtx := nats.NewOptionContext(context.Background(), &nats.Options{
    URL: "nats://localhost:4222",
})
subCtx := nats.NewSubscriberOptionsContext(initCtx, &nats.SubscriberOptions{
    Group: "workers", // queue group
})
_ = bkr.Init(initCtx)

_ = bkr.Publish(context.Background(), "user.created", &broker.Message{
    Header: map[string]string{"x-source": "my-service"},
    Body:   []byte(`{"id":"u-1"}`),
})

_, _ = bkr.Subscribe(subCtx, "user.created", func(ctx context.Context, p broker.Publication) error {
    msg, err := p.Message()
    if err != nil {
        return err
    }
    log.Println("topic:", p.Topic(), "body:", string(msg.Body))
    return nil
})

defer bkr.Destroy(context.Background())
```

`Broker` 接口：`Init / Publish / Subscribe / Destroy`。`Init` 时**必须**通过 `NewOptionContext` 把 `*Options` 挂到 ctx 上（broker 内部从 ctx 读 URL/Options）；订阅要 queue group 时用 `NewSubscriberOptionsContext`。

### 7. HTTP 客户端

```go
import "github.com/sjlit/aeus/pkg/httpclient"

client := httpclient.New() // 每个实例独立 *http.Client，避免共享 DefaultClient 状态

resp, err := client.Post("https://api.example.com/users").
    SetContext(ctx).
    SetHeader(http.Header{"X-Trace": {"abc"}}).
    AddQuery("limit", "10").
    SetBody(map[string]any{"name": "alice"}).
    Do()

// 自动 JSON / XML 解码 + 错误码透传
var out User
err = client.Get("users/1").SetContext(ctx).Response(&out)

// 流式下载（路径穿越会被拦截）
err = client.Get("files/report.pdf").SetContext(ctx).Download("/tmp/report.pdf")

// 拦截器 + tracer 注入 + 自定义 transport
client := httpclient.New().
    SetTracer(tracer).
    SetBaseURL("https://api.example.com").
    BeforeRequest(func(c *http.Client, r *http.Request) error {
        log.Println("→", r.Method, r.URL)
        return nil
    }).
    AfterRequest(func(c *http.Client, r *http.Request, res *http.Response) error {
        log.Println("←", res.StatusCode)
        return nil
    })
```

请求链 API：`SetContext / AddQuery / SetQuery / AddFormData / SetFormData / SetBody / SetContentType / AddHeader / SetHeader`，终态方法 `Do() (*http.Response, error)`、`Response(v any) error`（自动 JSON / XML 解码 + 错误响应包装）、`Download(path string) error`（含路径穿越检查）。客户端构造器侧：`SetBaseURL / SetCookieJar / SetClient / SetTransport / SetTracer / BeforeRequest / AfterRequest`。包级 `Get / Post / Do` 函数为 `Deprecated`。

### 8. 中间件

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

超过限流时返回 `errs.CodeTooManyAttempts (4010)`（HTTP 429），并通过 metadata TeeWriter 写入 `Retry-After` 响应头。详细决策、占位符语义、多实例部署影响见 [`middleware/auth/README.md`](middleware/auth/README.md)。

### 9. 可观测性

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

### 10. Logger

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

### 11. 错误码（pkg/errs）

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

### 12. Metadata

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

### 13. pkg/bytepool / pkg/netutil

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

## 测试与 CI

仓库根目录的 `.github/workflows/ci.yml` 对每个 Go 模块独立跑：

```
- gofmt -l $(find . -name '*.go')   # 格式检查
- go test -race -count=1 ./...      # 带 race 检测的单元 / 集成测试
- go vet ./...                       # 静态检查
```

矩阵包含：根模块、`transport/http`、`transport/grpc`、`transport/cli`、`registry/etcd`、`infra/cache/redis`、`infra/broker/nats`、`infra/telemetry/otel`、`infra/telemetry/prometheus`。

`registry/etcd` 与 `infra/broker/nats` 子模块的集成测试依赖外部服务；CI 会拉起 `quay.io/coreos/etcd:v3.5.18`（监听 `0.0.0.0:2379`）和 `nats:2.11-alpine`（监听 `4222`）作为 service container，并通过 runner 侧端口轮询确保就绪后再跑测试。

本地完整跑测试：

```bash
cp go.work.example go.work   # 多模块协作开发
go test -race ./...
```

---

## 发布

仓库是多模块结构：根模块与每个子模块都有**独立版本号**，只有内容发生变更的模块才会升版本并打 tag，未变更的模块保持原版本。版本号全部由 git tag 推导，无需手工维护。

标签格式：根模块 `v1.0.3`，子模块 `transport/http/v1.0.3`。

### 发布步骤

```bash
# 1. 工作区必须干净（release 会拒绝脏工作区）
git status   # 确保无未提交改动

# 2. 自动检测变更模块，patch+1 并打 tag
make release

# 可选参数
make release DRY_RUN=1                        # 只打印将发布的模块和版本，不执行
make release VERSION=v1.1.0                   # 根模块升到指定版本
make release MODULES="transport/http=v1.2.0"  # 指定子模块版本

# 3. 推送
git push origin main
git push origin --tags
```

若自上次发布以来没有模块内容变更，`make release` 会中止并打印 `nothing to release: 自上次发布以来没有模块内容变更`。

`scripts/release.sh` 还会自动同步本次发布的子模块对根模块的 `require`（即便根模块没发布，也会对齐当前最新根 tag），避免子模块引用过期根版本。

### 消费者侧配置

仓库托管在 GitHub，模块可直接 `go get github.com/sjlit/aeus/...` 引入，无需额外 GOPROXY 配置。

---

## 贡献指南

### 代码规范

- 遵循 Go 官方 `gofmt` 格式化标准（CI 强制）
- 公共导出函数和类型必须有 godoc 注释
- 错误处理优先用 `pkg/errs` 构造业务错误（`errs.New` / `errs.Newf` / `errs.Wrap`）；不要在 framework 层直接返回 `http.StatusXxx` 数字
- 子模块边界清晰：往 `transport/http` 加功能时不要 import `registry/etcd` 这种重型依赖

### 环境变量

| 环境变量 | 默认 | 行为 |
| --- | --- | --- |
| `AEUS_DEBUG` | `false` | 解析失败时回退 `false`，不 panic |
| `HTTP_PORT` | `:0` | 空字符串回退 `:0`；非空且非法 → **panic**（fail-fast） |
| `GRPC_PORT` | `:0` | 同上 |
| `CLI_PORT` | `:0` | 同上 |

> 生产环境请通过对应 transport 的 `WithAddress("host:port")` 显式指定地址，不要依赖环境变量。

### PR 流程

1. Fork 仓库并基于 `dev` 创建特性分支
2. 提交代码并保证本地 `go test -race ./...` 与 `gofmt -l` 通过
3. 提交 Pull Request 到 `dev` 分支
4. 经过代码审查与 CI 通过后合并到 `main`，由 maintainer 触发发布流程

### 测试要求

- 新增功能必须附带单元测试
- 并发代码路径至少一个 `-race` 通过的测试
- 集成测试在缺外部依赖时 `t.Skip` 即可（CI 会拉起对应容器）

### 本地多模块开发

```bash
cp go.work.example go.work
# 编辑 go.work 按需调整
go test ./...
```

`go.work.example` 已列出所有子模块，按需启用即可在同一仓库中跨模块改动。