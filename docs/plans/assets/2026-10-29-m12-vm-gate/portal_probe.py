#!/usr/bin/env python3
"""Drives xdg-desktop-portal as an application would, for the M12 VM gate.

  portal_probe.py screenshot
  portal_probe.py pickcolor
  portal_probe.py screencast TYPES CURSOR_MODE FRAMES   (TYPES 1 monitor, 2 window)
"""
import os
import struct
import subprocess
import sys
import urllib.parse

import gi

gi.require_version("Gio", "2.0")
from gi.repository import Gio, GLib  # noqa: E402

BUS = Gio.bus_get_sync(Gio.BusType.SESSION)
SENDER = BUS.get_unique_name()[1:].replace(".", "_")
PORTAL = ("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
counter = [0]


def request(iface, method, args, options, timeout=90):
    """Calls a portal method that answers through a Request and returns
    (response code, results)."""
    counter[0] += 1
    token = f"sysc{os.getpid()}_{counter[0]}"
    path = f"/org/freedesktop/portal/desktop/request/{SENDER}/{token}"
    loop = GLib.MainLoop()
    result = {"code": None, "results": {}}

    def on_response(_conn, _sender, _path, _iface, _signal, params):
        result["code"], result["results"] = params.unpack()
        loop.quit()

    sub = BUS.signal_subscribe(PORTAL[0], "org.freedesktop.portal.Request", "Response",
                               path, None, Gio.DBusSignalFlags.NONE, on_response)
    options = dict(options)
    options["handle_token"] = GLib.Variant("s", token)
    BUS.call_sync(PORTAL[0], PORTAL[1], f"org.freedesktop.portal.{iface}", method,
                  GLib.Variant(f"({args[0]}a{{sv}})", (*args[1:], options)),
                  None, Gio.DBusCallFlags.NONE, -1, None)
    GLib.timeout_add_seconds(timeout, loop.quit)
    loop.run()
    BUS.signal_unsubscribe(sub)
    return result["code"], result["results"]


def png_size(path):
    with open(path, "rb") as f:
        header = f.read(24)
    return struct.unpack(">II", header[16:24])


def screenshot():
    code, results = request("Screenshot", "Screenshot", ("s", ""), {"interactive": GLib.Variant("b", False)})
    print("screenshot response", code)
    if code == 0:
        path = urllib.parse.unquote(urllib.parse.urlparse(results["uri"]).path)
        print("uri", results["uri"], "size", png_size(path))
    return code


def pickcolor():
    code, results = request("Screenshot", "PickColor", ("s", ""), {})
    print("pickcolor response", code, results.get("color"))
    return code


def screencast(types, cursor_mode, frames, frame=None):
    session_token = f"syscsession{os.getpid()}"
    code, results = request("ScreenCast", "CreateSession", ("",),
                            {"session_handle_token": GLib.Variant("s", session_token)})
    print("create session", code)
    if code != 0:
        return code
    session = results["session_handle"]
    code, _ = request("ScreenCast", "SelectSources", ("o", session),
                      {"types": GLib.Variant("u", types), "cursor_mode": GLib.Variant("u", cursor_mode),
                       "multiple": GLib.Variant("b", False)})
    print("select sources", code)
    if code != 0:
        return code
    print("start: choose a source in the dialog", flush=True)
    code, results = request("ScreenCast", "Start", ("os", session, ""), {})
    print("start", code)
    if code != 0:
        return code
    streams = results["streams"]
    for node, props in streams:
        print("stream node", node, {k: props[k] for k in props})
    fd_list = Gio.UnixFDList()
    reply, out_fds = BUS.call_with_unix_fd_list_sync(
        PORTAL[0], PORTAL[1], "org.freedesktop.portal.ScreenCast", "OpenPipeWireRemote",
        GLib.Variant("(oa{sv})", (session, {})), GLib.VariantType("(h)"),
        Gio.DBusCallFlags.NONE, -1, fd_list, None)
    fd = out_fds.get(reply.unpack()[0])
    node = streams[0][0]
    # With a path, the frames are written there as raw RGB instead of discarded.
    sink = (["video/x-raw,format=RGB", "!", "filesink", f"location={frame}", "name=sink0"] if frame
            else ["fakesink", "sync=false", "name=sink0"])
    gst = subprocess.run(
        ["gst-launch-1.0", "-v", "pipewiresrc", f"fd={fd}", f"path={node}", "do-timestamp=true",
         f"num-buffers={frames}", "!", "videoconvert", "!"] + sink,
        pass_fds=[fd], capture_output=True, text=True, timeout=60)
    caps = [line for line in gst.stdout.splitlines() if "sink0.GstPad:sink: caps" in line]
    print("gstreamer exit", gst.returncode, "frames requested", frames)
    seen = []
    for line in caps:
        value = line.split("caps = ", 1)[-1]
        if value not in seen:
            seen.append(value)
    for value in seen:
        print("caps", value)
    if not seen:
        print("caps none")
    if gst.returncode != 0:
        print(gst.stderr[-2000:])
    BUS.call_sync(PORTAL[0], session, "org.freedesktop.portal.Session", "Close",
                  None, None, Gio.DBusCallFlags.NONE, -1, None)
    return gst.returncode


if __name__ == "__main__":
    mode = sys.argv[1]
    if mode == "screenshot":
        sys.exit(screenshot())
    if mode == "pickcolor":
        sys.exit(pickcolor())
    if mode == "screencast":
        sys.exit(screencast(int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4]),
                            sys.argv[5] if len(sys.argv) > 5 else None))
    sys.exit(f"unknown mode {mode}")
