#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
[ "$(uname -s)-$(uname -m)" = Darwin-arm64 ] || { echo 'requires native Apple Silicon macOS' >&2; exit 1; }
for tool in go cmake clang otool lipo install_name_tool python3; do
  command -v "$tool" >/dev/null || { echo "missing tool: $tool" >&2; exit 1; }
done
export LLAMA_RUNTIME_ROOT="$root/.build/modeld-release/llama-runtime"
export LLAMA_CPP_BUILD_ROOT="$root/.build/modeld-release/llama-build"
export LLAMA_CPP_REF_DIR="$root/.build/modeld-release/llama-src"
export MODELD_RELEASE_OPENVINO=0
export LLAMA_CUDA=OFF LLAMA_HIP=OFF
export LLAMA_CPP_EXTRA_CMAKE_FLAGS='-DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON'
make deps-llamacpp-ref
make build-llamacpp-runtime
make bundle-modeld-deps
bundle="$root/bin/modeld-deps/modeld-deps-darwin-arm64-metal"
[ -f "$bundle/llama/runtime/lib/libggml-metal.dylib" ] || { echo 'Metal plugin missing; refusing CPU-only candidate' >&2; exit 1; }
make package-modeld-release-darwin MODELD_DEPS_ROOT="$bundle"
