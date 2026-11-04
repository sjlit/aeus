# HTTP 服务（基于 Gin）

```go
import (
    aeus "github.com/sjlit/aeus"
    "github.com/sjlit/aeus/transport/http"
)

httpSrv := http.New(
    http.WithAddress(":8080"),
    http.WithCORS(http.CORSConfig{
        AllowOrigins: []string{"https://admin.example.com"},
    }),
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
| `WithCORS(CORSConfig)` | 开启 CORS；`AllowOrigins` 必填，空配置或 `*` + credentials 会在启动时 panic |
| `WithHandler(http.Handler)` | 注入自定义 `http.Handler` |
| `WithGinOptions(...gin.OptionFunc)` | 透传 gin 选项 |
| `WithLogger(logger.Logger)` | 自定义 logger |
| `WithContext(ctx)` | 注入 context |
| `WithDebug(bool)` | 开启调试模式（需配合 `WithDebugAddr` 才暴露 pprof） |
| `WithDebugAddr("127.0.0.1:6060")` | pprof 监听地址；**仅 loopback**，非 loopback 启动会被拒绝 |
| `WithEnableHealth(bool)` | 暴露 `/health` + `/ready` |
| `WithEnableMetrics(bool)` | 暴露 `/metrics`（使用 `prometheus.DefaultRegisterer`） |

## CORS

```go
http.New(http.WithCORS(http.CORSConfig{
    AllowOrigins:     []string{"https://admin.example.com", "http://localhost:5173"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    ExposeHeaders:    []string{"X-Request-Id"},
    AllowCredentials: false,
    MaxAge:           12 * time.Hour,
}))
```

`AllowOrigins` 接受精确的 `scheme://host[:port]`，也支持 `"*"`。`"*"` 不能与
`AllowCredentials` 同时使用。未配置 `AllowHeaders` 时，预检请求会回显
`Access-Control-Request-Headers`。未配置 CORS 时浏览器只能同源访问。

## MCP

MCP server 位于独立子包 `transport/http/mcpserver`；只使用 HTTP transport 的
应用不会引入 MCP SDK，也不会在 `go.sum` 中下载它。

```go
import (
    ghttp "github.com/sjlit/aeus/transport/http"
    "github.com/sjlit/aeus/transport/http/mcpserver"
)

httpSrv := ghttp.New(ghttp.WithAddress(":8080"))
mcpSrv := mcpserver.New(mcpserver.Config{
    Authorization: "change-me",
    Instructions:  "internal tools",
})
mcpSrv.Attach(httpSrv) // 必须在 httpSrv.Start 之前调用

// 通过 mcpSrv.SDK() 注册 Tool / Prompt / Resource
sdk := mcpSrv.SDK()
```

`mcpserver.Config` 字段：`Name`（默认 `aeus`）、`Version`（默认 `0.0.0`）、
`Path`（默认 `/mcp`）、`Instructions`、`SessionTimeout`、`Authorization`、
`TokenVerifier`、`Streamable`。

`Server` 暴露的方法：`Engine() *gin.Engine`、`Endpoint(ctx)`、`Use(...)`、
`GET/POST/PUT/HEAD/PATCH/DELETE`、`Handle(method, uri, http.HandlerFunc)`、
`Webroot(prefix, autoCompress, fs)`、`SetTracer/SetMetrics`（实现 `Traceable`）。

`HandleFunc` 签名为 `func(ctx *http.Context) error`；通过 `ctx.Success(v)` /
`ctx.Error(code, msg)` 写响应，`ctx.Bind(&v)` 解析请求体。
