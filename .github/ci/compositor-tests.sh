#!/bin/bash
# Lane 2 of CI, run inside archlinux:latest with the runner's /dev/dri. It
# builds real compositors, so it belongs only on a machine with no desktop:
# never run it on a developer machine.
set -euo pipefail

pacman -Syu --noconfirm --needed git gcc pkgconf go wlroots0.20 scenefx wayland \
	wayland-protocols libxkbcommon xkeyboard-config mesa seatd \
	weston wayland-utils foot swaybg waybar wofi ttf-dejavu >/dev/null
git config --global --add safe.directory "$PWD"
install -d -m 0700 "$XDG_RUNTIME_DIR"
# vkms has a primary node only; wlroots renders on it when the DRM backend
# hands it over. The container has no udev, so the device is named.
WLR_DRM_DEVICES=$(ls /dev/dri/card* | head -n1)
export WLR_DRM_DEVICES
ls -l /dev/dri
echo "WLR_DRM_DEVICES=$WLR_DRM_DEVICES"
go generate ./internal/wlr

# A test skips when it cannot build a compositor or find its client. Here a
# skip means the lane checked less than it claims, so any skip fails it.
status=0
for pkg in ./internal/wlr ./internal/server; do
	log=$(mktemp)
	go test -count=1 -v -timeout 15m "$pkg" 2>&1 | tee "$log" || status=1
	if grep -n -- '--- SKIP' "$log"; then
		echo "$pkg: tests skipped" >&2
		status=1
	fi
done
exit $status
