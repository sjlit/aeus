# __AEUS_NAME__

An AEUS service with the admin console, JWT/RBAC, SQLite and a Vue 3
management UI.

## Quick start

```bash
# 1. Build the web UI (Node 20+)
make web

# 2. Start the server
make run
```

Open <http://localhost:8080> and sign in with:

```text
admin / Admin123
```

The default tenant, super role, permission catalog and admin user are
seeded on first start. Change both the password and `JWT_SECRET` before
exposing the service.

## Configuration

Copy `.env.example` and export the values you need:

| Variable | Default | Purpose |
|---|---|---|
| `APP_NAME` | `__AEUS_NAME__` | service name |
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `DB_DSN` | `./data/aeus.db` | SQLite file (`:memory:` for tests) |
| `JWT_SECRET` | `change-me` | HMAC signing key — replace in production |
| `ADMIN_USER` | `admin` | seeded super user |
| `ADMIN_PASSWORD` | `Admin123` | seeded password — replace after login |
| `WEB_ROOT` | `./web/dist` | built SPA directory |
| `DEBUG` | `true` | framework debug mode |
| `MULTI_TENANT` | `false` | enable GORM tenant scoping |
| `AUTO_MIGRATE` | `true` | boot-time table DDL |
| `SEED_ON_START` | `true` | seed the admin/role/menu catalog on boot |
| `CORS_ORIGINS` | empty | comma-separated allowed browser origins; empty = same-origin |
| `CORS_ALLOW_CREDENTIALS` | `false` | allow cookies on cross-origin requests |

## Production migration

`AUTO_MIGRATE=true` is convenient in development but should be off in
production:

```bash
make migrate                 # create/update the schema explicitly
AUTO_MIGRATE=false make run
```

`make seed` runs the idempotent seed outside the server process. In a
multi-instance deployment, set `SEED_ON_START=false` and run it as a
one-off Job instead of seeding in every replica.

## Commands

```bash
make run       # start the API + SPA
make web       # npm ci && npm run build
make build     # build the SPA and bin/__AEUS_NAME__
make test      # go test ./...
make migrate   # explicit schema migration
make seed      # explicit admin/role/menu seed
```

## Layout

```text
cmd/server/          HTTP entrypoint + Golden Path smoke test
cmd/seed/            one-off seed command
cmd/migrate/         explicit schema migration
internal/config/     environment-backed configuration
internal/bootstrap/  DB, admin server, JWT/RBAC wiring
web/                 Vue 3 + Element Plus + @sjlit/rest-ui
```

## Adding a business model

Business resources are proto-first. The template ships a complete
example at `proto/shop/v1/order.proto`:

1. Edit or copy a proto under `proto/<module>/<version>/`.
2. Declare the entity with `(aeus.entity)`, the admin menu with
   `(aeus.admin)`, and custom RPCs with `(aeus.api)` / `(aeus.policy)`.
3. Generate and wire:

   ```bash
   make proto
   go mod tidy
   aeus gen
   # or: make proto PROTO_PATHS="shop warehouse"
   ```

   `aeus gen` reads the `_aeus.json` manifests and rewrites
   `internal/bootstrap/generated.go`, registering module menus, models
   and route policies. It also creates
   `web/src/views/shop/order/Index.vue` and
   `web/src/api/generated/shop/v1/order.ts` when they do not exist yet.
   The TS file includes proto enums, nested messages, cross-file imports
   and custom-RPC wrappers, and is refreshed on later runs when it still
   carries the generated header.
   Use `--web-dir` / `--api-dir` to change the output directories, or
   `--no-web` / `--no-api` to skip frontend generation.

4. `make migrate` uses the generated `bootstrap.MigrateGenerated`, so
   generated entities are included automatically.
5. Replace the generated placeholder view under
   `web/src/views/shop/order/Index.vue` when you need custom behavior.
   Later `aeus gen` runs never overwrite an existing view.

The generic REST CRUD endpoints, menu row, permission catalog entries,
and placeholder Vue page are all derived from the proto annotations.
For complex domain behavior, add a service implementation and register
it with the generated `RegisterXxxRouter` function.

> `make proto` requires `buf` plus `protoc-gen-go`,
> `protoc-gen-go-aeus`, and `protoc-gen-validate` on `PATH`.
> `aeus gen` requires the AEUS CLI (`go install
> github.com/sjlit/aeus/cmd/aeus@latest`).
> The generated `_model.pb.go` file is intentionally a persistence
> entity; define a separate `OrderView` message for wire responses that
> need base fields such as `id` / `created_at`.
