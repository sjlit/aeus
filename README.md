# AEUS

**轻量级 Go 微服务脚手架框架**

AEUS 是一个用于快速构建微服务的轻量级 Go 框架，支持 HTTP、gRPC、CLI 多协议同时运行，提供服务注册发现、缓存、消息代理等常用组件。

**项目地址**: github.com/sjlit/aeus

## 核心特性

- **多协议支持**：HTTP（Gin）、gRPC、CLI 同时运行
- **服务注册与发现**：基于 etcd 的服务注册中心抽象
- **缓存抽象**：内存缓存与 Redis 缓存统一接口
- **消息代理**：NATS pub/sub 消息模式
- **依赖注入**：基于反射的字段注入
- **链式 HTTP 客户端**：拦截器 + 中间件支持
- **结构化日志**：基于 Go slog 的日志接口
- **可观测性**：OpenTelemetry 分布式追踪 + Prometheus 指标采集
- **Protobuf 扩展**：自定义 MCP/Command/REST 代码生成选项

## 安装

引入核心模块（仅含接口和轻量默认实现）：

```bash
go get github.com/sjlit/aeus
```

按需引入传输层和基础设施实现：

```bash
# HTTP 服务（Gin）
go get github.com/sjlit/aeus/transport/http

# gRPC 服务
go get github.com/sjlit/aeus/transport/grpc

# CLI 服务
go get github.com/sjlit/aeus/transport/cli

# ETCD 注册中心
go get github.com/sjlit/aeus/registry/etcd

# Redis 缓存
go get github.com/sjlit/aeus/infra/cache/redis

# NATS 消息队列
go get github.com/sjlit/aeus/infra/broker/nats

# OpenTelemetry 链路追踪
go get github.com/sjlit/aeus/infra/telemetry/otel

# Prometheus 指标
go get github.com/sjlit/aeus/infra/telemetry/prometheus
```

> `middleware/auth`、`pkg/errs`、`infra/logger`、`pkg/httpclient`、`metadata` 等随核心模块一起发布，无需单独 `go get`。

## 架构图

```
app.go (Service)
    ├── transport/
    │   ├── http/   (Gin)
    │   ├── grpc/
    │   └── cli/
    └── registry/   (etcd)

infra/
    ├── broker/     (NATS)
    ├── cache/      (Memory / Redis)
    ├── telemetry/  (OTel / Prometheus)
    └── logger/

pkg/
    ├── errs/       # 错误码 + HTTP/gRPC 映射
    ├── httpclient/
    ├── bytepool/
    ├── netutil/
    └── proto/
```

## 设计立场

AEUS 定位为**脚手架框架**，在抽象与务实之间做了明确的取舍：

- **传输层绑定底层引擎，不承诺可替换**：HTTP 传输直接基于 Gin，gRPC 传输直接基于 grpc-go，CLI 为自研实现。`Server` / `Endpointer` / `Traceable` 接口负责的是**生命周期管理与框架集成**（启动、停止、端点上报、可观测性注入），**不是**底层引擎的可替换抽象。引入新引擎（如 Echo、Connect-RPC）意味着重写对应传输模块。
- **基础设施接口可替换**：Registry、Cache、Broker、Logger、Tracer 均为薄接口，底层实现（etcd / Redis / NATS / OpenTelemetry 等）可通过子模块替换，不修改业务代码。
- **依赖按需引入**：核心模块只含接口与轻量默认实现；重型依赖（Gin、gRPC、etcd、NATS、Redis、OTel）位于独立子模块，按需 `go get`。
- **环境变量端口约定**：`HTTP_PORT` / `GRPC_PORT` / `CLI_PORT` 用于本地开发默认端口；值为空或非法时仅输出告警并回退到随机端口（`:0`），生产环境请通过 `WithAddress` 显式指定。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Service` | 核心服务容器，管理生命周期和依赖注入 |
| `Server` | 协议服务器（HTTP/gRPC/CLI） |
| `Registry` | 服务注册与发现抽象 |
| `Broker` | 消息代理抽象（NATS） |
| `Cache` | 缓存抽象，支持 Memory 和 Redis |
| `HTTPClient` | 链式 API 的 HTTP 客户端 |
| `Tracer` | OpenTelemetry 分布式追踪接口 |
| `MetricsRecorder` | Prometheus 指标采集接口 |

## 快速开始

> 说明：`cmd/aeus` CLI 脚手架（`aeus new`）规划中，暂未提供。当前通过 `go get github.com/sjlit/aeus`
> 引入框架后，在你的 `main.go` 中直接组装 `Service` 即可运行。

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

各传输层与基础设施的组装方式见下方「开发指南」。

> `admin/` 子模块演示了一套完整的后台服务（含 Proto 定义与代码生成），用法见
> [`admin/README.md`](admin/README.md)；在 `admin/` 目录下执行 `make proto` 可重新生成 Proto 代码。

## 项目结构

AEUS 框架源码结构：

```
aeus/
├── app.go              # Service 核心
├── options.go          # Functional options
├── types.go            # 核心接口定义
├── metadata/           # 请求元数据与 context 传播
├── middleware/         # 中间件
│   └── auth/           # JWT 认证组件
├── infra/              # 可替换基础设施（接口 + 实现）
│   ├── broker/         # 消息代理抽象 & NATS 实现
│   │   └── nats/       # NATS 子模块
│   ├── cache/          # 缓存抽象 & Memory/Redis 实现
│   │   ├── memory/     # 内存缓存
│   │   └── redis/      # Redis 子模块
│   ├── telemetry/      # 可观测性接口与实现
│   │   ├── otel/       # OpenTelemetry 追踪器
│   │   └── prometheus/ # Prometheus 指标采集器
│   └── logger/         # 日志接口
├── pkg/                # 跨切面可复用库
│   ├── errs/           # 错误处理（错误码 + HTTP/gRPC 映射）
│   ├── httpclient/     # 链式 HTTP 客户端
│   ├── bytepool/       # 字节 / 缓冲对象池
│   ├── netutil/        # 网络工具
│   └── proto/          # Protobuf 扩展（command / mcp / rest）
├── registry/           # 服务发现抽象 & etcd 实现
├── transport/          # 协议服务器
│   ├── cli/            # CLI 传输
│   ├── grpc/           # gRPC 传输
│   └── http/           # HTTP 传输
├── admin/              # 完整的 RBAC 后台服务（独立子模块）
├── scripts/            # 发布 / 版本管理脚本
└── .github/            # CI 工作流
```

| 目录/文件 | 描述 |
|-----------|------|
| `app.go` | Service 核心，框架入口 |
| `metadata/` | 请求元数据（header）在 context 中的存取与传播 |
| `middleware/` | 中间件链与 JWT 认证 |
| `infra/broker/` | 消息代理抽象，NATS 实现 |
| `infra/cache/` | 缓存抽象，Memory 和 Redis 实现 |
| `pkg/errs/` | 统一错误码，映射 HTTP / gRPC 状态 |
| `pkg/httpclient/` | 链式 HTTP 客户端 |
| `infra/logger/` | 结构化日志接口 |
| `pkg/proto/` | Protobuf 代码生成扩展选项 |
| `infra/telemetry/` | OpenTelemetry 追踪 + Prometheus 指标 |
| `registry/` | 服务注册发现抽象，etcd 实现 |
| `transport/` | HTTP、gRPC、CLI 协议服务器 |

## 开发指南

### HTTP 服务开发

使用 Gin 框架提供 HTTP 接口：

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/http"
)

httpServer := http.New()
// 注册路由，handler 签名为 func(ctx *http.Context) error
httpServer.GET("/ping", func(ctx *http.Context) error {
    return ctx.Success(map[string]string{"status": "up"})
})

svc := aeus.New(
    aeus.WithServer(httpServer),
)
```

### gRPC 服务开发

定义 Proto 文件后，实现生成的接口并注册到 gRPC Server：

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/grpc"
)

grpcServer := grpc.New()
// 实现 proto 生成的接口并注册
// grpcServer.RegisterService(&yourpb.YourService_ServiceDesc, &yourService{})

svc := aeus.New(
    aeus.WithServer(grpcServer),
)
```

### 服务注册

使用 etcd 进行服务注册与发现（需在 `Run` 前显式 `Init`）：

```go
import (
    "log"

    "github.com/sjlit/aeus/registry"
    "github.com/sjlit/aeus/registry/etcd"
)

reg := etcd.New()
if err := reg.Init(registry.WithAddress("localhost:2379")); err != nil {
    log.Fatal(err)
}

svc := aeus.New(
    aeus.WithRegistry(reg),
)
```

### 缓存使用

缓存以独立包形式使用（不注入 `Service`），选择内存缓存或 Redis 缓存：

```go
import (
    "log"

    "github.com/sjlit/aeus/infra/cache"
    "github.com/sjlit/aeus/infra/cache/memory"
)

c := memory.NewCache()

// 存储与读取
if err := c.Store(ctx, "key", someValue, cache.NoExpiration); err != nil {
    log.Fatal(err)
}
var out MyType
if err := c.Load(ctx, "key", &out); err != nil {
    log.Fatal(err)
}
```

Redis 缓存位于 `infra/cache/redis` 子模块，接口与内存缓存一致。

### 消息发布/订阅

使用 NATS broker：

```go
import (
    "context"
    "log"

    "github.com/sjlit/aeus/infra/broker"
    "github.com/sjlit/aeus/infra/broker/nats"
)

bkr := nats.New()
ctx := nats.NewOptionContext(context.Background(), &nats.Options{
    Url: "nats://localhost:4222",
})
if err := bkr.Init(ctx); err != nil {
    log.Fatal(err)
}

// 发布消息
if err := bkr.Publish(context.Background(), "topic", &broker.Message{Body: []byte("hi")}); err != nil {
    log.Fatal(err)
}

// 订阅消息（Handler 携带 context.Context，可用于链路追踪与超时控制）
if _, err := bkr.Subscribe(ctx, "topic", func(ctx context.Context, p broker.Publication) error {
    msg, err := p.Message() // 解码消息
    _ = msg
    // 处理消息
    return err
}); err != nil {
    log.Fatal(err)
}
```

### HTTP 客户端

使用链式 API 调用外部服务：

```go
import "github.com/sjlit/aeus/pkg/httpclient"

client := httpclient.NewClient()

resp, err := client.Get("https://api.example.com/data").
    WithHeader("Authorization", "Bearer token").
    Do(context.Background())
```

### 可观测性

AEUS 内置 OpenTelemetry 分布式追踪和 Prometheus 指标采集：

```go
import (
    "log"

    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/infra/telemetry/otel"
    "github.com/sjlit/aeus/infra/telemetry/prometheus"
)

// 创建 Provider（进程级）
provider, shutdownProvider, err := otel.NewProvider("my-service")
if err != nil {
    log.Fatal(err)
}
defer shutdownProvider()

// 从 Provider 创建 Tracer（服务级）
tracer, _, err := otel.NewTracer("my-service", provider)
if err != nil {
    log.Fatal(err)
}

// 创建指标采集器（nil 使用 DefaultRegisterer）
recorder := prometheus.NewRecorder(nil)

svc := aeus.New(
    aeus.WithName("my-service"),
    aeus.WithTracer(tracer),
    aeus.WithMetrics(recorder),
)
```

启用后，HTTP/gRPC/CLI 服务器会自动：
- 为每个请求创建追踪 Span，标记 `SpanKindServer`
- 记录请求耗时、总量等指标
- 在日志中注入 `trace_id` / `span_id`
- 对外暴露 `/metrics` 端点（需 `WithEnableMetrics(true)`）

## 发布

AEUS 是多模块仓库，每个模块（根模块与各子模块）拥有**独立版本号**：只有内容发生变更的模块才会升版本并打标签，未变更的模块保持原版本不动。版本号全部由 git tag 推导，无需手工维护版本文件。

标签格式：根模块 `v1.0.3`，子模块 `transport/http/v1.0.3`。

### 发布步骤

```bash
# 1. 提交你的改动（工作区必须干净，release 会拒绝脏工作区）
# 2. 发布:自动检测变更模块，patch+1 并打标签
make release

# 可选参数:
make release DRY_RUN=1                        # 只打印将发布的模块和版本，不执行
make release VERSION=v1.1.0                   # 根模块升到指定版本
make release MODULES="transport/http=v1.2.0"  # 指定子模块版本

# 3. 推送到远端（代码和标签都要推）
git push origin main
git push origin --tags
```

若自上次发布以来没有模块内容变更，`make release` 会中止并提示 `nothing to release`。

### 消费者侧配置

仓库托管在 GitHub，模块可直接 `go get github.com/sjlit/aeus/...` 引入，无需额外配置。

## 贡献指南

### 代码规范

- 遵循 Go 官方 `gofmt` 格式化标准
- 公共导出函数和类型必须添加注释说明
- 错误处理使用 `pkg/errs` 包进行封装

### 环境变量

| 环境变量 | 描述 |
| --- | --- |
| AEUS_DEBUG | 是否开启 debug 模式 |
| HTTP_PORT | HTTP 服务端口（无效值仅告警，回退随机端口） |
| GRPC_PORT | gRPC 服务端口（无效值仅告警，回退随机端口） |
| CLI_PORT | CLI 服务端口（无效值仅告警，回退随机端口） |

### PR 流程

1. Fork 项目并创建特性分支
2. 提交代码并确保通过现有测试
3. 提交 Pull Request 到 main 分支
4. 经过代码审查后合并

### 测试要求

提交前请确保所有测试通过：

```bash
go test ./...
```

### 本地开发（Workspace）

如需同时修改主模块和子模块，使用 Go Workspace：

```bash
cp go.work.example go.work
# 编辑 go.work 按需调整，然后运行测试
go test ./...
```
