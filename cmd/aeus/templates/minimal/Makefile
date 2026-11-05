APP := __AEUS_NAME__

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X main.appName=$(APP) -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)
BUILD_FLAGS := -trimpath -ldflags "-s -w $(LDFLAGS)"

.PHONY: run test build fmt vet

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/server

test:
	go test ./...

build:
	go build $(BUILD_FLAGS) -o bin/$(APP) ./cmd/server

fmt:
	gofmt -w $$(find . -name '*.go')

vet:
	go vet ./...
