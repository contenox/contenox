#!/usr/bin/env bash
# Emit a deterministic fingerprint of the inputs a native dependency bundle is built
# from: the pinned source/version/accelerator profile. Two builds with the same
# fingerprint produce equivalent bundles, so a device can check S3 for the
# fingerprint and skip rebuilding/re-uploading a version we already have.
#
# The fingerprint is computed from identifiers only (no built artifacts), so it can
# be evaluated before the expensive runtime build. Both the bundle producer and the
# pre-build check call this one definition, so they can never drift.
#
# Inputs (env):
#   PLATFORM                 e.g. linux-amd64
#   LLAMA_CPP_COMMIT         pinned llama.cpp commit
#   LLAMA_BUILD_TYPE         e.g. Release
#   LLAMA_RUNTIME_ABI        e.g. dl-v2
#   CUDA                     ON | OFF
#   HIP                      ON | OFF
#   OPENVINO                 1 | 0
#   OPENVINO_GENAI_VERSION   pinned version (empty when OPENVINO=0)
#   MODELD_ABI_BASELINE      OS/toolchain baseline the natives are compiled against
#   CUDA_TOOLKIT             CUDA toolkit version linked into the CUDA backend
set -euo pipefail

: "${PLATFORM:?missing PLATFORM}"
: "${LLAMA_CPP_COMMIT:?missing LLAMA_CPP_COMMIT}"
LLAMA_BUILD_TYPE=${LLAMA_BUILD_TYPE:-Release}
LLAMA_RUNTIME_ABI=${LLAMA_RUNTIME_ABI:-dl-v2}
CUDA=${CUDA:-OFF}
HIP=${HIP:-OFF}
OPENVINO=${OPENVINO:-0}
OPENVINO_GENAI_VERSION=${OPENVINO_GENAI_VERSION:-}
[ "$OPENVINO" = "1" ] || OPENVINO_GENAI_VERSION=""
MODELD_ABI_BASELINE=${MODELD_ABI_BASELINE:-host}
CUDA_TOOLKIT=${CUDA_TOOLKIT:-}

# Stable, ordered canonical form. Do not reorder: the hash is a contract.
canonical=$(printf '%s\n' \
  "bundle_layout=4" \
  "platform=$PLATFORM" \
  "llama_cpp_commit=$LLAMA_CPP_COMMIT" \
  "llama_build_type=$LLAMA_BUILD_TYPE" \
  "llama_runtime_abi=$LLAMA_RUNTIME_ABI" \
  "cuda=$CUDA" \
  "hip=$HIP" \
  "openvino=$OPENVINO" \
  "openvino_genai_version=$OPENVINO_GENAI_VERSION" \
  "abi_baseline=$MODELD_ABI_BASELINE" \
  "cuda_toolkit=$CUDA_TOOLKIT")

if command -v sha256sum >/dev/null; then
  printf '%s' "$canonical" | sha256sum | cut -d' ' -f1
else
  printf '%s' "$canonical" | shasum -a 256 | cut -d' ' -f1
fi
