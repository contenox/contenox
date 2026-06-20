PROJECT_ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
.DEFAULT_GOAL := help

# Shared native build flags for modeld and native backend tests.
include $(PROJECT_ROOT)mk/openvino-flags.mk
include $(PROJECT_ROOT)mk/llama-flags.mk

# Limit native C++ compile parallelism. Increase on hosts with enough RAM.
MODELD_BUILD_JOBS ?= 4
MODELD_SERVE_ARGS ?=

# llama.cpp runtime library source used by package targets.
# LLAMA_RUNTIME_SRC overrides the autodetected runtime directory.
LLAMA_RUNTIME_LIB_SRC ?= $(if $(strip $(LLAMA_RUNTIME_SRC)),$(LLAMA_RUNTIME_SRC),$(LLAMA_RUNTIME_LIB_DIR))
LLAMA_LIBS_DIR ?= $(PROJECT_ROOT)lib/llamacpp
LLAMA_LIBS_COPY ?=
MODELD_DIST_DIR ?= $(PROJECT_ROOT)bin/dist

MODELD_LLAMA_LD_FLAGS = -X 'github.com/contenox/runtime/modeld/llama.llamaCPPCommit=$(LLAMA_CPP_COMMIT)'
MODELD_OPENVINO_LD_FLAGS = -X 'github.com/contenox/runtime/modeld/openvino.buildTokenizersPath=$(OPENVINO_TOKENIZERS_SO)'

# modeld always includes llama.cpp. OpenVINO is enabled when its SDK is present;
# CUDA support follows the llama.cpp runtime build. Set CONTENOX_MODELD_BACKEND
# at runtime to pin backend selection.
MODELD_HAVE_OPENVINO := $(shell test -n "$(strip $(OPENVINO_PKG))" && test -d "$(OPENVINO_GENAI_SRC)/src/cpp/include" && echo 1)

ifeq ($(MODELD_HAVE_OPENVINO),1)
MODELD_TAGS := llamanode llamacpp_direct openvino openvino_genai
MODELD_LD_FLAGS := $(MODELD_LLAMA_LD_FLAGS) $(MODELD_OPENVINO_LD_FLAGS)
MODELD_OV_CXXFLAGS = $(OPENVINO_GENAI_CGO_CXXFLAGS)
MODELD_OV_DEV_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS) -Wl,-rpath,\$$ORIGIN/modeld-libs
MODELD_OV_PKG_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS)
else
MODELD_TAGS := llamanode llamacpp_direct
MODELD_LD_FLAGS := $(MODELD_LLAMA_LD_FLAGS)
MODELD_OV_CXXFLAGS =
MODELD_OV_DEV_LDFLAGS =
MODELD_OV_PKG_LDFLAGS =
endif

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
	build-contenox build-contenox-windows build-llamacpp-runtime build-modeld bundle-modeld-libs bundle-llama-libs package-modeld build-vscode package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev \
	check-modeld-llama-deps \
	clean clean-vscode \
	deps-modeld deps-llamacpp-ref deps-openvino deps-vscode \
	dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink vscode-dev-install \
	run-modeld \
	test test-unit test-llamacpp-direct test-vllm test-system test-contenox-verbose test-contenox-help

help:
	@echo "build-*    build-contenox build-contenox-windows build-llamacpp-runtime build-modeld build-vscode"
	@echo "package-*  package-modeld package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev"
	@echo "test-*     test test-unit test-llamacpp-direct test-vllm test-system test-contenox-verbose test-contenox-help"
	@echo "dev-*      dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink run-modeld"
	@echo "           (modeld includes llama.cpp, adds OpenVINO/CUDA when available, and selects backend at runtime)"
	@echo "deps-*     deps-modeld deps-llamacpp-ref deps-openvino deps-vscode"
	@echo "Version (maintainers): make -f Makefile.version help"
	@echo "clean"

# build
# Build the pure-Go CLI. Native inference is handled by modeld.
build-contenox:
	CGO_ENABLED=0 go build -o $(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/cmd/contenox

build-contenox-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(PROJECT_ROOT)/bin/contenox-windows-amd64.exe $(PROJECT_ROOT)/cmd/contenox

build-llamacpp-runtime:
	$(MAKE) -f $(PROJECT_ROOT)Makefile.llamacpp-direct runtime

check-modeld-llama-deps:
	@test -f "$(LLAMA_CPP_REF_DIR)/common/chat.h" || { echo "missing pinned llama.cpp common headers ($(LLAMA_CPP_REF_DIR)) — run: make deps-llamacpp-ref"; exit 1; }
	@test -f "$(LLAMA_RUNTIME_LIB_DIR)/libcommon.a" || { echo "missing direct llama.cpp common library ($(LLAMA_RUNTIME_LIB_DIR)/libcommon.a) — run: make build-llamacpp-runtime"; exit 1; }

# Build modeld with llama.cpp and, when available, OpenVINO GenAI.
# Run `make deps-modeld` before building with OpenVINO support.
build-modeld: build-llamacpp-runtime check-modeld-llama-deps
	@echo "building modeld: tags=[$(MODELD_TAGS)] openvino=$(if $(MODELD_HAVE_OPENVINO),yes,no)"
	CGO_ENABLED=1 \
	CGO_CPPFLAGS="$(LLAMA_COMMON_CPPFLAGS) $(LLAMA_DIRECT_CPPFLAGS)" \
	CGO_CXXFLAGS="$(MODELD_OV_CXXFLAGS)" \
	CGO_LDFLAGS="$(LLAMA_DIRECT_LDFLAGS) $(MODELD_OV_DEV_LDFLAGS)" \
	go build -a -p $(MODELD_BUILD_JOBS) -tags '$(MODELD_TAGS)' \
		-ldflags "$(MODELD_LD_FLAGS)" \
		-o $(PROJECT_ROOT)/bin/modeld $(PROJECT_ROOT)/cmd/modeld
	@if [ "$(MODELD_HAVE_OPENVINO)" = "1" ]; then $(MAKE) --no-print-directory MODELD_LIBS_DIR=$(PROJECT_ROOT)/bin/modeld-libs MODELD_LIBS_COPY= bundle-modeld-libs; fi

# Place OpenVINO runtime libraries next to modeld.
# MODELD_LIBS_COPY=1 copies files instead of symlinking them.
bundle-modeld-libs:
	@rm -rf "$(MODELD_LIBS_DIR)" && mkdir -p "$(MODELD_LIBS_DIR)"
	@if [ -n "$(MODELD_LIBS_COPY)" ]; then \
		cp -L $(OPENVINO_PKG)/libs/* "$(MODELD_LIBS_DIR)/"; \
		cp -L $(OPENVINO_GENAI_PKG)/libopenvino_genai.so.2620 "$(MODELD_LIBS_DIR)/"; \
		cp -L $(OPENVINO_TOKENIZERS_SO) "$(MODELD_LIBS_DIR)/"; \
		echo "bundled OpenVINO runtime (copies) -> $(MODELD_LIBS_DIR)"; \
	else \
		ln -sf $(OPENVINO_PKG)/libs/* "$(MODELD_LIBS_DIR)/"; \
		ln -sf $(OPENVINO_GENAI_PKG)/libopenvino_genai.so.2620 "$(MODELD_LIBS_DIR)/"; \
		ln -sf $(OPENVINO_TOKENIZERS_SO) "$(MODELD_LIBS_DIR)/"; \
		echo "bundled OpenVINO runtime (symlinks) -> $(MODELD_LIBS_DIR)"; \
	fi

# Place llama.cpp runtime libraries next to modeld.
bundle-llama-libs:
	@test -n "$(LLAMA_RUNTIME_LIB_SRC)" || { echo "missing direct llama.cpp runtime lib directory. Fetch ref code with: make deps-llamacpp-ref"; exit 1; }
	@test -d "$(LLAMA_RUNTIME_LIB_SRC)" || { echo "direct llama.cpp runtime lib directory does not exist: $(LLAMA_RUNTIME_LIB_SRC)"; exit 1; }
	@test -f "$(LLAMA_RUNTIME_LIB_SRC)/libllama.so" || { echo "direct llama.cpp runtime at $(LLAMA_RUNTIME_LIB_SRC) does not contain libllama.so"; exit 1; }
	@rm -rf "$(LLAMA_LIBS_DIR)" && mkdir -p "$(dir $(LLAMA_LIBS_DIR))"
	@if [ -n "$(LLAMA_LIBS_COPY)" ]; then \
		mkdir -p "$(LLAMA_LIBS_DIR)"; \
		cp -a "$(LLAMA_RUNTIME_LIB_SRC)"/. "$(LLAMA_LIBS_DIR)/"; \
		echo "bundled direct llama.cpp runtime (copies) -> $(LLAMA_LIBS_DIR)"; \
	else \
		ln -s "$(LLAMA_RUNTIME_LIB_SRC)" "$(LLAMA_LIBS_DIR)"; \
		echo "bundled direct llama.cpp runtime (symlink) -> $(LLAMA_LIBS_DIR)"; \
	fi

# Build a relocatable modeld bundle under MODELD_DIST_DIR.
# The bundle contains the wrapper, native binary, llama.cpp runtime libraries,
# and OpenVINO libraries when that backend is compiled in.
# CUDA hosts need libcudart.so.12 available to the bundled llama.cpp plugin.
package-modeld: build-llamacpp-runtime check-modeld-llama-deps
	@rm -rf "$(MODELD_DIST_DIR)" && mkdir -p "$(MODELD_DIST_DIR)"
	@echo "packaging modeld: tags=[$(MODELD_TAGS)] openvino=$(if $(MODELD_HAVE_OPENVINO),yes,no)"
	CGO_ENABLED=1 \
	CGO_CPPFLAGS="$(LLAMA_COMMON_CPPFLAGS) $(LLAMA_DIRECT_CPPFLAGS)" \
	CGO_CXXFLAGS="$(MODELD_OV_CXXFLAGS)" \
	CGO_LDFLAGS="-L$(LLAMA_RUNTIME_LIB_DIR) -Wl,--disable-new-dtags -Wl,-rpath,\$$ORIGIN/lib/llamacpp -Wl,-rpath,\$$ORIGIN/modeld-libs -Wl,-rpath-link,$(LLAMA_RUNTIME_LIB_DIR) $(LLAMA_DIRECT_LINK_LIBS) $(MODELD_OV_PKG_LDFLAGS)" \
	go build -a -p $(MODELD_BUILD_JOBS) -tags '$(MODELD_TAGS)' \
		-ldflags "$(MODELD_LD_FLAGS)" \
		-o "$(MODELD_DIST_DIR)/modeld.bin" $(PROJECT_ROOT)/cmd/modeld
	@{ \
		printf '%s\n' '#!/usr/bin/env sh'; \
		printf '%s\n' 'set -eu'; \
		printf '%s\n' 'SELF_DIR=$$(CDPATH= cd -- "$$(dirname -- "$$0")" && pwd)'; \
		printf '%s\n' 'LIB_DIR="$$SELF_DIR/lib/llamacpp"'; \
		printf '%s\n' 'if [ -d "$$LIB_DIR" ]; then'; \
		printf '%s\n' '  export LD_LIBRARY_PATH="$$LIB_DIR$${LD_LIBRARY_PATH:+:$$LD_LIBRARY_PATH}"'; \
		printf '%s\n' '  export CONTENOX_LLAMA_BACKEND_DIR="$${CONTENOX_LLAMA_BACKEND_DIR:-$$LIB_DIR}"'; \
		printf '%s\n' 'fi'; \
		printf '%s\n' 'exec "$$SELF_DIR/modeld.bin" "$$@"'; \
	} > "$(MODELD_DIST_DIR)/modeld"
	@chmod +x "$(MODELD_DIST_DIR)/modeld"
	@if [ "$(MODELD_HAVE_OPENVINO)" = "1" ]; then $(MAKE) --no-print-directory MODELD_LIBS_DIR="$(MODELD_DIST_DIR)/modeld-libs" MODELD_LIBS_COPY=1 bundle-modeld-libs; fi
	@$(MAKE) --no-print-directory LLAMA_LIBS_DIR="$(MODELD_DIST_DIR)/lib/llamacpp" LLAMA_LIBS_COPY=1 bundle-llama-libs
	@echo "relocatable modeld package -> $(MODELD_DIST_DIR) (run: $(MODELD_DIST_DIR)/modeld serve)"

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

# test
test:
	GOMAXPROCS=1 go test -C $(PROJECT_ROOT) ./...

test-unit:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -short -timeout 15m -run '^TestUnit_' ./...

test-llamacpp-direct:
	$(MAKE) -f $(PROJECT_ROOT)Makefile.llamacpp-direct test-shim

test-vllm:
	CONTENOX_RUN_VLLM_TESTS=1 GOMAXPROCS=1 go test -C $(PROJECT_ROOT) -run '^TestSystem_VLLM' ./runtime/modelrepo

test-system:
	GOMAXPROCS=1 go test -C $(PROJECT_ROOT) -run '^TestSystem_' ./...

test-contenox-verbose:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -v ./runtime/contenoxcli/...

test-contenox-help: build-contenox
	@chmod +x $(PROJECT_ROOT)/scripts/verify_cli_help.sh
	@CONTENOX_BIN=$(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/scripts/verify_cli_help.sh

# dev
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

# Run modeld from the local build output.
# Set CONTENOX_MODELD_BACKEND=llama|openvino to pin backend selection.
run-modeld: build-modeld
	CONTENOX_LLAMA_BACKEND_DIR=$(LLAMA_RUNTIME_LIB_DIR) \
	$(PROJECT_ROOT)/bin/modeld serve $(MODELD_SERVE_ARGS)

# deps
# Native backend dependencies for build-modeld.
deps-modeld: deps-llamacpp-ref deps-openvino

deps-llamacpp-ref:
	$(MAKE) -f $(PROJECT_ROOT)Makefile.llamacpp-direct deps-ref

deps-openvino:
	$(MAKE) -f $(PROJECT_ROOT)Makefile.openvino deps-genai genai-src

deps-vscode:
	cd $(PROJECT_ROOT)/packages/vscode && npm ci

# clean
clean:
	rm -rf $(PROJECT_ROOT)/bin $(PROJECT_ROOT)/lib/llamacpp $(PROJECT_ROOT).llamacpp-runtime $(PROJECT_ROOT).build/llamacpp
	@rmdir $(PROJECT_ROOT)/lib 2>/dev/null || true

clean-vscode:
	rm -rf $(VSCODE_DIR)/bin $(VSCODE_DIR)/dist $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/*.vsix
