GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)
DATETIME:=$(shell date "+%Y-%m-%d %H:%M:%S")


PROTO_DIR="pb"
PROTO_OUT_DIR="pb"
PROTO_FILES=$(shell find pb -name *.proto)

.PHONY: proto
proto:
	protoc --proto_path=$(PROTO_DIR) \
		--proto_path=../pkg/proto/rest \
		--proto_path=../pkg/proto/mcp \
		--proto_path=./third_party \
		--go_out=paths=source_relative:$(PROTO_OUT_DIR) \
		--go-aeus_out=paths=source_relative:$(PROTO_OUT_DIR) \
		--validate_out=paths=source_relative,lang=go:$(PROTO_OUT_DIR) \
		$(PROTO_FILES)

.PHONY: proto-clean
proto-clean:
	rm -rf $(PROTO_OUT_DIR)/*.pb.go
	rm -rf $(PROTO_OUT_DIR)/*.pb.validate.go