PROJECT_ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
.DEFAULT_GOAL := help

# Optional local environment. This lets ignored repo-local `.env` files provide
# release-store settings such as MODELD_DEPS_S3_URI and AWS_REGION.
LOCAL_ENV_FILE := $(PROJECT_ROOT).env
ifneq (,$(wildcard $(LOCAL_ENV_FILE)))
include $(LOCAL_ENV_FILE)
export $(shell sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)[[:space:]]*=.*/\1/p' $(LOCAL_ENV_FILE))
endif

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

# Release packaging (see docs/blueprints/modeld-release-artifacts.md).
# MODELD_PLATFORM names the goos-goarch of this host's outputs.
# bundle-modeld-deps writes native dependency bundles under MODELD_DEPS_OUT; they
# are pushed to S3 (push-modeld-deps) and consumed by package-modeld-release via
# MODELD_DEPS_ROOT, which links modeld without rebuilding llama.cpp / OpenVINO.
MODELD_PLATFORM ?= $(shell go env GOOS)-$(shell go env GOARCH)
MODELD_DEPS_OUT ?= $(PROJECT_ROOT)bin/modeld-deps
MODELD_DEPS_ROOT ?=
MODELD_PULL_DIR ?= $(MODELD_DEPS_OUT)/pulled
# Artifact store URIs. s3:// uses the aws CLI; any other value is a local directory
# (so the whole push/pull/dedup flow is testable without aws). Dep bundles and final
# modeld packages both live in the store, not GitHub Releases.
MODELD_DEPS_S3_URI ?=
MODELD_RELEASE_S3_URI ?=
MODELD_STORE := bash $(PROJECT_ROOT)scripts/modeld-store.sh
MODELD_RELEASE_DIST_DIR ?= $(PROJECT_ROOT)dist
MODELD_RELEASE_NAME ?= modeld-$(MODELD_VERSION)-$(MODELD_PLATFORM)
MODELD_PROTOCOL_VERSION ?= $(shell sed -n 's/^const ProtocolVersion = //p' $(PROJECT_ROOT)/runtime/transport/protocol.go | head -1)
MODELD_MIN_PROTOCOL ?= $(shell sed -n 's/^const MinProtocol = //p' $(PROJECT_ROOT)/runtime/transport/protocol.go | head -1)
# Release requires OpenVINO by default; package-modeld-release hard-fails if the
# bundle lacks it. Set MODELD_RELEASE_OPENVINO=0 for llama-only platforms.
MODELD_RELEASE_OPENVINO ?= 1

# Fingerprint profile (modeld-deps-fingerprint). Defaults describe THIS host's
# variant; override to compute the fingerprint of a variant built on another device
# (e.g. a windows/darwin bundle this Linux box cannot build). Build-type/ABI mirror
# Makefile.llamacpp-direct.
MODELD_FP_OPENVINO_WAS_SET := $(filter-out undefined default,$(origin MODELD_FP_OPENVINO))
MODELD_FP_CUDA ?= $(if $(shell command -v nvcc 2>/dev/null),ON,OFF)
MODELD_FP_HIP ?= $(if $(shell command -v hipcc 2>/dev/null),ON,OFF)
# macOS is llama + Metal only; OpenVINO is never part of a darwin build, so its
# fingerprint always uses openvino=0 (matching the darwin producer and package).
MODELD_FP_OPENVINO ?= $(if $(filter darwin%,$(MODELD_PLATFORM)),0,$(if $(MODELD_HAVE_OPENVINO),1,0))
MODELD_FP_BUILD_TYPE ?= Release
MODELD_FP_RUNTIME_ABI ?= dl-v1

# Consumer/preflight profile. Defaults describe the bundle we want to consume, not
# what this checkout can build locally. That lets dev/release machines without
# OpenVINO installed still discover and pull the official OpenVINO bundle.
MODELD_EXPECT_CUDA ?= $(MODELD_FP_CUDA)
MODELD_EXPECT_HIP ?= $(MODELD_FP_HIP)
MODELD_EXPECT_BUILD_TYPE ?= $(MODELD_FP_BUILD_TYPE)
MODELD_EXPECT_RUNTIME_ABI ?= $(MODELD_FP_RUNTIME_ABI)
ifneq ($(MODELD_FP_OPENVINO_WAS_SET),)
MODELD_EXPECT_OPENVINO ?= $(MODELD_FP_OPENVINO)
else
MODELD_EXPECT_OPENVINO ?= $(if $(filter darwin%,$(MODELD_PLATFORM)),0,$(MODELD_RELEASE_OPENVINO))
endif

# modeld release version, stamped into `modeld version`. This is intentionally
# independent from runtime/version/version.txt, which belongs to the CLI and
# VS Code extension release cadence.
MODELD_VERSION ?= $(shell tr -d '\r\n' < $(PROJECT_ROOT)/cmd/modeld/version.txt 2>/dev/null)
# cmd/modeld is package main, so the linker binds -X against `main`, not the
# full import path (the import-path form is silently ignored for main packages).
MODELD_VERSION_LD_FLAGS = -X 'main.version=$(MODELD_VERSION)'
MODELD_LLAMA_LD_FLAGS = -X 'github.com/contenox/runtime/modeld/llama.llamaCPPCommit=$(LLAMA_CPP_COMMIT)'
MODELD_OPENVINO_LD_FLAGS = -X 'github.com/contenox/runtime/modeld/openvino.buildTokenizersPath=$(OPENVINO_TOKENIZERS_SO)' -X 'github.com/contenox/runtime/modeld/openvino.buildGenAIVersion=$(OPENVINO_GENAI_VERSION)'

# modeld always includes llama.cpp. OpenVINO is enabled when its SDK is present;
# CUDA support follows the llama.cpp runtime build. Set CONTENOX_MODELD_BACKEND
# at runtime to pin backend selection.
MODELD_HAVE_OPENVINO := $(shell test -n "$(strip $(OPENVINO_PKG))" && test -d "$(OPENVINO_GENAI_SRC)/src/cpp/include" && echo 1)

ifeq ($(MODELD_HAVE_OPENVINO),1)
MODELD_TAGS := llamanode llamacpp_direct openvino openvino_genai
MODELD_LD_FLAGS := $(MODELD_VERSION_LD_FLAGS) $(MODELD_LLAMA_LD_FLAGS) $(MODELD_OPENVINO_LD_FLAGS)
MODELD_OV_CXXFLAGS = $(OPENVINO_GENAI_CGO_CXXFLAGS)
MODELD_OV_DEV_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS) -Wl,-rpath,\$$ORIGIN/modeld-libs
MODELD_OV_PKG_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS)
else
MODELD_TAGS := llamanode llamacpp_direct
MODELD_LD_FLAGS := $(MODELD_VERSION_LD_FLAGS) $(MODELD_LLAMA_LD_FLAGS)
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
UI_DIR := $(PROJECT_ROOT)/packages/ui
BEAM_DIR := $(PROJECT_ROOT)/packages/beam
BEAM_DEV_HOST ?= 127.0.0.1
BEAM_DEV_PORT ?= 5173
BEAM_DEV_PROXY_URL ?= http://127.0.0.1:$(BEAM_DEV_PORT)
CONTENOX_DEV_ADDR ?= 127.0.0.1
CONTENOX_DEV_PORT ?= 32123
CONTENOX_DEV_URL ?= http://$(CONTENOX_DEV_ADDR):$(CONTENOX_DEV_PORT)

.PHONY: help \
	openapi \
	build-contenox build-contenox-windows build-ui build-llamacpp-runtime build-modeld bundle-modeld-libs bundle-llama-libs package-modeld package-modeld-prebuilt build-vscode package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev \
	bundle-modeld-deps bundle-modeld-deps-linux bundle-modeld-deps-darwin bundle-modeld-deps-windows \
	push-modeld-deps pull-modeld-deps push-modeld-release push-modeld-index modeld-release-metadata modeld-deps-fingerprint modeld-deps-profile modeld-deps-pull-dir check-modeld-deps-store check-modeld-deps-bundle deps-modeld-prebuilt \
	package-modeld-release package-modeld-release-linux package-modeld-release-darwin package-modeld-release-windows \
	check-modeld-llama-deps \
	clean clean-vscode \
	deps-modeld deps-llamacpp-ref deps-openvino deps-ui deps-vscode \
	dev-beam dev-web-proxy dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink vscode-dev-install \
	run-modeld \
	test test-unit test-api test-ui test-llamacpp-direct test-vllm test-system test-contenox-verbose test-contenox-help \
	verify-ui-embed

help:
	@echo "build-*    build-contenox build-contenox-windows build-ui build-llamacpp-runtime build-modeld build-vscode"
	@echo "package-*  package-modeld package-modeld-prebuilt package-modeld-release package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev"
	@echo "release-*  bundle-modeld-deps[-linux|-darwin|-windows] push/pull-modeld-deps package-modeld-release[-<os>] modeld-release-metadata push-modeld-release push-modeld-index"
	@echo "           (devices publish native dep bundles; release assembly later pulls a bundle and packages modeld; see docs/modeld-release-runbook.md)"
	@echo "test-*     test test-unit test-api test-ui test-llamacpp-direct test-vllm test-system test-contenox-verbose test-contenox-help"
	@echo "dev-*      dev-beam dev-web-proxy dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink run-modeld"
	@echo "           (modeld includes llama.cpp, adds OpenVINO/CUDA when available, and selects backend at runtime)"
	@echo "deps-*     deps-modeld deps-modeld-prebuilt deps-llamacpp-ref deps-openvino deps-ui deps-vscode"
	@echo "           (deps-modeld-prebuilt checks/pulls the expected native dep bundle from the store)"
	@echo "verify-*   verify-ui-embed"
	@echo "Version (maintainers): make -f Makefile.version help"
	@echo "clean"

# Regenerate the OpenAPI spec (runtime/internal/openapidocs/openapi.json) from
# the route annotations. Run after changing any HTTP route or its @request/
# @response/@param annotations; the result is embedded into the binary.
openapi:
	go run $(PROJECT_ROOT)/tools/openapi-gen

# build
# Build the pure-Go CLI. Native inference is handled by modeld.
build-contenox: verify-ui-embed
	CGO_ENABLED=0 go build -o $(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/cmd/contenox

build-contenox-windows: verify-ui-embed
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(PROJECT_ROOT)/bin/contenox-windows-amd64.exe $(PROJECT_ROOT)/cmd/contenox

build-ui: deps-ui
	cd $(UI_DIR) && npm run build
	cd $(BEAM_DIR) && npm run build

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

# Produce a native dependency bundle from this host's build outputs. The bundle is a
# build input for package-modeld-release, not a user-facing package. A device builds
# whatever variant it can compile (CPU / CUDA / HIP / Metal, with or without OpenVINO);
# the bundle name and manifest record the accelerator profile. There is one producer
# per OS (scripts/modeld-deps-bundle-<os>.sh) since the native library names differ;
# run the one matching the build device. Run `make deps-modeld` first for OpenVINO.
MODELD_DEPS_BUNDLE_ENV = PLATFORM="$(MODELD_PLATFORM)" OUT="$(MODELD_DEPS_OUT)" \
	LLAMA_REF="$(LLAMA_CPP_REF_DIR)" LLAMA_RUNTIME="$(LLAMA_RUNTIME_DIR)" \
	LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" OPENVINO_PKG="$(OPENVINO_PKG)" \
	GENAI_SRC="$(OPENVINO_GENAI_SRC)" GENAI_PKG="$(OPENVINO_GENAI_PKG)" \
	TOKENIZERS_LIB="$(OPENVINO_TOKENIZERS_LIB)" OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)"

# Dispatch to the producer for the host OS.
bundle-modeld-deps:
	@$(MAKE) --no-print-directory bundle-modeld-deps-$$(go env GOOS)

# Linux builds the runtime first as a convenience; darwin/windows producers consume a
# runtime the device already built (their scripts validate the inputs and fail clearly).
bundle-modeld-deps-linux: build-llamacpp-runtime check-modeld-llama-deps
	@$(MODELD_DEPS_BUNDLE_ENV) bash $(PROJECT_ROOT)scripts/modeld-deps-bundle-linux.sh

bundle-modeld-deps-darwin:
	@$(MODELD_DEPS_BUNDLE_ENV) bash $(PROJECT_ROOT)scripts/modeld-deps-bundle-darwin.sh

bundle-modeld-deps-windows:
	@$(MODELD_DEPS_BUNDLE_ENV) bash $(PROJECT_ROOT)scripts/modeld-deps-bundle-windows.sh

# Print the fingerprint of a bundle's build inputs (pins). Pin-only, so it needs no
# built artifacts. Producer checks can override MODELD_PLATFORM/MODELD_FP_*;
# consumer preflight/pull targets use MODELD_PLATFORM/MODELD_EXPECT_*.
modeld-deps-fingerprint:
	@PLATFORM="$(MODELD_PLATFORM)" \
	LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
	LLAMA_BUILD_TYPE="$(MODELD_FP_BUILD_TYPE)" \
	LLAMA_RUNTIME_ABI="$(MODELD_FP_RUNTIME_ABI)" \
	CUDA="$(MODELD_FP_CUDA)" HIP="$(MODELD_FP_HIP)" \
	OPENVINO="$(MODELD_FP_OPENVINO)" \
	OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
	bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh

# Print the consumer-side dependency profile and fingerprint. This is the pre-build
# answer to "which native dep bundle do I need?" and does not require built deps.
modeld-deps-profile:
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_EXPECT_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_EXPECT_RUNTIME_ABI)" \
		CUDA="$(MODELD_EXPECT_CUDA)" HIP="$(MODELD_EXPECT_HIP)" OPENVINO="$(MODELD_EXPECT_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	echo "platform=$(MODELD_PLATFORM)"; \
	echo "fingerprint=$$fp"; \
	echo "llama_cpp_commit=$(LLAMA_CPP_COMMIT)"; \
	echo "llama_build_type=$(MODELD_EXPECT_BUILD_TYPE)"; \
	echo "llama_runtime_abi=$(MODELD_EXPECT_RUNTIME_ABI)"; \
	echo "cuda=$(MODELD_EXPECT_CUDA)"; \
	echo "hip=$(MODELD_EXPECT_HIP)"; \
	echo "openvino=$(MODELD_EXPECT_OPENVINO)"; \
	echo "openvino_genai_version=$(OPENVINO_GENAI_VERSION)"; \
	echo "pull_dir=$(MODELD_PULL_DIR)/$(MODELD_PLATFORM)-$$fp"; \
	if [ -n "$(MODELD_DEPS_S3_URI)" ]; then \
		echo "manifest=$(MODELD_DEPS_S3_URI)/$(MODELD_PLATFORM)/$$fp/manifest.json"; \
	else \
		echo "manifest=(set MODELD_DEPS_S3_URI to check a store)"; \
	fi

modeld-deps-pull-dir:
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_EXPECT_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_EXPECT_RUNTIME_ABI)" \
		CUDA="$(MODELD_EXPECT_CUDA)" HIP="$(MODELD_EXPECT_HIP)" OPENVINO="$(MODELD_EXPECT_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	echo "$(MODELD_PULL_DIR)/$(MODELD_PLATFORM)-$$fp"

# Preflight the store before building native deps. Exit 0 only when the exact
# expected platform/fingerprint manifest exists.
check-modeld-deps-store:
	@test -n "$(MODELD_DEPS_S3_URI)" || { echo "set MODELD_DEPS_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_EXPECT_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_EXPECT_RUNTIME_ABI)" \
		CUDA="$(MODELD_EXPECT_CUDA)" HIP="$(MODELD_EXPECT_HIP)" OPENVINO="$(MODELD_EXPECT_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	key="$(MODELD_DEPS_S3_URI)/$(MODELD_PLATFORM)/$$fp"; \
	echo "checking modeld deps: platform=$(MODELD_PLATFORM) fingerprint=$$fp"; \
	echo "profile: llama=$(LLAMA_CPP_COMMIT) build=$(MODELD_EXPECT_BUILD_TYPE) abi=$(MODELD_EXPECT_RUNTIME_ABI) cuda=$(MODELD_EXPECT_CUDA) hip=$(MODELD_EXPECT_HIP) openvino=$(MODELD_EXPECT_OPENVINO) genai=$(OPENVINO_GENAI_VERSION)"; \
	if $(MODELD_STORE) exists "$$key/manifest.json"; then \
		echo "available: $$key/manifest.json"; \
		echo "pull: make pull-modeld-deps"; \
	else \
		echo "missing: $$key/manifest.json"; \
		echo "build/push on a capable producer device: make deps-modeld bundle-modeld-deps push-modeld-deps"; \
		exit 1; \
	fi

# Upload native dependency bundles to the store as plain files (no archive), keyed by
# platform/fingerprint. Each device pushes the variants it can build; the union in the
# store lets the release assemble platforms no single device can build (windows/darwin).
# A fingerprint already present is skipped, so we never re-upload a version we have.
push-modeld-deps:
	@test -n "$(MODELD_DEPS_S3_URI)" || { echo "set MODELD_DEPS_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@found=0; \
	for envf in $(MODELD_DEPS_OUT)/*/bundle.env; do \
		[ -f "$$envf" ] || continue; found=1; \
		bdir=$$(dirname "$$envf"); \
		( . "$$envf"; \
		  key="$(MODELD_DEPS_S3_URI)/$$MODELD_BUNDLE_PLATFORM/$$MODELD_BUNDLE_FINGERPRINT"; \
		  if $(MODELD_STORE) exists "$$key/manifest.json"; then \
		    echo "skip (already in store): $$MODELD_BUNDLE_PLATFORM/$$MODELD_BUNDLE_FINGERPRINT"; \
		  else \
		    echo "put $$bdir -> $$key/"; \
		    $(MODELD_STORE) put "$$bdir" "$$key"; \
		  fi ) || exit 1; \
	done; \
	[ "$$found" = 1 ] || { echo "no bundles in $(MODELD_DEPS_OUT); run: make bundle-modeld-deps"; exit 1; }

# Fetch a native dependency bundle from the store into a local dir for packaging. The
# fingerprint is computed from the expected consumer profile, so override
# MODELD_PLATFORM/MODELD_EXPECT_* to pull a different platform or variant.
pull-modeld-deps:
	@test -n "$(MODELD_DEPS_S3_URI)" || { echo "set MODELD_DEPS_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_EXPECT_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_EXPECT_RUNTIME_ABI)" \
		CUDA="$(MODELD_EXPECT_CUDA)" HIP="$(MODELD_EXPECT_HIP)" OPENVINO="$(MODELD_EXPECT_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	key="$(MODELD_DEPS_S3_URI)/$(MODELD_PLATFORM)/$$fp"; \
	dest="$(MODELD_PULL_DIR)/$(MODELD_PLATFORM)-$$fp"; \
	$(MODELD_STORE) exists "$$key/manifest.json" || { echo "variant not in store: $$key — build+push it on a $(MODELD_PLATFORM) device first"; exit 1; }; \
	$(MODELD_STORE) get "$$key" "$$dest"; \
	echo "pulled $(MODELD_PLATFORM) ($$fp) -> $$dest"; \
	echo "next: make package-modeld-release MODELD_DEPS_ROOT=$$dest"

# Dev/release consumer convenience: preflight, pull, and validate the expected
# prebuilt native dependency bundle without building llama.cpp/OpenVINO locally.
deps-modeld-prebuilt:
	@test -n "$(MODELD_DEPS_S3_URI)" || { echo "set MODELD_DEPS_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_EXPECT_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_EXPECT_RUNTIME_ABI)" \
		CUDA="$(MODELD_EXPECT_CUDA)" HIP="$(MODELD_EXPECT_HIP)" OPENVINO="$(MODELD_EXPECT_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	key="$(MODELD_DEPS_S3_URI)/$(MODELD_PLATFORM)/$$fp"; \
	dest="$(MODELD_PULL_DIR)/$(MODELD_PLATFORM)-$$fp"; \
	echo "checking modeld deps: platform=$(MODELD_PLATFORM) fingerprint=$$fp"; \
	$(MODELD_STORE) exists "$$key/manifest.json" || { echo "prebuilt modeld deps missing: $$key/manifest.json"; echo "build/push on a capable producer device: make deps-modeld bundle-modeld-deps push-modeld-deps"; exit 1; }; \
	$(MODELD_STORE) get "$$key" "$$dest"; \
	$(MAKE) --no-print-directory check-modeld-deps-bundle MODELD_DEPS_ROOT="$$dest" MODELD_RELEASE_OPENVINO="$(MODELD_EXPECT_OPENVINO)"; \
	echo "prebuilt modeld deps ready: MODELD_DEPS_ROOT=$$dest"

# Local/dev packaging path that consumes the prebuilt native dependency bundle
# instead of building heavy C/C++ dependencies on this machine. It does not upload.
package-modeld-prebuilt:
	@test -n "$(MODELD_DEPS_S3_URI)" || { echo "set MODELD_DEPS_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_EXPECT_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_EXPECT_RUNTIME_ABI)" \
		CUDA="$(MODELD_EXPECT_CUDA)" HIP="$(MODELD_EXPECT_HIP)" OPENVINO="$(MODELD_EXPECT_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	dest="$(MODELD_PULL_DIR)/$(MODELD_PLATFORM)-$$fp"; \
	$(MAKE) --no-print-directory deps-modeld-prebuilt; \
	$(MAKE) --no-print-directory package-modeld-release MODELD_DEPS_ROOT="$$dest" MODELD_RELEASE_OPENVINO="$(MODELD_EXPECT_OPENVINO)"

# Upload final modeld packages to the store, keyed by version. Final binaries live in
# the store (S3), not GitHub Releases.
push-modeld-release: modeld-release-metadata
	@test -n "$(MODELD_RELEASE_S3_URI)" || { echo "set MODELD_RELEASE_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@set -- $(MODELD_RELEASE_DIST_DIR)/modeld-$(MODELD_VERSION)-*.tar.gz $(MODELD_RELEASE_DIST_DIR)/modeld-$(MODELD_VERSION)-*.zip; \
	found=0; \
	for f do \
		[ -f "$$f" ] || continue; found=1; \
		[ -f "$$f.sha256" ] || { echo "missing checksum for $$f; run: make package-modeld-release"; exit 1; }; \
		[ -f "$$f.build.json" ] || { echo "missing release metadata for $$f; run: make package-modeld-release with the updated packaging scripts"; exit 1; }; \
	done; \
	[ "$$found" = 1 ] || { echo "no modeld $(MODELD_VERSION) packages in $(MODELD_RELEASE_DIST_DIR); run: make package-modeld-release"; exit 1; }; \
	for f do \
		[ -f "$$f" ] || continue; \
		base=$$(basename "$$f"); dest="$(MODELD_RELEASE_S3_URI)/$(MODELD_VERSION)"; \
		echo "cp $$f -> $$dest/$$base"; \
		$(MODELD_STORE) cp "$$f" "$$dest/$$base" || exit 1; \
		$(MODELD_STORE) cp "$$f.sha256" "$$dest/$$base.sha256" || exit 1; \
		$(MODELD_STORE) cp "$$f.build.json" "$$dest/$$base.build.json" || exit 1; \
	done; \
	$(MAKE) --no-print-directory push-modeld-index

push-modeld-index:
	@test -n "$(MODELD_RELEASE_S3_URI)" || { echo "set MODELD_RELEASE_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@bash $(PROJECT_ROOT)scripts/modeld-index-refresh.sh "$(MODELD_RELEASE_S3_URI)"

modeld-release-metadata:
	@set -- $(MODELD_RELEASE_DIST_DIR)/modeld-$(MODELD_VERSION)-*.tar.gz $(MODELD_RELEASE_DIST_DIR)/modeld-$(MODELD_VERSION)-*.zip; \
	found=0; \
	for f do [ ! -f "$$f" ] || found=1; done; \
	[ "$$found" = 1 ] || { echo "no modeld $(MODELD_VERSION) packages in $(MODELD_RELEASE_DIST_DIR); run: make package-modeld-release"; exit 1; }; \
	MODELD_RELEASE_PROTOCOL="$(MODELD_MIN_PROTOCOL)" bash $(PROJECT_ROOT)scripts/modeld-release-metadata.sh "$$@"

# Validate that an extracted dependency bundle has everything the release link needs.
# Hard-fails when OpenVINO is required but the bundle does not declare/contain it, so
# a release can never silently fall back to a reduced backend set.
check-modeld-deps-bundle:
	@test -n "$(MODELD_DEPS_ROOT)" || { echo "set MODELD_DEPS_ROOT=/path/to/modeld-deps-<platform>"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/manifest.json" || { echo "bundle missing manifest.json: $(MODELD_DEPS_ROOT)"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/llama/ref/common/chat.h" || { echo "bundle missing llama.cpp ref headers"; exit 1; }
	@ls "$(MODELD_DEPS_ROOT)"/llama/runtime/lib/libllama.* >/dev/null 2>&1 || ls "$(MODELD_DEPS_ROOT)"/llama/runtime/lib/llama.dll >/dev/null 2>&1 || { echo "bundle missing llama runtime lib (libllama.{so,dylib,dll})"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/llama/runtime/lib/libcommon.a" || { echo "bundle missing llama libcommon.a"; exit 1; }
	@if [ "$(MODELD_RELEASE_OPENVINO)" = "1" ]; then \
		grep -q '"openvino": *true' "$(MODELD_DEPS_ROOT)/manifest.json" || { echo "MODELD_RELEASE_OPENVINO=1 but bundle manifest does not declare openvino:true (refusing to silently drop OpenVINO)"; exit 1; }; \
		test -d "$(MODELD_DEPS_ROOT)/openvino/genai/src/cpp/include" || { echo "bundle missing OpenVINO GenAI headers"; exit 1; }; \
		ls "$(MODELD_DEPS_ROOT)"/openvino/genai/*openvino_genai* >/dev/null 2>&1 || { echo "bundle missing openvino_genai lib"; exit 1; }; \
		ls "$(MODELD_DEPS_ROOT)"/openvino/tokenizers/lib/*openvino_tokenizers* >/dev/null 2>&1 || { echo "bundle missing openvino_tokenizers lib"; exit 1; }; \
		ls "$(MODELD_DEPS_ROOT)"/openvino/openvino/libs/*openvino.* >/dev/null 2>&1 || { echo "bundle missing openvino runtime lib"; exit 1; }; \
	fi
	@echo "modeld deps bundle OK: $(MODELD_DEPS_ROOT) (openvino required=$(MODELD_RELEASE_OPENVINO))"

# Package a release modeld bundle by linking against an extracted native dependency
# bundle (MODELD_DEPS_ROOT) instead of rebuilding native deps. There is one target per
# OS (package-modeld-release-<os>); package-modeld-release dispatches to the host OS.
# The root paths/tags are re-pointed at the bundle via target-specific variables shared
# by all OSes; only the link flags, wrapper, and archive differ per OS. Deterministic
# and self-contained: it does not rebuild llama.cpp/OpenVINO and refuses to drop a
# backend. Linux is verified end-to-end; the darwin/windows link flags follow platform
# convention (ld64 @loader_path / MinGW import libs + DLL-next-to-exe) — verify on a
# build host of that OS.
MODELD_RELEASE_OSES := linux darwin windows
MODELD_RELEASE_TARGETS := $(addprefix package-modeld-release-,$(MODELD_RELEASE_OSES))

# Per-OS pieces, selected in the recipe as $(VAR_$*).
MODELD_PKG_RPATH_linux   = -Wl,--disable-new-dtags -Wl,-rpath,\$$ORIGIN/lib/llamacpp -Wl,-rpath,\$$ORIGIN/modeld-libs -Wl,-rpath-link,$(LLAMA_RUNTIME_LIB_DIR)
MODELD_PKG_RPATH_darwin  = -Wl,-rpath,@loader_path/lib/llamacpp -Wl,-rpath,@loader_path/modeld-libs
MODELD_PKG_RPATH_windows =
MODELD_PKG_LLAMA_LIBS_linux   = $(LLAMA_DIRECT_LINK_LIBS)
MODELD_PKG_LLAMA_LIBS_darwin  = -lcommon -lllama -lggml -lggml-base -lstdc++
MODELD_PKG_LLAMA_LIBS_windows = -lcommon -lllama -lggml -lggml-base -lstdc++ -static-libgcc -static-libstdc++
MODELD_PKG_OV_LIBS_linux   = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),$(OPENVINO_GENAI_LINK_FLAGS),)
MODELD_PKG_OV_LIBS_darwin  = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),-L$(OPENVINO_PKG)/libs -L$(OPENVINO_GENAI_PKG) -L$(OPENVINO_TOKENIZERS_LIB) -lopenvino_genai -lopenvino -lstdc++,)
MODELD_PKG_OV_LIBS_windows = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),-L$(OPENVINO_PKG)/libs -L$(OPENVINO_GENAI_PKG) -L$(OPENVINO_TOKENIZERS_LIB) -lopenvino_genai -lopenvino,)
MODELD_PKG_BIN_linux   = modeld.bin
MODELD_PKG_BIN_darwin  = modeld.bin
MODELD_PKG_BIN_windows = modeld.exe
MODELD_PKG_LAUNCHER_linux   = modeld
MODELD_PKG_LAUNCHER_darwin  = modeld
MODELD_PKG_LAUNCHER_windows = modeld.cmd

# Dispatch to the packager for the host OS.
package-modeld-release:
	@$(MAKE) --no-print-directory package-modeld-release-$$(go env GOOS) MODELD_DEPS_ROOT="$(MODELD_DEPS_ROOT)"

# Bundle-path/tag overrides shared by every per-OS target.
$(MODELD_RELEASE_TARGETS): MODELD_DIST_DIR := $(MODELD_RELEASE_DIST_DIR)/$(MODELD_RELEASE_NAME)
$(MODELD_RELEASE_TARGETS): LLAMA_CPP_REF_DIR := $(MODELD_DEPS_ROOT)/llama/ref
$(MODELD_RELEASE_TARGETS): LLAMA_RUNTIME_DIR := $(MODELD_DEPS_ROOT)/llama/runtime
$(MODELD_RELEASE_TARGETS): LLAMA_RUNTIME_LIB_DIR := $(MODELD_DEPS_ROOT)/llama/runtime/lib
$(MODELD_RELEASE_TARGETS): OPENVINO_PKG := $(MODELD_DEPS_ROOT)/openvino/openvino
$(MODELD_RELEASE_TARGETS): OPENVINO_GENAI_SRC := $(MODELD_DEPS_ROOT)/openvino/genai
$(MODELD_RELEASE_TARGETS): OPENVINO_GENAI_PKG := $(MODELD_DEPS_ROOT)/openvino/genai
$(MODELD_RELEASE_TARGETS): OPENVINO_TOKENIZERS_LIB := $(MODELD_DEPS_ROOT)/openvino/tokenizers/lib
# Recursive (=) so they honor a per-target MODELD_RELEASE_OPENVINO (darwin sets 0).
$(MODELD_RELEASE_TARGETS): MODELD_TAGS = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),llamanode llamacpp_direct openvino openvino_genai,llamanode llamacpp_direct)
$(MODELD_RELEASE_TARGETS): MODELD_OV_CXXFLAGS = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),$(OPENVINO_GENAI_CGO_CXXFLAGS),)
$(MODELD_RELEASE_TARGETS): MODELD_LD_FLAGS = $(MODELD_VERSION_LD_FLAGS) $(MODELD_LLAMA_LD_FLAGS) $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),$(MODELD_OPENVINO_LD_FLAGS),)
# Apple Silicon is llama + Metal; OpenVINO GenAI is not supported there, so the darwin
# package never requires/links OpenVINO (override on the command line if that changes).
package-modeld-release-darwin: MODELD_RELEASE_OPENVINO := 0

$(MODELD_RELEASE_TARGETS): package-modeld-release-%: check-modeld-deps-bundle
	@rm -rf "$(MODELD_DIST_DIR)" && mkdir -p "$(MODELD_DIST_DIR)"
	@echo "packaging modeld release ($*): $(MODELD_RELEASE_NAME) tags=[$(MODELD_TAGS)] openvino=$(MODELD_RELEASE_OPENVINO)"
	CGO_ENABLED=1 \
	CGO_CPPFLAGS="$(LLAMA_COMMON_CPPFLAGS) $(LLAMA_DIRECT_CPPFLAGS)" \
	CGO_CXXFLAGS="$(MODELD_OV_CXXFLAGS)" \
	CGO_LDFLAGS="-L$(LLAMA_RUNTIME_LIB_DIR) $(MODELD_PKG_RPATH_$*) $(MODELD_PKG_LLAMA_LIBS_$*) $(MODELD_PKG_OV_LIBS_$*)" \
	go build -a -p $(MODELD_BUILD_JOBS) -tags '$(MODELD_TAGS)' \
		-ldflags "$(MODELD_LD_FLAGS)" \
		-o "$(MODELD_DIST_DIR)/$(MODELD_PKG_BIN_$*)" $(PROJECT_ROOT)/cmd/modeld
	@if [ "$*" = "windows" ]; then \
		{ printf '%s\r\n' '@echo off'; \
		  printf '%s\r\n' 'set "SELF=%~dp0"'; \
		  printf '%s\r\n' 'set "PATH=%SELF%lib\llamacpp;%SELF%modeld-libs;%PATH%"'; \
		  printf '%s\r\n' '"%SELF%modeld.exe" %*'; \
		} > "$(MODELD_DIST_DIR)/modeld.cmd"; \
	else \
		{ printf '%s\n' '#!/usr/bin/env sh'; \
		  printf '%s\n' 'set -eu'; \
		  printf '%s\n' 'SELF_DIR=$$(CDPATH= cd -- "$$(dirname -- "$$0")" && pwd)'; \
		  printf '%s\n' 'LIB_DIR="$$SELF_DIR/lib/llamacpp"'; \
		  printf '%s\n' 'if [ -d "$$LIB_DIR" ]; then'; \
		  printf '%s\n' '  export LD_LIBRARY_PATH="$$LIB_DIR$${LD_LIBRARY_PATH:+:$$LD_LIBRARY_PATH}"'; \
		  printf '%s\n' '  export DYLD_LIBRARY_PATH="$$LIB_DIR$${DYLD_LIBRARY_PATH:+:$$DYLD_LIBRARY_PATH}"'; \
		  printf '%s\n' '  export CONTENOX_LLAMA_BACKEND_DIR="$${CONTENOX_LLAMA_BACKEND_DIR:-$$LIB_DIR}"'; \
		  printf '%s\n' 'fi'; \
		  printf '%s\n' 'exec "$$SELF_DIR/$(MODELD_PKG_BIN_$*)" "$$@"'; \
		} > "$(MODELD_DIST_DIR)/modeld"; \
		chmod +x "$(MODELD_DIST_DIR)/modeld"; \
	fi
	@if [ "$(MODELD_RELEASE_OPENVINO)" = "1" ]; then $(MAKE) --no-print-directory \
		OPENVINO_PKG="$(OPENVINO_PKG)" OPENVINO_GENAI_PKG="$(OPENVINO_GENAI_PKG)" \
		OPENVINO_TOKENIZERS_SO="$(OPENVINO_TOKENIZERS_LIB)/libopenvino_tokenizers.so" \
		MODELD_LIBS_DIR="$(MODELD_DIST_DIR)/modeld-libs" MODELD_LIBS_COPY=1 bundle-modeld-libs; fi
	@$(MAKE) --no-print-directory LLAMA_RUNTIME_LIB_SRC="$(LLAMA_RUNTIME_LIB_DIR)" LLAMA_LIBS_DIR="$(MODELD_DIST_DIR)/lib/llamacpp" LLAMA_LIBS_COPY=1 bundle-llama-libs
	@if [ -d "$(MODELD_DEPS_ROOT)/licenses" ]; then rm -rf "$(MODELD_DIST_DIR)/LICENSES"; cp -a "$(MODELD_DEPS_ROOT)/licenses" "$(MODELD_DIST_DIR)/LICENSES"; fi
	@DIST_DIR="$(MODELD_DIST_DIR)" RELEASE_OUT="$(MODELD_RELEASE_DIST_DIR)" \
	NAME="$(MODELD_RELEASE_NAME)" VERSION="$(MODELD_VERSION)" PLATFORM="$(MODELD_PLATFORM)" \
	MIN_PROTOCOL="$(MODELD_MIN_PROTOCOL)" PROTOCOL_VERSION="$(MODELD_PROTOCOL_VERSION)" \
	EXPECT_OPENVINO="$(MODELD_RELEASE_OPENVINO)" TARGET_OS="$*" LAUNCHER="$(MODELD_PKG_LAUNCHER_$*)" \
	bash $(PROJECT_ROOT)scripts/modeld-package-release.sh

build-vscode: deps-vscode
	cd $(VSCODE_DIR) && npm run build

package-vscode: build-ui deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && npm run package
	@test -f "$(VSCODE_VSIX)" || { echo "expected VSIX was not created: $(VSCODE_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION).vsix"
	@echo "Built VS Code extension: $(VSCODE_VSIX)"

package-vscode-dev: build-ui deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && CONTENOX_VSCODE_SKIP_VSCE_SECRET_SCAN=1 npm run package
	@test -f "$(VSCODE_VSIX)" || { echo "expected VSIX was not created: $(VSCODE_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION).vsix"
	@echo "Built dev VS Code extension: $(VSCODE_VSIX)"

package-vscode-proposed: build-ui deps-vscode
	rm -rf $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/dist $(VSCODE_DIR)/bin
	cd $(VSCODE_DIR) && npm run package:proposed
	@test -f "$(VSCODE_PROPOSED_VSIX)" || { echo "expected proposed VSIX was not created: $(VSCODE_PROPOSED_VSIX)"; exit 1; }
	cd $(VSCODE_DIR) && CONTENOX_ALLOW_PROPOSED=1 npm run package:check -- "artifacts/contenox-runtime-$(VSCODE_TARGET)-$(VSCODE_VERSION)-proposed.vsix"
	@echo "Built proposed VS Code extension: $(VSCODE_PROPOSED_VSIX)"

package-vscode-proposed-dev: build-ui deps-vscode
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

test-api: build-ui
	$(MAKE) --no-print-directory build-contenox
	@CONTENOX_BIN=$(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/scripts/run_apitests.sh $(PYTEST_ARGS)

test-ui: deps-ui
	cd $(BEAM_DIR) && npm test

verify-ui-embed:
	@test -f "$(PROJECT_ROOT)/runtime/internal/web/beam/dist/index.html" || { echo "missing Beam dist; run: make build-ui"; exit 1; }
	go test -C $(PROJECT_ROOT) ./runtime/internal/web

test-contenox-verbose:
	GOMAXPROCS=4 go test -C $(PROJECT_ROOT) -v ./runtime/contenoxcli/...

test-contenox-help: build-contenox
	@chmod +x $(PROJECT_ROOT)/scripts/verify_cli_help.sh
	@CONTENOX_BIN=$(PROJECT_ROOT)/bin/contenox $(PROJECT_ROOT)/scripts/verify_cli_help.sh

# dev
dev-beam:
	@test -d "$(BEAM_DIR)/node_modules" || { echo "missing Beam node_modules; run: make deps-ui"; exit 1; }
	@set -eu; \
	cd "$(BEAM_DIR)" && VITE_DEV_API_PROXY=1 VITE_DEV_PROXY_TARGET="$(CONTENOX_DEV_URL)" npm run dev -- --host "$(BEAM_DEV_HOST)" --port "$(BEAM_DEV_PORT)" --strictPort & \
	vite_pid=$$!; \
	trap 'kill $$vite_pid 2>/dev/null || true; wait $$vite_pid 2>/dev/null || true' EXIT INT TERM; \
	sleep 1; \
	if ! kill -0 $$vite_pid 2>/dev/null; then wait $$vite_pid; exit $$?; fi; \
	echo "Beam dev UI: $(BEAM_DEV_PROXY_URL)"; \
	echo "contenox serve API/UI proxy: $(CONTENOX_DEV_URL)"; \
	BEAM_DEV_PROXY_URL="$(BEAM_DEV_PROXY_URL)" ADDR="$(CONTENOX_DEV_ADDR)" PORT="$(CONTENOX_DEV_PORT)" \
		go run $(PROJECT_ROOT)/cmd/contenox serve

dev-web-proxy: dev-beam

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

deps-ui:
	cd $(UI_DIR) && npm ci
	cd $(BEAM_DIR) && npm ci

deps-vscode:
	cd $(PROJECT_ROOT)/packages/vscode && npm ci

# clean
clean:
	rm -rf $(PROJECT_ROOT)/bin $(PROJECT_ROOT)/lib/llamacpp $(PROJECT_ROOT).llamacpp-runtime $(PROJECT_ROOT).build/llamacpp
	@rmdir $(PROJECT_ROOT)/lib 2>/dev/null || true

clean-vscode:
	rm -rf $(VSCODE_DIR)/bin $(VSCODE_DIR)/dist $(VSCODE_DIR)/artifacts $(VSCODE_DIR)/*.vsix
