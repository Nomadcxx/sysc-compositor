#!/usr/bin/env python3
# Hold QMP keys together for a while: qmphold.py MS key [key...]
import json, socket, sys

ms, keys = int(sys.argv[1]), sys.argv[2:]
s = socket.socket(socket.AF_UNIX)
s.connect("qmp.sock")
f = s.makefile("rw")
f.readline()
for cmd in ({"execute": "qmp_capabilities"},
            {"execute": "send-key", "arguments": {
                "keys": [{"type": "qcode", "data": k} for k in keys], "hold-time": ms}}):
    f.write(json.dumps(cmd) + "\n")
    f.flush()
    print(f.readline().strip())
