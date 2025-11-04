.PHONY: build up run down restart logs ps clean test test-unit test-system compose-wipe \
        test-api-init wait-for-server test-api test-api-logs test-api-docker

# --------------------------------------
# Docker Compose
# --------------------------------------
COMPOSE_FILES := -f compose.yaml
ifneq ("$(wildcard compose.local.yaml)","")
  COMPOSE_FILES += -f compose.local.yaml
endif
COMPOSE_CMD := docker compose $(COMPOSE_FILES)

# --------------------------------------
# Default model configuration (override at call-time)
# --------------------------------------
EMBED_MODEL ?= nomic-embed-text:latest
EMBED_PROVIDER ?= ollama
EMBED_MODEL_CONTEXT_LENGTH ?= 2048

TASK_MODEL ?= phi3:3.8b
TASK_MODEL_CONTEXT_LENGTH ?= 2048
TASK_PROVIDER ?= ollama

CHAT_MODEL ?= phi3:3.8b
CHAT_MODEL_CONTEXT_LENGTH ?= 2048
CHAT_PROVIDER ?= ollama

TENANCY ?= 54882f1d-3788-44f9-aed6-19a793c4568f

export EMBED_MODEL EMBED_PROVIDER EMBED_MODEL_CONTEXT_LENGTH
export TASK_MODEL TASK_MODEL_CONTEXT_LENGTH TASK_PROVIDER
export CHAT_MODEL CHAT_MODEL_CONTEXT_LENGTH CHAT_PROVIDER
export TENANCY

# --------------------------------------
# Lifecycle
# --------------------------------------
build:
	$(COMPOSE_CMD) build --build-arg TENANCY=$(TENANCY)

up:
	$(COMPOSE_CMD) up -d

run: build up

down:
	$(COMPOSE_CMD) down --volumes --remove-orphans

restart:
	$(MAKE) down
	$(MAKE) run

logs:
	$(COMPOSE_CMD) logs -f

ps:
	$(COMPOSE_CMD) ps

clean:
	@rm -rf apitests/.venv || true

compose-wipe:
	$(COMPOSE_CMD) down --volumes --rmi all --remove-orphans

# --------------------------------------
# Go tests
# --------------------------------------
test-unit:
	GOMAXPROCS=4 go test -C ./ -run '^TestUnit_' ./...

test-system:
	GOMAXPROCS=4 go test -C ./ -run '^TestSystem_' ./...

test:
	GOMAXPROCS=4 go test -C ./ ./...

# --------------------------------------
# API tests (python)
# --------------------------------------
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

test-api: down up wait-for-server
	. apitests/.venv/bin/activate && pytest apitests/$(TEST_FILE)

test-api-logs: down up wait-for-server
	. apitests/.venv/bin/activate && pytest --log-cli-level=DEBUG --capture=no -v apitests/$(TEST_FILE)
