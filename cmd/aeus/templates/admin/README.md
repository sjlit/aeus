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

Copy the example when you need file-based settings:

```bash
cp configs/config.example.yaml configs/config.yaml
```

The loader reads `configs/config.yaml` when present, then applies
environment variables. Explicit `--config` and `CONFIG_FILE` paths must
exist; the default development path is optional. `.env.example` documents
the environment overrides.

```yaml
app:
  name: __AEUS_NAME__
  debug: true
  multi_tenant: false
server:
  http_addr: ":8080"
  web_root: "./web/dist"
  cors:
    origins: []
    allow_credentials: false
database:
  dsn: "./data/aeus.db"
auth:
  jwt_secret: change-me
  admin_user: admin
  admin_password: Admin123
bootstrap:
  auto_migrate: true
  seed_on_start: true
log:
  level: info
  format: text      # text | json
  output: stdout    # stdout | stderr | ./logs/app.log
```

Environment variables (`APP_NAME`, `HTTP_ADDR`, `DB_DSN`,
`JWT_SECRET`, `ADMIN_USER`, `ADMIN_PASSWORD`, `WEB_ROOT`, `DEBUG`,
`MULTI_TENANT`, `AUTO_MIGRATE`, `SEED_ON_START`, `CORS_ORIGINS`,
`CORS_ALLOW_CREDENTIALS`, `LOG_LEVEL`, `LOG_FORMAT`, `LOG_OUTPUT`)
override the file. File output is not rotated; use external logrotate or
container log collection.

## Build and version

`make build` injects the git version, commit and UTC build time through
`-ldflags`. The same metadata appears in startup logs and the registry.

```bash
make build
./bin/__AEUS_NAME__ -version
./bin/__AEUS_NAME__ -config configs/config.yaml
```

`make run` also injects the build metadata, so the local version is not a
static `dev` value.

## Production migration

`bootstrap.auto_migrate=true` is convenient in development but should be
off in production:

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
make proto     # buf generate
make gen       # aeus gen (after make proto)
```

## Layout

```text
cmd/server/          HTTP entrypoint + Golden Path smoke test
cmd/seed/            one-off seed command
cmd/migrate/         explicit schema migration
internal/app/        Scope and Loader lifecycle; wiring in wire.go
internal/config/     YAML + environment configuration
internal/gen/        aeus gen output; do not edit
internal/logging/    slog handler + infra/logger factory
internal/store/      DB connection and catalog migration
configs/             config.example.yaml (copy to config.yaml)
web/                 Vue 3 + Element Plus + @sjlit/rest-ui
```

`Loader.Run` is the place for future background work (queue consumers,
cron jobs, watchers); `Loader.Init` owns startup wiring.

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
   make gen
   # or: make proto PROTO_PATHS="shop warehouse"
   ```

   `aeus gen` reads the `_aeus.json` manifests and rewrites
   `internal/gen/wiring.go`, registering module menus, models and route
   policies. It also creates
   `web/src/views/shop/order/Index.vue` and
   `web/src/api/generated/shop/v1/order.ts` when they do not exist yet.
   The TS file includes proto enums, nested messages, cross-file imports
   and custom-RPC wrappers, and is refreshed on later runs when it still
   carries the generated header.
   Use `--web-dir` / `--api-dir` to change the output directories, or
   `--no-web` / `--no-api` to skip frontend generation.

4. `make migrate` calls `gen.Migrate`, so generated entities are included
   automatically.
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
