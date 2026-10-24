#!/bin/bash
# Boots the sysc-session VM headless: no window on this desktop.
#   ./run.sh        virtio-vga-gl (virgl) for the session gate; VNC on
#                   127.0.0.1:5959 for gvnccapture, because QEMU cannot
#                   screendump the GL console
#   ./run.sh lane2  no GPU at all, so vkms is the only DRM device and there is
#                   no render node, as on the CI runner
# QMP on qmp.sock (qmp.py: send-key; screendump in lane2 mode), serial console
# in serial.log, SSH on 127.0.0.1:2222 (vssh). Stop: ./vssh sudo systemctl poweroff
set -euo pipefail
cd "$(dirname "$0")"
gpu=(-device virtio-vga-gl -display egl-headless,rendernode=/dev/dri/renderD128 -vnc 127.0.0.1:59)
[ "${1:-}" = lane2 ] && gpu=(-vga none -display none)
exec qemu-system-x86_64 \
	-name sysc-session -machine q35,accel=kvm -cpu host -smp 2 -m 6G \
	-drive if=pflash,format=raw,readonly=on,file=/usr/share/edk2/x64/OVMF_CODE.4m.fd \
	-drive if=pflash,format=raw,file=vars.fd \
	-drive if=none,id=hd,format=qcow2,file=disk.qcow2 -device virtio-blk-pci,drive=hd,bootindex=0 \
	-drive if=virtio,format=raw,media=cdrom,readonly=on,file=seed.iso \
	-nic user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:2222-:22 \
	"${gpu[@]}" \
	-device virtio-keyboard-pci -device virtio-tablet-pci \
	-qmp unix:qmp.sock,server=on,wait=off \
	-serial file:serial.log -monitor none \
	-pidfile qemu.pid -daemonize
