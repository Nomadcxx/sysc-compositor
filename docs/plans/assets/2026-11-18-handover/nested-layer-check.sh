#!/bin/bash
# Nested live check for layer stacking (used for PR #117, sysc-2312).
# Runs a sysc-compositor build nested in the laptop session with a red
# waybar on the Top layer and a kitty, drives fullscreen and overview over
# IPC, and takes a grim shot of the nested output after each step.
#
# Laptop only (pre-approved test bench), never on the owner's desktop.
# The build must contain #116 (sysc-2296), or WL-1 fails its swapchain test
# and nothing maps. kitty there is 75% opaque, so a correctly covered bar
# still shows faintly through a fullscreen kitty.
#
# Usage: nested-layer-check.sh /path/to/sysc-compositor OUTDIR
set -u
bin=${1:?usage: nested-layer-check.sh BINARY OUTDIR}
out=${2:?usage: nested-layer-check.sh BINARY OUTDIR}
rm -rf "$out"; mkdir -p "$out"
export XDG_RUNTIME_DIR=/run/user/1000

cat >"$out/min.kdl" <<'KDL'
animations { off; }
KDL
cat >"$out/waybar.json" <<'JSON'
{ "layer": "top", "position": "top", "height": 30, "modules-left": ["custom/label"],
  "custom/label": { "format": "TOP LAYER BAR" } }
JSON
cat >"$out/waybar.css" <<'CSS'
* { font-size: 16px; }
window#waybar { background: #d02020; color: #ffffff; }
CSS

pids=()
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null; done; sleep 1; kill "$(cat "$out/pid")" 2>/dev/null; }
trap cleanup EXIT

# Kill by pid only: a pkill -f pattern that matches this command line kills the ssh shell.
env -u NIRI_SOCKET WAYLAND_DISPLAY=wayland-0 setsid "$bin" -c "$out/min.kdl" >"$out/log" 2>&1 &
echo $! >"$out/pid"
for _ in $(seq 50); do grep -q 'WAYLAND_DISPLAY=' "$out/log" && break; sleep 0.2; done
nwd=$(grep -o 'WAYLAND_DISPLAY=[^ ]*' "$out/log" | head -1 | cut -d= -f2)
nsock=$(grep -o 'NIRI_SOCKET=[^ ]*' "$out/log" | head -1 | cut -d= -f2)
[ -n "$nwd" ] || { cat "$out/log"; exit 1; }
echo "nested display=$nwd socket=$nsock"

msg() { NIRI_SOCKET=$nsock "$bin" msg "$@"; }
shot() { sleep 1.5; WAYLAND_DISPLAY=$nwd grim "$out/$1.png" && echo "shot $1"; }

WAYLAND_DISPLAY=$nwd setsid waybar -c "$out/waybar.json" -s "$out/waybar.css" >"$out/waybar.log" 2>&1 & pids+=($!)
sleep 1.5
WAYLAND_DISPLAY=$nwd setsid kitty --class fstest -o background=#1e3a5f -o foreground=#ffffff -o font_size=14 \
	sh -c 'echo FULLSCREEN KITTY TOP ROW; exec sleep 600' >"$out/kitty.log" 2>&1 & pids+=($!)
shot 1-tiled
msg action fullscreen-window; shot 2-fullscreen
msg action toggle-overview; shot 3-overview-open
msg action toggle-overview; shot 4-overview-closed
msg action fullscreen-window; shot 5-left-fullscreen
msg --json layers >"$out/layers.json" 2>&1
# Bar under kitty: pixel (5,5) of 2-fullscreen.png is kitty's colour, not #d02020.
