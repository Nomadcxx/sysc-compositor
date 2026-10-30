#!/bin/sh
# castrun.sh NAME [CURSOR]: on the laptop, beside run.sh. Starts a monitor cast
# through the portal in the VM session (cursor mode 1 hidden, 2 embedded,
# 4 metadata), presses Share in GNOME's chooser and sweeps the pointer, since
# casts are damage-driven. The VM's ~/NAME.rgb then holds ten 1280x800 RGB
# frames; take the last with `tail -c 3072000` and convert it with
# `magick -size 1280x800 -depth 8 rgb:FILE out.png`.
set -e
NAME=$1; CURSOR=${2:-2}
cd ~/vm/sysc-session
./vssh "export DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus; rm -f ~/$NAME.rgb; nohup python3 ~/portal_probe.py screencast 1 $CURSOR 10 /home/nomadx/$NAME.rgb > ~/probe-$NAME.log 2>&1 < /dev/null &"
sleep 4
python3 qmpclick.py 1219 78
python3 qmpsweep.py 6 >/dev/null
sleep 3
./vssh "cat ~/probe-$NAME.log"
