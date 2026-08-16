# admin ↔ dashboard/web 集成 — Design Spec

| 项 | 值 |
|---|---|
| 日期 | 2026-08-10 |
| 决策者 | goelea |
| 范围 | P0 契约层 + 最小可登录闭环 |
| 后端模块 | `/mobe/workspace/aeus/admin` (Go · rest/v3 + GORM · JWT) |
| 前端骨架 | `/mobe/js/dashboard/web` (Vue 3 · Vite · TS · Element Plus) |
| 状态 | §1 契约 13 项全部拍板;待 P0 实施 |
| 父文档 | `admin/docs/INTEGRATION-TODO.md`(细化任务清单) |
| 配套产物 | `admin/docs/CONTRACT.md`(契约基线,待新建)· `admin/docs/DECISIONS.md`(决策记录,待新建) |

---

## 1. 目标与非目标

**目标(P0)**:打开前端 → 看到登录页 → 输入 admin/admin → 调通真实后端 `/auth/login` → 拿到 token → 进 Dashboard → 401 自动刷新 → 登出。

**非目标(P0 不做,留给 P1/P2)**:
- 用户/角色/菜单的 CRUD 页面(`INTEGRATION-TODO.md §3`)
- 部门 / 权限 / 日志 / 审计(`§4`)
- 多租户切换 UI(`§4.5`)
- 按钮级权限指令(`§4.6`)
- proto → TS 自动生成(本 spec 决定**手写 DTO**)

---

## 2. 架构总览

```
┌────────────── dashboard/web (Vue 3) ──────────────┐
│  views/login/index.vue                            │
│        │                                          │
│        ▼                                          │
│  api/auth.ts ──► src/api/pb/auth.ts (手写 DTO)    │
│        │                                          │
│        ▼                                          │
│  utils/request.ts (axios + 拦截器)                │
│    · Authorization: Bearer ${accessToken}         │
│    · 业务码分支:4002 refresh / 4003·4005 跳登录   │
│        │                                          │
└────────┼──────────────────────────────────────────┘
         │ /auth/login  (相对路径 /,走 vite proxy)
         ▼
┌────────────── aeus HTTP Server (Go) ──────────────┐
│  transport/http + rest/v3                          │
│    · CORS 中间件(已自带)                          │
│    · 响应外壳:{code, message, data}                │
│        │                                          │
│        ▼                                          │
│  admin/pb.AuthService                              │
│    · /auth/login → JWT pair                       │
│    · 业务码:pkg/errs 语义表                     │
│    · TokenExpired(4002) → HTTP 401                │
│        │                                          │
│        ▼                                          │
│  admin/middleware/auth.JWT → 写 auth.Claims       │
│  {Uid, Role, TenantID} 到 ctx                      │
└───────────────────────────────────────────────────┘
```

**数据流关键路径**:
1. 前端 `POST /auth/login {username, password}` → 后端
2. 后端查 `sys_users` → 校验密码 → 生成 JWT(写 `tid/uid/role` 三个 claim)→ 响应 `{code: 0, message: "", data: {uid, username, expires, access_token, refresh_token, tenant_id, tenant_name}}`
3. 前端 `fromLoginDto(raw)` 把 snake 转 camel → `LoginResponseView {accessToken, refreshToken, expires: 相对秒, uid, username, tenant: {id, name}}`
4. `stores/auth.login(view)` 写 store + localStorage
5. 后续请求带 `Authorization: Bearer ${accessToken}` → 后端 `JWT()` 解析 → `ClaimsFromContext` → REST handler 读 `ctx.UserValue(auth.CtxKey)`
6. token 过期 → 后端响应 `{code: 4002, message: "token expired"}` + HTTP 401 → 前端拦截器进 refresh 队列

---

## 3. 契约层 13 项决策(逐条锁定)

> 每项给出**后端实现点** + **前端实现点** + **决策理由**。详细叙事见 `INTEGRATION-TODO.md §1`。

### 3.1 响应外壳 `{code, reason, data}` → `{code, message, data}`

- **后端**:
  - `admin/responder.go:9-13` 改 JSON tag `reason` → `message`;**成功 `Code: errors.OK = 0`,HTTP 仍为 200**;失败 `Code: errors.Invalid = 1001` 起步,`Message` 走 `*errors.Error.Message`(unwrap 后)
  - `transport/http/response.go:11` 同步改 `Response` struct 的 JSON tag(全 transport 共享,不止 admin)
  - **同步改的连带**: `transport/cli/context.go` 的 `Context.Error(code, reason string)` 参数名 → `message`;`responsePayload.Reason` → `Message`
- **前端**:零改动 ✅
- **理由**:前端已有 `data.message` 字段读取逻辑,改字段名免于两端适配

### 3.2 业务码分流按 `body.code`(不依赖 HTTP status)

- **后端**:
  - `pkg/errs/const.go` 不变(0/1001/4002/4003/4004/4005/...)
  - `pkg/errs/error.go:55-69` `HTTPStatus()` **加** `case TokenExpired: return http.StatusUnauthorized`(§3.2.1 bug 修复)
  - §3.2.1 修完后,token 过期响应 = HTTP **401** + body `code: 4002`
- **前端**:
  - `utils/request.ts:75-89` 响应拦截器业务码分支**改**:
    - 成功:`code === 0 | 200 | undefined` → 成功
    - 刷新:`code === 4002` → 触发 `tryRefresh()` 队列(原代码用 `code === 401`,**改成 4002**)
    - 跳登录:`code === 4003 | 4005` → `auth.logout('expired')` + `router.push('/login')`
    - 其它:`ElMessage.error(data.message)`
  - `utils/request.ts:147-152` HTTP 失败分支的 `status === 401` 处理**保留**(双保险:即便 body 不带 code,HTTP 401 仍触发 refresh)
- **理由**:后端业务码 4002 已稳定多年,前端按 body.code 触发刷新语义最准;修 HTTPStatus 让 HTTP 401 与业务码对齐,axios 默认 catch 也能识别

### 3.3 Token Header `Authorization: Bearer`

- **后端**:零动 ✅(`middleware/auth.JWT` 已用 `Authorization`)
- **前端**:
  - `constants/http.ts:14-17` **删** `HEADER.TOKEN = 'X-Console-Token'`
  - `utils/request.ts:67-69` 改 `config.headers.set('Authorization', 'Bearer ' + auth.accessToken)`
- **理由**:符合 aeus 默认鉴权约定,免维护自定义 header

### 3.4 LoginRequest/Response 字段对齐 proto

- **后端**:
  - `pb/auth.proto:11-21` LoginRequest={username, password}(已对齐),不动
  - `pb/auth.proto:15-21` LoginResponse **加**:
    ```proto
    string tenant_id   = 6;
    string tenant_name = 7;
    ```
  - `service/auth_service.go` Login 方法末尾:从 user record 读 `TenantID`,再用 `sys_tenants` 表查 `name`(fallback `name = tenant_id`);塞进 response
  - **proto 生成**:`make proto` 重生成 `*.pb.go`(已有 Makefile target,执行即可)
- **前端**:
  - `types/user.ts:32-36` `LoginResponse` 重写为视图层:
    ```ts
    interface LoginResponse {
      accessToken: string
      refreshToken: string
      /** 相对秒(UI 用) */
      expires: number
      uid: string
      username: string
      tenant?: { id: string; name: string }
    }
    ```
  - `stores/auth.ts:43-47` `login(payload)` 适配新结构:
    - `accessToken/refreshToken` 直接赋值
    - `expires: payload.expires`(相对秒)+ `Date.now()` 算 `expiresAt` 存 localStorage
    - `user: { uid, username, role: '' }`(role 暂时空,登录后异步调 `/user/permissions` 补)
    - `tenant: payload.tenant` 单独存一份(暂不渲染 UI,§4.5 扩展时用)
- **DTO 映射**:新建 `src/api/pb/auth.ts`,手写 `LoginRequestDto/ResponseDto`(snake)+ `fromLoginDto(raw)`(snake→camel)

### 3.5 Role.DataScope MVP 不消费

- **后端**:零动 ✅(`role.go:96-106` 已有 DataScope 字段)
- **前端**:`UserDto` 加 `dataScope?: number`(DTO 层),UI 不渲染;`stores/auth.user.role` 类型从 `string` 改为保留原 `string`(但 role 内容从 `GET /user/permissions` 拿,不在 LoginResponse)

### 3.6 多租户 MVP 单租户

- **后端**:`LoginResponse` 加 `tenant_id / tenant_name`(已在 §3.4);**不引入** Tenant 模型、不引入多对多表
- **前端**:
  - **不**加租户切换器 UI
  - **不**加租户输入框到登录页
  - `LoginResponse.tenant` 仅供后续扩展展示
- **决策存档**:`§4.5 多租户`整段降级为"未来扩展"

### 3.7 路径前缀不加统一前缀

- **后端**:零动 ✅(`pb/*.proto` 业务 RPC 无前缀,`/auth/login` 直出)
- **前端**:
  - 新建 `.env.development`:`VITE_API_BASE=/`(相对路径走 vite proxy)
  - 新建 `.env.production`:同 `VITE_API_BASE=/`(prod 跟后端同源)
  - `vite.config.ts` 加 `server.proxy`(详见 §5.3)
  - `api/*.ts` 写完整路径,如 `request.post('/auth/login', ...)`

### 3.8 错误文案直接显示后端 `message`

- **后端**:零动 ✅(§3.1 改完字段名后,前端直接 `data.message`)
- **前端**:
  - `utils/request.ts` toast 文案**直接用** `data.message`(不再走 `i18n.global.t('errors.xxx')`)
  - **前端 i18n 表只用于"前端业务事件"**(如"操作成功"、"保存成功"),不用于 API 报错

### 3.9 proto → TS 手写 DTO

- **决策**:**不引入 ts-proto**,**手写** `src/api/pb/`,每个 DTO 文件加注释 `// 同步自 admin/pb/<file>.proto`
- **DTO 文件清单(P0)**:
  - `src/api/pb/auth.ts` — LoginRequestDto, LoginResponseDto, RefreshTokenRequestDto, RefreshTokenResponseDto, LogoutRequestDto
  - `src/api/pb/user.ts` — UserDto, ProfileDto, ChangePasswordRequestDto(预留 P1)
- **DTO 一致性验证**(§6 P0 验证项 #4):`grep -E 'uid|username|access_token|refresh_token|expires|tenant' src/api/pb/auth.ts admin/pb/auth.proto` 必须能一一对应

### 3.10 字段命名手写 `fromXxxDto / toXxxDto`

- **DTO 内字段**:全 camelCase(前端 TS 习惯)
- **wire 层**:snake_case(后端 proto JSON)
- **转换位置**:每个 `api/*.ts` 顶部,统一 `fromXxxDto(raw)` / `toXxxDto(input)`
- **示例**(`src/api/pb/auth.ts`):
  ```ts
  export interface LoginRequestDto { username: string; password: string }
  export interface LoginResponseDto {
    uid: string
    username: string
    expires: number       // TTL 相对秒(后端 res.Expires = ttl,如 7200 = 2h)
    access_token: string
    refresh_token: string
    tenant_id?: string
    tenant_name?: string
  }
  export interface LoginResponse {
    accessToken: string
    refreshToken: string
    expires: number       // TTL 相对秒(wire 直接透传)
    uid: string
    username: string
    tenant?: { id: string; name: string }
  }
  export function fromLoginDto(raw: LoginResponseDto): LoginResponse {
    return {
      accessToken: raw.access_token,
      refreshToken: raw.refresh_token,
      expires: raw.expires,
      uid: raw.uid,
      username: raw.username,
      tenant: raw.tenant_id && raw.tenant_name
        ? { id: raw.tenant_id, name: raw.tenant_name }
        : undefined,
    }
  }
  ```

### 3.11 时间格式默认 ISO 8601

- **后端**:零动 ✅(proto 默认 ISO 8601 + GORM `datetime`)
- **前端**:用 `dayjs` 直解,无转换

### 3.12 跨域配置

- **后端**:aeus HTTP Server 已带 CORS 中间件;只需确认默认 origins 包含 `http://localhost:5173`(dev)+ prod 同源
- **前端**:加 `vite.config.ts server.proxy`(见 §5.3);dev 期同源,prod 跟后端同源

### 3.13 删除 `X-Console-Locale` header

- **后端**:零动 ✅
- **前端**:
  - `constants/http.ts:14-17` 删 `HEADER.LOCALE`
  - `utils/request.ts` 请求拦截器**不再注入** locale header
  - 未来要双语:后端按 `Accept-Language` 切换,**不依赖** 自定义 header

---

## 4. P0 后端实施清单

> 全部文件路径相对 `/mobe/workspace/aeus/admin/`。

### 4.1 改响应外壳 `Reason → Message`

| 文件 | 改动 |
|---|---|
| `responder.go:9-13` | `response` struct JSON tag `reason` → `message`;字段名 `Reason` → `Message`(全文一致) |
| `transport/http/response.go:11` | `Response` struct 同步改;`SetReason` 改名 `SetMessage`;`newResponse` 内部 `Reason:` → `Message:` |
| `transport/cli/types.go:60` | CLI 响应 struct `Reason` → `Message`(同步,免留遗漏) |
| `transport/cli/context.go:99-100` | `Context.Error(code, reason string)` 参数名 `reason` → `message`(内部调用 `send`) |
| `transport/cli/context.go:119` | `res.Reason` → `res.Message` |
| `transport/http/context.go:67-68` | `Context.Error(code, reason string)` 参数名改;`r := newResponse(code, reason, nil)` 内部 newResponse 仍收 message 字段 |

### 4.2 修 `TokenExpired` HTTPStatus bug

| 文件 | 改动 |
|---|---|
| `pkg/errs/error.go:62-77` | `HTTPStatus()` switch 加 `case TokenExpired: return http.StatusUnauthorized` |

**验证**:`go test ./pkg/errs/...` 加新用例 `TestHTTPStatus_TokenExpired`,断言返回 401

### 4.3 LoginResponse 加 tenant 字段

| 文件 | 改动 |
|---|---|
| `pb/auth.proto:15-21` | LoginResponse 加 `string tenant_id = 6;` `string tenant_name = 7;` |
| Makefile 跑 `make proto` | 重生成 `auth.pb.go` / `auth_http.pb.go`(本地有 protoc + grpc 插件) |
| `service/auth_service.go` Login 方法末尾 | `tenantID := user.TenantID`;`tenantName := ""`;若 `sys_tenants` 表存在,`db.Table("sys_tenants").Where("id = ?", tenantID).Scan(&tenantName).Error == nil`;否则 `tenantName = tenantID`;塞进 response |
| `pb/auth.proto:31-37` RefreshTokenResponse **不加** tenant(refresh 已知用户身份,不必每次查) |

### 4.4 (已废弃 2026-08-12) WithAutoRegisterAuth 选项

原 P0 计划提供 `WithAutoRegisterAuth(secret)` 让 Setup 自动 `pb.RegisterAuthServiceRouter`,以便 demo 一键跑通。**已废弃**:admin 模块在后续重构中把 `s.Http *ghttp.Server` 替换为 `rest.Router` 接口(`WithRouter(router)` option),这意味着 admin 不再持有具体 HTTP server 句柄,无法在 Setup 内部"自动"注册到某个外部 router 上。AuthService 的装配完全迁出 admin,一律由应用自行调用 `pb.RegisterAuthServiceRouter(httpSrv, service.NewAuthService(...))`,完整示例见 `admin/cmd/mock/service.go` Init。详见 README "AuthService 注册路径" 段。

### 4.5 Seed 函数

| 文件 | 改动 |
|---|---|
| `admin/seed.go` (新建) | `func Seed(db *gorm.DB, adminUser, adminPassword string) error` — 幂等创建 `admin` 角色(`Role.Key = "admin"`,`Role.Name = "系统管理员"`) + 一个 admin 用户(`User.Username = adminUser`,`Password = bcrypt(adminPassword)`,`Role.Key = "admin"`,`TenantID` 随机生成) |
| `seed_test.go` (新建) | 跑两次 Seed 验证幂等;密码校验通过 |

### 4.6 集成测试

| 文件 | 改动 |
|---|---|
| `admin_test.go` 末尾追加 | `TestE2E_Login_Profile`:用 httptest 起 server → `POST /auth/login` 拿 token → `GET /user/profile` 带 Bearer → 验证返回 user.uid 与 login 一致;验证 JWT `tid/uid/role` 三个 claim 在 `/user/profile` 响应里隐式生效 |

---

## 5. P0 前端实施清单

> 全部文件路径相对 `/mobe/js/dashboard/web/`。

### 5.1 改响应拦截器业务码分支

| 文件 | 改动 |
|---|---|
| `utils/request.ts:75-89` | 成功判断不变;**刷新分支 `code === 401` 改 `code === 4002`**;**跳登录分支**加 `code === 4003` 与 `code === 4005` |
| `utils/request.ts:147-152` | HTTP 401 双保险分支**保留**(即便 body 漏带 code,HTTP 401 仍触发 refresh) |

### 5.2 改 Token Header

| 文件 | 改动 |
|---|---|
| `constants/http.ts:14-17` | **删** `HEADER.TOKEN = 'X-Console-Token'`;**删** `HEADER.LOCALE = 'X-Console-Locale'`;保留 `HEADER.REQUEST_ID`(P0 验证项会用到) |
| `utils/request.ts:67-69` | 改 `config.headers.set('Authorization', 'Bearer ' + auth.accessToken)`;删 `HEADER.LOCALE` 注入(本就没有,确认即可) |

### 5.3 加 vite proxy + env 文件

| 文件 | 改动 |
|---|---|
| `vite.config.ts` | 加 `server: { proxy: { '/auth': { target: 'http://localhost:8080', changeOrigin: true }, '/user': { ... 同 }, '/role': { ... 同 }, '/menu': { ... 同 }, '/permission': { ... 同 }, '/system': { ... 同 } } }`(配合 §3.7 不加前缀,所有路径都走 proxy) |
| `.env.development`(新建) | `VITE_API_BASE=/`(相对路径,经 proxy 到 8080) |
| `.env.production`(新建) | `VITE_API_BASE=/`(prod 跟后端同源) |

### 5.4 新建手写 DTO 层

| 文件 | 改动 |
|---|---|
| `src/api/pb/auth.ts`(新建) | `LoginRequestDto`, `LoginResponseDto`, `RefreshTokenRequestDto`, `RefreshTokenResponseDto`, `LogoutRequestDto` + `fromLoginDto(raw)`, `toLoginRequestDto(input)`, `fromRefreshDto(raw)`, `fromLogoutDto(raw)`;每个 export 加注释 `// 同步自 admin/pb/auth.proto` |
| `src/api/pb/user.ts`(新建,骨架) | `UserDto`, `ProfileDto`(字段先列基础,P1 CRUD 时补齐);加 `fromUserDto(raw)`, `toUserDto(input)` |

### 5.5 改 api/auth.ts 接真实接口

| 文件 | 改动 |
|---|---|
| `api/auth.ts:1-120` 整文件重写 | 删除 `sleep()` mock;`login(body: LoginBody)` 改 `request.post<LoginResponseDto>('/auth/login', toLoginRequestDto(body))` → `fromLoginDto(resp)`;`refresh(token)` 改 `request.post<RefreshTokenResponseDto>('/auth/refresh-token', {refresh_token: token})` → `fromRefreshDto(resp)`;`logout()` 改 `request.post<void>('/auth/logout', {access_token: auth.accessToken})`;**LoginBody 字段 `account` → `username`** |
| `api/auth.ts` 顶部 import | 加 `import type { LoginRequestDto, LoginResponseDto, RefreshTokenResponseDto, LogoutRequestDto } from './pb/auth'`;加 `import { fromLoginDto, fromRefreshDto, toLoginRequestDto } from './pb/auth'` |

### 5.6 改 types/user.ts 视图层

| 文件 | 改动 |
|---|---|
| `types/user.ts:24-40` | `LoginResponse` 重写为 camelCase 视图层(已在 §3.10 给定义);保留 `RefreshResponse`(字段名同 LoginResponse);`AuthUser` 简化字段:`{ uid: string; username: string; role: string; tenant?: {id, name} }` |

### 5.7 改 stores/auth.ts 适配新 LoginResponse

| 文件 | 改动 |
|---|---|
| `stores/auth.ts:43-47` `login(payload)` | 适配新结构:`accessToken = payload.accessToken`;`refreshToken = payload.refreshToken`;`user = { uid, username, role: '' }`;**新增** `tenant = payload.tenant`(ref + localStorage);计算 `expiresAt = Date.now() + payload.expires * 1000`,存 `console.auth.expiresAt`(给将来"自动登出"用,P1) |
| `stores/auth.ts:74` `setTokens` 同步写 expiresAt | — |
| `stores/auth.ts:81-87` `logout` 同步清 expiresAt | — |

### 5.8 登录页文案 + dev 调试口保留

| 文件 | 改动 |
|---|---|
| `views/login/index.vue` | 提示文案改"admin 账号需联系管理员创建"(去 demo 凭据自动填充);**不加** tenant 输入框 |
| `main.ts:46-51` | 三个调试口(`__forceTokenExpired / __forceRefreshFail / __toggleMaintenance`)**全部保留**;`__forceTokenExpired` 改为通过 `auth.debug.expireAccess()`(stores/auth.ts 已有)+ 后续任一请求触发 4002 |

### 5.9 登录后异步补 role

| 文件 | 改动 |
|---|---|
| `stores/auth.ts` 末尾追加 | `async fetchRole()` 调 `GET /user/permissions` 拉权限码列表 → 缓存 `permissions: string[]`;登录后异步 fire-and-forget 触发;后续 P1 §4.6 按钮级权限用 |

---

## 6. 验证清单(每完成一节就勾选)

> 完整版见 `INTEGRATION-TODO.md §6`。本 spec 只列 P0 子集。

### 后端验证

- [ ] `go test ./pkg/errs/...` 全绿,新增 `TestHTTPStatus_TokenExpired` 通过
- [ ] `go test ./admin/...` 全绿,新增 `TestE2E_Login_Profile` 通过
- [ ] 手工跑:`curl -X POST http://localhost:8080/auth/login -d '{"username":"admin","password":"admin"}' -H 'Content-Type: application/json'`,响应 body 是 `{"code":0,"message":"","data":{"uid":"...","username":"admin","expires":7200,"access_token":"...","refresh_token":"...","tenant_id":"...","tenant_name":"..."}}`(`expires` 是 TTL 相对秒,如 7200 = 2h)
- [ ] 手工跑:删 `tenant_id` / `tenant_name` 不应影响其它字段(向后兼容)

### 前端验证

- [ ] `bun run build` 无 TS 报错(`types/user.ts` 改字段后联动改所有引用方)
- [ ] `bun run dev` → 登录页 → admin/admin → 进 Dashboard,顶部显示当前用户名(从 `user.username` 读)
- [ ] 浏览器关掉再开,仍能直接进 Dashboard(token 持久化生效)
- [ ] 浏览器 DevTools Network:任意请求 header 是 `Authorization: Bearer ...`,**看不到** `X-Console-Token` / `X-Console-Locale`
- [ ] 浏览器 DevTools Response:任意接口 body 是 `{"code":0,"message":"","data":{...}}`(成功)或 `{"code":<业务码>,"message":"<文案>"}`(失败);**字段名是 `message` 不是 `reason`,成功码是 0 不是 200**
- [ ] 浏览器 console 调 `window.__forceTokenExpired()`(或 `auth.debug.expireAccess()`)→ 触发任一请求 → 业务码 4002 → 自动 refresh → 继续操作不卡
- [ ] 浏览器 console 调 `auth.debug.breakRefresh()` → 调任一请求 → refresh 失败 → 跳回登录页

### 跨端验证(对照 §3.9)

- [ ] **DTO 一致性 grep**:`grep -E 'uid|username|access_token|refresh_token|expires|tenant' src/api/pb/auth.ts admin/pb/auth.proto`,两边应能一一对应(`auth.proto` 的字段在 `auth.ts` 都出现)

---

## 7. 风险与回退

| 风险 | 影响 | 回退方案 |
|---|---|---|
| §4.3 `sys_tenants` 表不存在 → tenant_name 取 tenant_id | 展示丑但不崩 | P1 加迁移 SQL `CREATE TABLE sys_tenants(id CHAR(60), name VARCHAR(100))` |
| §4.4 `WithAutoRegisterAuth` 误把生产 secret 烘焙进镜像 | 已废弃(见 §4.4 注),AuthService 一律手装 | N/A |
| §5.1 业务码 401 触发刷新逻辑被误删 | 刷新失效,所有过期请求直接失败 | 保留 HTTP 401 双保险分支(`utils/request.ts:147-152`),HTTP 失败仍能识别 |
| §5.5 `api/auth.ts` 改完后忘记同步 `stores/auth.ts` | TS 编译失败,登录崩 | §6 P0 验证 #2 已覆盖 `bun run build` |
| 前端缓存的旧 token(还是 mock 的 `demo.xxx`)与后端 JWT 混 | 旧 token 直接 401 触发 refresh,refresh 用 mock refresh_token 又 401 → 跳登录(行为正确,但体验差) | README 提示:首次集成时**清 localStorage**;§6 P0 验证 #5 隐含 |

---

## 8. 不在范围(P0 不做)

| 项 | 推迟到 | 备注 |
|---|---|---|
| 用户/角色/菜单 CRUD 页面 | P1 §3 | `INTEGRATION-TODO.md §3` |
| 部门/权限/日志/审计 | P2 §4 | `INTEGRATION-TODO.md §4.1-4.4` |
| 多租户切换 UI | P2 §4.5 | `INTEGRATION-TODO.md §4.5` |
| 按钮级权限指令 | P2 §4.6 | `INTEGRATION-TODO.md §4.6` |
| ts-proto 自动生成 | 暂不引入 | §3.9 决策手写;若 P1/P2 手写 DTO 维护成本高再评估 |
| 头像上传 multipart | P2 | `INTEGRATION-TODO.md §3.1` 注脚 |
| TokenStore Redis 版 | P1 | 当前用内存版,重启即失效 |
| 2FA / SSO / OAuth / LDAP | P3+ | `INTEGRATION-TODO.md §5.1` |

---

## 9. 自审(写完立刻跑)

1. **占位符扫描**:无 TBD / TODO / "稍后再说"。所有 P0 任务都有文件路径 + 行号或新建文件名。
2. **内部一致性**:
   - §3 与 §4 / §5 是否一致?§3.2 说后端改 HTTPStatus 加 401,§4.2 落地一致 ✅
   - §3.4 说 LoginResponse 加 tenant,§4.3 落地一致 ✅
   - §3.7 说 VITE_API_BASE=/,§5.3 落地一致 ✅
   - §5.5 说 LoginBody 字段 `account` → `username`,§3.4 决策对齐 ✅
3. **范围检查**:只覆盖 P0;P1/P2 在 §8 明列。
4. **歧义检查**:
   - §3.10 `expires` 语义:wire 与视图都是 TTL 相对秒,直接透传(后端 `res.Expires = ttl`,前端 `fromLoginDto` 不再换算)✅
   - §4.3 `sys_tenants` 表是否存在未确定(spec 已明确 fallback `tenantName = tenantID`)✅
   - §5.7 `tenant` 字段是否入 store(spec 已明确入 ref + localStorage,P1 才渲染 UI)✅