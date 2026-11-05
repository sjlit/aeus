APP := __AEUS_NAME__
PROTO_PATHS ?= shop

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X main.appName=$(APP) -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)
BUILD_FLAGS := -trimpath -ldflags "-s -w $(LDFLAGS)"

.PHONY: run web build test migrate seed fmt vet proto gen proto-lint proto-breaking

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/server

web:
	cd web && npm ci && npm run build

build: web
	go build $(BUILD_FLAGS) -o bin/$(APP) ./cmd/server

test:
	go test ./...

# Production path: build the schema with bootstrap.auto_migrate=true here,
# then run the server with bootstrap.auto_migrate=false. Add application
# models to internal/store/migrate.go as you introduce them.
migrate:
	go run -ldflags "$(LDFLAGS)" ./cmd/migrate

seed:
	go run -ldflags "$(LDFLAGS)" ./cmd/seed

fmt:
	gofmt -w $$(find . -name '*.go')

vet:
	go vet ./...

# Generate Go code from proto/. Requires buf and the protoc plugins
# (protoc-gen-go, protoc-gen-go-aeus, protoc-gen-validate) on PATH.
proto:
	cd proto && buf generate $(addprefix --path ,$(PROTO_PATHS))
	gofmt -w ./proto

gen:
	aeus gen

proto-lint:
	cd proto && buf lint

proto-breaking:
	cd proto && buf breaking --against '../.git#branch=main'
