#!/bin/bash
# Lane 2 of CI, run inside archlinux:latest with the runner's /dev/dri. It
# builds real compositors, so it belongs only on a machine with no desktop:
# never run it on a developer machine.
set -euo pipefail

pacman -Syu --noconfirm --needed git gcc pkgconf go wlroots0.20 scenefx wayland \
	wayland-protocols libxkbcommon xkeyboard-config mesa seatd dbus adwaita-cursors \
	weston wayland-utils foot swaybg waybar wofi ttf-dejavu grim wl-clipboard >/dev/null
git config --global --add safe.directory "$PWD"
install -d -m 0700 "$XDG_RUNTIME_DIR"
# waybar needs a machine id; the tests load the "default" pointer theme.
dbus-uuidgen --ensure=/etc/machine-id
printf '[Icon Theme]\nInherits=Adwaita\n' | install -D -m 0644 /dev/stdin /usr/share/icons/default/index.theme
# vkms has a primary node only; wlroots renders on it when the DRM backend
# hands it over. The container has no udev, so the device is named.
WLR_DRM_DEVICES=$(ls /dev/dri/card* | head -n1)
export WLR_DRM_DEVICES
echo "WLR_DRM_DEVICES=$WLR_DRM_DEVICES"
go generate ./internal/wlr

quarantine=$(grep -v '^#' .github/ci/quarantine.txt | awk 'NF {print $1}' | sort -u)
status=0
for pkg in ./internal/wlr ./internal/server; do
	log=$(mktemp)
	passed=1
	go test -count=1 -v -timeout 15m "$pkg" 2>&1 | tee "$log" || passed=0
	# Top-level tests that failed or skipped. A skip means the lane checked
	# less than it claims, so it counts like a failure.
	bad=$({ grep -oE '^--- (FAIL|SKIP): [A-Za-z0-9_]+' "$log" || true; } | awk '{print $3}' | sort -u)
	unexpected=$(comm -23 <(printf '%s\n' "$bad" | sed '/^$/d') <(printf '%s\n' "$quarantine"))
	if [ -n "$unexpected" ]; then
		echo "::error::$pkg: failed or skipped outside the quarantine: $(echo $unexpected)"
		status=1
	fi
	# A crash ends the test binary early, so later tests never ran: it fails
	# the lane even when every test that did fail is quarantined.
	if grep -qE '^(panic: |fatal error: |SIGSEGV|SIGABRT|SIGBUS|SIGILL|SIGFPE)' "$log" ||
		{ [ $passed = 0 ] && [ -z "$bad" ]; }; then
		echo "::error::$pkg: the test binary crashed or failed without a failing test to name"
		status=1
	fi
	for name in $quarantine; do
		if grep -q "^--- PASS: $name " "$log"; then
			echo "::warning::$name passes now: remove it from .github/ci/quarantine.txt"
		fi
	done
	for name in $(comm -12 <(printf '%s\n' "$bad" | sed '/^$/d') <(printf '%s\n' "$quarantine")); do
		echo "::warning::quarantined $name failed or skipped (see quarantine.txt)"
	done
done
exit $status
