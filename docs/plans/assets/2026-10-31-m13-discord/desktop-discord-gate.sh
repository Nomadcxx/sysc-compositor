#!/bin/bash
# M13 Discord gate (sysc-1278) on the desktop: the official Discord client
# through X11 (xwayland-satellite) under a nested sysc-compositor, on a network
# Discord accepts. The laptop's VPN exit gets HTTP 403 from discord.com.
#
# Run it yourself from a terminal in your Niri session; it opens a nested sysc
# window. Quit Discord in Niri first: Discord is single-instance and would hand
# the launch to the running copy. Nothing else is spawned (empty config).
#
# In the nested window, check that Discord logs in and loads your servers,
# that typing into a message box works, that clicking between Discord and the
# foot terminal moves focus, and that copy/paste works both ways. Then press
# Enter here. Results go to ~/.cache/sysc-discord-gate/<time>/.
set -u
REPO=$(cd "$(dirname "$0")/../../../.." && pwd)
OUT=${XDG_CACHE_HOME:-$HOME/.cache}/sysc-discord-gate/$(date +%Y%m%d-%H%M%S)
mkdir -p "$OUT"
for tool in discord foot niri go grim; do
	command -v "$tool" > /dev/null || { echo "missing $tool"; exit 1; }
done
pgrep -x Discord > /dev/null && { echo "Quit Discord in Niri first."; exit 1; }
[ -n "${WAYLAND_DISPLAY:-}" ] || { echo "Run this from your Niri session."; exit 1; }

git -C "$REPO" rev-parse --short HEAD > "$OUT/commit"
echo "Building sysc-compositor at $(cat "$OUT/commit")..."
mkdir -p "$REPO/.gotmp"
(cd "$REPO" && CGO_CFLAGS_ALLOW=-fno-strict-overflow GOTMPDIR="$REPO/.gotmp" TMPDIR="$REPO/.gotmp" \
	GOMAXPROCS=2 go build -o "$OUT/sysc-compositor" ./cmd/sysc-compositor)
built=$?
rm -rf "$REPO/.gotmp"
[ $built -eq 0 ] || exit 1
: > "$OUT/gate.kdl"

(unset DISPLAY NIRI_SOCKET; exec timeout 1800 "$OUT/sysc-compositor" -c "$OUT/gate.kdl") > "$OUT/sysc.log" 2>&1 &
T=$!
trap 'kill $T 2>/dev/null' EXIT
SOCK=
for _ in $(seq 50); do
	PID=$(pgrep -P $T | head -1)
	[ -n "$PID" ] && SOCK=$(ls "$XDG_RUNTIME_DIR"/niri.*."$PID".sock 2>/dev/null | head -1)
	[ -n "$SOCK" ] && break
	sleep 0.2
done
[ -n "$SOCK" ] || { echo "sysc-compositor did not start; see $OUT/sysc.log"; exit 1; }
SYSC_DISPLAY=$(basename "$SOCK" | cut -d. -f2)
echo "sysc pid=$PID socket=$SOCK display=$SYSC_DISPLAY" > "$OUT/steps.log"
NIRI_SOCKET=$SOCK niri msg action spawn -- foot >> "$OUT/steps.log" 2>&1
NIRI_SOCKET=$SOCK niri msg action spawn -- discord --ozone-platform=x11 >> "$OUT/steps.log" 2>&1

read -r -p "Check Discord in the nested window, then press Enter here. "
NIRI_SOCKET=$SOCK niri msg -j windows > "$OUT/windows.json" 2>&1
WAYLAND_DISPLAY=$SYSC_DISPLAY grim "$OUT/shot.png" >> "$OUT/steps.log" 2>&1
ps -o pid=,args= --ppid "$PID" > "$OUT/children.txt"
ask() { read -r -p "$1 [y/n] " a; echo "$1 $a" >> "$OUT/answers.txt"; }
: > "$OUT/answers.txt"
ask "Did Discord log in and load your servers and messages?"
ask "Did typing into a message box work?"
ask "Did clicking between Discord and foot move focus each time?"
ask "Did copy/paste work in both directions?"
NIRI_SOCKET=$SOCK niri msg action quit --skip-confirmation >> "$OUT/steps.log" 2>&1
wait $T 2> /dev/null
trap - EXIT
echo "satellite: $(grep -c xwayland-satellite "$OUT/children.txt")" >> "$OUT/steps.log"
cat "$OUT/steps.log" "$OUT/answers.txt"
echo "Results: $OUT"
