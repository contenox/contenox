#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${CONTENOX_BIN:-"$ROOT_DIR/bin/contenox"}"
HOST="${CONTENOX_APITEST_HOST:-127.0.0.1}"
PORT="${CONTENOX_APITEST_PORT:-32124}"
TMP_ROOT="${CONTENOX_APITEST_TMPDIR:-}"

if [[ -z "$TMP_ROOT" ]]; then
  TMP_ROOT="$(mktemp -d)"
  REMOVE_TMP=1
else
  mkdir -p "$TMP_ROOT"
  REMOVE_TMP=0
fi

LOG_FILE="$TMP_ROOT/serve.log"
HOME_DIR="$TMP_ROOT/home"
WORKSPACE_DIR="$TMP_ROOT/workspace"
DATA_DIR="$WORKSPACE_DIR/.contenox"
DB_PATH="$HOME_DIR/.contenox/local.db"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ "$REMOVE_TMP" == "1" ]]; then
    rm -rf "$TMP_ROOT"
  else
    printf 'apitest temp dir retained: %s\n' "$TMP_ROOT"
  fi
}
trap cleanup EXIT

python3 - <<'PY'
import importlib.util
import sys

missing = [name for name in ("pytest", "requests") if importlib.util.find_spec(name) is None]
if missing:
    print("missing Python packages: " + ", ".join(missing), file=sys.stderr)
    print("install with: python3 -m pip install -r apitests/requirements.txt", file=sys.stderr)
    sys.exit(1)
PY

mkdir -p "$HOME_DIR" "$WORKSPACE_DIR"

HOME="$HOME_DIR" "$BIN" --data-dir "$DATA_DIR" --db "$DB_PATH" init --force >/dev/null

HOME="$HOME_DIR" ADDR="$HOST" PORT="$PORT" TOKEN="" "$BIN" --data-dir "$DATA_DIR" --db "$DB_PATH" serve >"$LOG_FILE" 2>&1 &
SERVER_PID="$!"

python3 - "$HOST" "$PORT" "$SERVER_PID" "$LOG_FILE" <<'PY'
import pathlib
import sys
import time
import urllib.request

host, port, pid, log_file = sys.argv[1:]
url = f"http://{host}:{port}/health"
deadline = time.monotonic() + 30

while time.monotonic() < deadline:
    try:
        with urllib.request.urlopen(url, timeout=1) as response:
            if response.status == 200:
                sys.exit(0)
    except Exception:
        pass
    try:
        import os
        os.kill(int(pid), 0)
    except OSError:
        break
    time.sleep(0.25)

print(f"contenox serve did not become ready at {url}", file=sys.stderr)
print(pathlib.Path(log_file).read_text(errors="replace"), file=sys.stderr)
sys.exit(1)
PY

export CONTENOX_API_URL="http://$HOST:$PORT/api"
export CONTENOX_BEAM_ORIGIN="http://$HOST:$PORT"
python3 -m pytest "$ROOT_DIR/apitests" "$@"
