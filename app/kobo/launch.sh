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
for attempt in $(seq 1 20); do
  ip addr show wlan0 2>/dev/null | grep -q "inet " && break
  [ "$attempt" = 1 ] && "$RUNTIME/fbink" -q -m -y -6 "Connecting to Wi-Fi..."
  sleep 1
done
resume() {
  for name in hindenburg sickel nickel; do
    pid=$(pidof "$name") && kill -CONT $pid
  done
}
trap resume EXIT INT TERM
for name in nickel sickel hindenburg; do
  pid=$(pidof "$name") && kill -STOP $pid
done
"$RUNTIME/cockpit-kobo" -config "$RUNTIME/config.json" -fonts "$RUNTIME/fonts" -fbink "$RUNTIME/fbink"
echo "app exit: $?"
