#!/usr/bin/env bash
# Convert vhs-rendered MP4s in out/ into compact GIFs.
# Collapses long static waits (model thinking, cold starts) with mpdecimate,
# holds the final frame so the answer stays readable, and uses a two-pass
# palette for quality. Usage: ./mkgif.sh [name ...]  (default: all out/*.mp4)
set -euo pipefail
cd "$(dirname "$0")/out"

names=("$@")
if [ ${#names[@]} -eq 0 ]; then
  names=()
  for f in *.mp4; do names+=("${f%.mp4}"); done
fi

# Crop away unused bottom space per demo (source is 1200x700).
crop_h() {
  case "$1" in
    hero) echo 280 ;;
    quickstart) echo 400 ;;
    hitl-approve) echo 520 ;;
    chain-blocked) echo 330 ;;
    *) echo 700 ;;
  esac
}

for n in "${names[@]}"; do
  h=$(crop_h "$n")
  ffmpeg -y -loglevel error -i "$n.mp4" \
    -vf "crop=1200:$h:0:0,mpdecimate=max=48,setpts=N/(20*TB),tpad=stop_mode=clone:stop_duration=3,fps=15,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
    "$n.gif"
  echo "$n.gif: $(du -h "$n.gif" | cut -f1) ($(ffprobe -v error -show_entries format=duration -of csv=p=0 "$n.gif" 2>/dev/null || echo '?')s)"
done
