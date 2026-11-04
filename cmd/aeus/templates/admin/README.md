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

1. Create the model under `internal/models` (embed `models.TenantModel`
   if it belongs to a tenant).
2. Register it with `admin.Server.RegisterModel` after `Setup`, or add
   it to the template's bootstrap.
3. Add the model to `cmd/migrate/main.go`.
4. Add a `SchemaViewer` page under `web/src/views/<module>/<table>/`.

The admin console reads the menu and column metadata from the backend,
so a registered model with a `MenuProvider` shows up automatically.
