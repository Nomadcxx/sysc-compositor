#!/bin/bash
# sysc-2081 item 1: gamma control on the laptop's eDP-1.
# Run from a tty2 console login (Ctrl+Alt+F2), after: sudo pacman -S gammastep
# The screen shows sysc for about 25 s, then this console returns with questions.
set -u
D=~/sysc-compositor-hw-20261031
OUT=$D/gamma-gate
mkdir -p "$OUT"
: > "$OUT/empty.kdl"
snap() { modetest -M i915 -p > "$OUT/$1.txt" 2>&1; echo "$(date +%T) $1" >> "$OUT/steps.log"; }
: > "$OUT/steps.log"

snap 0-before
timeout 60 "$D/bin/sysc-compositor" -c "$OUT/empty.kdl" > "$OUT/sysc.log" 2>&1 &
SYSC=$!
trap 'kill $SYSC 2>/dev/null' EXIT
# Niri's own socket shares this runtime directory: match sysc's PID, which is
# timeout's child, not $SYSC.
SOCK=
for _ in $(seq 50); do
	PID=$(pgrep -P $SYSC | head -1)
	[ -n "$PID" ] && SOCK=$(ls "$XDG_RUNTIME_DIR"/niri.*."$PID".sock 2>/dev/null | head -1)
	[ -n "$SOCK" ] && break
	sleep 0.2
done
[ -n "$SOCK" ] || { kill $SYSC; echo "sysc-compositor did not start; see $OUT/sysc.log"; exit 1; }
WAYLAND_DISPLAY=$(basename "$SOCK" | cut -d. -f2)
export NIRI_SOCKET=$SOCK WAYLAND_DISPLAY
echo "socket=$SOCK display=$WAYLAND_DISPLAY" >> "$OUT/steps.log"
sleep 2
wayland-info 2>&1 | grep -i gamma >> "$OUT/steps.log"
snap 1-sysc

gammastep -m wayland -O 3000 > "$OUT/gammastep.log" 2>&1 &
GS=$!
sleep 3
kill -0 $GS 2>/dev/null && echo "gammastep running" >> "$OUT/steps.log"
snap 2-applied

niri msg action power-off-monitors >> "$OUT/steps.log" 2>&1
sleep 4
snap 3-off
niri msg action power-on-monitors >> "$OUT/steps.log" 2>&1
sleep 4
snap 4-on

kill $GS; wait $GS 2>/dev/null
sleep 3
snap 5-restored
kill $SYSC; wait $SYSC 2>/dev/null
sleep 1
snap 6-after
trap - EXIT

clear
ask() { read -r -p "$1 [y/n] " a; echo "$1 $a" >> "$OUT/answers.txt"; }
: > "$OUT/answers.txt"
ask "Did the screen turn orange (warm) a few seconds after sysc started?"
ask "After the screen went black and came back, was it still orange?"
ask "Did it return to normal colours before sysc exited?"
echo "Done. Results are in $OUT. Switch back with Ctrl+Alt+F1."
