#!/usr/bin/env python3
"""QMP helper for the sysc-session VM.
  qmp.py shot out.png      screendump of the VM display
  qmp.py type "text"       type text (ASCII) on the VM keyboard
  qmp.py key ctrl-alt-f2   press one chord (qcodes joined by -)
"""
import json, os, socket, sys, time

SOCK = os.path.join(os.path.dirname(os.path.abspath(__file__)), "qmp.sock")
SHIFTED = {'!': '1', '@': '2', '#': '3', '$': '4', '%': '5', '^': '6', '&': '7', '*': '8',
           '(': '9', ')': '0', '_': 'minus', '+': 'equal', ':': 'semicolon', '"': 'apostrophe',
           '<': 'comma', '>': 'dot', '?': 'slash', '|': 'backslash', '~': 'grave_accent',
           '{': 'bracket_left', '}': 'bracket_right'}
PLAIN = {' ': 'spc', '\n': 'ret', '-': 'minus', '=': 'equal', ';': 'semicolon', "'": 'apostrophe',
         ',': 'comma', '.': 'dot', '/': 'slash', '\\': 'backslash', '`': 'grave_accent',
         '[': 'bracket_left', ']': 'bracket_right', '\t': 'tab'}


def qmp(*commands):
    s = socket.socket(socket.AF_UNIX)
    s.connect(SOCK)
    f = s.makefile("rw")
    f.readline()
    for cmd in ({"execute": "qmp_capabilities"},) + commands:
        f.write(json.dumps(cmd) + "\n")
        f.flush()
        while True:
            reply = json.loads(f.readline())
            if "event" not in reply:
                break
        if "error" in reply:
            sys.exit(reply["error"])
    s.close()


def chord(keys):
    qmp({"execute": "send-key", "arguments": {"keys": [{"type": "qcode", "data": k} for k in keys]}})


def codes(ch):
    if ch.isalpha():
        return ["shift", ch.lower()] if ch.isupper() else [ch]
    if ch.isdigit():
        return [ch]
    if ch in SHIFTED:
        return ["shift", SHIFTED[ch]]
    return [PLAIN[ch]]


op, arg = sys.argv[1], sys.argv[2]
if op == "shot":
    qmp({"execute": "screendump", "arguments": {"filename": os.path.abspath(arg), "format": "png"}})
elif op == "type":
    for ch in arg:
        chord(codes(ch))
        time.sleep(0.03)
elif op == "key":
    chord(arg.split("-"))
