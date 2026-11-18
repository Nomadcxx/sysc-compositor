#!/bin/bash
# run-nested.sh PKG 'TEST REGEX': run named tests nested in a private headless
# sysc-compositor host, so nothing reaches a screen and the nested outputs are
# Wayland-backend (WL-n) ones. Run from a tree root after `make build` and
# `go generate ./internal/wlr`. One invocation per batch: never loop it, and run
# it only where nested tests are approved (the laptop).
set -u
export CGO_CFLAGS_ALLOW=-fno-strict-overflow
rt=$(mktemp -d /tmp/sysc-nested-XXXXXX)
chmod 700 "$rt"
env -u WAYLAND_DISPLAY -u DISPLAY XDG_RUNTIME_DIR="$rt" WLR_BACKENDS=headless \
	WLR_RENDER_DRM_DEVICE=/dev/dri/renderD128 \
	./bin/sysc-compositor -c /dev/null >"$rt/host.log" 2>&1 &
host=$!
sock=
for _ in $(seq 50); do
	sock=$(ls "$rt" | grep -E '^wayland-[0-9]+$' | head -1)
	[ -n "$sock" ] && break
	sleep 0.2
done
if [ -z "$sock" ]; then
	echo "host did not start"
	tail -20 "$rt/host.log"
	kill "$host" 2>/dev/null
	exit 1
fi
env -u DISPLAY XDG_RUNTIME_DIR="$rt" WAYLAND_DISPLAY="$sock" SYSC_NESTED_TESTS=1 \
	timeout 90s env GOMAXPROCS=2 go test -count=1 "$1" -run "$2" -v
status=$?
kill "$host" 2>/dev/null
wait "$host" 2>/dev/null
rm -rf "$rt"
exit "$status"
