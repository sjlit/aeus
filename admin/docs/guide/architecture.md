# 响应格式与设计约定

## 响应格式

默认 responder 统一输出 `{code, message, data}` envelope；业务结果由 `body.code` 决定：

```json
// 成功（code = errors.OK = 0）
{"code": 0, "message": "", "data": { ... }}

// 业务失败（message 字段直接给前端 toast 文案）
{"code": 4003, "message": "permission denied"}
```

业务码定义见 [`pkg/errs/const.go`](../../../pkg/errs/const.go)（`OK/Invalid/NotFound/PermissionDenied/AccessDenied/TokenExpired` …）。前端拦截器以 `body.code` 为主，同时对 HTTP 401 触发 refresh；分流表见下方"业务码分流"。可通过 `admin.WithResponder(...)` 替换为应用自己的 responder。

> HTTP 状态码约定：admin responder 处理的业务成功/失败固定 HTTP `200`；由 `transport/http` 的 error interceptor 处理的中间件错误（JWT、限流、404、panic）按 `*errs.Error.HTTPStatus()` 映射为 `400/401/403/404/429/500/503/504`。两条路径共用同一个 `{code,message,data}` envelope；**业务码才是客户端分流的依据**，HTTP status 供网关/浏览器层使用。

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
