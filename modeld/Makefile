# Makefile for the modeld daemon (model-repository service).
#
# modeld is the split-out, transport-facing owner of modelrepo state; the
# transport/ subpackage holds the gRPC surface. Run targets from this directory.

PKG   := ./...
PROTO := transport/grpc/modeld.proto

.PHONY: help build test test-unit test-system vet proto clean

help:
	@echo "modeld targets:"
	@echo "  build        compile all modeld packages"
	@echo "  test         run all tests"
	@echo "  test-unit    run unit tests (TestUnit_*)"
	@echo "  test-system  run system tests (TestSystem_*)"
	@echo "  vet          go vet"
	@echo "  proto        regenerate gRPC stubs from modeld.proto"
	@echo "  clean        go clean"

build:
	go build $(PKG)

test:
	go test $(PKG)

test-unit:
	go test -run 'TestUnit_' $(PKG)

test-system:
	go test -run 'TestSystem_' $(PKG)

vet:
	go vet $(PKG)

# Requires protoc with protoc-gen-go and protoc-gen-go-grpc on PATH.
proto:
	@command -v protoc >/dev/null || { echo "protoc not installed"; exit 1; }
	protoc \
		--go_out=.. --go_opt=module=github.com/contenox/runtime \
		--go-grpc_out=.. --go-grpc_opt=module=github.com/contenox/runtime \
		$(PROTO)

clean:
	go clean $(PKG)
