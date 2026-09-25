#!/usr/bin/env bash
# Build the RELEASE modeld worker on this machine, on the pinned native baseline.
#
# The baseline is the point. A worker compiled with the producing host's own
# toolchain carries that host's floor: on Fedora 43 the shipped llama.cpp libraries
# need glibc 2.43 and GLIBCXX_3.4.32, which no Ubuntu LTS, Debian or RHEL image
# provides, so the package runs nowhere but the machine that built it. The floor
# comes from the libc the linker resolves against, so it cannot be lowered by a flag
# — the build has to happen inside a userland at the baseline. That is all this
# script does: same machine, same checkout, same aws credentials, with the pinned
# image supplying the toolchain instead of the host.
#
# Steps, in order, all inside the image:
#   deps-modeld            fetch the pinned llama.cpp source and OpenVINO SDK
#   bundle-modeld-deps     compile the native dependencies for the baseline
#   package-modeld-release link the Go worker against it, vendor the runtime closure
#                          and gate the finished package (the release recipe runs the
#                          gate itself, so a package cannot leave here ungated)
#
# The output lands on the host under bin/modeld-deps/ and dist/, ready for
# `make push-modeld-release` (upload + index refresh) — this script never publishes.
#
# Environment:
#   MODELD_BUILD_IMAGE        pinned builder image
#   MODELD_ABI_BASELINE       baseline label recorded in the bundle fingerprint
#   MODELD_ABI_MAX_GLIBC      oldest glibc the package may require
#   MODELD_ABI_MAX_GLIBCXX    oldest libstdc++ the package may require
#   MODELD_CUDA_ARCHITECTURES optional explicit GPU list, e.g. "80;86;89;90;120"
#   MODELD_RELEASE_OPENVINO   1 (default) to build the OpenVINO half too
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image=${MODELD_BUILD_IMAGE:-nvidia/cuda:12.9.0-devel-ubuntu22.04}
baseline=${MODELD_ABI_BASELINE:-ubuntu22.04}
max_glibc=${MODELD_ABI_MAX_GLIBC:-2.35}
max_glibcxx=${MODELD_ABI_MAX_GLIBCXX:-3.4.30}
openvino=${MODELD_RELEASE_OPENVINO:-1}
arches=${MODELD_CUDA_ARCHITECTURES:-}
version=${MODELD_VERSION:-$(tr -d '\r\n' < "$root/internal/version/version.txt")}

if [ -z "${DOCKER_HOST:-}" ] && [ ! -S /var/run/docker.sock ]; then
  for sock in "/run/user/$(id -u)/docker.sock"; do
    [ -S "$sock" ] && export DOCKER_HOST="unix://$sock" && break
  done
fi
command -v docker >/dev/null || { echo "docker is required (rootless is fine: dockerd-rootless-setuptool.sh install)" >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo "no reachable docker daemon; start one (system: sudo systemctl start docker, rootless: systemctl --user start docker)" >&2; exit 1; }

goroot=$(go env GOROOT)
[ -d "$goroot" ] || { echo "no Go toolchain at GOROOT=$goroot" >&2; exit 1; }
toolchain_dir="$root/.build/modeld-release/go-toolchain/$(go env GOVERSION)"
mkdir -p "$toolchain_dir"
cp -aL "$goroot/." "$toolchain_dir/"

echo "modeld release build: image=$image baseline=$baseline ceiling_glibc=$max_glibc ceiling_glibcxx=$max_glibcxx openvino=$openvino"
[ -n "$arches" ] && echo "modeld release build: cuda architectures=$arches"

docker run --rm -i \
  -v "$root:/src" \
  -v "$toolchain_dir:/usr/local/go:ro" \
  -w /src \
  -e PATH=/usr/local/go/bin:/usr/local/cuda/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  -e DEBIAN_FRONTEND=noninteractive \
  -e MODELD_ABI_BASELINE="$baseline" \
  -e MODELD_ABI_MAX_GLIBC="$max_glibc" \
  -e MODELD_ABI_MAX_GLIBCXX="$max_glibcxx" \
  -e MODELD_RELEASE_OPENVINO="$openvino" \
  -e MODELD_VERSION="$version" \
  -e GOCACHE=/src/.build/modeld-release/go-cache \
  -e GOMODCACHE=/src/.build/modeld-release/go-mod \
  -e OPENVINO_WORKDIR=/src/.build/modeld-release/openvino \
  -e OPENVINO_GENAI_REPO=/src/.build/modeld-release/openvino-genai \
  -e LLAMA_CPP_EXTRA_CMAKE_FLAGS="$( [ -n "$arches" ] && printf -- '-DCMAKE_CUDA_ARCHITECTURES=%s' "$arches" )" \
  -e LLAMA_RUNTIME_ROOT=/src/.build/modeld-release/llama-runtime \
  -e LLAMA_CPP_BUILD_ROOT=/src/.build/modeld-release/llama-build \
  "$image" bash -euo pipefail -s <<'INNER'
echo "== builder: $(gcc --version | head -1) | $(nvcc --version | grep -o 'release [0-9.]*') | $(ldd --version | head -1) | $(go version)"
go list context fmt net/http runtime >/dev/null

apt-get update
apt-get install -y --no-install-recommends \
  build-essential cmake ninja-build git curl ca-certificates python3 python3-venv \
  unzip pkg-config binutils libcurl4-openssl-dev ocl-icd-libopencl1

echo "== fetching pinned native sources"
make_args=('OPENVINO_GENAI_SRC=/src/.build/modeld-release/openvino-genai-$(OPENVINO_GENAI_VERSION)')
if [ "$MODELD_RELEASE_OPENVINO" = 1 ]; then
  make "${make_args[@]}" deps-modeld
else
  make "${make_args[@]}" deps-llamacpp-ref
fi

echo "== compiling native dependencies for the baseline"
make "${make_args[@]}" bundle-modeld-deps MODELD_ABI_BASELINE="$MODELD_ABI_BASELINE" LLAMA_CUDA=ON LLAMA_HIP=OFF

bundle_dir="bin/modeld-deps/modeld-deps-$(go env GOOS)-$(go env GOARCH)-cuda"
echo "== bundle: $bundle_dir"
cat "$bundle_dir/bundle.env"

echo "== packaging the worker against that bundle"
make "${make_args[@]}" package-modeld-release MODELD_DEPS_ROOT="$bundle_dir" MODELD_RELEASE_OPENVINO="$MODELD_RELEASE_OPENVINO"

echo "== packaged:"
ls -la dist/ | tail -n +2
INNER

echo "modeld release build: done. Upload with: make push-modeld-release MODELD_RELEASE_S3_URI=<store>"
