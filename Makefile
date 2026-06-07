# Contenox — namespaces: build-*  test-*  dev-*  deps-*
# Default: make help

PROJECT_ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
.DEFAULT_GOAL := help

EMBED_MODEL ?= nomic-embed-text:latest
EMBED_PROVIDER ?= ollama
EMBED_MODEL_CONTEXT_LENGTH ?= 2048
TASK_MODEL ?= phi3:3.8b
TASK_MODEL_CONTEXT_LENGTH ?= 2048
TASK_PROVIDER ?= ollama
CHAT_MODEL ?= phi3:3.8b
CHAT_PROVIDER ?= ollama
CHAT_MODEL_CONTEXT_LENGTH ?= 2048
TENANCY ?= 54882f1d-3788-44f9-aed6-19a793c4568f
OLLAMA_HOST ?= 127.0.0.1:11434

export EMBED_MODEL EMBED_PROVIDER EMBED_MODEL_CONTEXT_LENGTH
export TASK_MODEL TASK_MODEL_CONTEXT_LENGTH TASK_PROVIDER
export CHAT_MODEL CHAT_MODEL_CONTEXT_LENGTH CHAT_PROVIDER
export TENANCY
export OLLAMA_HOST

AIR ?= $(shell command -v air 2>/dev/null || echo "$(shell go env GOPATH)/bin/air")
DEV_CONTENOX_BIN := $(HOME)/.local/bin/contenox

.PHONY: help \
	build-contenox \
	clean \
	deps-go-watch deps-ui \
	dev-install dev-link dev-unlink \
	dev-go-watch \
	test test-unit test-system test-api test-contenox-verbose test-contenox-help test-ui \
	build-ui verify-ui-embed

# -----------------------------------------------------------------------------
help:
	@echo "build-*    build-contenox build-ui"
	@echo "test-*     test test-unit test-system test-api test-contenox-verbose test-contenox-help test-ui"
	@echo "dev-*      dev-install dev-link dev-unlink dev-go-watch"
	@echo "deps-*     deps-go-watch deps-ui"
	@echo "verify-*   verify-ui-embed"
	@echo "Version (maintainers): make -f Makefile.version help"
	@echo "clean"

# —— build ————————————————————————————————————————————————————————————————
# Contenox binary: CLI entrypoint (cmd/contenox).
build-contenox:
	CGO_ENABLED=1 go build -o $(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/cmd/contenox

build-ui: deps-ui
	cd $(PROJECT_ROOT)/packages/ui && npm run build
	cd $(PROJECT_ROOT)/packages/beam && npm run build

# —— test ————————————————————————————————————————————————————————————————
test:
	GOMAXPROCS=1 go test -C $(PROJECT_ROOT) ./...

test-unit:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -short -timeout 15m -run '^TestUnit_' ./...

test-system:
	GOMAXPROCS=1 go test -C $(PROJECT_ROOT) -run '^TestSystem_' ./...

test-api: build-contenox
	@CONTENOX_BIN=$(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/scripts/run_apitests.sh $(PYTEST_ARGS)

test-contenox-verbose:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -v ./runtime/contenoxcli/...

test-contenox-help: build-contenox
	@chmod +x $(PROJECT_ROOT)/scripts/verify_cli_help.sh
	@CONTENOX_BIN=$(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/scripts/verify_cli_help.sh

test-ui: deps-ui
	cd $(PROJECT_ROOT)/packages/beam && npm test

verify-ui-embed:
	@test -f "$(PROJECT_ROOT)/runtime/internal/web/beam/dist/index.html" || { echo "missing Beam dist; run: make build-ui"; exit 1; }
	go test -C $(PROJECT_ROOT) ./runtime/internal/web

# —— dev —————————————————————————————————————————————————————————————————
dev-install: build-contenox dev-link

dev-link: build-contenox
	@mkdir -p $(dir $(DEV_CONTENOX_BIN))
	@ln -sf $(PROJECT_ROOT)/bin/contenox $(DEV_CONTENOX_BIN)
	@echo "Linked $(DEV_CONTENOX_BIN) -> $(PROJECT_ROOT)/bin/contenox"
	@echo "Use this binary: ensure $(dir $(DEV_CONTENOX_BIN)) is on PATH before other contenox installs (check: which contenox)"

dev-unlink:
	@rm -f $(DEV_CONTENOX_BIN)

dev-go-watch:
	@test -x "$(AIR)" || { echo "run: make deps-go-watch"; exit 1; }
	cd $(PROJECT_ROOT) && "$(AIR)" -c .air.toml

# —— deps ———————————————————————————————————————————————————————————————
deps-go-watch:
	go install github.com/air-verse/air@latest

deps-ui:
	cd $(PROJECT_ROOT)/packages/ui && npm ci
	cd $(PROJECT_ROOT)/packages/beam && npm ci

# —— clean ———————————————————————————————————————————————————————————————
clean:
	rm -rf $(PROJECT_ROOT)/bin
