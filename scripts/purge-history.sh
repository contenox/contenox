#!/usr/bin/env bash
# purge-history.sh — one-shot git history rewrite for the V1 release.
#
# Removes from ALL history:
#   1. Accidentally committed build binaries at the repo root
#      (contenox, contenox-cli, contenox-runtime, runtime, beam, vibe,
#      acp-stub-agent — matched by exact root path, blob type, and >1MB size,
#      so the runtime/ and beam-related SOURCE DIRECTORIES are never touched).
#   2. Historical website media now hosted on S3 or dead with retired products
#      (website/public demo media, website/assets/, scripts/demos videos).
#
# Prereqs: clean working tree (commit first!), git-filter-repo on PATH or
# GIT_FILTER_REPO pointing at the script.
# A full backup bundle is written next to the repo before anything is rewritten.
#
# After it finishes YOU still have to publish the rewrite:
#   git push --force origin main
#   git push --force origin --tags     # tags are rewritten too
# and every collaborator must re-clone (or hard-reset) — old clones share no
# history with the rewritten repo.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

FILTER_REPO="${GIT_FILTER_REPO:-$(command -v git-filter-repo || true)}"
if [[ -z "$FILTER_REPO" ]]; then
  echo "ERROR: git-filter-repo not found. Install it (pipx install git-filter-repo)" >&2
  echo "       or set GIT_FILTER_REPO=/path/to/git-filter-repo." >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "ERROR: working tree is not clean. Commit or stash everything first —" >&2
  echo "       filter-repo rewrites commits and will not protect pending work." >&2
  exit 1
fi

ORIGIN_URL="$(git remote get-url origin 2>/dev/null || true)"

echo "==> Repo size before:"
du -sh .git

# ── 1. Backup bundle (all refs) ─────────────────────────────────────────────
BUNDLE="../$(basename "$REPO_ROOT")-pre-purge-$(date +%Y%m%d-%H%M%S).bundle"
echo "==> Writing backup bundle: $BUNDLE"
git bundle create "$BUNDLE" --all
echo "    Restore any time with: git clone $BUNDLE restored-repo"

# ── 2. Collect the root-binary blob IDs ─────────────────────────────────────
# Exact root paths only (no '/'), blob objects only, >1MB — this is what keeps
# the runtime/ source directory (a tree, and small files) perfectly safe.
BLOBS_FILE="$(mktemp)"
git rev-list --objects --all \
  | awk '$2 ~ /^(contenox|contenox-cli|contenox-runtime|runtime|beam|vibe|acp-stub-agent)$/ {print $1}' \
  | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize)' \
  | awk '$1 == "blob" && $3 > 1048576 {print $2}' \
  | sort -u > "$BLOBS_FILE"

echo "==> Root binary blobs to strip: $(wc -l < "$BLOBS_FILE")"
[[ -s "$BLOBS_FILE" ]] || { echo "WARNING: no binary blobs found — already purged?"; }

# ── 3. Media paths to purge from history ────────────────────────────────────
# Dead-product media + media migrated to S3 (bucket
# contenox-website-assets-573643652148: media/ live, retired/ archive).
MEDIA_PATHS=(
  --path website/assets
  --path scripts/demos/video
  --path scripts/demos/video-modeld
  --path website/public/beam-demo.webm
  --path website/public/beam-video-cover.png
  --path website/public/beam-agent-view.png
  --path website/public/beam-login.png
  --path website/public/beam-new-chat.png
  --path website/public/beam-providers.png
  --path website/public/modeld-console.gif
  --path website/public/demo.webm
  --path website/public/hero.gif
  --path website/public/install.gif
  --path website/public/quickstart.gif
  --path website/public/chain-blocked.gif
  --path website/public/hitl-approve.gif
  --path website/public/agent-check.gif
  --path website/public/agent-permission-card.png
  --path website/public/agent-picker.png
  --path website/public/agent-slash-menu.png
  --path website/public/chain_flow_diagram.png
  --path website/public/hooks_architecture.png
  --path website/public/aionui-custom-agent.png
  --path website/public/hitl-plan-steps.png
  --path website/public/hitl-diff-review.png
  --path website/public/hitl-execution-thread.png
)

# ── 4. Rewrite ──────────────────────────────────────────────────────────────
echo "==> Running git filter-repo (this rewrites every affected commit)…"
"$FILTER_REPO" --force \
  --strip-blobs-with-ids "$BLOBS_FILE" \
  --invert-paths "${MEDIA_PATHS[@]}"

rm -f "$BLOBS_FILE"

# ── 5. Compact and verify ───────────────────────────────────────────────────
echo "==> Repacking…"
git reflog expire --expire=now --all
git gc --prune=now --aggressive --quiet

echo "==> Repo size after:"
du -sh .git

echo "==> Largest blobs remaining in history (should show no binaries/media):"
git rev-list --objects --all \
  | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' \
  | awk '/^blob/ && $3 >= 1000000 {printf "%8.1f MB  %s\n", $3/1048576, $4}' \
  | sort -rn | head -15

# filter-repo removes the origin remote on purpose; restore it but do not push.
if [[ -n "$ORIGIN_URL" ]]; then
  git remote add origin "$ORIGIN_URL" 2>/dev/null || true
  echo "==> Remote 'origin' restored: $ORIGIN_URL"
fi

cat <<'EOF'

Done. Nothing has been pushed. To publish the rewrite:

  git push --force origin main
  git push --force origin --tags

Then tell collaborators to re-clone — existing clones must not be merged back.
Backup bundle stays next to the repo until you delete it.
EOF
