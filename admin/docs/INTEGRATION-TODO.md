# admin ↔ dashboard/web 集成 TODO

> - 后端模块:`/mobe/workspace/aeus/admin`(Go · rest/v3 + GORM · JWT · GORM 回调租户隔离)
> - 前端骨架:`/mobe/js/dashboard/web`(Vue 3 · Vite · TS · Element Plus · Pinia · Axios,代号 Console.OS · Admin)
> - 文档版本:2026-08-10
> - 状态:**前端侧所有 API 仍走 `setTimeout` mock**;后端侧 REST/gRPC 路由均已生成,缺的是"按前端场景暴露合适的接口"。
> - 阅读对象:两端开发者(以责任方 `**[后端]**` / `**[前端]**` / `**[契约]**` 标注)。

---

## 0. 当前形态速览(背景)

### 0.1 后端(`admin`)已具备

| 维度 | 现状 |
|---|---|
| 入口 | `admin.New(opts...) → *Server`(通过 `WithRouter(rest.Router)` 接管 HTTP server);`Server.Setup(ctx)` 注册 8 个内置模型;`AuthService` **始终由应用**自行 `pb.RegisterAuthServiceRouter(...)` 注册 |
| 模型(8) | `User / Role / Menu / Department / Permission / RolePermission / Audit / LoginLog`(均位于 `models/`,ModuleName=`system`,自动建 `sys_menus` + `sys_permissions`) |
| REST 通用 CRUD | 由 rest/v3 自动生成 `/system/sys_users / sys_user / sys_user/:id` 等 list/detail/create/update/delete/export(详见 `server.go:32-48`) |
| 业务 RPC | `/auth/{login,refresh-token,logout}` · `/user/{profile,change-password,reset-password,set-avatar,menus,permissions}` · `/role/{permissions,options,menus}` · `/permission/catalog` · `/menu/{tree,options,breadcrumb}`(均在 `pb/*.proto` 的 `google.api.http`) |
| 鉴权 | 应用方挂 `middleware/auth.JWT(...)`;admin 提供 `auth.Claims{Uid,Role,TenantID}` 通过 `auth.ClaimsFromContext` 读 |
| 多租户 | GORM 回调层做 WHERE 隔离;`Menu/Permission` 全局共享,`Login` 路径天然跨租户;`tenant_id CHAR(60)` 是契约;**8 个模型里只有 6 个(User/Role/Department/RolePermission/Audit/LoginLog)带 `tenant_id` 列** |
| 自动衍生 | `menu_derive.go` 给每个模型自动建菜单行;`permission_derive.go` 给每个模型自动建 6 个场景(`create/update/delete/search/detail/export`)的 `POST/GET/PUT/DELETE /system/sys_xxx` 权限行 |
| 响应格式 | `{code, message, data}`(默认 responder;HTTP 200;业务码 0=OK,1001=Invalid,4002=TokenExpired,4003=PermissionDenied,4004=NotFound,4005=AccessDenied)|
| Migrations | 无自动 runner,SQL 文件需手动跑;`2026-08-07-split-permissions.sql` 和 `2026-08-09-role-key.sql` 已 apply |

### 0.2 前端(`dashboard/web`)已具备

| 维度 | 现状 |
|---|---|
| 框架 | Vue 3.5 · Vite 8 · TS · vue-tsc · 严格类型 |
| UI / 状态 | Element Plus 2.14 + Pinia 3 + Vue Router 5 |
| HTTP | Axios 1.18(`src/utils/request.ts`),`baseURL = VITE_API_BASE ?? '/'`(dev 同源走 vite proxy,prod 同源直连) |
| 拦截器 | 业务码 4002 自动 refresh(mutex + 等待队列) · 业务码 4003/4005 跳登录 · GET 网络/5xx 自动重试 1 次 · `Authorization: Bearer <jwt>` header · `X-Request-Id` 注入 · `ProgressBar` 集成 |
| 路由 | 服务端化 nav;`nav.ready` 后 `bootstrapRouter` 注入;`meta.roles` 路由级守卫 |
| 已有完整页面 | 登录页 · 个人中心(资料/改密/偏好) · 用户管理 · Dashboard · 4 个错误页 |
| Mock 路径示例 | `/api/users`、`/api/nav`、`/login` 等(均 `setTimeout` 模拟,**完全未连真实后端**) |
| 业务码期待 | `{code: 0, message, data}`(成功 `code === 0` 或 `code === undefined`),失败按 `code` 分流(4002=refresh / 4003/4005=跳登录 / 其余 toast);message 字段直接展示后端文案 |
| Token | 用自定义 header `X-Console-Token`,**不是** `Authorization: Bearer` |
| 角色常量 | `users/index.vue` 硬编码字符串联合 `Admin / Operator / Viewer`(在 4-5 处重复出现),**与 admin 后端 `role_key` 未对齐** |
| 按钮级权限 | 无 `usePermission` / `v-auth` / 角色指令 |
| 多租户 | 完全没有(无 tenant 概念、无租户切换器) |
| 文档已声明 | `docs/superpowers/specs/2026-07-16-admin-template-completion-design.md §13`:"真实后端对接"列为**明确不在范围** |

> **前端注释路径核对**:`api/auth.ts` 整文件 grep `passport` 零命中,**真实存在的"路径注释残留"只有 `// request.post('/auth/logout')` 一行**(L46);mock 的 login/refresh 函数体内根本没有 URL 字面量。但前端 `users.ts:61` 用了 `pageSize` 这个本地字段名,**与 rest/v3 wire 协议的 `page_size` 不一致**(`rest/types.go:11-12` 是 `QueryParamPageSize = "page_size"`)。本次集成以 `/mobe/workspace/aeus/admin` 为目标,所以前端 6 个 mock 文件的路径常量要全部按 admin 的 `pb/*.proto` 重新写。

---

## 1. 契约层 TODO(两端必须先对齐,阻塞 P0)

| # | 契约项 | 后端现状 | 前端现状 | 需决策 / 行动 |
|---|---|---|---|---|
| 1.1 | 响应外壳 | `{code, reason, data}`(`responder.go` 默认实现) | 期待 `{code, message, data}` | **后端改**:`reason` → `message`(`responder.go` 一行 + `transport/http/response.go:11` 同改);前端零改动 ✅ |
| 1.2 | 业务码 | `pkg/errs/const.go` 定义:`0=OK`、`4002=TokenExpired`、`4003=PermissionDenied`、`4004=NotFound`、`4005=AccessDenied`;`4002` 当前映射到 HTTP **500**(`pkg/errs/error.go:62-77` 的 `default` 分支,⚠️ 这是后端 bug,见 §5) | 期待 `0 / 200 / 401`(0/200 都视为成功) | **前端按 `body.code` 分流**:`4002` → refresh + 重试;`4005/4003` → 跳登录页;其它 → toast 报错。**不依赖 HTTP status**(后端没有 401,鉴权失败统一 403/500) |
| 1.3 | Token Header | `Authorization: Bearer <jwt>`(走 aeus `mwauth.JWT`) | `X-Console-Token: <jwt>`(`constants/http.ts` L17) | **前端改**:删 `HEADER.TOKEN`,`utils/request.ts` 拦截器 L67-76 改 `Authorization: Bearer ${accessToken}`;后端零动 ✅。`X-Console-Token` 无下游消费 |
| 1.4 | Token 字段名 | `AuthService.Login`(`pb/auth.proto:11-21`):Request=`{username, password}`;Response=`{uid, username, expires(TTL 相对秒), access_token, refresh_token}`;**没有 `user` 字段**。`RefreshTokenResponse` 同结构 | `LoginBody { account, password }`;`LoginResponse { accessToken, refreshToken, expiresIn, user: { id, name, role } }`(`types/user.ts`) | **前端改**:`LoginBody.account` → `username`;`LoginResponse` 重写为 `{access_token, refresh_token, expires: number(TTL 相对秒,如 7200 = 2h), uid, username, tenant?: {id, name}}`,顶部手写 `fromLoginDto / toLoginBody` 映射 wire snake ↔ camel;`expires` 直接透传,**不做绝对↔相对换算**(`stores/auth.login` 直接 `Date.now() + view.expires * 1000` 算 expiresAt);`role` 不在 LoginResponse,登录后调 `GET /user/permissions` 拿;**后端 LoginResponse 加 `tenant_id / tenant_name`** 字段(从 user 记录读,供前端展示用,见 §1.6) |
| 1.5 | 角色命名 | `Role.Key`(机器标识,30 字符,唯一,例 `admin / operator`);`Role.Name`(人类可读);`Role.DataScope` 字段存在(`role.go:96-106`) | `Admin / Operator / Viewer`(人类可读字符串联合,4-5 处硬编码) | UI 显示用 `Role.Name`;权限判断用 `Role.Key`;**`DataScope` MVP 不消费**,前端 DTO 加可选字段 `dataScope?: number`,UI 不渲染 |
| 1.6 | 多租户 | JWT `tid` claim;`tenant_id` 在 6/8 个模型列上(User/Role/Department/RolePermission/Audit/LoginLog);Menu/Permission 全局 | 无 tenant 概念 | **MVP 单租户**:后端从 `sys_users.tenant_id` 自动读,写 JWT `tid`;前端**无 UI 切换器、无租户输入框**;`LoginResponse` 加 `tenant: {id, name}` 给前端展示。`§4.5` 多租户整段降级为"未来扩展"备注 |
| 1.7 | 路径前缀 | 业务 RPC 无前缀(`/auth/login`);rest/v3 自动资源前缀 `/system/sys_xxx`;aeus HTTP Server 无 `Group` 方法 | 期待 `/api/*`,且用户/角色都想象成单一根(`/api/users`、`/api/roles`) | **不加统一前缀**:前端 `VITE_API_BASE='/'`,各 `api/*.ts` 写完整路径(`/auth/login`、`/system/sys_users`)。后端零动,前端也不需要拆单/双根 ✅ |
| 1.8 | 错误消息文案 | `reason` 字段英文为主(`"permission denied"`、`"invalid argument"`) | i18n 中英双语,toast 文案走 i18n key | **前端直接展示后端 `message`**(§1.1 改完字段名后),不另建 `errors.*` i18n 表;前端 i18n 表只用于"前端业务事件 toast"(如"操作成功"),不用于 API 报错 |
| 1.9 | proto → TS 生成 | `pb/*.proto` 是真源,生成 `*.pb.go` | 无 `pb/`,无生成脚本 | **手写 DTO 层**:`src/api/pb/` 目录手工写,字段对齐 `admin/pb/*.proto`,每个 DTO 加注释 `// 同步自 admin/pb/auth.proto:11-21`。**§6 加验证项**:"后端改 proto 必须同步更新前端 DTO,合入前 grep 比对字段名" |
| 1.10 | 字段命名 | GORM 列名 + proto JSON 字段名都是 snake_case(`uid / role_key / dept_id / data_scope`) | TS 接口 camelCase(`roleKey / deptId`) | §1.9 决定手写 DTO → **手写 `fromXxxDto(raw) / toXxxDto(input)` 映射**;DTO 内字段全 camelCase,wire 层 snake_case,转换在每个 `api/*.ts` 顶部统一 |
| 1.11 | 时间格式 | GORM 默认 `datetime`,JSON 序列化 `2026-08-10T12:00:00+08:00`(proto 默认 ISO 8601) | 期待 `string`,格式未约束 | **默认 ISO 8601 字符串**,前端 `dayjs` 直解,不额外转换 ✅ |
| 1.12 | 跨域 | aeus HTTP Server 自带 CORS 中间件 | dev 是 `http://localhost:5173`,prod 跟后端同源 | 后端无需新增 CORS,只需确认默认 origins 包含 `http://localhost:5173`(dev)+ prod 同源;前端 `vite.config.ts` 加 `server.proxy` 把 `/` 转发到 `http://localhost:8080`(配合 §1.7 不加前缀,baseURL 是 `/`,所有路径都走 proxy),双保险 |
| 1.13 | i18n locale 同步 | 后端无 locale 概念,所有 message 是英文 | `X-Console-Locale` header 在 `constants/http.ts:15` 定义,请求拦截器 L67-76 注入 | **删 `HEADER.LOCALE` 常量 + 删拦截器注入**。前端纯靠后端 message(`§1.8`)。未来要双语,后端按 `Accept-Language` 切换,不依赖自定义 header |

---

## 2. P0 — 跑通最小可登录闭环

> 目标:打开前端 → 看到登录页 → 输入 admin/admin → 调通 `/api/v1/auth/login` → 拿到 token → 进 Dashboard → 401 自动刷新 → 登出。

### 2.1 后端(admin)

- [x] **[后端]** AuthService **始终由应用**自行 `pb.RegisterAuthServiceRouter(...)` 注册(原 P0 提供的 `WithAutoRegisterAuth(secret)` 已废弃,见 README "AuthService 注册路径" 段)。`admin/cmd/mock/service.go` Init 是完整示例。✅ 2026-08-12
- [x] **[后端]** 自定义 Responder 把响应外壳的 `reason` 改名为 `message`(`responder.go` + `transport/http/response.go:11` 同改,字段名 JSON tag `reason` → `message`;`option.go` 加 `WithResponder`)✅ 2026-08-10
- [x] **[后端]** **LoginResponse 加 `tenant_id` / `tenant_name` 字段**(`pb/auth.proto:30-31` 末尾追加;从 `sys_users` 记录读 `tenant_id`,再用 `sys_tenants` 表查 `name`;若 `sys_tenants` 不存在,fallback `name = tenant_id`)。§1.6 决定 MVP 单租户,前端无 UI 切换,但展示用 ✅ 2026-08-10
- [ ] **[后端]** aeus HTTP Server **自带 CORS 中间件**,无需新增;只需确认默认 origins 配置允许 `http://localhost:5173`(dev)+ prod 同源。§1.7 决定**不加统一前缀**,所以无需改 `transport/http/server.go` 加 `Group` 方法
- [x] **[后端]** `AuthService.Login` / `RefreshToken` proto 字段已经是 `uid / username / expires / access_token / refresh_token`(`pb/auth.proto:11-41`);无需改动,**前端按这个对接**(§1.4 已对齐)✅
- [x] **[后端]** **修 `pkg/errs/error.go:62-77` 的 `TokenExpired` HTTP status bug**:在 `HTTPStatus()` 加 `case TokenExpired: return http.StatusUnauthorized`(401)。详见 §5.1 ✅ 2026-08-10
- [x] **[后端]** 写一个集成测试覆盖 `/auth/login` → 拿 token → 调 `/user/profile` 闭环,验证 JWT `tid/uid/role` 三个 claim 正确写回(`admin_test.go` 已有类似用例,扩展一个 e2e)✅ admin_e2e_test.go
- [x] **[后端]** 加 `Seed` 函数(`admin.Seed(adminUser string, password string)`),首次启动自动写一个内置 admin 角色 + admin 用户,便于前端开箱即用 ✅ 2026-08-10

### 2.2 前端(dashboard/web)

- [ ] **[前端]** `.env.development`:`VITE_API_BASE=/`(相对路径,配合 vite.config.ts proxy);`.env.production`:同 `/`(跟后端同源);`§1.7` 决定不加 `/api/v1` 前缀
- [ ] **[前端]** `vite.config.ts` 加 `server.proxy`:所有 `/auth/*`、`/user/*`、`/role/*`、`/menu/*`、`/permission/*`、`/system/*` 都 proxy 到 `http://localhost:8080`,`changeOrigin: true`(dev 全程同源,免去跨域)
- [ ] **[前端]** `constants/http.ts` 删 `HEADER.TOKEN` 和 `HEADER.LOCALE`(两个常量都没人读了);保留 `HEADER.REQUEST_ID`
- [ ] **[前端]** `utils/request.ts` 请求拦截器 L67-76 改:`config.headers.Authorization = 'Bearer ' + auth.accessToken`(从 stores/auth 拿 token,不再读 `HEADER.TOKEN`);不再注入 `X-Console-Locale`
- [ ] **[前端]** `utils/request.ts` 业务码分支(L82-89)按 §1.2 改:
  - `code === 0 | 200 | undefined` → 成功
  - `code === 4002` (TokenExpired) → 触发 refresh + 重试原请求
  - `code === 4005` (AccessDenied) 或 `code === 4003` (PermissionDenied) → 跳登录页
  - 其它 → `ElMessage.error(data.message)`(`message` 来自 §1.1 后端字段名)
- [ ] **[前端]** `src/api/pb/` 新建,手写 DTO(§1.9 决定):
  - `auth.ts`:`LoginRequest = {username, password}`;`LoginResponse = {access_token, refresh_token, expires: number, uid, username, tenant?: {id, name}}`;导出 `fromLoginDto(raw): LoginResponseView`(wire snake → camel,`expires` 仍为 TTL 相对秒)
  - `user.ts`:`UserDto = {uid, username, role_key?, dept_id?, status, created_at, updated_at, ...}`;`fromUserDto(raw): UserView`(snake → camel)
- [ ] **[前端]** `api/auth.ts` 三个函数全部接真实接口(路径写完整,不加前缀):
  - `login(body: LoginBody)` → `request.post<RawLoginResponse>('/auth/login', {username, password})` → 经 `fromLoginDto` 转 `LoginResponseView`
  - `refresh(token)` → `request.post<RawRefreshResponse>('/auth/refresh-token', {refresh_token: token})`
  - `logout()` → `request.post<void>('/auth/logout')`
- [ ] **[前端]** `types/user.ts` `LoginResponse` 类型重写为 camelCase 视图层(对齐 §1.4):`{accessToken, refreshToken, expires: number(相对秒), uid, username, tenant?: {id, name}}`,`user.role` 从 `GET /user/permissions` 拉,不放 LoginResponse
- [ ] **[前端]** `stores/auth.ts` `login(r: LoginResponseView)` 写入 `console.auth.access / refresh / user / tenant`,`user.role` 暂存空,登录后异步调 `GET /user/permissions` 补齐;`expires` 相对秒 + 当前 Date.now() 算出绝对秒存 `console.auth.expiresAt`
- [ ] **[前端]** `views/login/index.vue` 文案提示改为"`admin` 账号需联系管理员创建",去掉 demo 凭据自动填充;登录页**不加 tenant 输入框**(§1.6)
- [ ] **[前端]** `main.ts` L46-51 的三个 dev 调试口(`__forceTokenExpired / __forceRefreshFail / __toggleMaintenance`)**全部保留**;`__forceTokenExpired` 对应 §1.2 `4002` 分支,触发 refresh;`__toggleMaintenance` 仍待后端 P2 补 503 端点
- [ ] **[前端]** 跑 `bun run build`,确保 TS 类型错误清零(`types/user.ts` 改字段后要联动改 `stores/auth.ts` 等所有引用方);§6 加的验证项"后端改 proto 必须同步更新前端 DTO"首次跑通

### 2.3 契约

- [ ] **[契约]** 把"成功响应外壳 `{code, message, data}`"作为契约基线写进 `admin/docs/CONTRACT.md`(新建)
- [ ] **[契约]** 业务码语义表(`pkg/errs/const.go` 全表 + HTTP status 映射)写进 `admin/docs/CONTRACT.md`,前端 §1.2 分流逻辑据此
- [ ] **[契约]** JWT 四个 claim(`uid / uro / tid / token_type`)的填法与前端读取位置写进 `admin/docs/CONTRACT.md`
- [ ] **[契约]** "MVP 单租户:后端从 user 记录自动区分,前端无 UI"写进 `admin/docs/CONTRACT.md`,作为 §1.6 决策存档

---

## 3. P1 — 跑通核心 CRUD(用户 / 角色 / 菜单)

> 目标:登录后能进 `系统管理 / 用户管理`,看到后端真实数据;能新增/编辑/删除用户、角色、菜单。

### 3.1 用户模块

#### 后端
- [ ] **[后端]** 前端用户列表走 `/system/sys_users` REST(rest/v3 自动生成的 list),它已经支持 search / pagination / sort / filter
- [ ] **[后端]** **Password 字段已经安全**:`models/user.go:84` `Password` 字段 tag 是 `scenarios:"create"`,rest/v3 通过 `schema.GetVisibleSchemas(ScenarioList)` 自动从 list/detail/export 响应里过滤掉;**不需要手动 `Omit` 或写 ListUserResponse**。但是前端 create 用户时要能传密码 — 走 create 场景即可
- [ ] **[后端]** `/system/sys_user/:id` 的 update 接口需要能改密码(否则前端改密只能走 `ChangePassword` 自助,管理员重置还得走 `ResetPassword`);若 rest/v3 update 场景不暴露 password,可能要加白名单或新场景
- [ ] **[后端]** 集成测试覆盖"管理员列出所有用户 → 改某用户角色 → 该用户重新登录看到新菜单"
- [ ] **[后端]** 备选:`UserService` 补 `ListUsers` / `SearchUsers` 专用 RPC 只在 REST 列表无法满足"复杂筛选 + 联表"需求时再上(MVP 暂不做)

#### 前端
- [ ] **[前端]** `api/users.ts` 全部 mock 替换为真实:`list / detail / create / update / remove / resetPassword` 走 `/system/sys_users / sys_user / sys_user/:id`(注意单复数:列表用 `/sys_users`,详情用 `/sys_user/:id`)
- [ ] **[前端]** `api/users.ts` 里的 `pageSize` 字段名**改成 `page_size`**(wire 协议是下划线,`rest/types.go:12` 是 `QueryParamPageSize = "page_size"`);同步把后端响应里的字段也读 `page_size`(如果 rest/v3 响应也用下划线)或做映射
- [ ] **[前端]** `views/users/index.vue` 的 URL 同步、分页、筛选保留;改数据源;删除 setTimeout 模拟(原 L57-101)。注意:**URL 同步的 `knownHash` 在默认 page/size 边界值下存在潜在重复 fetch 风险**,接真实后端时建议改为显式比对 query 对象而非序列化字符串
- [ ] **[前端]** `api/users.ts` 的模块级 `store` **不持久化**,刷新页面即丢失 seed;接真实后端后这块直接删掉
- [ ] **[前端]** 改密流程拆为两种:自助改密走 `/user/change-password`;管理员重置别人密码走 `/user/reset-password`,前端 `users/index.vue` 行操作加"重置密码"按钮,弹窗只输入新密码
- [ ] **[前端]** 头像暂时走 `/user/set-avatar`(URL 模式);真正的头像上传是 P2(后端要补 multipart 端点)
- [ ] **[前端]** `api/users.ts` 顶部加 `fromUserDto / toUserDto`(§1.10 决定),手写 snake ↔ camel 映射;`role` 字段从 `UserDto.role_key` 转 `UserView.role: string`(不再是字符串联合)

#### 契约
- [ ] **[契约]** User DTO 字段命名统一;password 字段**已自动隐藏**(见后端项第 2 条),前端 list/detail 响应不用再过滤
- [ ] **[契约]** 分页参数统一为 `page` + `page_size`(**下划线**,wire 协议决定);前端 `users.ts:61` 现有 `pageSize` 命名错误,一并修正

### 3.2 角色模块

#### 后端
- [ ] **[后端]** 角色管理页面要"创建 / 编辑 / 删除角色";优先用 `/system/sys_roles / sys_role / sys_role/:id` REST
- [ ] **[后端]** 当前 `RoleService.ReplaceRolePermissions`(PUT `/role/permissions`)已可用;前端角色编辑页要带"勾选菜单/权限"的多选弹窗
- [ ] **[后端]** `RoleService.ListRoleOptions`(GET `/role/options`)已可用,作为下拉选择数据源

#### 前端
- [ ] **[前端]** 新增 `src/views/role/index.vue`(列表 + 编辑弹窗 + 权限分配树);同时新增 `src/api/role.ts`
- [ ] **[前端]** `api/role.ts` 暴露 `list / detail / create / update / remove / listOptions / getPermissions / replacePermissions / getMenus`
- [ ] **[前端]** 权限分配弹窗要支持"按菜单树勾选 + 按 API/按钮权限目录勾选"两种 tab(数据来自 `/menu/tree` 和 `/permission/catalog`)
- [ ] **[前端]** 角色删除时,前端要 confirm 二次确认 + 提示"将清空该角色所有用户的角色绑定";后端 `Role.BeforeDelete` 已自动清 RolePermission,但**不清** `sys_users.role_key`(只软删时清,硬删不清)— 这是个 bug,要后端顺手补(详见 §5.1)

### 3.3 菜单模块

#### 后端
- [ ] **[后端]** 菜单管理页面要 CRUD 菜单 + 拖拽排序;优先用 `/system/sys_menus` REST
- [ ] **[后端]** `MenuService.GetMenuTree / GetMenuOptions / GetMenuBreadcrumb` 已可用,前端菜单管理页用 `/menu/tree` 渲染树

#### 前端
- [ ] **[前端]** 新增 `src/views/menu/index.vue`(树形表格 + 拖拽 + 编辑);新增 `src/api/menu.ts`
- [ ] **[前端]** `api/menu.ts` 暴露 `listTree / listOptions / getBreadcrumb / create / update / remove`;顶部 `fromMenuDto / toMenuDto`(§1.10)
- [ ] **[前端]** nav 注入(`router/bootstrap.ts` L82-99)改数据源:`navApi.fetch()` → `request.get<NavItem[]>('/user/menus')`,把后端返回的菜单树映射为前端 NavItem(注意:后端菜单 Component 用 PascalCase,前端 `view` 字段要 `import.meta.glob` 解析,逻辑已现成)
- [ ] **[前端]** `validateNavTree` 校验后端返回的菜单树;后端菜单的 `parent` 字段是 Component 而非 ID,前端要拉平后按 `parent` 重建父子关系
- [ ] **[前端]** 菜单删除时,前端要 confirm + 提示"将同时清掉关联角色的菜单权限"(后端 `Menu.AfterDelete` 已自动清,前端只是提示)

#### 契约
- [ ] **[契约]** 后端菜单 `Component / Uri / Parent` 三个字段在前端如何映射?
  - 提议:后端菜单 `Component` = 前端 `view`(物理路径,如 `views/system/user/index.vue`)
  - 后端菜单 `Uri` = 前端 `path`(路由路径,如 `/system/user`)
  - 后端菜单 `Parent` = 前端 `parentComponent`(字符串,不是 ID)
- [ ] **[契约]** 决定菜单的"图标 / 排序 / 隐藏 / 外链"等字段是否都要支持

---

## 4. P2 — 完善扩展模块(部门 / 权限 / 日志 / 审计 / 多租户)

### 4.1 部门管理

#### 后端
- [ ] **[后端]** 部门管理页面 CRUD,走 `/system/sys_departments` REST
- [ ] **[后端]** `Department` 模型目前只有 `parent_id + name`,要补 `sort / status / leader` 字段(`models/department.go:3-10`)
- [ ] **[后端]** 部门树形 API 可加 `GET /department/tree`(可选,目前前端可以自己拼)

#### 前端
- [ ] **[前端]** 新增 `src/views/department/index.vue`(树形表格 + 编辑);新增 `src/api/department.ts`
- [ ] **[前端]** 用户管理页"所属部门"列要走 `/department/tree` 或 `/system/sys_departments` 下拉(目前硬编码)

### 4.2 权限目录

#### 后端
- [ ] **[后端]** `PermissionService.ListCatalog`(GET `/permission/catalog`)已可用,前端权限分配页用
- [ ] **[后端]** 决定权限目录的"新建 / 编辑"是否需要 UI;**admin 现状是只读**(自动衍生),如果前端要手动加权限行,要走 `/system/sys_permissions` REST

#### 前端
- [ ] **[前端]** 角色编辑弹窗的"API/按钮权限"Tab 用 `/permission/catalog?type=api / button` 拉数据
- [ ] **[前端]** 决定是否做"权限目录管理页";若不做,权限行只能靠后端自动衍生 + 手工 SQL

### 4.3 登录日志

#### 后端
- [ ] **[后端]** 加 `GET /login-log/list`(或用 `/system/sys_login_logs` REST)供前端查询
- [ ] **[后端]** 加 `GET /login-log/stats`(按日期聚合,可选)

#### 前端
- [ ] **[前端]** 新增 `src/views/login-log/index.vue`(表格 + 日期筛选 + 导出);新增 `src/api/login-log.ts`
- [ ] **[前端]** Dashboard "最近登录" 卡片改走真实数据(`views/dashboard/AlertFeed.vue` 等)

### 4.4 操作审计

> **2026-08 后端已落地**:`admin.WithAudit(true)` 开启后,`Setup` 在注册模型前安装 rest/v3 进程级全局 after-hooks,所有 REST create/update/delete 自动记 `sys_audits`(create/update 记 DiffAttr JSON,delete 只记 `{"id":<pk>}`);内置排除 `system/sys_audits`(防递归)与 `system/sys_login_logs`,可用 `WithAuditExcludes` 追加;UID 经 `WithUserResolve`(默认 JWT claims)解析。详见 README「操作审计」一节。覆盖边界:仅 REST CRUD 路径,service 层裸 gorm 写不在内。

#### 后端
- [x] **[后端]** 在 `Setup/RegisterModel` 注入审计钩子:create/update/delete 操作记录到 `sys_audits`
- [x] **[后端]** 列表查询走通用 REST:`/system/sys_audits`(搜索/详情/导出由 rest/v3 场景自动提供)
- [ ] **[后端]** 决策:审计是否要"按 user / 按模块 / 按时间"聚合统计?P3

#### 前端
- [ ] **[前端]** 新增 `src/views/audit/index.vue`(表格 + 高级筛选);新增 `src/api/audit.ts`(可直接用 SchemaViewer 通用页过渡)

### 4.5 多租户 — **未来扩展**(MVP 不启用,见 §1.6)

> §1.6 已决定 MVP 单租户:**后端从 `sys_users.tenant_id` 自动区分,前端无 UI、无切换器、无多对多绑租户**。本节是后续如果业务需要"用户跨多个 tenant"时的扩展蓝图,不是当前要做的事。

#### 后端(若启用)
- [x] **[后端]** 加 `Tenant` 模型(`models/tenant.go`)与 `/system/sys_tenants` REST;字段:`id / code / name / status` — 已按 spec `docs/superpowers/specs/2026-08-13-admin-tenant-model-service-design.md` 落地(2026-08-13):`id` 为 char(60) 字符串主键(= 各表 tenant_id),**去掉 `code`**(唯一 id 已够,name 负责展示);`TenantService`(`/tenant/options`、`/tenant/detail`);Login 内置 disabled 租户拒绝;Seed 收敛 admin 租户实体行
- [ ] **[后端]** `sys_users` 加多对多表 `sys_user_tenants(user_id, tenant_id)`,迁移 SQL
- [ ] **[后端]** 加"当前用户可选租户列表"端点 `GET /user/tenants`(从 `sys_user_tenants` 查)
- [ ] **[后端]** 登录响应里追加 `tenants: Tenant[]`,前端登录后让用户选择(或自动选上次)

#### 前端(若启用)
- [ ] **[前端]** 登录页加"租户"下拉(若后端用户绑多租户)
- [ ] **[前端]** 顶栏加 `TenantSelector`,切换租户后刷新 JWT + 重新拉 nav
- [ ] **[前端]** `stores/auth.ts` 持久化加 `console.auth.tenant`(目前已存,只是 UI 不渲染)
- [ ] **[前端]** request 拦截器把当前 tenant 写到 header(`X-Tenant-Id`),后端从 header 读(目前从 JWT tid 读,改成 header 优先或可切换)

### 4.6 按钮级权限

#### 前端(后端已通过 `/user/permissions` 暴露)
- [ ] **[前端]** 新增 `src/composables/usePermission.ts`,封装 `hasPerm(code: string): boolean`(从 `auth.user.permissions` 数组判断)
- [ ] **[前端]** 新增自定义指令 `v-auth="'POST /system/sys_user'"`(注册在 `main.ts`),内部调 `usePermission`
- [ ] **[前端]** 用户/角色/菜单管理页的所有"新增 / 编辑 / 删除"按钮加 `v-auth`
- [ ] **[前端]** 登录后从 `GET /user/permissions` 拉全量权限码列表,缓存到 `stores/auth.ts`,登出时清空

---

## 5. 已发现的问题 / Bug / 待修(两端都要动)

### 5.1 后端
- [ ] **[后端]** **`pkg/errs/error.go:62-77` 的 `TokenExpired(4002)` 映射到 HTTP 500 是 bug**:`HTTPStatus()` switch 命中 `default` → 500。语义上 token 过期应映射 `401 Unauthorized`,否则前端 axios 默认按 500 处理进 catch 不会触发 refresh。修法:加 `case TokenExpired: return http.StatusUnauthorized`。**§1.2 已据此建议前端按 body.code(而非 HTTP status)分流,这条修了后两者都能用**
- [ ] **[后端]** **`Role` 软删和硬删都不会清 `sys_users.role_key`**(`role.go:73-77` 软删只删 RolePermission,`role.go:83-90` 硬删只调 `purgeRolePermissions`,**两条路径都不碰 `sys_users` 表**),导致用户表残留无效角色引用;**`BeforeUpdate` 的 `keyChanged` 分支会在 Role Key 重命名时同步 `sys_users.role_key`**(`role.go:62-66`),那是另一种场景。建议:**软删时**也补 `db.Model(&User{}).Where("role_key = ? AND tenant_id = ?", old.Key, old.TenantID).Update("role_key", "")`,**硬删时**补同样的清理;并加测试覆盖两条路径
- [ ] **[后端]** `LoginLog` 写日志时如果 user 不存在,`uid` 留空(`models/user.go:71-74`),但前端列表期待"用户名"字段;要 join users 表或前端接受"未知用户"
- [ ] **[后端]** `permission_derive.go` 镜像了 rest/v3 `buildUri` 公式(7 行);**rest/v3 升版时必须同步改**,否则自动建出的 permission 行 URI 跟实际 REST 路由对不上。建议在 `permission_derive_test.go` 加一个 e2e:启动 Setup 后,断言每个 permission 的 `Data` 字段都能被路由系统解析到 handler
- [ ] **[后端]** `pb/empty_alias.go:11` 是手工桥,**不能删**;在 `Makefile` 加注释 `make proto-clean` 后必须恢复这个文件
- [x] **[后端]** rest/v3 `TenantResolve` 曾无条件给 Search/Export 追加 `WHERE tenant_id = ?`(无列存在检查),`Menu`/`Permission` 的 REST search 实为 SQL 报错;Tenant 模型落地时一并修复(`server.go` 按 `modelHasTenantIDColumn` 守卫注入),并修复了 `registerModel` 里 `AutoMigrate(&models.Menu{})` 复用被 rest/v3 解析污染的 statement 的隐患(字符串主键模型会触发 sqlite 双主键重建错误)✅ 2026-08-13
- [ ] **[后端]** `server.go:80-81` 注释提到 gorm v1.31.1 session schema 缓存陷阱;升级 gorm 前必须回归所有 `*_test.go`
- [x] **[后端]** AuthService 一律走 `pb.RegisterAuthServiceRouter(...)` 手装路径(`server.go` Setup 不再自动注册);README "AuthService 注册路径" 段已说明 ✅ 2026-08-12
- [ ] **[后端]** 缺"忘记密码 / 找回密码"端点(只有 `ResetPassword` 要求 admin);P3 再补
- [ ] **[后端]** 缺"邮箱 / 手机号"字段;P3 再补
- [ ] **[后端]** 缺 2FA / TOTP;P3
- [ ] **[后端]** 缺"用户封禁 / 解封"端点(只能改 status);P3
- [ ] **[后端]** 缺"会话列表 / 强制下线"端点(`TokenStore` 是后端内部,前端看不到活跃 session);P3
- [ ] **[后端]** 缺"API 速率限制"中间件;P3
- [ ] **[后端]** 缺 SSO / OAuth2 / LDAP;P3

### 5.2 前端
- [ ] **[前端]** `api/auth.ts` L46 的 `// request.post('/auth/logout')` 注释路径已对齐后端,无需改;**整个文件 grep `passport` 零命中,没有 `/passport/login` 字面量**(我之前整理时记错了);保留这个注释作为"切换 mock → real"的标记即可
- [ ] **[前端]** `views/users/index.vue` 的角色选项 `['Admin', 'Operator', 'Viewer']` 字符串联合硬编码(在 4-5 处重复:`api/users.ts` L17 / L21、`views/users/index.vue` L27-31 / L369-371 / L229-231) → 改成从 `/role/options` 拉,同时把 `User['role']` 类型从字符串联合改成 `string`(因为真实角色数由后端决定)
- [ ] **[前端]** `views/dashboard/*` 5 个组件(`data.ts / index.vue / TrafficChart.vue / EventStream.vue / AlertFeed.vue / NodeList.vue`)全部用 mock data,接真实端点(P2);`TrafficChart.vue:5` 已自带 TODO 注释
- [ ] **[前端]** `views/{analytics,alerts,nodes,reports,logs,billing,team,settings}/index.vue` 是占位(`<PlaceholderView />`,8 个文件每个只有 6-9 行);按业务优先级逐个填充,或干脆删掉 nav 里的入口
- [ ] **[前端]** `stores/tabs.ts` 持久化跨租户切换时需要重置(若启用多租户)
- [ ] **[前端]** `vite.config.ts` 没配 `server.proxy`,dev 默认 `baseURL='/api'` 会打到 `localhost:5173/api`(Vite 静态服务无目标),**不是直接打 8080**;加 `server: { proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: true } } }`,并配套 `.env.development` 写 `VITE_API_BASE=/api`(相对路径走 proxy,避免 dev 跨域)
- [ ] **[前端]** `.mimocode/plans/1784281210146-brave-eagle.md` 是历史实现记录,与 admin 对接无关,**保留作为参考即可**,不需要清理
- [ ] **[前端]** `docs/superpowers/specs/2026-07-16-admin-template-completion-design.md §13` 把"真实后端对接"列为不在范围,**对接开始时该文档已"过期",但作为实现记录保留**;新增 `docs/superpowers/specs/2026-08-10-admin-integration-design.md` 记录本次对接设计
- [ ] **[前端]** `dev` 调试口只有 `__forceTokenExpired / __forceRefreshFail / __toggleMaintenance` 三个(`main.ts:46-51`),**没有 `__forceNavError`**(我之前误以为有);保留这三个即可

---

## 6. 验证清单(每完成一节就勾选)

### P0 完成后必过
- [ ] 后端 `go test ./...` 全绿
- [ ] 前端 `bun run build` 无 TS 报错
- [ ] curl 调 `/auth/login` 拿到 JWT,响应 body 是 `{code: 0, message: "", data: {uid, username, access_token, refresh_token, expires: 7200, tenant: {id, name}}}`(`expires` 是 TTL 相对秒)
- [ ] **DTO 一致性比对**:后端 `pb/auth.proto:11-41` 字段 vs 前端 `src/api/pb/auth.ts` 字段名 / 类型 / 必填性,手工 grep `grep -E 'uid|username|access_token|refresh_token|expires|tenant' src/api/pb/auth.ts admin/pb/auth.proto` 应能一一对应
- [ ] 前端 `bun run dev` → 登录页 → admin/admin → 进 Dashboard,顶部显示当前用户名 + 角色
- [ ] 浏览器关掉再开,仍能直接进 Dashboard(token 持久化生效)
- [ ] 手动让 token 过期,触发 `code: 4002` → 自动 refresh → 继续操作不卡
- [ ] 手动点登出,跳回登录页,localStorage 清空
- [ ] 网络里看不到 `X-Console-Token`、`X-Console-Locale` 请求头(§1.3 / §1.13 已删)
- [ ] 后端响应 body 字段是 `message`(不是 `reason`)(§1.1)

### P1 完成后必过
- [ ] 用户列表能拉到后端数据(分页 / 筛选 / 排序生效)
- [ ] 创建 / 编辑 / 删除用户后,刷新页面列表同步更新
- [ ] 角色列表能拉到后端数据;角色编辑弹窗"权限分配" tab 能勾选菜单树 + API 目录
- [ ] 改一个角色的菜单权限后,用该角色账号登录,看到的菜单/按钮级权限已变
- [ ] 菜单管理页能 CRUD 菜单;新增菜单后用 admin 账号登录,nav 多了入口
- [ ] 删除一个菜单,该菜单从所有角色的勾选里消失

### P2 完成后必过
- [ ] 部门管理能 CRUD
- [ ] 登录日志列表能拉真实数据
- [ ] 操作审计列表能拉真实数据(后端审计钩子要先就位)
- [ ] 启用多租户后,登录页能选租户;切租户后 nav 刷新
- [ ] 按钮级权限:用非 admin 账号登录,看不到"删除"按钮;控制台 `__forceTokenExpired()` 仍能验证 refresh 链路

---

## 7. 决策记录(写进 `admin/docs/DECISIONS.md`,新建)

> 集成过程中遇到的"是 / 否"二选一问题,都记到这里,带日期 + 决策者。

| 日期 | 议题 | 选项 | 决策 | 决策者 | 对应 |
|---|---|---|---|---|---|
| 2026-08-10 | 响应外壳 | `{code, reason, data}` vs `{code, message, data}` | **后者** | goelea | §1.1 |
| 2026-08-10 | 业务码分流 | 按 HTTP status vs 按 body.code | **按 body.code** | goelea | §1.2 |
| 2026-08-10 | Token Header | `X-Console-Token` vs `Authorization: Bearer` | **后者** | goelea | §1.3 |
| 2026-08-10 | LoginResponse 字段 | `{accessToken, refreshToken, expiresIn, user}` vs `{access_token, refresh_token, expires, uid, username, tenant}` | **后者**(对齐 proto)+ `role` 走 `/user/permissions` | goelea | §1.4 |
| 2026-08-10 | LoginBody 字段 | `account` vs `username` | **`username`**(对齐 proto) | goelea | §1.4 |
| 2026-08-10 | Role.DataScope | MVP 消费 vs MVP 不消费 | **不消费**(前端加可选字段,UI 不渲染) | goelea | §1.5 |
| 2026-08-10 | 多租户 MVP | 启用 vs 单租户 | **单租户**(后端从 user 自动区分,前端无 UI);§4.5 降级为未来扩展 | goelea | §1.6 |
| 2026-08-10 | 路径前缀 | 无前缀 vs `/api/v1` | **不加统一前缀**(`VITE_API_BASE='/'`,各 api 写完整路径) | goelea | §1.7 |
| 2026-08-10 | 错误文案 | 走 i18n 表 vs 直接显示后端 message | **直接显示后端 message**(i18n 表只用于前端业务事件 toast) | goelea | §1.8 |
| 2026-08-10 | proto → TS | ts-proto 生成 vs 手写 DTO | **手写 DTO**(`src/api/pb/`)+ §6 加 DTO 一致性比对 | goelea | §1.9 |
| 2026-08-10 | 字段命名映射 | 后端加 json_name vs 手写 from/to DTO | **手写 fromXxxDto/toXxxDto**(DTO 内 camelCase,wire snake) | goelea | §1.10 |
| 2026-08-10 | 时间格式 | ISO 8601 vs Unix 秒 | **ISO 8601**(`dayjs` 直解) | goelea | §1.11 |
| 2026-08-10 | i18n locale header | 保留 `X-Console-Locale` vs 删除 | **删除**(纯后端文案) | goelea | §1.13 |
| 2026-08-10 | rest/v3 自动 CRUD vs 自建 RPC | 全部走 REST vs 全部走 RPC | REST 优先,RPC 仅用于"自助"/"业务"接口(见 §3.1) | goelea | §3.1 |

---

## 8. 参考资料

- 后端集成入口:`admin/README.md`
- 后端架构概览:`docs/superpowers/plans/2026-08-07-split-permissions.md`、`docs/superpowers/plans/2026-08-07-role-permission-menu.md`
- 前端架构概览:`dashboard/web/docs/superpowers/specs/2026-07-16-admin-template-completion-design.md`(注:对接开始后该文档"过期",作为实现历史保留)
- 前端多标签实现:`dashboard/web/.mimocode/plans/1784281210146-brave-eagle.md`
- 跨端契约基线(待新建):`admin/docs/CONTRACT.md`
- 决策记录(待新建):`admin/docs/DECISIONS.md`
