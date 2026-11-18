#!/bin/bash
# aur-container-check.sh RECIPE...: build AUR recipes in a clean archlinux
# container, run the sysc repository's aur/check.py on them, and install the
# results. Work on a copy so the tracked tree stays clean:
#
#   cp -a ~/sysc/aur /tmp/aur-copy
#   cp aur-container-check.sh /tmp/aur-copy/
#   podman run --rm -v /tmp/aur-copy:/build:Z docker.io/library/archlinux:latest \
#     bash /build/aur-container-check.sh sysc-compositor sysc-compositor-git
#   podman unshare chown -R 0:0 /tmp/aur-copy   # the build user owns it afterwards
#
# A -git recipe's pkgver moves when it builds: copy its PKGBUILD and the
# regenerated .SRCINFO back before pushing. Runtime dependencies from the AUR
# (sysc-notify and the like) are not installable here, so builds use --nodeps
# and install steps use --nodeps too.
set -euo pipefail
[ "$#" -gt 0 ] || { echo "usage: $0 RECIPE..." >&2; exit 2; }
pacman -Syu --noconfirm --needed base-devel git go pacman-contrib python \
	wlroots0.20 scenefx libpipewire libinput libxkbcommon wayland pixman libdrm \
	dbus xkeyboard-config wayland-protocols pkgconf >/dev/null
go version
id b >/dev/null 2>&1 || useradd -m b
chown -R b /build
for r in "$@"; do
	echo "== build $r"
	su b -c "cd /build/$r && makepkg --noconfirm --nodeps >/build/$r.log 2>&1" ||
		{ tail -30 "/build/$r.log"; exit 1; }
	su b -c "cd /build/$r && makepkg --printsrcinfo > .SRCINFO"
done
echo "== check"
su b -c "cd /build && python3 check.py $*"
for r in "$@"; do
	echo "== install $r"
	pkg=$(ls /build/$r/$r-[0-9]*.pkg.tar.zst | grep -v -- -debug- | head -1)
	pacman -U --noconfirm --nodeps "$pkg" >/dev/null
	bsdtar -xOf "$pkg" .PKGINFO | grep -E '^pkgver = '
	pacman -R --noconfirm --nodeps "$(bsdtar -xOf "$pkg" .PKGINFO | sed -n 's/^pkgname = //p')" >/dev/null
done
echo OK
