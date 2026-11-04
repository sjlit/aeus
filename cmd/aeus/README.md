# aeus CLI

Scaffold a new AEUS project:

```bash
go run ./cmd/aeus new my-service
go run ./cmd/aeus new my-admin --template=admin
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `--template` | `minimal` | `minimal` or `admin` |
| `--module` | project name | Go module path (`github.com/acme/my-service`) |
| `--local` | auto-detected in a source checkout | emit `replace` directives to a local AEUS checkout |
| `--force` | false | write into a non-empty directory |

The generated project runs `go mod tidy` at the end. The web app in the
`admin` template is built separately with `make web`.

## Templates

- `templates/minimal`: HTTP + config + health + graceful shutdown.
- `templates/admin`: minimal + GORM/SQLite + admin REST/RPC + JWT/RBAC
  + Seed + Vue 3 management console.

Template files use `__AEUS_*__` placeholders. Files ending in `.tmpl`
are written without that suffix (notably `go.mod` and the Go sources,
so the Go toolchain does not treat the template tree as part of the
root module).

The admin web console is generated from `admin/web` with:

```bash
make sync-web   # equivalent to: go generate ./cmd/aeus
```

Template-owned files (`README.md`, `.gitignore`, the smoke test and
`src/components.d.ts`) are preserved; everything else is copied with the
scaffold placeholders applied. CI runs the generator and fails on drift.

## Releases

`versions.json` holds the versions embedded into generated `go.mod`
files. Update it in the same coordinated release that tags the root,
`transport/http` and `admin` modules. Until then, source checkouts use
the auto-detected `--local` path.
