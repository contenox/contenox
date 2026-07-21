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

# Seed the no-model chain-agent fixture(s) into the workspace .contenox/ BEFORE
# boot. Chain-agent discovery runs once at `contenox serve` startup and walks
# this directory, so a fixture placed here is declared as a fleet-dispatchable
# agent by the time the API is up. This is what lets test_fleet.py exercise a
# REAL, hermetic dispatch -> running -> stop lifecycle: each fixture chain is a
# single noop task that resolves no model, so no backend and no network are
# needed. Additive and self-contained — every other suite simply ignores the
# extra agent. See apitests/fixtures/agent-apitest-noop.json.
FIXTURE_DIR="$ROOT_DIR/apitests/fixtures"
if [[ -d "$FIXTURE_DIR" ]]; then
  shopt -s nullglob
  for fixture in "$FIXTURE_DIR"/agent-*.json; do
    cp "$fixture" "$DATA_DIR/"
  done
  shopt -u nullglob
fi

# A dispatched chain unit is a subprocess of this same binary (see the C9
# "self-spawn" note in docs/development/blueprints/acp/fleet-consolidation.md).
# Its runtime engine hard-requires a configured default model at boot even
# though the noop fixture chain never resolves one, so hand it a fake default
# via the environment the subprocess inherits. The name is intentionally fake:
# a noop chain never touches it, and any accidental model resolution then fails
# loudly instead of finding a real backend. No existing suite reads default-*.
export CONTENOX_DEFAULT_MODEL="${CONTENOX_APITEST_DEFAULT_MODEL:-apitest-fixture-model}"
export CONTENOX_DEFAULT_PROVIDER="${CONTENOX_APITEST_DEFAULT_PROVIDER:-ollama}"

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
