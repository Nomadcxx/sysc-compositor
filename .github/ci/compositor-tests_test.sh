#!/bin/bash
# Exercise real card selection against sysfs-shaped fixtures; no compositor.
set -euo pipefail
# Load only the selector; the lane script installs packages and runs compositors.
source <(sed -n '/^vkms_card()/,/^}/p' "$(dirname "$0")/compositor-tests.sh")
declare -F vkms_card >/dev/null || { echo 'vkms selector missing' >&2; exit 1; }
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/sys/class/drm" "$fixture/sys/bus/pci/drivers/other" \
 "$fixture/sys/bus/faux/drivers/faux_driver" "$fixture/sys/bus/platform/drivers/vkms" \
 "$fixture/sys/devices/gpu" "$fixture/sys/devices/vgem" "$fixture/sys/devices/vkms"
ln -s "$fixture/sys/bus/pci/drivers/other" "$fixture/sys/devices/gpu/driver"
ln -s "$fixture/sys/bus/faux/drivers/faux_driver" "$fixture/sys/devices/vgem/driver"
ln -s "$fixture/sys/bus/faux/drivers/faux_driver" "$fixture/sys/devices/vkms/driver"
for card in card0 card1 card10; do mkdir -p "$fixture/sys/class/drm/$card"; done
ln -s "$fixture/sys/devices/gpu" "$fixture/sys/class/drm/card0/device"
ln -s "$fixture/sys/devices/vgem" "$fixture/sys/class/drm/card1/device"
if vkms_card "$fixture/sys/class/drm" /dev/dri >"$fixture/rejected.out" 2>"$fixture/rejected.err"; then
 echo 'selected a GPU or non-vkms faux device' >&2; exit 1
fi
ln -s "$fixture/sys/devices/vkms" "$fixture/sys/class/drm/card10/device"
[[ $(vkms_card "$fixture/sys/class/drm" /dev/dri) == /dev/dri/card10 ]]
rm "$fixture/sys/devices/vkms/driver"
ln -s "$fixture/sys/bus/platform/drivers/vkms" "$fixture/sys/devices/vkms/driver"
[[ $(vkms_card "$fixture/sys/class/drm" /dev/dri) == /dev/dri/card10 ]]
