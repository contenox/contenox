#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: dev-install.sh --cli PATH [--modeld DIR] [--uninstall]" >&2
  exit 2
}

cli=
modeld=
uninstall=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --cli) [ "$#" -ge 2 ] || usage; cli=$2; shift 2 ;;
    --modeld) [ "$#" -ge 2 ] || usage; modeld=$2; shift 2 ;;
    --uninstall) uninstall=1; shift ;;
    *) usage ;;
  esac
done

data_root=${CONTENOX_DATA_ROOT:-$HOME/.contenox}
local_target=$HOME/.local/bin/contenox
record=$data_root/dev-install-cli-path

if [ "$uninstall" = 1 ]; then
  targets=$local_target
  if [ -f "$record" ]; then
    recorded=$(cat "$record")
    [ -n "$recorded" ] && targets="$targets $recorded"
  fi
  for target in $targets; do
    if [ -L "$target" ]; then
      rm -f "$target"
      echo "Removed $target"
    fi
    if [ -e "$target.pre-dev-install" ]; then
      mv "$target.pre-dev-install" "$target"
      echo "Restored $target"
    fi
  done
  rm -f "$record"
  exit 0
fi

[ -n "$cli" ] || usage
[ -x "$cli" ] || { echo "built CLI is not executable: $cli" >&2; exit 1; }
cli=$(realpath "$cli")

mkdir -p "$HOME/.local/bin" "$data_root"
effective=$(command -v contenox 2>/dev/null || true)
target=$local_target
if [ -n "$effective" ] && [ "$effective" != "$local_target" ]; then
  case "$effective" in
    "$HOME"/*) target=$effective ;;
    *)
      echo "contenox resolves outside your home directory: $effective" >&2
      echo "Put $HOME/.local/bin before it on PATH, then rerun task dev-install." >&2
      exit 1
      ;;
  esac
fi

mkdir -p "$(dirname "$target")"
if [ -e "$target" ] && [ ! -L "$target" ]; then
  backup=$target.pre-dev-install
  suffix=0
  while [ -e "$backup" ] || [ -L "$backup" ]; do
    suffix=$((suffix + 1))
    backup=$target.pre-dev-install.$suffix
  done
  mv "$target" "$backup"
  echo "Backed up $target -> $backup"
fi
ln -sfn "$cli" "$target"
ln -sfn "$cli" "$local_target"
printf '%s\n' "$target" > "$record"

resolved=$(realpath "$(command -v contenox)")
[ "$resolved" = "$cli" ] || {
  echo "dev install is shadowed: contenox resolves to $(command -v contenox), expected $target" >&2
  exit 1
}
echo "Installed contenox -> $target"

if [ -z "$modeld" ]; then
  exit 0
fi

[ -x "$modeld/modeld" ] || { echo "packaged modeld launcher is missing: $modeld/modeld" >&2; exit 1; }
report=$($modeld/modeld version --json)
printf '%s' "$report" | grep -q '"llama"' || { echo "packaged modeld lacks llama" >&2; exit 1; }
if [ "$(go env GOOS)" != darwin ]; then
  printf '%s' "$report" | grep -q '"openvino"' || { echo "packaged modeld lacks OpenVINO" >&2; exit 1; }
fi

platform=$(go env GOOS)-$(go env GOARCH)
install_dir=$data_root/modeld/dev/$platform
staging=$data_root/modeld/.dev-$platform-$$
rm -rf "$staging"
mkdir -p "$staging"
cp -a "$modeld"/. "$staging"/
"$staging/modeld" version --json >/dev/null
rm -rf "$install_dir"
mkdir -p "$(dirname "$install_dir")"
mv "$staging" "$install_dir"
pointer=$data_root/modeld/current
pointer_tmp=$data_root/modeld/.current-$$
printf '%s\n' "$install_dir" > "$pointer_tmp"
mv "$pointer_tmp" "$pointer"

installed_report=$($install_dir/modeld version --json)
printf '%s\n' "$installed_report"
if [ -f "$modeld/lib/llamacpp/libggml-cuda.so" ]; then
  [ -f "$install_dir/lib/llamacpp/libggml-cuda.so" ] || { echo "CUDA backend was lost during install" >&2; exit 1; }
  echo "Installed modeld with llama.cpp CUDA and OpenVINO -> $install_dir"
else
  echo "Installed modeld with llama.cpp and OpenVINO -> $install_dir"
fi
