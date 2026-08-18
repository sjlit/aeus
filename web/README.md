# AEUS Web

Vue 3 + Element Plus 管理后台骨架。侧边栏与业务路由完全由 admin 的
`GET /menu/tree` 驱动 —— **菜单 uri 即客户端路由**。

## 启动

```bash
# 1. 后端(mock, 固定 :8080, 种子账号 admin / Admin123)
cd ../admin/cmd/mock && go run .

# 2. 前端(dev, :5173, /api 代理到 :8080)
npm install
npm run dev
```

打开 http://localhost:5173 ，用 `admin / Admin123` 登录。

## 命令

| 命令 | 作用 |
|------|------|
| `npm run dev` | 开发服务器(:5173) |
| `npm run typecheck` | vue-tsc 类型检查 |
| `npm test` | vitest 单测(信封/菜单分组纯函数、auth store) |
| `npm run build` | 类型检查 + 构建到 dist/ |

## 约定

- 信封:所有响应 `{code, message, data}`,HTTP 恒 200;只看 `code`。
- 认证失败码 `4001/4002/4006` 触发静默刷新;`4003/4005` 只 toast。
- `refresh()` 不覆盖 profile;refresh token 不轮换。
- 路由:Hash 模式;业务路由从 MenuTree 动态注册,uri 即路径。
- 视觉:`src/styles/style.scss` 提供共享 token + 玻璃拟态工具类;
  `src/styles/app.scss` 桥接 Element Plus 主题并承载管理台布局。`app.scss` 顶部用 `@use './style'` 引入共享样式。