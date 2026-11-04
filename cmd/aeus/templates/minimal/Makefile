APP := __AEUS_NAME__

.PHONY: run test build fmt vet

run:
	go run ./cmd/server

test:
	go test ./...

build:
	go build -trimpath -o bin/$(APP) ./cmd/server

fmt:
	gofmt -w $$(find . -name '*.go')

vet:
	go vet ./...
