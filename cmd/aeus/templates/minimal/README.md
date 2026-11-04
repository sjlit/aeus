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

Override the defaults with environment variables (see `.env.example`):

```bash
HTTP_ADDR=:9090 DEBUG=false make run
```

## Build and test

```bash
make test
make build      # bin/__AEUS_NAME__
make vet
```

## Docker

```bash
docker build -t __AEUS_PACKAGE__ .
docker run --rm -p 8080:8080 __AEUS_PACKAGE__
```

## Layout

```text
cmd/server/        entrypoint
internal/config/   environment-backed configuration
```
