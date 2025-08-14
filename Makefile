.PHONY: echo-version test-unit test-system test compose-wipe benchmark run build down logs test-api test-api-logs test-api-docker test-api-init wait-for-server docs-gen docs-markdown
PROJECT_ROOT := $(shell pwd)
DEFAULT_VERSION := $(shell git describe --tags --always --dirty)


echo-version:
	@echo $(DEFAULT_VERSION)

test-unit:
	GOMAXPROCS=4 go test -C ./ -run '^TestUnit_' ./...

test-system:
	GOMAXPROCS=4 go test -C ./ -run '^TestSystem_' ./...

test:
	GOMAXPROCS=4 go test -C ./ ./...

benchmark:
	go test -C ./core -bench=. -run=^$ -benchmem ./...

build:
	docker compose build --build-arg DEFAULT_ADMIN_USER=$(DEFAULT_ADMIN_USER) --build-arg VERSION=$(DEFAULT_VERSION)

down:
	docker compose down

run: down build
	docker compose up -d

logs: run
	docker compose logs -f runtime

compose-wipe:
	docker compose down --volumes --rmi all

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
	. apitests/.venv/bin/activate && pytest apitests/

test-api-logs: run wait-for-server
	. apitests/.venv/bin/activate && pytest --log-cli-level=DEBUG --capture=no -v apitests


test-api-docker:
	docker build -f Dockerfile.apitests -t contenox-apitests .
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
				--language_tabs 'python' \
				--summary \
				--resolve \
				--verbose"
	@echo "✅ Markdown documentation generated at $(PROJECT_ROOT)/docs/api-reference.md"
