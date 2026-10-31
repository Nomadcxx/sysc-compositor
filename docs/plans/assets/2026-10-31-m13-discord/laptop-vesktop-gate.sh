#!/bin/bash
# M13 Discord gate on the laptop: Discord's web app (Vesktop) through X11 under
# a nested sysc-compositor in the laptop's Niri session. A throwaway profile
# keeps the owner's account and running Vesktop out of it.
set -u
D=~/sysc-compositor-m13-discord-20261031
OUT=$D/discord-gate
rm -rf "$OUT"
mkdir -p "$OUT/profile"
: > "$OUT/gate.kdl"
cp ~/.config/vesktop/settings.json ~/.config/vesktop/state.json "$OUT/profile/"
export XDG_RUNTIME_DIR=/run/user/1000 WAYLAND_DISPLAY=wayland-1
unset DISPLAY NIRI_SOCKET
HOST_SOCK=$(ls "$XDG_RUNTIME_DIR"/niri.wayland-1.*.sock | head -1)

timeout 120 "$D/bin/sysc-compositor" -c "$OUT/gate.kdl" > "$OUT/sysc.log" 2>&1 &
T=$!
SOCK=
for _ in $(seq 50); do
	PID=$(pgrep -P $T | head -1)
	[ -n "$PID" ] && SOCK=$(ls "$XDG_RUNTIME_DIR"/niri.*."$PID".sock 2>/dev/null | head -1)
	[ -n "$SOCK" ] && break
	sleep 0.2
done
[ -n "$SOCK" ] || { echo "no sysc socket"; kill $T; exit 1; }
SYSC_DISPLAY=$(basename "$SOCK" | cut -d. -f2)
echo "sysc pid=$PID socket=$SOCK display=$SYSC_DISPLAY" > "$OUT/steps.log"
sleep 2

NIRI_SOCKET=$SOCK niri msg action spawn -- env VENCORD_USER_DATA_DIR="$OUT/profile" \
	vesktop --ozone-platform=x11 --user-data-dir="$OUT/profile" >> "$OUT/steps.log" 2>&1
sleep 30

NIRI_SOCKET=$SOCK niri msg -j windows > "$OUT/windows.json" 2>&1
WAYLAND_DISPLAY=$SYSC_DISPLAY grim "$OUT/shot.png" >> "$OUT/steps.log" 2>&1
ps -o pid=,ppid=,args= --ppid "$PID" > "$OUT/children.txt"
VPID=
for p in $(pgrep -f -- "--user-data-dir=$OUT/profile"); do
	grep -qa -- "--type=" /proc/"$p"/cmdline || { VPID=$p; break; }
done
echo "vesktop main pid=$VPID" >> "$OUT/steps.log"
if [ -n "$VPID" ]; then
	tr '\0' '\n' < /proc/"$VPID"/environ | grep -E '^(DISPLAY|WAYLAND_DISPLAY)=' >> "$OUT/steps.log"
	tr '\0' ' ' < /proc/"$VPID"/cmdline >> "$OUT/steps.log"; echo >> "$OUT/steps.log"
fi
ss -tnp 2>/dev/null | grep -c vesktop | sed 's/^/vesktop tcp connections: /' >> "$OUT/steps.log"
NIRI_SOCKET=$HOST_SOCK niri msg -j focused-window > "$OUT/host-focus.json" 2>&1

NIRI_SOCKET=$SOCK niri msg action quit --skip-confirmation >> "$OUT/steps.log" 2>&1
sleep 3
for p in $(pgrep -f -- "--user-data-dir=$OUT/profile"); do kill "$p" 2>/dev/null; done
wait $T 2>/dev/null
echo "sysc exit=$?" >> "$OUT/steps.log"
cat "$OUT/steps.log"
