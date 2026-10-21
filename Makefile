PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.6.1
PROTO_TOOLS := $(CURDIR)/.tools/bin

.PHONY: proto test
# Requires protoc 33.5 on PATH. Generated files are committed.
proto:
	GOBIN=$(PROTO_TOOLS) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PROTO_TOOLS) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	PATH="$(PROTO_TOOLS):$$PATH" protoc -I api/proto --go_out=gen --go_opt=paths=source_relative --go-grpc_out=gen --go-grpc_opt=paths=source_relative auth/v1/auth.proto

test:
	go test ./...
