#!/usr/bin/env python3
"""qmpclick.py X Y [button]: move the VM tablet to screen pixel (X, Y) of the
1280x800 console and click (left by default), through QMP input-send-event."""
import json, socket, sys, time, os
W, H = 1280, 800
x, y = int(sys.argv[1]), int(sys.argv[2])
button = sys.argv[3] if len(sys.argv) > 3 else "left"
s = socket.socket(socket.AF_UNIX)
s.connect(os.path.join(os.path.dirname(os.path.abspath(__file__)), "qmp.sock"))
f = s.makefile("rw")
def cmd(c):
    f.write(json.dumps(c) + "\n"); f.flush()
    while True:
        r = json.loads(f.readline())
        if "return" in r or "error" in r:
            return r
f.readline()
cmd({"execute": "qmp_capabilities"})
abs_ev = lambda axis, v: {"type": "abs", "data": {"axis": axis, "value": v}}
cmd({"execute": "input-send-event", "arguments": {"events": [abs_ev("x", x * 32767 // (W - 1)), abs_ev("y", y * 32767 // (H - 1))]}})
time.sleep(0.15)
for down in (True, False):
    cmd({"execute": "input-send-event", "arguments": {"events": [{"type": "btn", "data": {"down": down, "button": button}}]}})
    time.sleep(0.1)
