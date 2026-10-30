#!/usr/bin/env python3
"""qmpsweep.py SECONDS: sweep the VM pointer back and forth to generate damage."""
import json, socket, sys, time, os
s = socket.socket(socket.AF_UNIX)
s.connect(os.path.join(os.path.dirname(os.path.abspath(__file__)), "qmp.sock"))
f = s.makefile("rw")
def cmd(c):
    f.write(json.dumps(c) + "\n"); f.flush()
    while True:
        r = json.loads(f.readline())
        if "return" in r or "error" in r:
            return r
f.readline(); cmd({"execute": "qmp_capabilities"})
end = time.time() + float(sys.argv[1]); x = 0
while time.time() < end:
    x = (x + 600) % 30000
    cmd({"execute": "input-send-event", "arguments": {"events": [
        {"type": "abs", "data": {"axis": "x", "value": 2000 + x}},
        {"type": "abs", "data": {"axis": "y", "value": 12000 + (x // 3)}}]}})
    time.sleep(0.02)
