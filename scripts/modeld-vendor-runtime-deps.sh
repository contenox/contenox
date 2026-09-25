#!/usr/bin/env bash
# Close a packaged modeld worker's shared-library closure: for every SONAME its
# binaries and libraries need, copy the library into the package unless the target
# host is guaranteed to provide it (base OS, or the NVIDIA driver).
#
# The failure this prevents is silent, not loud. GGML_BACKEND_DL dlopens
# libggml-cuda.so at runtime; if that library's own dependencies (libcudart,
# libcublas, libcublasLt) are absent, the dlopen fails, the accelerator probe
# reports no usable GPU, and the worker runs on CPU — a GPU host that looks like a
# CPU host. The same applies to libgomp for the CPU variants of ggml on an image
# without the GCC OpenMP runtime.
#
# Usage: modeld-vendor-runtime-deps.sh <package-dir> [lib-dir-to-fill]
set -euo pipefail

dir=${1:?usage: modeld-vendor-runtime-deps.sh <package-dir> [lib-dir]}
dest=${2:-$dir/modeld-libs}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_dir/modeld-base-soname-allowlist.sh"

[ -d "$dir" ] || { echo "no such package dir: $dir" >&2; exit 1; }
mkdir -p "$dest"

resolve() {
  local name=$1 path
  if command -v ldconfig >/dev/null 2>&1; then
    path=$(ldconfig -p 2>/dev/null | awk -v n="$name" '$1 == n && !found {print $NF; found=1}')
    [ -n "$path" ] && { printf '%s' "$path"; return 0; }
  fi
  for d in /usr/local/cuda/targets/x86_64-linux/lib /usr/lib/x86_64-linux-gnu /usr/lib64 /usr/lib /lib/x86_64-linux-gnu /lib64; do
    [ -d "$d" ] || continue
    path=$(find "$d" -maxdepth 1 -name "$name" -type f -o -maxdepth 1 -name "$name" -type l 2>/dev/null | head -1)
    [ -n "$path" ] && { printf '%s' "$path"; return 0; }
  done
  return 1
}

copied=0
missing=""
manifest=""
stage_license() {
  local name=$1 src=$2 dest=$3
  [ -f "$src" ] || return 0
  mkdir -p "$dir/LICENSES/$dest"
  cp -a "$src" "$dir/LICENSES/$dest/"
  manifest="$manifest$name -> LICENSES/$dest/$(basename "$src")
"
}

resolve_license() {
  local name=$1 source=$2 cand owner
  if command -v dpkg-query >/dev/null 2>&1; then
    source=$(readlink -f "$source")
    owner=$(dpkg-query -S "$source" 2>/dev/null | sed -n '1s/: \/.*//p' || true)
    owner=${owner%%:*}
    if [ -n "$owner" ] && [ -f "/usr/share/doc/$owner/copyright" ]; then
      printf '%s' "/usr/share/doc/$owner/copyright"
      return 0
    fi
  fi
  for cand in "/usr/share/licenses/$name"/* "/usr/share/doc/$name/copyright" "/usr/share/doc/${name}1/copyright"; do
    [ -f "$cand" ] && { printf '%s' "$cand"; return 0; }
  done
  return 1
}
shipped() {
  local file
  while IFS= read -r file; do
    [ -f "$file" ] && return 0
  done < <(find "$dir" -name "$1")
  return 1
}

while :; do
previous_copied=$copied
missing=""
while IFS= read -r file; do
  head -c4 "$file" | grep -q ELF || continue
  while IFS= read -r need; do
    [ -n "$need" ] || continue
    base_os_or_driver "$need" && continue
    if shipped "$need"; then continue; fi
    src=$(resolve "$need") || { missing="$missing $need"; continue; }
    cp -L "$src" "$dest/$need"
    echo "vendored $need -> $dest/$need"
    copied=$((copied + 1))
    case "$need" in
      libcudart.so*|libcublas.so*|libcublasLt.so*|libnvrtc.so*|libnvJitLink.so*)
        for eula in "${CUDA_HOME:-}/EULA.txt" "${CUDA_PATH:-}/EULA.txt" /usr/local/cuda/EULA.txt; do
          [ -f "$eula" ] && { stage_license "$need" "$eula" cuda; break; }
        done
        ;;
      *)
        lic=$(resolve_license "${need%%.so*}" "$src") && stage_license "$need" "$lic" "${need%%.so*}"
        ;;
    esac
  done < <(objdump -p "$file" 2>/dev/null | awk '/NEEDED/ {print $2}')
done < <(find "$dir" -type f)
[ "$copied" -gt "$previous_copied" ] || break
done

if find "$dir" -type f \( -name 'libcudart.so*' -o -name 'libcublas*.so*' -o -name 'libnvrtc.so*' -o -name 'libnvJitLink.so*' \) | grep -q .; then
  for eula in "${CUDA_HOME:-}/EULA.txt" "${CUDA_PATH:-}/EULA.txt" /usr/local/cuda/EULA.txt /usr/share/doc/cuda-cudart-*/copyright; do
    if [ -f "$eula" ]; then
      stage_license cuda "$eula" cuda
      if [ "$(basename "$eula")" != EULA.txt ]; then
        cp "$eula" "$dir/LICENSES/cuda/EULA.txt"
      fi
      break
    fi
  done
fi

if [ -n "$manifest" ]; then
  {
    echo "# Third-party runtime libraries shipped inside this package."
    echo "# Each is redistributed unmodified under its own license; none is covered by"
    echo "# the project's license. NVIDIA runtime/BLAS libraries are redistributable as"
    echo "# incorporated in an application per the CUDA Toolkit EULA (Attachment A)."
    printf '%s' "$manifest"
  } > "$dir/LICENSES/vendored-libraries.txt"
  cat "$dir/LICENSES/vendored-libraries.txt"
fi

echo "modeld-vendor-runtime-deps: copied=$copied"
if [ -n "$missing" ]; then
  echo "modeld-vendor-runtime-deps: unresolved on this build host:$missing" >&2
  exit 1
fi
