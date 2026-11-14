#!/bin/sh
# Smoke-check an installed sysc-compositor package: the version it reports,
# the shipped default config, and a compositor answering IPC.
# SceneFX renders only with a DRM device: give the container a render node
# for the default headless backend, or a KMS device (vkms) with
# WLR_BACKENDS=drm LIBSEAT_BACKEND=noop.
# Usage: packaging/smoke.sh VERSION
set -eu

want=${1:?usage: smoke.sh VERSION}

got=$(sysc-compositor --version)
case $got in
"sysc-compositor $want (niri-ipc "*) ;;
*)
	echo "smoke: --version printed '$got', want release $want" >&2
	exit 1
	;;
esac
sysc-compositor validate -c /usr/share/sysc-compositor/default-config.kdl

XDG_RUNTIME_DIR=$(mktemp -d)
chmod 0700 "$XDG_RUNTIME_DIR"
export XDG_RUNTIME_DIR
cd "$XDG_RUNTIME_DIR"
ulimit -c unlimited
WLR_BACKENDS=${WLR_BACKENDS:-headless} WLR_RENDERER_ALLOW_SOFTWARE=1 sysc-compositor -c /dev/null >compositor.log 2>&1 &
pid=$!
sleep 3
if ! kill -0 "$pid" 2>/dev/null; then
	echo "smoke: the compositor exited" >&2
	cat compositor.log >&2
	exit 1
fi
NIRI_SOCKET=$(ls "$XDG_RUNTIME_DIR"/sysc-compositor.*.sock) sysc-compositor msg version
kill "$pid"
wait "$pid" || true
if ls core* >/dev/null 2>&1; then
	echo "smoke: the compositor left a core dump" >&2
	exit 1
fi
echo "smoke: $got OK"
