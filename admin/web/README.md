# AEUS Web

Vue 3 + Element Plus 管理后台骨架。侧边栏与业务路由完全由 admin 的
`GET /user/menus` 驱动 —— **菜单 uri 即客户端路由,view_path 决定视图组件**。

> ↩ 父模块文档：[`admin/README.md`](../README.md)。本文件只覆盖前端日常开发命令与内部约定；**前后端契约清单**（侧栏/CRUD 页面/角色授权/租户切换各依赖哪些后端端点）写在主 README 的 [前端管理台（admin/web）](../README.md#前端管理台adminweb) 章节里——后端改 RBAC 端点或 SchemaViewer 模板前先去那里同步确认。

## 启动

```bash
# 1. 后端(mock, 固定 :8080, 种子账号 admin / Admin123)
#    注意:mock 位于 admin/cmd/mock,从 admin/web 进入是 ../cmd/mock
cd ../cmd/mock && go run .

# 2. 前端(dev, :5173, /api 代理到 :8080)
npm install
npm run dev
```

打开 http://localhost:5173 ，用 `admin / Admin123` 登录。

如果要在本项目里直接调试 `@sjlit/rest-ui` 源码，设置两个环境变量：

```bash
REST_UI_LOCAL=1 REST_UI_ROOT=/path/to/rest-ui npm run dev
```

`REST_UI_ROOT` 指向 rest-ui checkout 根目录（支持绝对路径或相对当前目录的路径）。只设 `REST_UI_LOCAL` 不设 `REST_UI_ROOT` 会直接报错，避免落到机器相关的默认路径上。

## 命令

| 命令 | 作用 |
|------|------|
| `npm run dev` | 开发服务器(:5173) |
| `npm run typecheck` | vue-tsc 类型检查 |
| `npm test` | vitest 单测(信封/菜单分组/品牌解析/view_path 规范化/auth store) |
| `npm run build` | 类型检查 + 构建到 dist/ |

## 约定

- 信封:所有响应 `{code, message, data}`;业务响应 HTTP 200,中间件错误(401/403/404/429 等)使用标准状态码,前端同时按 `code` 和 401 处理。
- 认证失败码 `4001/4002/4006` 触发静默刷新;传输层 HTTP 403 + `4005` 也按会话失效处理;HTTP 200 的业务 `4005`(如旧密码错)只 toast。
- `refresh()` 不覆盖 profile;后端 refresh token **会轮换**,前端把 access/refresh 都持久化到 localStorage(`aeus.access` / `aeus.refresh`),刷新页面后仍可静默续期。对 XSS 更敏感的场景应改为 HttpOnly refresh cookie。
- 路由:Hash 模式;业务路由从 MenuTree 动态注册,uri 即路径,view_path 决定视图文件。
- 视觉:`src/styles/style.scss` 提供共享 token + 玻璃拟态工具类;
  `src/styles/app.scss` 桥接 Element Plus 主题并承载管理台布局。`app.scss` 顶部用 `@use './style'` 引入共享样式。

## 排版

字号统一走 `src/styles/style.scss` 的 `--fs-*` token:

| token | 值 | 用途 |
|-------|----|------|
| `--fs-2xs` | 11px | 仅 Latin/mono 辅助信息、kbd、图表刻度 |
| `--fs-xs` | 12px | 辅助文字 |
| `--fs-sm` | 13px | 紧凑正文、表格 |
| `--fs-base` | 14px | 正文、表单控件 |
| `--fs-md` | 16px | 卡片标题 |
| `--fs-lg` | 20px | 页面标题、个人资料名 |
| `--fs-xl` | 22px | 品牌名 |
| `--fs-2xl` | 32px | KPI 数字、登录页主标题 |
| `--fs-display` | 36px | 展示型大标题 |
| `--fs-hero` | 64px | 404 等超大标题 |

新增样式不要再写裸 `px` 字号；中文正文最小 12px。小图标沿用最近的字号档位，15/18/21/22 等装饰性大图标保留视觉尺寸。中文标题负字距不超过 `-0.01em`，品牌名可配置为中文，不使用负字距。中文字重由系统字体提供，不同系统未必有 600 档，不要只靠字重区分层级。

## 品牌配置

侧栏、登录页和浏览器标题共用运行时品牌配置 `public/app-config.js`：

```js
window.__AEUS_CONFIG__ = {
  appName: 'Board',
  appMark: 'D',
}
```

- `appName`：产品名，显示在侧栏、登录页和浏览器标题。
- `appMark`：品牌方块里的字符；留空时取 `appName` 首字符。
- Vite 会把 `public/` 原样复制到 `dist/`；部署后直接修改 `dist/app-config.js` 并刷新即可，不必重新构建。
- 生产环境建议给该文件设置 `Cache-Control: no-cache`，否则改完可能被浏览器缓存。

## 关键端点速查

前端实际只依赖后端**五类**端点（按使用频率）：

| 用途 | 端点 |
|------|------|
| 登录 / 刷新 / 登出 | `/auth/login` · `/auth/refresh-token` · `/auth/logout` |
| 当前用户自助 | `/user/profile` · `/user/change-password` · `/user/menus` · `/user/permissions` |
| CRUD 渲染（SchemaViewer 自动驱动） | `/schema/:module/:table` · `/rest/model-types/:module/:table` · `/rest/model-tiers/:module/:table` |
| 角色授权编辑 | `/role/options` · `/role/permissions` · `/role/menus` · `/permission/catalog` · `/menu/tree` |
| 租户切换（多租户模式下） | `/tenant/options` · `/tenant/detail` |

> 完整列表与契约细节见主 README 的 [前端管理台（admin/web）](../README.md#前端管理台adminweb)。改动这五类端点的请求 / 响应形状前，先看一眼主 README 的 [接口权限校验（RBAC）](../README.md#接口权限校验rbac) 章节——自助路由（profile / menu / password 等）由 checker 的内置公开清单放行；新增的管理端点默认 fail-closed，必须进入 `sys_permissions` 目录并由角色授权。`WithCheckerAllowlist` 只是显式 bypass，不是“登记后更严格”。