# 测试、发布与贡献

## 测试与 CI

仓库根目录的 `.github/workflows/ci.yml` 对每个 Go 模块独立跑：

```
- gofmt -l $(find . -name '*.go')   # 格式检查
- go test -race -count=1 ./...      # 带 race 检测的单元 / 集成测试
- go vet ./...                       # 静态检查
```

矩阵包含：根模块、`transport/http`、`transport/grpc`、`transport/cli`、`registry/etcd`、`infra/cache/redis`、`infra/broker/nats`、`infra/telemetry/otel`、`infra/telemetry/prometheus`。

`registry/etcd` 与 `infra/broker/nats` 子模块的集成测试依赖外部服务；CI 会拉起 `quay.io/coreos/etcd:v3.5.18`（监听 `0.0.0.0:2379`）和 `nats:2.11-alpine`（监听 `4222`）作为 service container，并通过 runner 侧端口轮询确保就绪后再跑测试。

本地完整跑测试：

```bash
cp go.work.example go.work   # 多模块协作开发
go test -race ./...
```

`scaffold` job 还会：

- 运行 `go generate ./cmd/aeus` 并检查 admin web 模板没有 drift；
- 运行 `python3 scripts/check_docs_links.py` 检查文档相对链接；
- 分别生成 `minimal` / `admin` 项目并执行 `go test` / `go build`；
- 构建并测试 admin 模板的 Vue 应用。

> 生成器只使用标准库；CI 的 diff 检查会阻止手工修改模板 web 后忘记运行 `make sync-web`。

---

## 发布

仓库是多模块结构：根模块与每个子模块都有**独立版本号**，只有内容发生变更的模块才会升版本并打 tag，未变更的模块保持原版本。版本号全部由 git tag 推导，无需手工维护。

标签格式：根模块 `v1.0.3`，子模块 `transport/http/v1.0.3`。

### 发布步骤

```bash
# 1. 工作区必须干净（release 会拒绝脏工作区）
git status   # 确保无未提交改动

# 2. 自动检测变更模块，patch+1 并打 tag
make release

# 可选参数
make release DRY_RUN=1                        # 只打印将发布的模块和版本，不执行
make release VERSION=v1.1.0                   # 根模块升到指定版本
make release MODULES="transport/http=v1.2.0"  # 指定子模块版本

# 3. 推送
git push origin main
git push origin --tags
```

若自上次发布以来没有模块内容变更，`make release` 会中止并打印 `nothing to release: 自上次发布以来没有模块内容变更`。

`scripts/release.sh` 还会自动同步本次发布的子模块对根模块的 `require`（即便根模块没发布，也会对齐当前最新根 tag），避免子模块引用过期根版本。

`cmd/aeus/versions.json` 保存脚手架生成的模块版本，发布时需与本次 tag 对齐；否则生成的项目会引用旧版本。

本次脚手架/传输层变更包含新增公开 API 与 MCP/CORS 调整，建议：

```bash
make release VERSION=v1.1.0 \
  MODULES="admin=v1.1.0 transport/http=v1.1.0 transport/cli=v1.0.1 infra/telemetry/prometheus=v1.0.1"
```

> **发布后验证**：在干净工作区、`GOWORK=off` 下对受影响模块执行 `go build ./...`。子模块新增了根模块包（例如 `pkg/response`）时，根模块必须与 `transport/http`、`admin` 在同一次发布中打 tag；否则子模块会指向不含该包的旧根版本。

#### 同步 go.sum

`release.sh` 只更新模块间的 `require`，无法为尚未推送到远端的 tag 计算 `go.sum`。
推送 tags 后执行：

```bash
git push origin main
git push origin --tags

make sync-sums MODULES="admin transport/http transport/cli infra/telemetry/prometheus"
git add -A && git commit -m "chore: sync go.sum after release"
git push origin main
```

> tag 指向 sync-sums 之前的提交，因为发布时新 tag 还不在远端。模块消费者不受影响
> （他们使用自己的 `go.sum`）；从 tag checkout 本地开发时，先在该模块执行
> `go mod tidy` 即可。

然后验证：

```bash
go install github.com/sjlit/aeus/cmd/aeus@v1.1.0
aeus new /tmp/aeus-minimal
aeus new /tmp/aeus-admin --template=admin
```

分别在各生成项目中执行 `go test ./...` / `go build ./...`；admin 还要执行
`make web` 或 `npm ci && npm test && npm run build`。

### 消费者侧配置

仓库托管在 GitHub，模块可直接 `go get github.com/sjlit/aeus/...` 引入，无需额外 GOPROXY 配置。

---

## 贡献指南

### 代码规范

- 遵循 Go 官方 `gofmt` 格式化标准（CI 强制）
- 公共导出函数和类型必须有 godoc 注释
- 错误处理优先用 `pkg/errs` 构造业务错误（`errs.New` / `errs.Newf` / `errs.Wrap`）；不要在 framework 层直接返回 `http.StatusXxx` 数字
- 子模块边界清晰：往 `transport/http` 加功能时不要 import `registry/etcd` 这种重型依赖

### 环境变量

| 环境变量 | 默认 | 行为 |
| --- | --- | --- |
| `AEUS_DEBUG` | `false` | 解析失败时回退 `false`，不 panic |
| `HTTP_PORT` | `:0` | 空字符串回退 `:0`；非空且非法 → **panic**（fail-fast） |
| `GRPC_PORT` | `:0` | 同上 |
| `CLI_PORT` | `:0` | 同上 |

> 生产环境请通过对应 transport 的 `WithAddress("host:port")` 显式指定地址，不要依赖环境变量。

### PR 流程

1. Fork 仓库并基于 `dev` 创建特性分支
2. 提交代码并保证本地 `go test -race ./...` 与 `gofmt -l` 通过
3. 提交 Pull Request 到 `dev` 分支
4. 经过代码审查与 CI 通过后合并到 `main`，由 maintainer 触发发布流程

### 测试要求

- 新增功能必须附带单元测试
- 并发代码路径至少一个 `-race` 通过的测试
- 集成测试在缺外部依赖时 `t.Skip` 即可（CI 会拉起对应容器）

### 本地多模块开发

```bash
cp go.work.example go.work
# 编辑 go.work 按需调整
go test ./...
```

`go.work.example` 已列出所有子模块，按需启用即可在同一仓库中跨模块改动。
