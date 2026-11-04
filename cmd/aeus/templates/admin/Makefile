APP := __AEUS_NAME__

.PHONY: run web build test migrate seed fmt vet

run:
	go run ./cmd/server

web:
	cd web && npm ci && npm run build

build: web
	go build -trimpath -o bin/$(APP) ./cmd/server

test:
	go test ./...

# Production path: build the schema with AUTO_MIGRATE=true here, then
# run the server with AUTO_MIGRATE=false. Add application models to
# cmd/migrate/main.go as you introduce them.
migrate:
	go run ./cmd/migrate

seed:
	go run ./cmd/seed

fmt:
	gofmt -w $$(find . -name '*.go')

vet:
	go vet ./...
