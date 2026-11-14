#!/bin/sh
# Check a `make install DESTDIR=… PREFIX=…` tree against the package layout:
# every expected file and no other, and no unsubstituted or foreign bin path.
# Usage: packaging/check-install.sh DESTDIR PREFIX
set -eu

root=${1:?usage: check-install.sh DESTDIR PREFIX}
prefix=${2:?usage: check-install.sh DESTDIR PREFIX}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

sort >"$work/expected" <<LIST
$prefix/bin/sysc-compositor
$prefix/bin/sysc-compositor-session
$prefix/share/wayland-sessions/sysc-compositor.desktop
$prefix/lib/systemd/user/sysc-compositor.service
$prefix/lib/systemd/user/sysc-compositor-shutdown.target
$prefix/share/xdg-desktop-portal/sysc-portals.conf
$prefix/share/sysc-compositor/default-config.kdl
$prefix/share/licenses/sysc-compositor/LICENSE
$prefix/share/licenses/sysc-compositor/NOTICE
$prefix/share/doc/sysc-compositor/README.md
LIST
(cd "$root" && find . ! -type d | sed 's|^\.||') | sort >"$work/actual"

for f in $(comm -23 "$work/expected" "$work/actual"); do
	echo "missing: $f"
	fail=1
done
for f in $(comm -13 "$work/expected" "$work/actual"); do
	echo "unexpected: $f"
	fail=1
done

# Text files only: the binary holds neither string.
for f in $(grep -v "^$prefix/bin/sysc-compositor\$" "$work/actual"); do
	[ -f "$root$f" ] || continue
	if grep -q '@BINDIR@' "$root$f"; then
		echo "unsubstituted @BINDIR@: $f"
		fail=1
	fi
	if [ "$prefix" != /usr/local ] && grep -q '/usr/local' "$root$f"; then
		echo "/usr/local under PREFIX=$prefix: $f"
		fail=1
	fi
done

desktop=$root$prefix/share/wayland-sessions/sysc-compositor.desktop
if [ -f "$desktop" ] && ! grep -qx "Exec=$prefix/bin/sysc-compositor-session" "$desktop"; then
	echo "session Exec= is not $prefix/bin/sysc-compositor-session"
	fail=1
fi
unit=$root$prefix/lib/systemd/user/sysc-compositor.service
if [ -f "$unit" ] && ! grep -qx "ExecStart=$prefix/bin/sysc-compositor --session" "$unit"; then
	echo "unit ExecStart= is not $prefix/bin/sysc-compositor --session"
	fail=1
fi

[ "$fail" = 0 ] && echo "install layout OK under $prefix"
exit "$fail"
