# 操作审计


## 启用

```go
s := admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
    admin.WithAudit(true),                      // 开启操作审计
    // 可选:追加排除项(内置排除始终生效)
    admin.WithAuditExcludes("app/high_freq_table"),
)
```

## 机制

开启后 `Setup` 在注册任何模型**之前**调用 `installAuditHooks`,向 rest/v3 注册**进程级全局** after-hooks(`RegisterAfterCreate/AfterUpdate/AfterDelete`)。每次 REST 写操作成功后,异步于业务事务地同步追加一行 `sys_audits`:

| 列 | 来源 |
|----|------|
| `uid` | `ResourceConfig.UserResolve`(默认读 JWT claims 的 `UID`;可用 `WithUserResolve` 覆盖) |
| `tenant_id` | `RuntimeScope.TenantID`(与租户隔离同一 resolver) |
| `action` | `create` / `update` / `delete` |
| `module` / `table` | 模型的 `ModuleName()` + `TableName()` |
| `data` | create/update:`[]DiffAttr`(column/label/previous/current)的 JSON;delete:仅 `{"id":<主键>}`(**不序列化整行**,避免把 `User.Password` 等 bcrypt 哈希写进审计表) |

- **全局生效**:由于 rest/v3 在资源构造时快照全局钩子,凡在 `Setup` 之后注册的资源——包括其它模块通过 `NewResourceWithOptions` 注册的应用模型——都自动被审计;
- **排除项**:内置排除 `system/sys_audits`(防递归)与 `system/sys_login_logs`(登录审计有自己的 `WithLoginLogger` 管线),`WithAuditExcludes` 可按 `"<module>/<table>"` 追加;
- **无变更不记录**:update 的 diff 为空(未实际修改)时跳过;
- **best-effort**:审计写入失败仅记 Warn 日志,绝不影响业务响应(after-hook 本就运行在业务事务提交之后、且被库内 `safelog.SafeRun` 包裹);
- **列宽截断**:`data` 按 10240 字节、`uid`/`module`/`table` 按各自列宽做 UTF-8 安全截断。

## 覆盖边界

审计只拦 **REST CRUD 路径**(rest/v3 的 Create/Update/Delete)。service 层裸 gorm 写、定时任务、gRPC 直写不会产生审计行;如需覆盖这些路径,需另行接入 GORM callback 并处理去重。
