# 脚手架 CLI

`aeus` 用 Go 编写，发布后可直接安装：

```bash
go install github.com/sjlit/aeus/cmd/aeus@latest
```

在源码 checkout 内开发 CLI 时：

```bash
go run ./cmd/aeus new /tmp/demo --local="$PWD"
```

## 命令

```bash
# 最小 HTTP 服务（默认模板）
aeus new my-service

# 带 admin 后台：JWT/RBAC + Seed + Vue 管理界面
aeus new my-admin --template=admin

# 指定 Go module path
aeus new my-admin --template=admin --module=github.com/acme/my-admin
```

| Flag | 默认 | 含义 |
|---|---|---|
| `--template` | `minimal` | `minimal` 或 `admin` |
| `--module` | 项目名 | 生成 `go.mod` 的 module path |
| `--local` | 源码 checkout 内自动检测 | 生成指向本地 AEUS checkout 的 `replace` |
| `--force` | `false` | 允许写入非空目录 |

生成结束后 CLI 会执行 `gofmt` 与 `go mod tidy`。

## 模板

### minimal

- HTTP transport、配置、健康检查、优雅退出
- 一个 smoke test
- 不依赖 admin、GORM、MCP SDK

### admin

- 在 minimal 之上加入 GORM/SQLite、admin REST/RPC、JWT/RBAC、审计
- `cmd/migrate`、`cmd/seed`、`Makefile`、`Dockerfile`、`.env.example`
- Vue 3 + Element Plus 管理台（构建后由 Go binary 托管）
- 默认账号 `admin / Admin123`，首次启动后必须修改
- `make web` 构建前端；`make run` 启动后端

## 模板占位符

模板文件使用 `__AEUS_*__` 占位符；`.tmpl` 后缀在生成时去掉。`go.mod`
和 Go 源码因此不会被根模块的 `go test ./...` 当作本仓库代码。

`cmd/aeus/versions.json` 保存生成项目引用的根模块、`transport/http` 与
`admin` 版本；发布脚本会随 tag 一起更新它。

## 同步 admin web 模板

`cmd/aeus/templates/admin/web` 由 `admin/web` 生成，不要手工修改共享文件：

```bash
make sync-web   # 等价于 go generate ./cmd/aeus
```

生成器会复制共享文件并替换标题、包名和 API base；模板专属的
`README.md`、`.gitignore`、smoke test 与 `src/components.d.ts` 会被保留。
CI 会运行生成器并检查是否有 drift。
