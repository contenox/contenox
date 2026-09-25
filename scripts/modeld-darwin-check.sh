#!/usr/bin/env bash
set -euo pipefail

dir=${1:?usage: modeld-darwin-check.sh PACKAGE_DIRECTORY}
[ "$(uname -s)" = Darwin ] || { echo 'requires macOS otool' >&2; exit 1; }
[ -f "$dir/modeld.bin" ] || { echo 'missing modeld.bin' >&2; exit 1; }
[ -f "$dir/lib/llamacpp/libggml-metal.dylib" ] || { echo 'missing Metal plugin' >&2; exit 1; }
status=0
while IFS= read -r -d '' file; do
  file -b "$file" | grep -q 'Mach-O' || continue
  lipo -verify_arch arm64 "$file" || { echo "missing arm64 slice: $file" >&2; status=1; }
  while IFS= read -r need; do
    case "$need" in
      /usr/lib/*|/System/Library/*) continue ;;
      @loader_path/*) resolved="$(dirname "$file")/${need#@loader_path/}" ;;
      @executable_path/*) resolved="$dir/${need#@executable_path/}" ;;
      @rpath/*) resolved="$dir/lib/llamacpp/${need#@rpath/}" ;;
      *) echo "nonportable dependency: $file -> $need" >&2; status=1; continue ;;
    esac
    [ -f "$resolved" ] || { echo "missing dependency: $file -> $need" >&2; status=1; }
  done < <(otool -L "$file" | sed -n '2,$s/^[[:space:]]*\(.*\) (compatibility version.*$/\1/p')
done < <(find "$dir" -type f -print0)
exit "$status"
