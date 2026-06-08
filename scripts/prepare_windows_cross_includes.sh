#!/usr/bin/env sh
set -eu

PROJECT_ROOT="${PROJECT_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}"
SOURCE_ROOT="${NLOHMANN_INCLUDE_ROOT:-/usr/include}"
DEST_ROOT="${WINDOWS_CROSS_INCLUDE:-$PROJECT_ROOT/.build/windows/include}"

if [ ! -f "$SOURCE_ROOT/nlohmann/json.hpp" ]; then
  echo "nlohmann json headers not found under $SOURCE_ROOT; install nlohmann-json3-dev or set NLOHMANN_INCLUDE_ROOT" >&2
  exit 1
fi

mkdir -p "$DEST_ROOT"
rm -rf "$DEST_ROOT/nlohmann"
cp -R "$SOURCE_ROOT/nlohmann" "$DEST_ROOT/nlohmann"

echo "$DEST_ROOT"
