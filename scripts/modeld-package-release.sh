#!/usr/bin/env bash
# Finalize a packaged release modeld bundle: smoke-test the backend set, write the
# package manifest, and produce the archive + checksum.
#
# The smoke gate is the whole point of a separate release path: it runs
# `modeld version --json` against the freshly packaged binary and asserts the
# compiled-in backends match what the release expects, so a build can never silently
# ship fewer backends than intended.
#
# Inputs (env, set by the Makefile package-modeld-release target):
#   DIST_DIR         the packaged bundle dir (contains modeld wrapper + modeld.bin)
#   RELEASE_OUT      directory to write the archive + checksum into
#   NAME             archive base name, e.g. modeld-v0.32.5-linux-amd64
#   VERSION          expected modeld version, e.g. v0.32.5
#   PLATFORM         e.g. linux-amd64
#   EXPECT_OPENVINO  1 to require the openvino backend, 0 for llama-only
set -euo pipefail

fail() { echo "modeld-package-release: $*" >&2; exit 1; }
for v in DIST_DIR RELEASE_OUT NAME VERSION PLATFORM EXPECT_OPENVINO; do
  [ -n "${!v:-}" ] || fail "missing required env var: $v"
done
[ -x "$DIST_DIR/modeld" ] || fail "missing packaged launcher: $DIST_DIR/modeld"

# Smoke gate: the packaged binary must run and report the expected backends. The
# wrapper resolves the bundled native libs, so this also proves the link is sound.
echo "modeld-package-release: smoke -> $DIST_DIR/modeld version --json"
report=$("$DIST_DIR/modeld" version --json) || fail "packaged binary failed to run 'version'"
echo "$report"

reported_version=$(printf '%s' "$report" | sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' | head -1)
[ "$reported_version" = "$VERSION" ] || fail "version mismatch: binary reports '$reported_version', expected '$VERSION'"

have_backend() { printf '%s' "$report" | grep -q "\"$1\""; }
have_backend llama || fail "packaged binary does not report the 'llama' backend"
if [ "$EXPECT_OPENVINO" = "1" ]; then
  have_backend openvino || fail "EXPECT_OPENVINO=1 but packaged binary does not report the 'openvino' backend (release refuses to ship a reduced backend set)"
fi

llama_commit=$(printf '%s' "$report" | sed -n 's/.*"llama_cpp_commit": *"\([^"]*\)".*/\1/p' | head -1)
genai_version=$(printf '%s' "$report" | sed -n 's/.*"openvino_genai_version": *"\([^"]*\)".*/\1/p' | head -1)

backends_json=$(printf '%s' "$report" | tr -d '\n' | sed -n 's/.*"backends": *\(\[[^]]*\]\).*/\1/p' | tr -s ' ')
[ -n "$backends_json" ] || backends_json="[]"

cat > "$DIST_DIR/manifest.json" <<EOF
{
  "modeld_version": "$VERSION",
  "platform": "$PLATFORM",
  "backends": $backends_json,
  "llama_cpp_commit": "$llama_commit",
  "openvino_genai_version": "$genai_version",
  "openvino": $([ "$EXPECT_OPENVINO" = "1" ] && echo true || echo false),
  "built_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

mkdir -p "$RELEASE_OUT"
parent=$(CDPATH= cd -- "$(dirname -- "$DIST_DIR")" && pwd)
base=$(basename -- "$DIST_DIR")
archive="$RELEASE_OUT/$NAME.tar.gz"
tar -czf "$archive" -C "$parent" "$base"
( cd "$RELEASE_OUT" && sha256sum "$NAME.tar.gz" > "$NAME.tar.gz.sha256" )
echo "modeld-package-release: archive -> $archive"
echo "modeld-package-release: checksum -> $RELEASE_OUT/$NAME.tar.gz.sha256"
