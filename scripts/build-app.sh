#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
app_dir="$repo_dir/app"
fonts_dir="$app_dir/fonts"
fonts_url="https://github.com/notofonts/noto-cjk/raw/main/Sans/SubsetOTF/JP"

mkdir -p "$fonts_dir"
for weight in Regular Bold; do
  font="$fonts_dir/NotoSansJP-$weight.otf"
  [ -s "$font" ] || curl -sfL -o "$font" "$fonts_url/NotoSansJP-$weight.otf"
done

cd "$app_dir"
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$app_dir/build/cockpit-kobo" ./cmd/cockpit-kobo
echo "Built $app_dir/build/cockpit-kobo"
