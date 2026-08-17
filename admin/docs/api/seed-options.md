# 11. Seed 与 Functional Options

`admin.New(...).Setup(ctx)` 是模块入口,本节列出所有可调用的装配选项与引导流程。

## 11.1 admin.New 的选项

```go
s := admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
    admin.WithOpenAPI(true),
    admin.WithResponder(myResponder),
    admin.WithTenantResolver(myResolver),
)
```

| Option | 类型 | 默认 | 说明 |
|---|---|---|---|
| `WithDB(db)` | `*gorm.DB` | nil | 必填。Setup / RegisterModel 缺则返回 `ErrDBRequired` |
| `WithRouter(router)` | `rest.Router` | nil | 必填。Setup / RegisterModel 缺则返回 `ErrHTTPRequired` |
| `WithOpenAPI(enabled)` | bool | false | 每个资源暴露 `/system/<table>/openapi.json` |
| `WithResponder(r)` | `rest.Responder` | `newResponder()` | 替换为应用自定义 envelope writer |
| `WithTenantResolver(r)` | `middleware.Resolver` | `FromClaimsResolver` | 见 [tenant-scope.md](./tenant-scope.md) |

### 返回的 Server

```go
type Server struct { opts *Options }

func New(opts ...Option) *Server
```

- `New(...)`:构造,不触发任何迁移/注册。
- `Server.Setup(ctx) error`:核心入口——安装租户回调、迁移 `schema.Schema` 元数据表、注册 8 个内置模型、校验孤儿 Parent、挂载 `/schema/:module/:table`。
- `Server.RegisterModel(model) error`:挂载应用自定义模型(Setup 之后调用);同样触发自动菜单 / 自动权限目录。

## 11.2 Seed

```go
func Seed(db *gorm.DB, adminUser, adminPassword string) error
```

幂等地把数据库收敛到引导状态。**每次启动都应调用**(同 Setup 顺序:先 Setup,后 Seed)。

**保证的行为**:

| 项 | 第一次 | 已存在 |
|---|---|---|
| 超管角色(`Key="admin"`, `IsSuper=true`, `Builtin="true")` | 新建(随机 uuid 作为 `tenant_id`) | 保留,缺失 `IsSuper` 字段时补齐升级 |
| 用户(`UID="admin"`, `Username=adminUser`, `Password=bcrypt(adminPassword)`) | 新建 | 保留 |
| 租户实体行(`Name="默认租户"`, `Status="enabled"`, `id=role.TenantID`) | 新建 | 保留;旧数据库孤儿 uuid 也会落地 |
| 超管角色授权 | 全目录(菜单 + API 权限码)插入 | 仅 diff 缺失项,add-only,不删任何现有授权 |

**事务语义**:role + user + grant 写入都在同一个 `db.Transaction(...)` 里,失败回滚——保证不会留下半成品。

**参数校验**:

| 失败 | Code | Message |
|---|---|---|
| `db == nil` | `1001` | `admin: Seed requires a non-nil *gorm.DB` |
| `adminUser == ""` 或 `adminPassword == ""` | `1001` | `admin: Seed requires non-empty adminUser and adminPassword` |

**时序约定**:

```
Setup(ctx)    // 先建 sys_menus / sys_permissions 目录
Seed(db, ...) // 再以调用时刻的目录为准,补全超管授权
```

颠倒顺序的后果:新目录这次不会授权,下次启动才会补齐。应用升级注册新模型后,同样靠下一次启动的 Seed 补齐。

## 11.3 AuthServiceOptions

```go
service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret(secret),
    service.WithTokenExpireSeconds(7200),
    service.WithTokenStore(store),
    service.WithBeforeLogin(fn),
    service.WithAfterLogin(fn),
    service.WithLoginLogger(fn),
)
```

| Option | 默认 | 校验 |
|---|---|---|
| `WithAuthServiceDB(db)` | nil | **必填**,nil → 构造时 panic |
| `WithAuthSecret(secret)` | "" | **必填非空**,空 → 构造时 panic |
| `WithTokenExpireSeconds(n)` | `7200`(2h) | ≤ 0 按默认处理 |
| `WithTokenStore(store)` | `NewMemoryTokenStore()` | nil → 默认内存存储(进程内,不持久化) |
| `WithBeforeLogin(fn)` | nil | 触发于 `req.Validate()` 之后,凭据查询之前;返回 error 短路 Login |
| `WithAfterLogin(fn)` | nil | 触发于 Login 全部成功之后,通知用,返回值忽略 |
| `WithLoginLogger(fn)` | nil | 成功+失败都触发;IP 来自 `metadata.RequestClientIP`,UA 来自 `User-Agent`;**同步执行**,panic 透传 |

详见 [auth.md](./auth.md)。

## 11.4 UserServiceOptions / RoleServiceOptions / MenuServiceOptions

```go
service.NewUserService(service.WithUserServiceDB(db), service.WithUserServiceCache(c))
service.NewRoleService(service.WithRoleServiceDB(db), service.WithRoleServiceCache(c))
service.NewMenuService(service.WithMenuServiceDB(db), service.WithMenuServiceCache(c))
```

三个 Service 的选项形状一致:

| Option | 默认 | 说明 |
|---|---|---|
| `With*DB(db)` | nil | UserService 必填(nil → panic);RoleService / MenuService nil 时返回 `1003 Unavailable` |
| `With*Cache(c)` | nil | 共享 `cache.Cache`;nil → dbcache 默认内存缓存 |

详见 [user.md](./user.md)、[role.md](./role.md)、[menu.md](./menu.md)。

## 11.5 PermissionServiceOptions / TenantServiceOptions

```go
service.NewPermissionService(service.WithPermissionServiceDB(db))
service.NewTenantService(service.WithTenantServiceDB(db))
```

只有 `With*DB(db)`(必填);无缓存。详见 [permission.md](./permission.md)、[tenant.md](./tenant.md)。

## 11.6 RegisterModel

应用自定义 GORM 模型通过 `Server.RegisterModel(model)` 挂载:

```go
if err := s.Setup(ctx); err != nil { /* ... */ }
if err := s.RegisterModel(&MyAppModel{}); err != nil { /* ... */ }
```

模型需要实现(按需):

| 接口 | 用途 |
|---|---|
| `gorm.Tabler` | 提供 `TableName() string` |
| `rest.ModuleNamer` | 提供 `ModuleName() string`(默认 `"system"`) |
| `rest.ScenarioProvider`(可选) | 声明该资源暴露的场景集(create / update / delete / search / detail / export);默认 6 个 |
| `models.MenuProvider`(可选) | 提供 `MenuEntry() MenuSpec`,自动建菜单行 |

`RegisterModel` 触发的副作用:

1. AutoMigrate 模型对应的表;
2. (若有 MenuProvider)AutoMigrate `sys_menus` 并插入/更新菜单行,`Component`/`Uri`/`ViewPath` 在 `MenuEntry()` 留空时由模块名 + 表名派生;
3. 按 ScenarioProvider 写入 `sys_permissions` 行,`Data` 形如 `"<METHOD> <URI>"`。

**前置条件**:

- `Server.Setup` 必须先跑完(rest/v3 需要 `schema.Schema` 元数据表已迁移);
- `s.opts.Router` 必须非空(否则 `ErrHTTPRequired`);
- `s.opts.DB` 必须非空(否则 `ErrDBRequired`)。

**批量注册 + 孤儿校验**:

应用批量调用 `RegisterModel` 之后,可以手动调一次 `validateMenuParentsRef`(`internal`)确保所有 `Parent` 引用闭合;Setup 末尾会自动做这一步。

## 11.7 错误类型

```go
var (
    ErrHTTPRequired  = errors.New("http server required")
    ErrDBRequired    = errors.New("db required")
    ErrRouterRequired = errors.New("router required")
)
```

- `ErrHTTPRequired`:`Setup` / `RegisterModel` 在 `WithRouter(nil)` 时返回。
- `ErrDBRequired`:`Setup` / `RegisterModel` / `RegisterSchemaEndpoint` 在 `WithDB(nil)` 时返回。
- `ErrRouterRequired`:`RegisterSchemaEndpoint` 单独校验。

## 11.8 装配模板

### 完整装配(生产)

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/glebarez/sqlite"
    "github.com/golang-jwt/jwt/v5"
    "github.com/sjlit/aeus/admin"
    "github.com/sjlit/aeus/admin/auth"
    "github.com/sjlit/aeus/admin/pb"
    "github.com/sjlit/aeus/admin/service"
    mwauth "github.com/sjlit/aeus/middleware/auth"
    ghttp "github.com/sjlit/aeus/transport/http"
    "gorm.io/gorm"
)

func main() {
    db, _ := gorm.Open(sqlite.Open("admin.db"), &gorm.Config{})

    httpSrv := ghttp.New()

    // JWT 中间件(必须在 Setup 之前挂好,这样 Setup 注册的路由也受保护)
    secret := os.Getenv("JWT_SECRET")
    httpSrv.Use(mwauth.JWT(
        func(*jwt.Token) (any, error) { return []byte(secret), nil },
        mwauth.WithClaims(auth.Claims{}),
        mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
        mwauth.WithAllow("/auth/login", "/auth/refresh-token"),
    ))

    // 装配 admin
    s := admin.New(
        admin.WithDB(db),
        admin.WithRouter(httpSrv),
        admin.WithOpenAPI(true),
    )
    if err := s.Setup(context.Background()); err != nil {
        log.Fatal(err)
    }

    // 应用自定义模型
    if err := s.RegisterModel(&MyAppModel{}); err != nil {
        log.Fatal(err)
    }

    // 引导(必须 Setup 之后)
    if err := admin.Seed(db, "admin", os.Getenv("ADMIN_PASSWORD")); err != nil {
        log.Fatal(err)
    }

    // 业务 RPC
    pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
        service.WithAuthServiceDB(db),
        service.WithAuthSecret(secret),
    ))
    pb.RegisterUserServiceRouter(httpSrv, service.NewUserService(service.WithUserServiceDB(db)))
    pb.RegisterRoleServiceRouter(httpSrv, service.NewRoleService(service.WithRoleServiceDB(db)))
    pb.RegisterPermissionServiceRouter(httpSrv, service.NewPermissionService(service.WithPermissionServiceDB(db)))
    pb.RegisterMenuServiceRouter(httpSrv, service.NewMenuService(service.WithMenuServiceDB(db)))
    pb.RegisterTenantServiceRouter(httpSrv, service.NewTenantService(service.WithTenantServiceDB(db)))

    log.Fatal(httpSrv.Start(context.Background()))
}
```

### P0 一键集成(本地开发)

```go
httpSrv := ghttp.New()
s := admin.New(admin.WithDB(db), admin.WithRouter(httpSrv))
if err := s.Setup(context.Background()); err != nil { log.Fatal(err) }

pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(
    service.WithAuthServiceDB(db),
    service.WithAuthSecret("dev-secret"),
))
if err := admin.Seed(db, "admin", "admin123"); err != nil { log.Fatal(err) }

log.Fatal(httpSrv.Start(context.Background()))
```

> 生产装配务必按"完整装配"补上 `mwauth.JWT(...)`(含 `WithPermissionChecker`)与 `WithAuthSecret` 从运行时通道取值。