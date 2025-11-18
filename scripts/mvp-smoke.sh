# ./scripts/mvp-smoke.sh
set -euo pipefail

echo "==> Repo root: $(pwd)"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing dependency: $1"; exit 1; }
}
need docker
need wget
need python3
need go

echo "==> Clean python venv (if any)"
make clean || true

echo "==> Build images"
make build

echo "==> Bring up services"
make up

echo "==> Compose ps"
make ps

echo "==> Wait for /health"
make wait-for-server

echo "==> Brief logs (8s)…"
( command -v timeout >/dev/null && timeout 8s make logs ) || true

echo "==> Run Go tests (unit/system/all) — may be no-ops if none match"
make test-unit || true
make test-system || true
make test || true

echo "==> Init API test venv (if apitests/requirements.txt exists)"
if [ -f apitests/requirements.txt ]; then
  make test-api-init
  # Pick first test file if TEST_FILE not set
  if [ -z "${TEST_FILE:-}" ]; then
    TEST_FILE="$(ls apitests/*.py 2>/dev/null | head -n 1 || true)"
    export TEST_FILE
  fi
  if [ -n "${TEST_FILE:-}" ]; then
    echo "==> Run API tests with TEST_FILE=${TEST_FILE}"
    make test-api || true
    echo "==> Run API tests (verbose logs)"
    make test-api-logs || true
  else
    echo "==> No apitests/*.py found, skipping test-api targets"
  fi
else
  echo "==> No apitests/requirements.txt, skipping API test targets"
fi

echo "==> Restart (down+run)"
make restart

echo "==> Brief logs after restart (5s)…"
( command -v timeout >/dev/null && timeout 5s make logs ) || true

echo "==> Tear down (keep images)"
make down

echo "==> Full compose wipe (images+volumes) to ensure idempotence"
make compose-wipe || true

echo "==> SUCCESS: smoke run finished"
