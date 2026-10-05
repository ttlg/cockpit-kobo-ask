#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/install-kobo.sh [--url <relay-url>] [--no-eject]

Copies the KOReader plugin to a Kobo mounted over USB and writes its config.lua.

  --url <relay-url>  Relay URL the Kobo should call (default: http://<this Mac's Wi-Fi IP>:<port>)
  --no-eject         Leave the Kobo mounted after copying

Environment:
  KOBO_MOUNT         Kobo mount point (default: /Volumes/KOBOeReader)
  WIFI_INTERFACE     Interface used to detect the Mac's IP (default: en0)
USAGE
}

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
kobo_mount="${KOBO_MOUNT:-/Volumes/KOBOeReader}"
relay_config="$repo_dir/relay/config.json"
url=""
eject=true

while [ $# -gt 0 ]; do
  case "$1" in
    --url) url="$2"; shift 2 ;;
    --no-eject) eject=false; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 1 ;;
  esac
done

[ -d "$kobo_mount" ] || { echo "Kobo is not mounted at $kobo_mount. Connect it over USB and choose Connect on the Kobo." >&2; exit 1; }
[ -d "$kobo_mount/.adds/koreader/plugins" ] || { echo "KOReader is not installed on the Kobo. See README: Install NickelMenu and KOReader." >&2; exit 1; }
[ -f "$relay_config" ] || { echo "Missing $relay_config. Start the relay once to generate it." >&2; exit 1; }

port="$(node -e 'console.log(require(process.argv[1]).port)' "$relay_config")"
token="$(node -e 'console.log(require(process.argv[1]).token)' "$relay_config")"
if [ -z "$url" ]; then
  ip="$(ipconfig getifaddr "${WIFI_INTERFACE:-en0}" || true)"
  [ -n "$ip" ] || { echo "Could not detect this Mac's IP. Pass --url http://<ip>:$port." >&2; exit 1; }
  url="http://$ip:$port"
fi

plugin_dir="$kobo_mount/.adds/koreader/plugins/askterminal.koplugin"
mkdir -p "$plugin_dir"
cp "$repo_dir"/askterminal.koplugin/*.lua "$plugin_dir/"
cat > "$plugin_dir/config.lua" <<LUA
return {
    url = "$url",
    token = "$token",
    interval = 15,
    enabled = true,
}
LUA
dot_clean -m "$kobo_mount/.adds" 2>/dev/null || true
sync
echo "Installed the plugin. The Kobo will call $url."

if $eject; then
  diskutil eject "$kobo_mount"
  echo "Unplug the Kobo, then restart KOReader to load the plugin."
fi
