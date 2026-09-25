#!/usr/bin/env bash
set -euo pipefail

archive=${1:?usage: modeld-linux-package-smoke.sh WORKER.tar.gz}
archive=$(realpath "$archive")
[ -f "$archive" ] || { echo "missing archive: $archive" >&2; exit 1; }
name=$(basename "$archive" .tar.gz)
[[ "$name" == modeld-*-linux-* ]] || { echo 'expected a Linux modeld archive' >&2; exit 1; }
docker run --rm --network none \
  -v "$archive:/worker.tar.gz:ro" \
  -e PACKAGE_NAME="$name" \
  "${MODELD_SMOKE_IMAGE:-ubuntu:22.04}" bash -euo pipefail -c '
mkdir /tmp/package
tar -xzf /worker.tar.gz -C /tmp/package
dir="/tmp/package/$PACKAGE_NAME"
export LD_LIBRARY_PATH="$dir/modeld-libs:$dir/lib/llamacpp"
"$dir/modeld" version --json
status=0
while IFS= read -r -d "" file; do
  head -c4 "$file" | grep -q ELF || continue
  report=$(ldd "$file" 2>&1) || { printf "%s\n" "$report"; exit 1; }
  while IFS= read -r missing; do
    case "$missing" in
      libcuda.so.*|libnvidia-*|libcudadebugger.so.*) printf "host driver required: %s\n" "$missing" ;;
      *) printf "unresolved: %s needs %s\n" "$file" "$missing"; status=1 ;;
    esac
  done < <(printf "%s\n" "$report" | awk '\''$2 == "=>" && $3 == "not" {print $1}'\'')
done < <(find "$dir" -type f -print0)
[ "$status" = 0 ] || exit 1
echo "clean Linux package startup and dependency resolution passed; GPU inference not tested"
'
