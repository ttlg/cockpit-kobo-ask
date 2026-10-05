#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/install-app.sh [--url <relay-url>] [--language en|ja] [--fbink <path>] [--no-eject]

Installs the native AGI Cockpit app on a Kobo mounted over USB and adds it to NickelMenu.
Run scripts/build-app.sh first.

  --url <relay-url>   Relay URL the Kobo should call (default: http://<this Mac's Wi-Fi IP>:<port>)
  --language en|ja    Interface language (default: en)
  --fbink <path>      FBInk binary to install (default: the one bundled with KOReader on the Kobo)
  --no-eject          Leave the Kobo mounted after copying

Environment:
  KOBO_MOUNT          Kobo mount point (default: /Volumes/KOBOeReader)
  WIFI_INTERFACE      Interface used to detect the Mac's IP (default: en0)
USAGE
}

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
app_dir="$repo_dir/app"
kobo_mount="${KOBO_MOUNT:-/Volumes/KOBOeReader}"
relay_config="$repo_dir/relay/config.json"
url=""
language="en"
fbink=""
eject=true

while [ $# -gt 0 ]; do
  case "$1" in
    --url) url="$2"; shift 2 ;;
    --language) language="$2"; shift 2 ;;
    --fbink) fbink="$2"; shift 2 ;;
    --no-eject) eject=false; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 1 ;;
  esac
done

[ -d "$kobo_mount" ] || { echo "Kobo is not mounted at $kobo_mount. Connect it over USB and choose Connect on the Kobo." >&2; exit 1; }
[ -x "$app_dir/build/cockpit-kobo" ] || { echo "Missing app/build/cockpit-kobo. Run scripts/build-app.sh first." >&2; exit 1; }
[ -f "$relay_config" ] || { echo "Missing $relay_config. Start the relay once to generate it." >&2; exit 1; }
[ -d "$kobo_mount/.adds/nm" ] || { echo "NickelMenu is not installed on the Kobo. See README." >&2; exit 1; }
fbink="${fbink:-$kobo_mount/.adds/koreader/fbink}"
[ -f "$fbink" ] || { echo "FBInk not found at $fbink. Install KOReader or pass --fbink <path>." >&2; exit 1; }

port="$(node -e 'console.log(require(process.argv[1]).port)' "$relay_config")"
token="$(node -e 'console.log(require(process.argv[1]).token)' "$relay_config")"
if [ -z "$url" ]; then
  ip="$(ipconfig getifaddr "${WIFI_INTERFACE:-en0}" || true)"
  [ -n "$ip" ] || { echo "Could not detect this Mac's IP. Pass --url http://<ip>:$port." >&2; exit 1; }
  url="http://$ip:$port"
fi

target="$kobo_mount/.adds/cockpit"
staged_fbink="$(mktemp)"
cp "$fbink" "$staged_fbink"
rm -rf "$target"
mkdir -p "$target/fonts"
cp -X "$app_dir/build/cockpit-kobo" "$app_dir/kobo/launch.sh" "$target/"
cp -X "$staged_fbink" "$target/fbink"
rm -f "$staged_fbink"
cp -X "$app_dir"/fonts/NotoSansJP-*.otf "$target/fonts/"
node -e 'console.log(JSON.stringify({ url: process.argv[1], token: process.argv[2], interval: 15, language: process.argv[3] }, null, 2))' "$url" "$token" "$language" > "$target/config.json"
cp -X "$app_dir/kobo/nickelmenu" "$kobo_mount/.adds/nm/cockpit"
find "$target" "$kobo_mount/.adds/nm" -name '._*' -delete
sync

verify() {
  cmp -s "$1" "$2" || { echo "Verification failed for $(basename "$2"). Run the script again." >&2; exit 1; }
}
verify "$app_dir/build/cockpit-kobo" "$target/cockpit-kobo"
verify "$app_dir/kobo/launch.sh" "$target/launch.sh"
verify "$app_dir/kobo/nickelmenu" "$kobo_mount/.adds/nm/cockpit"
for font in "$app_dir"/fonts/NotoSansJP-*.otf; do
  verify "$font" "$target/fonts/$(basename "$font")"
done
echo "Installed AGI Cockpit. The Kobo will call $url."

if $eject; then
  diskutil eject "$kobo_mount"
  echo "Unplug the Kobo. If AGI Cockpit is missing from NickelMenu, restart the Kobo."
fi
