# dbcache 读穿透缓存


`admin/dbcache` 提供带依赖版本标记的读穿透缓存，是 `NewPermissionChecker` 内部 cache 层的实现。它解决两个具体问题：

- **避免每个请求一次目录/授权 SQL**：权限校验是热路径上每次请求都要跑的开销，原始实现每次请求两次 SQL（catalog + grant），在 catalog 稳定时几乎是浪费；
- **让授权变更在下一个请求生效**：写一次 `sys_role_permissions`，下一个请求立刻看到，而不是等缓存 TTL 过期。

## 核心 API

```go
c := dbcache.New(db,
    dbcache.WithDependency(dbcache.NewSqlDependency(
        dbcache.WithTable("sys_role_permissions"),
        dbcache.WithColumn("SUM(id)"),
        dbcache.WithCondition("deleted_at IS NULL"),
    )),
    dbcache.WithCacheDuration(time.Minute), // hard TTL
)
set, err := dbcache.Try(c, ctx, "perm:grant:tenant1:admin",
    func(tx *gorm.DB) (map[string]struct{}, error) { /* ... */ })
```

- `Try(c, ctx, key, f)`：命中且未过期则直接返回；否则跑 `f(tx)`（singleflight 保证并发只跑一次），结果存进 cache；
- `CacheDependency.GetValue(ctx, db)` 返回当前源表的一个紧凑版本标记；缓存项写入时记下当时的标记值，下次读取时若与当前值相等就视为未变更直接放行（节省源表扫描）。

## 版本标记策略

| 源表 | 标记列 | 备注 |
|------|--------|------|
| `sys_permissions` | `MAX(updated_at)` | catalog 行可在 admin CRUD 中被就地更新，MAX 跨秒数足够 |
| `sys_role_permissions` | `SUM(id)` | junction 行从不被原地更新（assignment 是 soft delete + re-insert），SUM 自动吃掉软删/重建；自增 id 总比历史大 |

两个标记都是**表全局**的，所以"任何位置一次写"都会让整张表的全部缓存键**惰性失效**——过度失效是安全的，失效不足才会泄露权限。

## 宽限窗与 TTL

- **1 秒宽限窗**：刚写入的缓存在 1 秒内不查 marker，避免写入瞬间的 burst read 每次都重跑 marker 查询；
- **TTL 是硬上限**：版本标记存在同秒级盲区（catalog 的 `MAX(updated_at)` 在秒级，grants 的 SUM 也对同秒级 revoke+grant 漏检），硬 TTL 给一个 bound。catalog 用 0 TTL（标记足够），grants 用 1 分钟 TTL。

## 后端切换

默认 `infra/cache/memory.NewCache()`（单进程内存，`map[string]any`）。多实例部署时通过 `WithCache(cache.Cache)` 切到共享后端（如 Redis）；Redis 后端走 JSON 往返所以不会发生值别名问题，内存后端的值会与缓存项共享底层 slice/map——调用方不能修改返回值。

测试覆盖见 `dbcache/cacher_test.go` 与 `dbcache/depend_test.go`。
