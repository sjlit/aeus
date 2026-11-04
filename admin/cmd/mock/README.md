# admin/cmd/mock — 真实可调试的 mock admin 服务

**目的**:把 admin 模块当作一个独立进程跑起来,便于:
- 用 curl / Postman / API 客户端调试全部接口(登录、CRUD、业务 RPC、OpenAPI)
- 给前端 (dashboard/web) 提供本地后端联调环境
- 验证改动对接口行为的影响(CRUD 路由、JWT、租户隔离等)

服务启动后监听在 `HTTP_PORT`(默认 `:8080`,对齐 dashboard/web 的 vite proxy)。所有 admin 模块的接口都在同一个端口上,**JWT 中间件只对白名单之外生效**。

## 运行

```bash
cd admin/cmd/mock
go run .
# 或用其他端口
HTTP_PORT=18080 go run .
```

或编译后再跑:

```bash
go build -o /tmp/mock .
/tmp/mock
```

启动成功后会看到类似:

```
time=... level=INFO msg=starting
time=... level=INFO msg="http server listen on: 192.168.6.76:8080"
time=... level=INFO msg=started
```

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `HTTP_PORT` | `8080` | 监听端口;transport/http 内部读取(由 `main.go` 在未设置时填入默认) |
| `JWT_SECRET` | `change-me` | JWT HMAC 签名密钥。**仅本地**,生产环境必须用真实来源(KMS / Vault)。 |
| `ADMIN_USER` | `admin` | Seed 创建的管理员用户名 |
| `ADMIN_PASSWORD` | `Admin123` | Seed 创建的管理员密码 |
| `AEUS_DB_PATH` | `./mock.db` | SQLite 文件路径；从仓库根目录运行时建议显式设置，避免在根目录生成 `mock.db` |
| `AEUS_VUE_OUTPUT_DIR` | `../../web/src/views` | 自动生成 Vue 视图的目录（仅启用生成时使用） |

dev 默认值写在 `scope.go` 的 `default*` 常量里,改名/换值直接改源码即可。

## 数据持久化

SQLite 文件固定在 `./mock.db`(在执行 `go run` 的目录下)。Seed 是 **幂等收敛** 的 —— 每次启动确保超管角色(`key='admin'`, `is_super=true`)、admin 用户存在,并把全部菜单/接口授权补齐给所有超管角色,所以多次启动会保留数据且新增模型自动获得授权。

**重置数据** 只需要删 `./mock.db` 重启。

## 默认账号 & 登录

```bash
curl -s -XPOST localhost:$PORT/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Admin123"}'
```

返回 `{code, message, data: {access_token, refresh_token, uid, username, expires, tenant_id, tenant_name}}`。

`expires` 是 access token TTL 的相对秒(默认 7200 = 2 小时);refresh token 固定 48 小时。

错密码返回 `{"code":4005,"message":"invalid username or password"}`,HTTP 状态码 200(业务码走 envelope 的 `code`,不走 HTTP status,见 admin/README §响应格式)。

## 接口速查

完整接口契约见 [admin/README.md](../README.md#api-一览)。本服务暴露的全部端点:

### 公开(无需 token,JWT 白名单)

| 路径 | 方法 | 说明 |
|---|---|---|
| `/auth/login` | POST | 登录 |
| `/auth/refresh-token` | POST | 刷新 access token |
| `/auth/logout` | POST | 登出（JWT 白名单放行，handler 自行校验 body 中的 access/refresh token） |
| `/health` | GET | 探针(transport/http `WithEnableHealth(true)`) |
| `/ready` | GET | 探针 |
| `/debug/pprof/*` | GET | 仅 loopback 独立端口 `127.0.0.1:6060`（`WithDebugAddr`），不在主监听端口 |

### 需 JWT(`Authorization: Bearer <access_token>`)

业务 RPC:

| 路径 | 方法 | 说明 |
|---|---|---|
| `/user/profile` | GET | 当前用户资料 |
| `/user/profile` | PATCH | 更新资料 |
| `/user/change-password` | POST | 改自己的密码 |
| `/user/reset-password` | POST | 超管重置他人密码(需 is_super 角色) |
| `/user/set-avatar` | POST | 改头像 |
| `/user/menus` | GET | 当前用户可见菜单(扁平) |
| `/user/permissions` | GET | 当前用户 api 权限码 |
| `/role/options` | GET | 角色下拉项 |
| `/role/permissions` | GET / PUT | 角色的菜单 + 权限(整体替换) |
| `/role/menus` | GET | 某角色可见菜单 |
| `/permission/catalog` | GET | 全局权限码目录 |
| `/menu/tree` | GET | 菜单树 |
| `/menu/options` | GET | 菜单下拉项 |
| `/menu/breadcrumb` | GET | 面包屑 |

通用 CRUD(rest/v3 自动注册,以 `User` 为例):

| 路径 | 方法 | 说明 |
|---|---|---|
| `/system/sys_users` | GET | 列表 / 搜索 |
| `/system/sys_user/detail/:id` | GET | 详情 |
| `/system/sys_user` | POST | 创建 |
| `/system/sys_user/:id` | PUT | 更新 |
| `/system/sys_user/:id` | DELETE | 删除 |
| `/system/sys_user/export` | GET | 导出 |
| `/system/sys_user/openapi.json` | GET | OpenAPI 文档 |

其余可写资源同模式,只是表名替换:`sys_role` / `sys_menu` / `sys_department` / `sys_tenant`。复数形式注意 `Menu` → `sys_menuses`(rest/v3 机械复数,不语义化)。

安全/审计表（`sys_permission` / `sys_role_permission` / `sys_audit` / `sys_login_log`）只挂载 detail/list/export，没有 POST/PUT/DELETE：权限目录由 Setup/Seed 管理，角色授权走 `/role/permissions`，审计/登录日志由各自钩子写入。

## 调试小抄

**带 token 的请求示例**:

```bash
TOKEN=$(curl -s -XPOST localhost:18080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Admin123"}' \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

# 调业务 RPC
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/user/profile

# 调 CRUD
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/system/sys_users

# 看 OpenAPI
curl -s localhost:8080/system/sys_user/openapi.json | jq .
```

**401 调试**:401 通常意味着 JWT 校验失败(secret 不匹配 / token 过期 / 路径不在白名单)。检查:
- `JWT_SECRET` 在重启后是否变了
- token 是否真的从这次登录拿到(旧 token 会失效)
- 路径是否被白名单意外放行(看 `service.go` 的 `mw.WithAllow(...)`)

**租户隔离**:admin 默认的 tenant resolver 从 JWT claims 读 `TenantID`,所以同一份 mock 数据在不同登录下会被按租户隔离(虽然只有一个 admin 账号 + 一个 tenant,但代码路径走的是真租户逻辑)。

## 文件结构

```
admin/cmd/mock/
├── main.go      # aeus.New + transport/http 装配,加 /health /ready
├── scope.go     # 单例:DB + admin.Server + env getter(不调 Setup / Use / Seed)
├── service.go   # Init:Setup → 挂 JWT → 注册 4 个 RPC → Seed;Run:no-op
└── README.md    # 本文件
```

边界硬规则:**scope 只"持有什么",ServiceLoader 才"做什么"**。改初始化 vs 加载的拆分时,先想清楚这一步属于哪一侧。
