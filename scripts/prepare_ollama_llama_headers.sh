#!/usr/bin/env sh
set -eu

OLLAMA_VERSION="${OLLAMA_VERSION:-$(go list -m -f '{{.Version}}' github.com/ollama/ollama)}"
OLLAMA_MOD="$(go env GOMODCACHE)/github.com/ollama/ollama@${OLLAMA_VERSION}"

go mod download "github.com/ollama/ollama@${OLLAMA_VERSION}"

if [ ! -d "$OLLAMA_MOD/llama" ]; then
  echo "ollama module llama directory not found: $OLLAMA_MOD/llama" >&2
  exit 1
fi

chmod -R u+w "$OLLAMA_MOD/llama" 2>/dev/null || true

MTMD="$OLLAMA_MOD/llama/llama.cpp/tools/mtmd"
mkdir -p "$MTMD/miniaudio" "$MTMD/stb"

fetch_if_missing() {
  url="$1"
  dest="$2"
  if [ -s "$dest" ]; then
    return 0
  fi
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
    return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
    return 0
  fi
  echo "curl or wget is required to fetch $url" >&2
  exit 1
}

fetch_if_missing \
  "https://raw.githubusercontent.com/mackron/miniaudio/master/miniaudio.h" \
  "$MTMD/miniaudio/miniaudio.h"
fetch_if_missing \
  "https://raw.githubusercontent.com/nothings/stb/master/stb_image.h" \
  "$MTMD/stb/stb_image.h"
