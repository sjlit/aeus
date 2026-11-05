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
- **业务 Proto-First**：`pkg/proto/{resource,admin,api,module}/v1` 提供 v2 注解，配合 `protoc-gen-go-aeus` 生成 GORM 实体、HTTP 路由、权限清单和后台菜单元数据；admin 内置安全模型仍保持手写。
- **Proto 扩展**：`pkg/proto/{command,mcp,rest}` 提供 protoc descriptor 扩展，兼容既有 v1 注解。
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

```bash
# 发布后：
go install github.com/sjlit/aeus/cmd/aeus@latest

# 不带 admin 的最小 HTTP 服务
aeus new my-service

# 带 admin 后台（JWT/RBAC + Seed + Vue 管理界面）
aeus new my-admin --template=admin

cd my-service && make run
# 或：
cd my-admin && make web && make run
```

> 在仓库内开发 CLI 时，请使用
> `go run ./cmd/aeus new ... --local="$PWD"`；`--local` 会生成指向当前
> checkout 的 `replace`，避免依赖尚未发布的根模块版本。

`admin` 模板首次启动后打开 <http://localhost:8080>，使用 `admin / Admin123`
登录；请立即修改默认密码和 `JWT_SECRET`。

在仓库内开发 CLI 时，用 `--local` 指向当前 checkout，避免依赖已发布版本：

```bash
go run ./cmd/aeus new /tmp/demo --template=admin --local="$PWD"
```

admin 模板内置业务 proto 示例。新增业务资源：

```bash
cd my-admin
make proto
go mod tidy
aeus gen
```

`aeus gen` 读取 `_aeus.json` manifest，自动生成 bootstrap 注册和迁移接线。详见
[业务 Proto-First](docs/guide/proto-first.md)。

手工组装 `Service` 仍然可用：

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
│   ├── response/             # 共享 {code,message,data} envelope（transport + admin）
│   └── proto/                # protoc descriptor 扩展（command / mcp / rest），配合
│                             #   protoc-gen-go-aeus 插件使用
├── registry/                 # Registrar / Service / Watcher 接口 + etcd 子模块
│   └── etcd/
├── transport/                # 协议服务器
│   ├── http/                 # Gin + pprof + 静态资源；MCP 在 http/mcpserver 子包
│   ├── grpc/                 # grpc-go + 内置 registry resolver
│   │   └── resolver/
│   └── cli/                  # 自研 TCP CLI（握手 / 补全 / 命令 / 帧）
├── admin/                    # 独立子模块：完整 RBAC 后台服务示例
├── cmd/aeus/                 # 脚手架 CLI 与 minimal/admin 模板
├── scripts/                  # release.sh / detect-changed-modules.sh / semver-bump.sh
├── docs/                     # 使用文档（guide / operations / security）
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
| `pkg/response/` | 共享 `{code,message,data}` envelope 与 `Write`，供 transport/http 与 admin 复用 |
| `pkg/proto/` | protoc descriptor 扩展，配合 `protoc-gen-go-aeus` 插件 |
| `registry/` | `Registrar` 接口 + `Service / Watcher` 数据结构 + 全部 options |
| `registry/etcd/` | etcd v3 实现，支持 TTL 续约 + `HealthChecker.Status()` |
| `transport/http/` | Gin 引擎；TLS、可配置 CORS、健康检查、metrics、pprof；MCP 在 `transport/http/mcpserver` 子包 |
| `transport/grpc/` | `Server` + `Dial(ctx, target, opts...)`（带 registry resolver）+ resolver 子包 |
| `transport/cli/` | 自研 CLI 协议（`Feature = []byte("CLI")`），提供 `Client` 与 `Handle(path, desc, fn)` |
| `admin/` | 独立子模块：完整 RBAC 后台，演示 JWT + 限流 + 自动建表 + REST 资源注册 |
| `cmd/aeus/` | `aeus new` 脚手架 CLI 与 `minimal` / `admin` 模板；`make sync-web` 同步 admin web 模板 |
| `docs/` | 使用文档入口，见 [`docs/README.md`](docs/README.md) |
| `scripts/release.sh` | 按模块变更自动打 tag（详见 [发布文档](docs/operations/testing-release.md)） |
| `.github/workflows/ci.yml` | 矩阵测试：每个子模块跑 `gofmt -l` + `go vet` + `go test -race -count=1 ./...`，并为 etcd/nats 子模块拉起对应容器 |

---

---

## 文档

| 文档 | 内容 |
| --- | --- |
| [脚手架 CLI](docs/scaffolding.md) | `aeus new`、`minimal` / `admin` 模板、`--local`、web 模板同步 |
| [业务 Proto-First](docs/guide/proto-first.md) | 业务 proto 注解、实体/权限/菜单生成 |
| [HTTP 服务](docs/guide/http.md) | Gin transport、CORS、MCP、静态资源、健康检查 |
| [gRPC 与 CLI](docs/guide/grpc-cli.md) | gRPC server/client、CLI 协议 |
| [基础设施](docs/guide/infrastructure.md) | etcd、缓存、NATS、HTTP 客户端 |
| [横切能力](docs/guide/cross-cutting.md) | 中间件、可观测性、logger、错误码、metadata |
| [测试、发布与贡献](docs/operations/testing-release.md) | CI 矩阵、多模块发布、PR 流程 |
| [安全上线清单](docs/security.md) | JWT、RBAC、迁移、Seed、CORS、默认口令 |

模块级文档：

- [`admin/README.md`](admin/README.md) — admin 模块入门与集成
- [`admin/docs/api/README.md`](admin/docs/api/README.md) — admin HTTP/RPC 接口参考

完整文档索引见 [`docs/README.md`](docs/README.md)。
