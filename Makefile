.PHONY: echo-version test-unit test-system test compose-wipe benchmark build down logs test-api test-api-logs test-api-docker test-api-init wait-for-server docs-gen docs-markdown clean set-version bump-major bump-minor bump-patch commit-docs

PROJECT_ROOT := $(shell pwd)
VERSION_FILE := internal/apiframework/version.txt

# Default model configuration - can be overridden when calling make
EMBED_MODEL ?= nomic-embed-text:latest
EMBED_PROVIDER ?= ollama
EMBED_MODEL_CONTEXT_LENGTH ?= 2048
TASK_MODEL ?= phi3:3.8b
TASK_MODEL_CONTEXT_LENGTH ?= 2048
TASK_MODEL_PROVIDER ?= ollama
TASK_PROVIDER ?= ollama
CHAT_MODEL ?= phi3:3.8b
CHAT_MODEL_CONTEXT_LENGTH ?= 2048
CHAT_PROVIDER ?= ollama

# Define the docker compose command with the local override file
# The environment variables will be automatically picked up by docker compose
COMPOSE_CMD := docker compose -f compose.yaml -f compose.local.yaml

validate-version:
	@if [ ! -f "$(VERSION_FILE)" ]; then \
		echo "ERROR: Version file $(VERSION_FILE) does not exist. Run 'make set-version' first."; \
		exit 1; \
	fi
	@VERSION_CONTENT=$$(cat $(VERSION_FILE) | tr -d '\n' | tr -d '\r'); \
	if [ -z "$$VERSION_CONTENT" ]; then \
		echo "ERROR: Version file $(VERSION_FILE) is empty. Run 'make set-version' first."; \
		exit 1; \
	fi

echo-version:
	@echo "Current version: $$(cat $(VERSION_FILE) 2>/dev/null || echo 'not set')"

clean:
	@rm -f $(VERSION_FILE) 2>/dev/null || true

build: set-version validate-version
	$(COMPOSE_CMD) build

down:
	$(COMPOSE_CMD) down --volumes --remove-orphans

run: down build
	# Start containers with model environment variables
	$(COMPOSE_CMD) up -d

logs: run
	$(COMPOSE_CMD) logs -f runtime

test-unit:
	GOMAXPROCS=4 go test -C ./ -run '^TestUnit_' ./...

test-system:
	GOMAXPROCS=4 go test -C ./ -run '^TestSystem_' ./...

test:
	GOMAXPROCS=4 go test -C ./ ./...

compose-wipe:
	$(COMPOSE_CMD) down --volumes --rmi all

test-api-init:
	python3 -m venv apitests/.venv
	. apitests/.venv/bin/activate && pip install -r apitests/requirements.txt

wait-for-server:
	@echo "Waiting for server to be ready..."
	@until wget --spider -q http://localhost:8081/health; do \
		echo "Server not yet available, waiting..."; \
		sleep 2; \
	done
	@echo "Server is up!"

test-api: run wait-for-server
	# Pass model variables to bootstrap script
	EMBED_MODEL=$(EMBED_MODEL) \
	TASK_MODEL=$(TASK_MODEL) \
	CHAT_MODEL=$(CHAT_MODEL) \
	. apitests/.venv/bin/activate && pytest apitests/

test-api-logs: run wait-for-server
	# Pass model variables to bootstrap script
	EMBED_MODEL=$(EMBED_MODEL) \
	TASK_MODEL=$(TASK_MODEL) \
	CHAT_MODEL=$(CHAT_MODEL) \
	. apitests/.venv/bin/activate && pytest --log-cli-level=DEBUG --capture=no -v apitests

test-api-docker:
	docker build -f Dockerfile.apitests --build-arg EMBED_MODEL=$(EMBED_MODEL) \
		--build-arg TASK_MODEL=$(TASK_MODEL) \
		--build-arg CHAT_MODEL=$(CHAT_MODEL) \
		-t contenox-apitests .
	docker run --rm --network=host contenox-apitests

docs-gen:
	@echo "📝 Generating OpenAPI spec..."
	@go run $(PROJECT_ROOT)/tools/openapi-gen --project="$(PROJECT_ROOT)" --output="$(PROJECT_ROOT)/docs"
	@echo "✅ OpenAPI spec generated at $(PROJECT_ROOT)/docs/openapi.json"

docs-markdown: docs-gen
	@echo "📝 Converting OpenAPI spec to Markdown..."
	@echo "🐳 Using Node.js Docker image to generate documentation..."
	@docker run --rm \
		-v $(PROJECT_ROOT)/docs:/local \
		node:18-alpine sh -c "\
			npm install -g widdershins@4 && \
			widdershins /local/openapi.json -o /local/api-reference.md \
				--language_tabs 'python:Python' \
				--summary \
				--resolve \
				--verbose"
	@echo "✅ Markdown documentation generated at $(PROJECT_ROOT)/docs/api-reference.md"

set-version: docs-markdown
	go run ./tools/version/main.go set

commit-docs: set-version
	@git add $(PROJECT_ROOT)/docs/
	@git commit -m "Update API reference"

bump-major:
	go run ./tools/version/main.go bump major

bump-minor:
	go run ./tools/version/main.go bump minor

bump-patch:
	go run ./tools/version/main.go bump patch
