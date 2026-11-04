# 基础设施


## 服务注册（etcd）

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


## 缓存（内存 / Redis / 泛型）

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


## 消息代理（NATS）

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


## HTTP 客户端

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
