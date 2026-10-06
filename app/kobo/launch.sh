#!/bin/sh
export PATH=/sbin:/usr/sbin:/bin:/usr/bin:$PATH
SOURCE=/mnt/onboard/.adds/cockpit
RUNTIME=/tmp/cockpit
if [ "$1" != "--from-tmp" ]; then
  rm -rf "$RUNTIME"
  mkdir -p "$RUNTIME"
  cp -r "$SOURCE/cockpit-kobo" "$SOURCE/fbink" "$SOURCE/config.json" "$SOURCE/fonts" "$SOURCE/launch.sh" "$RUNTIME/"
  cd /
  exec /bin/sh "$RUNTIME/launch.sh" --from-tmp
fi
cd /
exec >"$RUNTIME/launch.log" 2>&1
echo "launch $(date)"

wait_for_wifi() {
  for attempt in $(seq 1 20); do
    ip addr show wlan0 2>/dev/null | grep -q "inet " && return 0
    [ "$attempt" = 1 ] && "$RUNTIME/fbink" -q -m -y -6 "Connecting to Wi-Fi..."
    sleep 1
  done
  return 1
}

stop_nickel() {
  nickel_pid=$(pidof -s nickel)
  [ -n "$nickel_pid" ] && tr '\0' '\n' <"/proc/$nickel_pid/environ" >"$RUNTIME/nickel.env"
  sync
  for name in nickel hindenburg sickel fickel strickel fontickel adobehost foxitpdf iink; do
    pid=$(pidof "$name") && kill -CONT $pid && kill -TERM $pid
  done
  for attempt in $(seq 1 40); do
    pidof nickel >/dev/null || break
    usleep 250000
  done
  rm -f /tmp/nickel-hardware-status
}

stop_wifi() {
  interface=${INTERFACE:-wlan0}
  dhcpcd -d -k "$interface"
  killall -q -TERM udhcpc default.script
  wpa_cli -i "$interface" terminate
  ifconfig "$interface" down
  [ -e /dev/wmtWifi ] && echo 0 >/dev/wmtWifi
}

start_nickel() {
  if [ -f "$RUNTIME/nickel.env" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] && export "$line"
    done <"$RUNTIME/nickel.env"
  fi
  stop_wifi
  [ -x /etc/init.d/on-animator.sh ] && /etc/init.d/on-animator.sh &
  rm -f /tmp/nickel-hardware-status
  mkfifo /tmp/nickel-hardware-status
  sync
  cd /
  /usr/local/Kobo/hindenburg &
  LIBC_FATAL_STDERR_=1 /usr/local/Kobo/nickel -platform kobo -skipFontLoad &
  udevadm trigger &
}

finish() {
  echo "restarting nickel"
  cp "$RUNTIME/launch.log" "$SOURCE/last-run.log"
  start_nickel
}

wait_for_wifi || echo "Wi-Fi is not connected"
trap finish EXIT
trap 'exit 1' INT TERM
stop_nickel
"$RUNTIME/cockpit-kobo" -config "$RUNTIME/config.json" -fonts "$RUNTIME/fonts" -fbink "$RUNTIME/fbink"
echo "app exit: $?"
