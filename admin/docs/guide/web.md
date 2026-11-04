# 前端管理台（admin/web）


`admin/web/` 是配套的 Vue 3 + Element Plus SPA，使用 [`@sjlit/rest-ui`](https://github.com/sjlit/rest-ui) 的 `SchemaViewer` 直接驱动整张 CRUD 页面。日常开发命令、目录约定、与 Element Plus 的视觉桥接都写在 [`admin/web/README.md`](../../web/README.md)；本节只描述**前后端契约**——前端依赖哪些后端端点、每个端点在哪一章定义，便于后端改动时同步检查前端。

## 后端契约清单

| 前端用法 | 后端端点 | 后端定义位置 |
|---------|---------|-------------|
| 任意模型 CRUD 页面（`SchemaViewer` 自动驱动） | `GET /schema/:module/:table`（列元数据）+ `GET /rest/model-types/:module/:table` / `GET /rest/model-tiers/:module/:table`（下拉数据） | [元数据 / 选项查询](../api/rest-crud.md) |
| 侧边栏（菜单树） | `GET /user/menus` | [UserService](auth.md#userservice) |
| 每按钮 guard（前端按钮可见性） | `GET /user/permissions` | [UserService](auth.md#userservice) |
| 当前用户右上角菜单 | `GET /user/profile` | [UserService](auth.md#userservice) |
| 修改自己密码 | `POST /user/change-password` | [UserService](auth.md#userservice) |
| 登录页 | `POST /auth/login` | [AuthService](auth.md#authservice) |
| Token 刷新（access token 过期） | `POST /auth/refresh-token` | [AuthService](auth.md#authservice) |
| 登出 | `POST /auth/logout` | [AuthService](auth.md#authservice) |
| 角色编辑页下拉数据 | `GET /role/options` / `GET /permission/catalog` / `GET /menu/tree` | [RoleService / PermissionService / MenuService](../api/README.md) |
| 角色授权编辑（按角色 preview） | `GET /role/permissions` / `GET /role/menus` / `PUT /role/permissions` | [RoleService / PermissionService / MenuService](../api/README.md) |
| 切换租户（下拉） | `GET /tenant/options` / `GET /tenant/detail` | [TenantService](../api/tenant.md) |

> **契约变更原则**：后端改这五个 RBAC 端点的请求 / 响应形状时，前端 `@sjlit/rest-ui` 与 `admin/web/src/api/*` 都要同步更新；改自动生成的 `Index.vue` 模板（`vueTemplate` 常量在 [`vue_gen.go`](../../vue_gen.go)）时，前端手写的 9 个 `web/src/views/system/sys_*/Index.vue` 需要逐一对比——前者是契约源，后者是参考实现。

## 自动生成 vs 手写视图

`admin/web/src/views/system/sys_*` 下**目前 9 个模型视图都是手写的**（在 `web/` 引入 SchemaViewer 之前完成；保留是因为这些页面有手写的 #gridview 插槽、`scenarios` 调优与少量 i18n），`WithVueOutputDir` 默认**不会**覆盖它们：

```go
// 这条 RegisterModel 显式告诉生成器：不要碰这个手写视图
s.RegisterModel(&models.User{}, admin.WithRegisterVueOutputDir(""))
```

新增内置或应用模型时，`WithVueOutputDir` 自动生成的是"能跑起来就行"的占位 `SchemaViewer`——首屏跑通后再按需替换为带 `#gridview` 插槽的定制视图。详见 [Vue Index.vue 自动生成](vue-generation.md)。

## 开发联调

```bash
# 1. 启后端（demo 启动器，固定 :8080，种子账号 admin / Admin123）
cd admin/cmd/mock && go run .

# 2. 启前端（:5173，/api 代理到 :8080）
cd admin/web && npm install && npm run dev
```

打开 `http://localhost:5173`，用 `admin / Admin123` 登录。更多命令（`typecheck` / `test` / `build`）与前端内部约定（信封格式、auth 失败码、Hash 路由模式）见 [`admin/web/README.md`](../../web/README.md)。
