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
# Release requires OpenVINO by default; package-modeld-release hard-fails if the
# bundle lacks it. Set MODELD_RELEASE_OPENVINO=0 for llama-only platforms.
MODELD_RELEASE_OPENVINO ?= 1

# Fingerprint profile (modeld-deps-fingerprint). Defaults describe THIS host's
# variant; override to compute the fingerprint of a variant built on another device
# (e.g. a windows/darwin bundle this Linux box cannot build) so the release can find
# and download it from S3. Build-type/ABI mirror Makefile.llamacpp-direct.
MODELD_FP_CUDA ?= $(if $(shell command -v nvcc 2>/dev/null),ON,OFF)
MODELD_FP_HIP ?= $(if $(shell command -v hipcc 2>/dev/null),ON,OFF)
MODELD_FP_OPENVINO ?= $(if $(MODELD_HAVE_OPENVINO),1,0)
MODELD_FP_BUILD_TYPE ?= Release
MODELD_FP_RUNTIME_ABI ?= dl-v1

# modeld release version, stamped into `modeld version`. Defaults to the tracked
# version file; release builds may override MODELD_VERSION with the tag.
MODELD_VERSION ?= $(shell tr -d '\r\n' < $(PROJECT_ROOT)/runtime/version/version.txt 2>/dev/null)
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

.PHONY: help \
	build-contenox build-contenox-windows build-llamacpp-runtime build-modeld bundle-modeld-libs bundle-llama-libs package-modeld build-vscode package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev \
	bundle-modeld-deps push-modeld-deps pull-modeld-deps push-modeld-release modeld-deps-fingerprint check-modeld-deps-bundle package-modeld-release \
	check-modeld-llama-deps \
	clean clean-vscode \
	deps-modeld deps-llamacpp-ref deps-openvino deps-vscode \
	dev-install dev-install-vscode dev-install-vscode-proposed dev-link dev-unlink vscode-dev-install \
	run-modeld \
	test test-unit test-llamacpp-direct test-vllm test-system test-contenox-verbose test-contenox-help

help:
	@echo "build-*    build-contenox build-contenox-windows build-llamacpp-runtime build-modeld build-vscode"
	@echo "package-*  package-modeld package-modeld-release package-vscode package-vscode-dev package-vscode-proposed package-vscode-proposed-dev"
	@echo "release-*  bundle-modeld-deps push/pull-modeld-deps package-modeld-release push-modeld-release (device -> S3 store -> linked package; see docs/blueprints/modeld-release-artifacts.md)"
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

# Produce a native dependency bundle from this host's build outputs. The bundle is a
# build input for package-modeld-release, not a user-facing package. A device builds
# whatever variant it can compile (CPU / CUDA / HIP, with or without OpenVINO); the
# bundle name and manifest record the accelerator profile. Run `make deps-modeld`
# first for an OpenVINO-capable bundle.
bundle-modeld-deps: build-llamacpp-runtime check-modeld-llama-deps
	@PLATFORM="$(MODELD_PLATFORM)" \
	OUT="$(MODELD_DEPS_OUT)" \
	LLAMA_REF="$(LLAMA_CPP_REF_DIR)" \
	LLAMA_RUNTIME="$(LLAMA_RUNTIME_DIR)" \
	LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
	OPENVINO_PKG="$(OPENVINO_PKG)" \
	GENAI_SRC="$(OPENVINO_GENAI_SRC)" \
	GENAI_PKG="$(OPENVINO_GENAI_PKG)" \
	TOKENIZERS_LIB="$(OPENVINO_TOKENIZERS_LIB)" \
	OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
	bash $(PROJECT_ROOT)scripts/modeld-deps-bundle.sh

# Print the fingerprint of a bundle's build inputs (pins). Pin-only, so it needs no
# built artifacts: a device can compute another platform's fingerprint (override
# MODELD_PLATFORM/MODELD_FP_*) to locate that variant on S3 without building it.
modeld-deps-fingerprint:
	@PLATFORM="$(MODELD_PLATFORM)" \
	LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
	LLAMA_BUILD_TYPE="$(MODELD_FP_BUILD_TYPE)" \
	LLAMA_RUNTIME_ABI="$(MODELD_FP_RUNTIME_ABI)" \
	CUDA="$(MODELD_FP_CUDA)" HIP="$(MODELD_FP_HIP)" \
	OPENVINO="$(MODELD_FP_OPENVINO)" \
	OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
	bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh

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
# fingerprint is computed from the pin profile, so override MODELD_PLATFORM/MODELD_FP_*
# to pull a variant this device cannot build (e.g. a windows/darwin bundle).
pull-modeld-deps:
	@test -n "$(MODELD_DEPS_S3_URI)" || { echo "set MODELD_DEPS_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@fp=$$(PLATFORM="$(MODELD_PLATFORM)" LLAMA_CPP_COMMIT="$(LLAMA_CPP_COMMIT)" \
		LLAMA_BUILD_TYPE="$(MODELD_FP_BUILD_TYPE)" LLAMA_RUNTIME_ABI="$(MODELD_FP_RUNTIME_ABI)" \
		CUDA="$(MODELD_FP_CUDA)" HIP="$(MODELD_FP_HIP)" OPENVINO="$(MODELD_FP_OPENVINO)" \
		OPENVINO_GENAI_VERSION="$(OPENVINO_GENAI_VERSION)" \
		bash $(PROJECT_ROOT)scripts/modeld-deps-fingerprint.sh); \
	key="$(MODELD_DEPS_S3_URI)/$(MODELD_PLATFORM)/$$fp"; \
	dest="$(MODELD_PULL_DIR)/$(MODELD_PLATFORM)-$$fp"; \
	$(MODELD_STORE) exists "$$key/manifest.json" || { echo "variant not in store: $$key — build+push it on a $(MODELD_PLATFORM) device first"; exit 1; }; \
	$(MODELD_STORE) get "$$key" "$$dest"; \
	echo "pulled $(MODELD_PLATFORM) ($$fp) -> $$dest"; \
	echo "next: make package-modeld-release MODELD_DEPS_ROOT=$$dest"

# Upload final modeld packages to the store, keyed by version. Final binaries live in
# the store (S3), not GitHub Releases.
push-modeld-release:
	@test -n "$(MODELD_RELEASE_S3_URI)" || { echo "set MODELD_RELEASE_S3_URI=s3://bucket/prefix (or a local dir to test)"; exit 1; }
	@found=0; \
	for f in $(MODELD_RELEASE_DIST_DIR)/*.tar.gz; do \
		[ -f "$$f" ] || continue; found=1; \
		base=$$(basename "$$f"); dest="$(MODELD_RELEASE_S3_URI)/$(MODELD_VERSION)"; \
		echo "cp $$f -> $$dest/$$base"; \
		$(MODELD_STORE) cp "$$f" "$$dest/$$base" || exit 1; \
		$(MODELD_STORE) cp "$$f.sha256" "$$dest/$$base.sha256" || exit 1; \
	done; \
	[ "$$found" = 1 ] || { echo "no packages in $(MODELD_RELEASE_DIST_DIR); run: make package-modeld-release"; exit 1; }

# Validate that an extracted dependency bundle has everything the release link needs.
# Hard-fails when OpenVINO is required but the bundle does not declare/contain it, so
# a release can never silently fall back to a reduced backend set.
check-modeld-deps-bundle:
	@test -n "$(MODELD_DEPS_ROOT)" || { echo "set MODELD_DEPS_ROOT=/path/to/modeld-deps-<platform>"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/manifest.json" || { echo "bundle missing manifest.json: $(MODELD_DEPS_ROOT)"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/llama/ref/common/chat.h" || { echo "bundle missing llama.cpp ref headers"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/llama/runtime/lib/libllama.so" || { echo "bundle missing llama runtime libllama.so"; exit 1; }
	@test -f "$(MODELD_DEPS_ROOT)/llama/runtime/lib/libcommon.a" || { echo "bundle missing llama libcommon.a"; exit 1; }
	@if [ "$(MODELD_RELEASE_OPENVINO)" = "1" ]; then \
		grep -q '"openvino": *true' "$(MODELD_DEPS_ROOT)/manifest.json" || { echo "MODELD_RELEASE_OPENVINO=1 but bundle manifest does not declare openvino:true (refusing to silently drop OpenVINO)"; exit 1; }; \
		test -d "$(MODELD_DEPS_ROOT)/openvino/genai/src/cpp/include" || { echo "bundle missing OpenVINO GenAI headers"; exit 1; }; \
		ls "$(MODELD_DEPS_ROOT)"/openvino/genai/libopenvino_genai.so* >/dev/null 2>&1 || { echo "bundle missing libopenvino_genai.so"; exit 1; }; \
		test -f "$(MODELD_DEPS_ROOT)/openvino/tokenizers/lib/libopenvino_tokenizers.so" || { echo "bundle missing libopenvino_tokenizers.so"; exit 1; }; \
		ls "$(MODELD_DEPS_ROOT)"/openvino/openvino/libs/libopenvino.so* >/dev/null 2>&1 || { echo "bundle missing libopenvino.so"; exit 1; }; \
	fi
	@echo "modeld deps bundle OK: $(MODELD_DEPS_ROOT) (openvino required=$(MODELD_RELEASE_OPENVINO))"

# Package a release modeld bundle by linking against an extracted native dependency
# bundle (MODELD_DEPS_ROOT) instead of rebuilding native deps. The root paths and
# build tags are re-pointed at the bundle via target-specific variables; the existing
# mk/*.mk flag expressions recompute against them. Deterministic and self-contained:
# it does not rebuild llama.cpp / OpenVINO and refuses to drop an expected backend.
package-modeld-release: MODELD_DIST_DIR := $(MODELD_RELEASE_DIST_DIR)/$(MODELD_RELEASE_NAME)
package-modeld-release: LLAMA_CPP_REF_DIR := $(MODELD_DEPS_ROOT)/llama/ref
package-modeld-release: LLAMA_RUNTIME_DIR := $(MODELD_DEPS_ROOT)/llama/runtime
package-modeld-release: LLAMA_RUNTIME_LIB_DIR := $(MODELD_DEPS_ROOT)/llama/runtime/lib
package-modeld-release: OPENVINO_PKG := $(MODELD_DEPS_ROOT)/openvino/openvino
package-modeld-release: OPENVINO_GENAI_SRC := $(MODELD_DEPS_ROOT)/openvino/genai
package-modeld-release: OPENVINO_GENAI_PKG := $(MODELD_DEPS_ROOT)/openvino/genai
package-modeld-release: OPENVINO_TOKENIZERS_LIB := $(MODELD_DEPS_ROOT)/openvino/tokenizers/lib
package-modeld-release: OPENVINO_TOKENIZERS_SO := $(MODELD_DEPS_ROOT)/openvino/tokenizers/lib/libopenvino_tokenizers.so
package-modeld-release: MODELD_TAGS := $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),llamanode llamacpp_direct openvino openvino_genai,llamanode llamacpp_direct)
package-modeld-release: MODELD_OV_CXXFLAGS = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),$(OPENVINO_GENAI_CGO_CXXFLAGS),)
package-modeld-release: MODELD_OV_PKG_LDFLAGS = $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),$(OPENVINO_GENAI_LINK_FLAGS),)
package-modeld-release: MODELD_LD_FLAGS = $(MODELD_VERSION_LD_FLAGS) $(MODELD_LLAMA_LD_FLAGS) $(if $(filter 1,$(MODELD_RELEASE_OPENVINO)),$(MODELD_OPENVINO_LD_FLAGS),)
package-modeld-release: check-modeld-deps-bundle
	@rm -rf "$(MODELD_DIST_DIR)" && mkdir -p "$(MODELD_DIST_DIR)"
	@echo "packaging modeld release: $(MODELD_RELEASE_NAME) tags=[$(MODELD_TAGS)] openvino=$(MODELD_RELEASE_OPENVINO) deps=$(MODELD_DEPS_ROOT)"
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
	@if [ "$(MODELD_RELEASE_OPENVINO)" = "1" ]; then $(MAKE) --no-print-directory \
		OPENVINO_PKG="$(OPENVINO_PKG)" OPENVINO_GENAI_PKG="$(OPENVINO_GENAI_PKG)" OPENVINO_TOKENIZERS_SO="$(OPENVINO_TOKENIZERS_SO)" \
		MODELD_LIBS_DIR="$(MODELD_DIST_DIR)/modeld-libs" MODELD_LIBS_COPY=1 bundle-modeld-libs; fi
	@$(MAKE) --no-print-directory LLAMA_RUNTIME_LIB_SRC="$(LLAMA_RUNTIME_LIB_DIR)" LLAMA_LIBS_DIR="$(MODELD_DIST_DIR)/lib/llamacpp" LLAMA_LIBS_COPY=1 bundle-llama-libs
	@if [ -d "$(MODELD_DEPS_ROOT)/licenses" ]; then rm -rf "$(MODELD_DIST_DIR)/LICENSES"; cp -a "$(MODELD_DEPS_ROOT)/licenses" "$(MODELD_DIST_DIR)/LICENSES"; fi
	@DIST_DIR="$(MODELD_DIST_DIR)" \
	RELEASE_OUT="$(MODELD_RELEASE_DIST_DIR)" \
	NAME="$(MODELD_RELEASE_NAME)" \
	VERSION="$(MODELD_VERSION)" \
	PLATFORM="$(MODELD_PLATFORM)" \
	EXPECT_OPENVINO="$(MODELD_RELEASE_OPENVINO)" \
	bash $(PROJECT_ROOT)scripts/modeld-package-release.sh
	@echo "release modeld package -> $(MODELD_RELEASE_DIST_DIR)/$(MODELD_RELEASE_NAME).tar.gz"

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
