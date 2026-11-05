# __AEUS_NAME__

A minimal AEUS HTTP service.

## Run

```bash
go mod tidy
make run
```

The server listens on `:8080` by default and exposes:

- `GET /health`
- `GET /ready`

## Configuration

Copy the example when you need file-based settings:

```bash
cp configs/config.example.yaml configs/config.yaml
```

The loader reads `configs/config.yaml` when present, then applies
environment variables. Explicit `--config` and `CONFIG_FILE` paths must
exist; the default development path is optional.

```yaml
app:
  name: __AEUS_NAME__
  debug: true
server:
  http_addr: ":8080"
log:
  level: info
  format: text      # text | json
  output: stdout    # stdout | stderr | ./logs/app.log
```

Override the defaults with environment variables (see `.env.example`):

```bash
HTTP_ADDR=:9090 DEBUG=false LOG_FORMAT=json make run
```

File output is not rotated; use external logrotate or container log
collection.

## Build and version

`make build` injects the git version, commit and UTC build time through
`-ldflags`:

```bash
make build
./bin/__AEUS_NAME__ -version
```

## Test

```bash
make test
make vet
```

## Docker

```bash
docker build \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  -t __AEUS_PACKAGE__ .
docker run --rm -p 8080:8080 __AEUS_PACKAGE__
```

## Layout

```text
cmd/server/        entrypoint
internal/config/   YAML + environment configuration
configs/           config.example.yaml (copy to config.yaml)
```
