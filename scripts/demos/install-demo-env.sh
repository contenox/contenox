# Sourced by install.tape (hidden): builds a throwaway HOME and a minimal PATH
# without sudo, so install.sh takes its ~/.local/bin fallback, and stages the
# repo's install.sh into a scratch workdir.
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export DEMO_HOME=/tmp/contenox-demo-home
rm -rf "$DEMO_HOME" /tmp/contenox-demo-ws
mkdir -p "$DEMO_HOME/bin" /tmp/contenox-demo-ws
for t in sh bash uname curl grep sed mktemp chmod mv cp dirname mkdir cat ls head tail sleep nohup kill rm clear; do
  p="$(command -v "$t" 2>/dev/null)" && ln -sf "$p" "$DEMO_HOME/bin/" || true
done
cp "$REPO_DIR/website/public/install.sh" /tmp/contenox-demo-ws/
export HOME="$DEMO_HOME"
export PATH="$DEMO_HOME/bin"
cd /tmp/contenox-demo-ws
