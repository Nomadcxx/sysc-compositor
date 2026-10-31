#!/usr/bin/env python3
"""qmpwheel.py X Y GAP_MS up|down|left|right COUNT [key...]: move the VM tablet
to pixel (X, Y) of the 1280x800 console, hold the qcode keys, send COUNT wheel
clicks GAP_MS apart through QMP input-send-event, then release the keys."""
import json, os, socket, sys, time

W, H = 1280, 800
x, y, gap, direction, count = int(sys.argv[1]), int(sys.argv[2]), int(sys.argv[3]), sys.argv[4], int(sys.argv[5])
keys = sys.argv[6:]
s = socket.socket(socket.AF_UNIX)
s.connect(os.path.join(os.path.dirname(os.path.abspath(__file__)), "qmp.sock"))
f = s.makefile("rw")


def cmd(c):
    f.write(json.dumps(c) + "\n")
    f.flush()
    while True:
        r = json.loads(f.readline())
        if "return" in r or "error" in r:
            if "error" in r:
                sys.exit(r["error"])
            return r


def send(*events):
    cmd({"execute": "input-send-event", "arguments": {"events": list(events)}})


f.readline()
cmd({"execute": "qmp_capabilities"})
send({"type": "abs", "data": {"axis": "x", "value": x * 32767 // (W - 1)}},
     {"type": "abs", "data": {"axis": "y", "value": y * 32767 // (H - 1)}})
time.sleep(0.15)
for k in keys:
    send({"type": "key", "data": {"down": True, "key": {"type": "qcode", "data": k}}})
time.sleep(0.1)
for i in range(count):
    for down in (True, False):
        send({"type": "btn", "data": {"down": down, "button": "wheel-" + direction}})
    time.sleep(gap / 1000)
time.sleep(0.1)
for k in reversed(keys):
    send({"type": "key", "data": {"down": False, "key": {"type": "qcode", "data": k}}})
