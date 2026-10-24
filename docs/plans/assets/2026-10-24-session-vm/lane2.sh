#!/bin/bash
# Lane 2 (.github/ci/compositor-tests.sh) inside the session VM, minus pacman.
# WLR_DRM_DEVICES is the vkms card, found by driver, because card numbering
# is not fixed (CI's script takes the first card).
set -uo pipefail
sudo modprobe vkms
cd ~/sysc-compositor
for c in /sys/class/drm/card[0-9]; do
	[ "$(basename "$(readlink -f "$c/device/driver")")" = faux_driver ] && WLR_DRM_DEVICES=/dev/dri/$(basename "$c")
done
export WLR_DRM_DEVICES GOMAXPROCS=2 SYSC_NESTED_TESTS=1 WLR_BACKENDS=drm LIBSEAT_BACKEND=noop \
	WLR_RENDERER=gles2 WLR_RENDERER_ALLOW_SOFTWARE=1 XDG_RUNTIME_DIR=$HOME/lane2-xdg
rm -rf "$XDG_RUNTIME_DIR"; install -d -m 0700 "$XDG_RUNTIME_DIR"
echo "WLR_DRM_DEVICES=$WLR_DRM_DEVICES"
pkg=${1:?package}; shift
go test -count=1 -v -timeout 15m "$pkg" "$@"
