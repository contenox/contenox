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
VSCODE_DIR := $(PROJECT_ROOT)/packages/vscode
VSCODE_VERSION := $(patsubst v%,%,$(shell tr -d '\r\n' < $(PROJECT_ROOT)/runtime/version/version.txt))
VSCODE_TARGET ?= $(shell cd $(VSCODE_DIR) && node -e "console.log(require('./scripts/vscode-targets').targetFromEnv().name)")
VSCODE_CLI ?= code
VSCODE_EXTENSIONS_DIR ?=
VSCODE_CLI_EXTENSION_ARGS = $(if $(strip $(VSCODE_EXTENSIONS_DIR)),--extensions-dir "$(VSCODE_EXTENSIONS_DIR)",)
VSCODE_DEFAULT_EXTENSIONS_DIR := $(if $(findstring insiders,$(notdir $(VSCODE_CLI))),$(HOME)/.vscode-insiders/extensions,$(HOME)/.vscode/extensions)
VSCODE_INSTALL_EXTENSIONS_DIR := $(if $(strip $(VSCODE_EXTENSIONS_DIR)),$(VSCODE_EXTENSIONS_DIR),$(VSCODE_DEFAULT_EXTENSIONS_DIR))
VSCODE_VSIX := $(VSCODE_DIR)/artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION).vsix
VSCODE_PROPOSED_VSIX := $(VSCODE_DIR)/artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION)-proposed.vsix

.PHONY: help \
	build-contenox build-contenox-windows build-vscode package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev deps-ollama-headers \
	clean clean-vscode \
	deps-go-watch deps-vscode \
	dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink vscode-dev-install \
	dev-go-watch \
	test test-unit test-system test-contenox-verbose test-contenox-help

# -----------------------------------------------------------------------------
help:
	@echo "build-*    build-contenox build-contenox-windows build-vscode"
	@echo "package-*  package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev"
	@echo "test-*     test test-unit test-system test-contenox-verbose test-contenox-help"
	@echo "dev-*      dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink dev-go-watch"
	@echo "deps-*     deps-go-watch deps-ollama-headers deps-vscode"
	@echo "Version (maintainers): make -f Makefile.version help"
	@echo "clean"

# —— build ————————————————————————————————————————————————————————————————
# Contenox binary: CLI entrypoint (cmd/contenox). Pure Go (CGO_ENABLED=0): the
# native inference backends live in the separate modeld binary, so the runtime
# cross-compiles with no C toolchain. See docs/blueprints/modeld-interface-boundary.md.
build-contenox:
	CGO_ENABLED=0 go build -o $(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/cmd/contenox

build-contenox-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(PROJECT_ROOT)/bin/contenox-windows-amd64.exe $(PROJECT_ROOT)/cmd/contenox

build-vscode: deps-vscode
	cd $(VSCODE_DIR) && npm run build

package-vscode: deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && npm run package
	@test -f "$(VSCODE_VSIX)" || { echo "expected VSIX was not created: $(VSCODE_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION).vsix"
	@echo "Built VS Code extension: $(VSCODE_VSIX)"

package-vscode-dev: deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && CONTENOX_VSCODE_SKIP_VSCE_SECRET_SCAN=1 npm run package
	@test -f "$(VSCODE_VSIX)" || { echo "expected VSIX was not created: $(VSCODE_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION).vsix"
	@echo "Built dev VS Code extension: $(VSCODE_VSIX)"

package-vscode-proposed: deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && npm run package:proposed
	@test -f "$(VSCODE_PROPOSED_VSIX)" || { echo "expected proposed VSIX was not created: $(VSCODE_PROPOSED_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && CONTENOX_ALLOW_PROPOSED=1 npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION)-proposed.vsix"
	@echo "Built proposed VS Code extension: $(VSCODE_PROPOSED_VSIX)"

package-vscode-proposed-dev: deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && CONTENOX_VSCODE_SKIP_VSCE_SECRET_SCAN=1 npm run package:proposed
	@test -f "$(VSCODE_PROPOSED_VSIX)" || { echo "expected proposed VSIX was not created: $(VSCODE_PROPOSED_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && CONTENOX_ALLOW_PROPOSED=1 npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION)-proposed.vsix"
	@echo "Built proposed dev VS Code extension: $(VSCODE_PROPOSED_VSIX)"

# —— test ————————————————————————————————————————————————————————————————
test:
	GOMAXPROCS=1 go test -C $(PROJECT_ROOT) ./...

test-unit:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -short -timeout 15m -run '^TestUnit_' ./...

test-system:
	GOMAXPROCS=1 go test -C $(PROJECT_ROOT) -run '^TestSystem_' ./...

test-contenox-verbose:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -v ./runtime/contenoxcli/...

test-contenox-help: build-contenox
	@chmod +x $(PROJECT_ROOT)/scripts/verify_cli_help.sh
	@CONTENOX_BIN=$(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/scripts/verify_cli_help.sh

# —— dev —————————————————————————————————————————————————————————————————
dev-install: build-contenox dev-link

dev-install-vscode: package-vscode-dev
	@command -v "$(VSCODE_CLI)" >/dev/null 2>&1 || { echo "missing VS Code CLI '$(VSCODE_CLI)' on PATH"; exit 1; }
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.contenox-runtime >/dev/null 2>&1 || true
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.runtime >/dev/null 2>&1 || true
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.contenox-vscode >/dev/null 2>&1 || true
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.contenox >/dev/null 2>&1 || true
	@EXT_ROOT="$(VSCODE_INSTALL_EXTENSIONS_DIR)"; rm -rf "$$EXT_ROOT"/contenox.contenox-runtime-* "$$EXT_ROOT"/contenox.runtime-*
	"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --install-extension "$(VSCODE_VSIX)" --force
	cd $(VSCODE_DIR) && VSCODE_CLI="$(VSCODE_CLI)" VSCODE_EXTENSIONS_DIR="$(VSCODE_EXTENSIONS_DIR)" node scripts/assert-installed-dev.js "$(VSCODE_VERSION)"
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --list-extensions --show-versions | grep -E '^contenox\.contenox-runtime@$(VSCODE_VERSION)$$'
	@echo "Installed Contenox VS Code extension from $(VSCODE_VSIX)"
	@echo "Reload Window required before VS Code uses the new extension code."
	@echo "Then run: Contenox: Show Runtime Info"

dev-install-vscode-proposed: package-vscode-proposed-dev
	@command -v "$(VSCODE_CLI)" >/dev/null 2>&1 || { echo "missing VS Code CLI '$(VSCODE_CLI)' on PATH"; exit 1; }
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.contenox-runtime >/dev/null 2>&1 || true
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.runtime >/dev/null 2>&1 || true
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.contenox-vscode >/dev/null 2>&1 || true
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --uninstall-extension contenox.contenox >/dev/null 2>&1 || true
	@EXT_ROOT="$(VSCODE_INSTALL_EXTENSIONS_DIR)"; rm -rf "$$EXT_ROOT"/contenox.contenox-runtime-* "$$EXT_ROOT"/contenox.runtime-*
	"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --install-extension "$(VSCODE_PROPOSED_VSIX)" --force
	cd $(VSCODE_DIR) && VSCODE_CLI="$(VSCODE_CLI)" VSCODE_EXTENSIONS_DIR="$(VSCODE_EXTENSIONS_DIR)" node scripts/assert-installed-dev.js "$(VSCODE_VERSION)"
	@"$(VSCODE_CLI)" $(VSCODE_CLI_EXTENSION_ARGS) --list-extensions --show-versions | grep -E '^contenox\.contenox-runtime@$(VSCODE_VERSION)$$'
	@echo "Installed proposed Contenox VS Code extension from $(VSCODE_PROPOSED_VSIX)"
	@echo "Reload Window required before VS Code uses the new extension code."
	@echo "Launch VS Code with proposed APIs enabled:"
	@echo "  $(VSCODE_CLI) --enable-proposed-api contenox.contenox-runtime $(PROJECT_ROOT)"
	@echo "Then run: Contenox: Show Runtime Info"

vscode-dev-install: dev-install-vscode

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

deps-ollama-headers:
	@$(PROJECT_ROOT)/scripts/prepare_ollama_llama_headers.sh

deps-vscode:
	cd $(PROJECT_ROOT)/packages/vscode && npm ci

# —— clean ———————————————————————————————————————————————————————————————
clean:
	rm -rf $(PROJECT_ROOT)/bin

clean-vscode:
	rm -rf $(VSCODE_DIR)/bin $(VSCODE_DIR)/dist $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/*.vsix
