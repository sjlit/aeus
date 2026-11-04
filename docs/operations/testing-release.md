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

### 推荐：分阶段发布

跨模块依赖变更时，用两阶段发布让每个子模块 tag 自带完整 `go.sum`：

```bash
# 1. 准备发布提交 + 根模块 tag（子模块暂不打 tag）
make release-staged VERSION=v1.2.0 \
  MODULES="admin=v1.2.0 transport/http=v1.2.0"

# 2. 推送发布提交和根 tag
git push origin main
git push origin v1.2.0

# 3. 推送子模块、同步 go.sum、打子模块 tag 并推送
make release-publish
```

`release-staged` 会：

- 检测变更模块并写 `cmd/aeus/versions.json`；
- 对齐子模块对根模块 / `transport/http` 的 `require`；
- 提交发布改动并只给根模块打 tag；
- 在 `.git/aeus-release-state` 写入待发布子模块。

`release-publish` 会按依赖顺序（`admin` 最后）执行：

- 推送 `main` 和根 tag；
- 在每个子模块运行 `GOWORK=off go mod tidy`；
- 提交 `go.sum` 变化；
- 在同步后的提交上打子模块 tag 并推送。

若自上次发布以来没有模块内容变更，`release-staged` 会中止并打印
`nothing to release: 自上次发布以来没有模块内容变更`。

### 简单发布（兼容路径）

只发布不涉及跨模块依赖的模块时，仍可使用：

```bash
make release
git push origin main
git push origin --tags
```

这条路径的子模块 tag 指向 `go.sum` 同步之前的提交，需要额外执行：

```bash
make sync-sums MODULES="admin transport/http transport/cli infra/telemetry/prometheus"
git add -A && git commit -m "chore: sync go.sum after release"
git push origin main
```

### 发布后验证

在干净工作区、`GOWORK=off` 下对受影响模块执行 `go build ./...`。子模块新增了根模块包
（例如 `pkg/response`）时，根模块必须与 `transport/http`、`admin` 在同一次发布中打 tag；
否则子模块会指向不含该包的旧根版本。

```bash
go install github.com/sjlit/aeus/cmd/aeus@v1.2.0
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
