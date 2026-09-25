#!/usr/bin/env bash
# Assert the native baseline a packaged modeld worker was built against: the
# oldest glibc and libstdc++ a target image has to provide, plus which CUDA
# runtime libraries the accelerator backend expects to find.
#
# A worker built on the producing machine's own toolchain is not a release
# artifact. Fedora 43 / GCC 14 raises the floor to glibc 2.43 and GLIBCXX_3.4.32,
# which no Ubuntu LTS, Debian or RHEL image provides, and the package then loads
# nowhere but the machine that built it. The floor is a property of the shipped
# bytes, so it is read from them rather than declared.
#
# Usage:
#   modeld-abi-check.sh <package-dir> <max-glibc> <max-glibcxx> [--require-bundled-cuda]
#
# Exit 0 only when every shipped binary and library stays within both ceilings,
# and — with --require-bundled-cuda — the CUDA runtime the backend links is
# present inside the package rather than assumed on the target host.
set -euo pipefail
. "$(dirname -- "$0")/modeld-base-soname-allowlist.sh"

dir=${1:?usage: modeld-abi-check.sh <package-dir> <max-glibc, e.g. 2.35> <max-glibcxx, e.g. 3.4.30> [--require-bundled-cuda]}
max_glibc=${2:?missing max glibc, e.g. 2.35}
max_gxx=${3:?missing max glibcxx, e.g. 3.4.30}
require_cuda=0
[ "${4:-}" = "--require-bundled-cuda" ] && require_cuda=1

[ -d "$dir" ] || { echo "no such package dir: $dir" >&2; exit 1; }
command -v objdump >/dev/null || { echo "objdump is required (binutils)" >&2; exit 1; }

ver_le() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | tail -1)" = "$2" ]; }

symbol_ceiling() {
  local file=$1 prefix=$2
  objdump -T "$file" 2>/dev/null | grep -oE "${prefix}_[0-9.]+" | sed "s/^${prefix}_//" | sort -Vu | tail -1 || true
}

status=0
worst_glibc=0.0
worst_gxx=0.0
scanned=0
cuda_libs=""

while IFS= read -r file; do
  case "$file" in
    *.so|*.so.*|*.bin|*/modeld) ;;
    *) continue ;;
  esac
  if ! head -c4 "$file" | grep -q ELF; then continue; fi
  scanned=$((scanned + 1))

  glibc=$(symbol_ceiling "$file" GLIBC)
  gxx=$(symbol_ceiling "$file" GLIBCXX)
  glibc=${glibc:-none}
  gxx=${gxx:-none}

  if [ "$glibc" != none ]; then
    if ! ver_le "$glibc" "$max_glibc"; then
      echo "FAIL $(basename "$file"): needs glibc $glibc > $max_glibc"
      status=1
    fi
    if ver_le "$worst_glibc" "$glibc"; then worst_glibc=$glibc; fi
  fi
  if [ "$gxx" != none ]; then
    if ! ver_le "$gxx" "$max_gxx"; then
      echo "FAIL $(basename "$file"): needs GLIBCXX $gxx > $max_gxx"
      status=1
    fi
    if ver_le "$worst_gxx" "$gxx"; then worst_gxx=$gxx; fi
  fi

  case "$(basename "$file")" in
    libcudart.so*|libcublas.so*|libcublasLt.so*) cuda_libs="$cuda_libs $(basename "$file")" ;;
  esac
done < <(find "$dir" -type f)

echo "scanned=$scanned ceiling_glibc=$max_glibc worst_glibc=$worst_glibc ceiling_glibcxx=$max_gxx worst_glibcxx=$worst_gxx"
[ "$scanned" -gt 0 ] || { echo "FAIL no ELF binaries found" >&2; exit 1; }

# Self-containment: every SONAME a shipped binary needs must either be provided by
# the base OS or the NVIDIA driver on any target, or be shipped inside this package.
# A CUDA backend whose BLAS runtime is missing does not fail loudly on a driver-only
# host — GGML_BACKEND_DL's dlopen fails, and the worker falls back to CPU — so the
# closure is asserted here rather than discovered in the field.
shipped() {
  local name=$1 file
  while IFS= read -r file; do
    [ -f "$file" ] && return 0
  done < <(find "$dir" -name "$name")
  return 1
}

missing=0
while IFS= read -r file; do
  head -c4 "$file" | grep -q ELF || continue
  while IFS= read -r need; do
    [ -n "$need" ] || continue
    base_os_or_driver "$need" && continue
    if ! shipped "$need"; then
      echo "FAIL $(basename "$file") needs $need: not shipped in the package and not provided by the base OS or driver"
      missing=$((missing + 1))
      status=1
    fi
  done < <(objdump -p "$file" 2>/dev/null | awk '/NEEDED/ {print $2}')
done < <(find "$dir" -type f)

cuda_backend=$(find "$dir" -name "libggml-cuda.so*" -type f | head -1)
if [ -n "$cuda_backend" ]; then
  needed=$(objdump -p "$cuda_backend" 2>/dev/null | awk '/NEEDED/ {print $2}' | grep -E "lib(cudart|cublas|cublasLt)" | sort -u | tr '\n' ' ' || true)
  echo "cuda_backend=$(basename "$cuda_backend") links=[${needed% }] unresolved=$missing"
  if [ "$require_cuda" = 1 ] && [ -z "$(find "$dir" -type f \( -name 'libcudart.so*' -o -name 'libcublas.so*' -o -name 'libcublasLt.so*' \) | head -1)" ]; then
    echo "FAIL the CUDA backend links [$needed] but the package bundles none of it; a host with only the driver falls back to CPU silently"
    status=1
  fi
fi

# Redistribution condition, not a formality: NVIDIA's CUDA Toolkit EULA permits
# shipping the runtime and BLAS libraries only with the agreement alongside them
# (Attachment A, distribution requirements 1.1.2). A package carrying them without
# the EULA is not distributable, whatever its ABI floor.
if find "$dir" -type f \( -name 'libcudart.so*' -o -name 'libcublas.so*' -o -name 'libcublasLt.so*' -o -name 'libnvrtc.so*' -o -name 'libnvJitLink.so*' \) | grep -q .; then
  if ! find "$dir" -type f -name 'EULA.txt' | grep -q .; then
    echo "FAIL the package ships NVIDIA runtime libraries but no CUDA EULA; redistribution requires the agreement alongside them"
    status=1
  fi
fi

[ "$status" = 0 ] || { echo "abi check failed"; exit 1; }
echo "abi check passed"
