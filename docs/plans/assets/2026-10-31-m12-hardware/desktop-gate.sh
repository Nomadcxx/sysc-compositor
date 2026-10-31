#!/bin/bash
# sysc-2081 on the desktop: gamma control on the physical monitor, and explicit
# sync presented through DRM on NVIDIA.
#
# 1. sudo pacman -S gammastep
# 2. Ctrl+Alt+F2, log in on the console (the Niri session waits on tty1).
# 3. Run this script from the sysc-compositor checkout you want to test.
#
# It builds sysc-compositor from this checkout, then the screen shows sysc for
# about 40 s: a spinning cube, then an orange tint, a few seconds of black, the
# tint again, and normal colours. The console then asks what you saw. Ctrl+Alt+F1
# returns to Niri. Results go to ~/.cache/sysc-hw-gate/<time>/.
set -u
REPO=$(cd "$(dirname "$0")/../../../.." && pwd)
OUT=${XDG_CACHE_HOME:-$HOME/.cache}/sysc-hw-gate/$(date +%Y%m%d-%H%M%S)
mkdir -p "$OUT"
for tool in gammastep modetest vkcube wayland-info niri go; do
	command -v "$tool" > /dev/null || { echo "missing $tool (gammastep: sudo pacman -S gammastep)"; exit 1; }
done
[ "$(cat /sys/module/nvidia_drm/parameters/modeset 2>/dev/null)" = Y ] ||
	echo "warning: nvidia_drm modeset is not Y; wlroots needs it"

git -C "$REPO" rev-parse --short HEAD > "$OUT/commit"
echo "Building sysc-compositor at $(cat "$OUT/commit")..."
mkdir -p "$REPO/.gotmp"
(cd "$REPO" && CGO_CFLAGS_ALLOW=-fno-strict-overflow GOTMPDIR="$REPO/.gotmp" TMPDIR="$REPO/.gotmp" \
	GOMAXPROCS=2 go build -o "$OUT/sysc-compositor" ./cmd/sysc-compositor)
built=$?
rm -rf "$REPO/.gotmp"
[ $built -eq 0 ] || exit 1
# No spawn-at-startup: nothing but the test clients runs in this session.
printf 'screenshot-path "%s/window.png"\n' "$OUT" > "$OUT/config.kdl"

: > "$OUT/steps.log"
step() { echo "$(date +%T) $*" >> "$OUT/steps.log"; }
snap() { modetest -M nvidia-drm -p > "$OUT/$1.txt" 2>&1; step "$1"; }
lut() { awk '/[0-9]+ GAMMA_LUT:/{f=1;next} f&&/^\t[0-9]+ [A-Z_]+:/{f=0} f' "$OUT/$1.txt" | md5sum | cut -c1-8; }

snap 0-before
timeout 120 "$OUT/sysc-compositor" -c "$OUT/config.kdl" > "$OUT/sysc.log" 2>&1 &
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
step "socket=$SOCK display=$WAYLAND_DISPLAY"
sleep 2
wayland-info > "$OUT/wayland-info.txt" 2>&1
snap 1-sysc

# Explicit sync: vkcube on NVIDIA, presented by DRM, and a window screenshot
# that waits on the synced buffer's acquire point.
WAYLAND_DEBUG=client timeout 10 vkcube --wsi wayland > "$OUT/vkcube.log" 2>&1 &
VK=$!
sleep 5
niri msg action screenshot-window >> "$OUT/steps.log" 2>&1
wait $VK
step "vkcube done"

gammastep -m wayland -O 3000 > "$OUT/gammastep.log" 2>&1 &
GS=$!
sleep 3
kill -0 $GS 2> /dev/null && step "gammastep running"
snap 2-applied
niri msg action power-off-monitors >> "$OUT/steps.log" 2>&1
sleep 4
snap 3-off
niri msg action power-on-monitors >> "$OUT/steps.log" 2>&1
sleep 4
snap 4-on
kill $GS
wait $GS 2> /dev/null
sleep 3
snap 5-restored
kill $SYSC
wait $SYSC 2> /dev/null
trap - EXIT
sleep 1
snap 6-after

{
	echo "commit $(cat "$OUT/commit")"
	echo "syncobj manager: $(grep -c wp_linux_drm_syncobj_manager_v1 "$OUT/wayland-info.txt")"
	echo "gamma manager: $(grep -c zwlr_gamma_control_manager_v1 "$OUT/wayland-info.txt")"
	echo "vkcube acquire points: $(grep -c 'wp_linux_drm_syncobj_surface_v1#.*set_acquire_point' "$OUT/vkcube.log")"
	echo "vkcube release points: $(grep -c 'wp_linux_drm_syncobj_surface_v1#.*set_release_point' "$OUT/vkcube.log")"
	echo "window screenshot: $([ -s "$OUT/window.png" ] && echo written || echo missing)"
	for s in 0-before 1-sysc 2-applied 3-off 4-on 5-restored 6-after; do
		echo "GAMMA_LUT $s $(lut $s)"
	done
} > "$OUT/summary.txt"

clear
ask() { read -r -p "$1 [y/n] " a; echo "$1 $a" >> "$OUT/answers.txt"; }
: > "$OUT/answers.txt"
ask "Did the cube spin smoothly, without tearing or stalls?"
ask "Did the screen turn orange (warm) after the cube?"
ask "After the screen went black and came back, was it still orange?"
ask "Did it return to normal colours before sysc exited?"
cat "$OUT/summary.txt"
echo "Results: $OUT. Ctrl+Alt+F1 returns to Niri."
