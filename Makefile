PROJECT_ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
VERSION_FILE := $(PROJECT_ROOT)/apiframework/version.txt

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
OLLAMA_HOST ?= 172.17.0.1:11434

export EMBED_MODEL EMBED_PROVIDER EMBED_MODEL_CONTEXT_LENGTH
export TASK_MODEL TASK_MODEL_CONTEXT_LENGTH TASK_PROVIDER
export CHAT_MODEL CHAT_MODEL_CONTEXT_LENGTH CHAT_PROVIDER
export TENANCY
export OLLAMA_HOST

# Allow user override of COMPOSE_CMD
COMPOSE_CMD ?= docker compose -f compose.yaml -f compose.local.yaml

.PHONY: docker-build-runtime docker-up-runtime docker-run-runtime \
        docker-down-runtime docker-clear-runtime docker-logs-runtime \
        build-contenox run-contenox \
        start-ollama-pull start-ollama ollama-status \
        test test-unit test-system test-contenoxcli \
        test-api test-api-full test-api-init wait-for-server \
        docs-gen docs-markdown docs-html \
        website-dev site-build website-install website-clean \
        set-version bump-major bump-minor bump-patch \
        commit-docs release enterprise-clean


# --------------------------------------------------------------------
# Docker Runtime lifecycle
# --------------------------------------------------------------------
docker-build-runtime:
	$(COMPOSE_CMD) build --build-arg TENANCY=$(TENANCY)

docker-up-runtime:
	$(COMPOSE_CMD) up -d

docker-run-runtime: docker-down-runtime docker-build-runtime docker-up-runtime

docker-down-runtime:
	@docker rm -f mcp-testserver-bg 2>/dev/null || true
	$(COMPOSE_CMD) down

docker-clear-runtime:
	$(COMPOSE_CMD) down --volumes --remove-orphans

docker-logs-runtime:
	$(COMPOSE_CMD) logs -f runtime-api

# --------------------------------------------------------------------
# Contenox CLI
# --------------------------------------------------------------------
build-contenox:
	go build \
		-ldflags="-X github.com/contenox/contenox/internal/contenoxcli.Version=$(shell cat $(VERSION_FILE))" \
		-o $(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/cmd/contenox

# Run the contenox binary (builds if needed). Example: make run-contenox ARGS="hello"
run-contenox: build-contenox
	$(PROJECT_ROOT)/bin/contenox $(ARGS)

# ── Local dev shorthand ──────────────────────────────────────────────────────
# Build and symlink ./bin/contenox → ~/.local/bin/contenox so the dev binary
# shadows any system-installed release binary automatically.
#
#   make dev          # build + link (idempotent)
#   make dev-unlink   # remove the symlink (restores system binary)
#
# Requires ~/.local/bin to be on PATH before any system prefix. Add to your
# shell profile if not already there:
#   export PATH="$$HOME/.local/bin:$$PATH"
DEV_BIN := $(HOME)/.local/bin/contenox

dev: build-contenox dev-link
	@echo "→ dev binary: $(PROJECT_ROOT)/bin/contenox"
	@echo "→ symlink:    $(DEV_BIN)"
	@echo "   Make sure ~/.local/bin appears before /usr/local/bin in PATH."

dev-link: build-contenox
	@mkdir -p $(dir $(DEV_BIN))
	@ln -sf $(PROJECT_ROOT)/bin/contenox $(DEV_BIN)
	@echo "Linked $(DEV_BIN) → $(PROJECT_ROOT)/bin/contenox"

dev-unlink:
	@rm -f $(DEV_BIN)
	@echo "Removed $(DEV_BIN)"


# --------------------------------------------------------------------
# MCP Test Server: stateful session fixture for CLI / API tests
# --------------------------------------------------------------------
build-mcp-testserver:
	go build -o $(PROJECT_ROOT)/bin/mcp-testserver $(PROJECT_ROOT)/cmd/mcp-testserver

docker-build-mcp-testserver:
	docker build -t contenox/mcp-testserver:local \
		-f $(PROJECT_ROOT)/cmd/mcp-testserver/Dockerfile \
		$(PROJECT_ROOT)

run-mcp-testserver: build-mcp-testserver
	$(PROJECT_ROOT)/bin/mcp-testserver

docker-run-mcp-testserver: docker-build-mcp-testserver
	docker run --rm -p 8090:8090 contenox/mcp-testserver:local

# Start MCP test server in the background (idempotent: kills previous instance first).
run-mcp-testserver-bg: build-mcp-testserver
	@pkill -f bin/mcp-testserver 2>/dev/null || true
	@sleep 1
	@$(PROJECT_ROOT)/bin/mcp-testserver &
	@sleep 1
	@curl -sf http://localhost:8090/health && echo " mcp-testserver ready" || (echo "ERROR: mcp-testserver failed to start"; exit 1)

# Start MCP test server in background via Docker (bypasses go-sdk localhost DNS-rebinding protection).
# The container binds on 0.0.0.0:8090 inside Docker, so the server-local addr is not loopback.
docker-run-mcp-testserver-bg: docker-build-mcp-testserver
	@docker rm -f mcp-testserver-bg 2>/dev/null || true
	@docker run --rm -d --name mcp-testserver-bg -p 8090:8090 contenox/mcp-testserver:local
	@sleep 2
	@curl -sf http://localhost:8090/health && echo " mcp-testserver (docker) ready" || (echo "ERROR: mcp-testserver container failed"; docker logs mcp-testserver-bg; exit 1)

# Run the full MCP session persistence test against the CLI binary.
# Verifies that all tool calls within one agentic loop share the same session_token.
test-mcp-session: build-contenox docker-run-mcp-testserver-bg
	@echo "=== MCP session persistence test ==="
	$(PROJECT_ROOT)/bin/contenox run \
		--chain $(PROJECT_ROOT)/.contenox/chain-mcp-session-test.json \
		"call session_set key=player value=Alice, then call session_dump. Confirm if the session_tokens match exactly."
	@echo "=== Expected: session_token identical across all tool calls ==="


# --------------------------------------------------------------------
# Ollama
# --------------------------------------------------------------------

# Check if Ollama is reachable
ollama-status:
	@echo "Checking Ollama status at $(OLLAMA_HOST)..."
	@curl -s -f http://$(OLLAMA_HOST)/api/tags > /dev/null || (echo "Error: Ollama server not responding at $(OLLAMA_HOST). Start it with 'ollama serve' or check OLLAMA_HOST." && exit 1)
	@echo "Ollama is reachable."

# Pull the Ollama model used by contenox-cli (default: phi3:3.8b).
start-ollama-pull: ollama-status
	OLLAMA_HOST=$(OLLAMA_HOST) ollama pull $(TASK_MODEL)

# Ensure Ollama is ready: check connection and pull the model.
start-ollama: start-ollama-pull
	@echo "Model $(TASK_MODEL) ready at $(OLLAMA_HOST)."


# --------------------------------------------------------------------
# Tests
# --------------------------------------------------------------------

test:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) ./...

test-unit:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -run '^TestUnit_' ./...

test-system:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -run '^TestSystem_' ./...

test-contenoxcli:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -v ./internal/contenoxcli/...


# --------------------------------------------------------------------
# API tests
# --------------------------------------------------------------------

APITEST_VENV := $(PROJECT_ROOT)/apitests/.venv
APITEST_ACTIVATE := $(APITEST_VENV)/bin/activate

test-api-init:
	test -d $(APITEST_VENV) || python3 -m venv $(APITEST_VENV)
	. $(APITEST_ACTIVATE) && pip install -r $(PROJECT_ROOT)/apitests/requirements.txt

wait-for-server:
	@echo "Waiting for server..."
	@until wget --spider -q http://localhost:8081/health; do \
		echo "Still waiting..."; sleep 2; \
	done

test-api: test-api-init wait-for-server
	. $(APITEST_ACTIVATE) && pytest $(PROJECT_ROOT)/apitests/$(TEST_FILE)

test-api-full: docker-run-runtime test-api


# --------------------------------------------------------------------
# Documentation & Versioning
# --------------------------------------------------------------------

docs-gen:
	go run $(PROJECT_ROOT)/tools/openapi-gen \
		--project="$(PROJECT_ROOT)" \
		--output="$(PROJECT_ROOT)/docs"

docs-markdown: docs-gen
	docker run --rm \
		-v $(PROJECT_ROOT)/docs:/local \
		node:24-alpine sh -c "\
			npm install -g widdershins@4 && \
			widdershins /local/openapi.json -o /local/api-reference.md \
			--summary --resolve --verbose \
		"

docs-html: docs-gen
	mkdir -p $(PROJECT_ROOT)/website/docs
	cp $(PROJECT_ROOT)/scripts/openapi-rapidoc.html $(PROJECT_ROOT)/website/docs/openapi.html
	cp $(PROJECT_ROOT)/docs/openapi.json $(PROJECT_ROOT)/website/docs/openapi.json
	cp $(PROJECT_ROOT)/docs/openapi.yaml $(PROJECT_ROOT)/website/docs/openapi.yaml

set-version:
	go run $(PROJECT_ROOT)/tools/version/main.go set

# Bump version and create release commit + tag. Then: git push && git push origin vX.Y.Z
bump-patch:
	go run $(PROJECT_ROOT)/tools/version/main.go bump patch

bump-minor:
	go run $(PROJECT_ROOT)/tools/version/main.go bump minor

bump-major:
	go run $(PROJECT_ROOT)/tools/version/main.go bump major

## Build the Next.js site (enterprise/site) — primarily for local validation.
## The live site deploys automatically via CI on push to main.
site-build: website-install
	cd $(PROJECT_ROOT)/enterprise/site && npm run build

## Local dev server for the docs/marketing site (hot-reload)
website-dev: website-install
	cd $(PROJECT_ROOT)/enterprise/site && npm run dev

## Install npm deps for enterprise/site (idempotent)
website-install:
	cd $(PROJECT_ROOT)/enterprise/site && npm install

## Wipe the Next.js build cache
website-clean:
	rm -rf $(PROJECT_ROOT)/enterprise/site/.next

commit-docs: docs-markdown docs-html
	git add $(PROJECT_ROOT)/docs
	git commit -m "chore: update docs"

release: docs-markdown docs-html set-version
	@echo "Release assets prepared. Next.js site deploys via CI — no local build needed."
