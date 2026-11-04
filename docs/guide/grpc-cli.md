# gRPC 与 CLI


## gRPC 服务与客户端

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


## CLI 服务

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
