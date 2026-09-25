#!/usr/bin/env bash
set -euo pipefail

: "${GENAI_PKG:?missing GENAI_PKG}"
: "${OPENVINO_GENAI_VERSION:?missing OPENVINO_GENAI_VERSION}"
sdk_root=$(dirname -- "$GENAI_PKG")
for package in openvino openvino_genai openvino_tokenizers; do
  expected=$OPENVINO_GENAI_VERSION
  [ "$package" != openvino ] || expected=${OPENVINO_GENAI_VERSION%.*}
  actual=$(sed -n 's/^Version: //p' "$sdk_root"/"$package"-*.dist-info/METADATA 2>/dev/null || true)
  actual=${actual//$'\r'/}
  if [ "$actual" != "$expected" ]; then
    echo "OpenVINO SDK/header mismatch: $package requires $expected; installed metadata reports '${actual:-missing}'. Reinstall the pinned SDK before bundling." >&2
    exit 1
  fi
done
