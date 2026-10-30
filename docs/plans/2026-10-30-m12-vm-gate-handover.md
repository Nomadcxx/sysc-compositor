# Handover: M12 Task 8, the session VM gate (2026-10-30)

Read this first, then the release assessment (`2026-10-28-release-readiness.md`) and the last
execution records of `2026-10-24-m12-capture-portals.md`.

**Update, later on 2026-10-30:** the remaining Task 8 work below is done. The unresolved
`block-out-from` check was a config bug, fixed by #72 (a config created after startup never
loaded). Protected content, pointer modes, the preview-contract decision and lane 2 are recorded
in the M12 plan's "Execution record: VM gate, 2026-10-29/30", and the matrix and release
assessment are updated. The VM was left powered off after lane 2, with #72's build installed and
no `~/.config/niri/config.kdl`; the old binary is `/usr/local/bin/sysc-compositor.bak-20261029`.
What remains is the owner's call on closing `sysc-1277` and the lane-2 flakes in `sysc-2078`. Standing rules from the 2026-10-23
continuation handover still hold. Tracker: `sysc-1277` (bd in `/home/nomadx/sysc-shell`).

## Where things stand

Main is `e4afdf8`. Merged on 2026-10-29, all with both CI lanes green:

| PR | Change |
|---|---|
| #64 | gamma control through SceneFX; captures no longer swallow a pending ramp |
| #65 | tearing control advertised, frames always vsynced (owner decision: Niri, Mango and Hyprland all default to never tearing) |
| #66 | linux-drm-syncobj behind the renderer/backend timeline gate; window screenshots wait on acquire points |
| #67 | tablet tools (Niri), pads and pointer emulation (Mango/Hyprland), `input.tablet` |
| #68 | portal screenshots create `~/Pictures/Screenshots` (found in the VM gate) |
| #69 | spawned programs get the session environment (found in the VM gate) |
| #70 | screencasts negotiate with WebRTC consumers; a failed link no longer kills ScreenCast (found in the VM gate) |

M12 Task 7 is done. Task 8, the VM gate, is mostly done; what remains is listed at the end.

## Task 8 evidence so far

The session VM (`2026-10-24-session-vm.md`) on the laptop was fully upgraded on 2026-10-29
(wlroots 0.20.2, SceneFX 0.5, PipeWire 1.6.10, WirePlumber 0.5.18, xdg-desktop-portal 1.22.1,
xdg-desktop-portal-gnome 51.0, -gtk 1.15.3, Firefox 159.0, Chromium 156.0.8078.73, OBS 33.0.0,
kernel 7.2.11) and booted with `./run.sh` (virgl, VNC). The session is the real greetd login through
sysc-greet: `sysc-compositor.service` and `graphical-session.target` active,
`XDG_CURRENT_DESKTOP=niri:sysc`, so xdg-desktop-portal reads `niri-portals.conf` (GNOME first, then
GTK), as under Niri.

| Check | Result |
|---|---|
| Portal discovery | xdg-desktop-portal with the GNOME and GTK backends; `ScreenCast` v5 exposed; sysc owns `org.gnome.Mutter.DisplayConfig`, `.ScreenCast`, `.ServiceChannel`, `org.gnome.Shell.Introspect`, `.Screenshot` |
| Screenshot portal | first use shows GNOME's "Allow Apps to Take Screenshots?"; after Allow, response 0 and a 1280×800 PNG of the real desktop. Failed before #68 on a fresh account. |
| PickColor portal | response 0, `(0, 0.502, 1.0)`, the bar's #0080FF pill |
| Monitor ScreenCast | GNOME chooser lists "Red Hat, Inc. 15″" (DisplayConfig); node 47, 60 RGBA frames 1280×800 through `pipewiresrc` |
| Window ScreenCast | chooser lists the window and "niri Dynamic Cast Target" (as Niri); 616×728 frames; stream `size` (1, 1) is what Niri publishes too |
| Dynamic target | Start answers only once a target is set, as Niri's pending dynamic casts; one stream renegotiated 616×728 (window) to 1280×800 (monitor) via `set-dynamic-cast-window` / `-monitor`; `Casts` showed Nothing, then Output |
| Firefox 159 | `getDisplayMedia` through the chooser: 85 frames in 4 s, 1280×800, real content. Needed #69 and #70. |
| Chromium 156 (`--ozone-platform=wayland`) | its own picker hands Entire Screen to the portal; live thumbnail; 96 frames, 1280×800, `displaySurface: monitor` |
| OBS 33 | PipeWire screen capture source: live preview; `Casts` shows an active PipeWire cast (pid null: the D-Bus caller is the portal); "Remember this selection" restore token works across an OBS restart with no chooser |

Frame counts come from the page's `requestVideoFrameCallback` while the pointer was swept: casts
are damage-driven, as Niri's, so a still screen sends no frames.

## In progress when the session ended: protected content

`~/.config/niri/config.kdl` in the VM now holds only:

```kdl
window-rule {
    match app-id="chromium"
    block-out-from "screencast"
}
```

`sysc-compositor validate` accepts it and `msg action load-config-file` returned, but OBS's live
preview still showed Chromium's content and the journal logged nothing. Unresolved. The session had
started with no config file at all. Check in order:

1. Whether the reload happened: `LoadConfigFile` in `internal/server/server.go:223` and
   `internal/server/niri.go:383`; `LoadWithSources` returns `Sources: [root]` for a missing file
   (`internal/config/config.go:1546`), and `server.New` only builds a watcher when sources exist.
2. Whether a reload re-applies `block-out-from` to windows already mapped (Niri recomputes rules
   for every window on reload), via `Reconfigure`'s `rulesChanged` path.
3. Whether `block-out-from "screencast"` applies to PipeWire casts (`config.CaptureCast` in
   `renderOutputForTarget` / `beginCapturePrivacy`). Compositor tests already cover privacy for
   capture paths; this is the real-session check.

A quick split: restart Chromium after the reload. If a freshly mapped window is blocked out, the
problem is re-applying rules on reload.

## Remaining Task 8 work

1. Finish protected content (above), with and without `block-out-from "screen-capture"`.
2. Pointer modes: run the probe with `cursor_mode` 1 (hidden) and 4 (metadata) and check that Start
   succeeds; mode 2 (embedded) is proved above.
3. Preview contract (plan item 3): the protocol behavior has compositor tests
   (`TestForeignToplevelIdentifierMatchesIPCFromFirstAnnouncement`,
   `TestWindowCaptureTracksDamageResizeAndUnmap`, the capture privacy tests). Decide whether to cite
   them or run a protocol client in the VM.
4. Lane 2 for the final tree (`run.sh lane2`, then `./vssh './lane2.sh ./internal/wlr'` in the
   foreground), plus the standing merge checks.
5. Close-out docs: a "VM gate" execution record in the M12 plan with the table above, matrix rows
   (D-Bus services, ScreenCast, Casts), the release assessment. M12 closes only when the plan's stop
   condition holds; owner-only closures stay with the owner.

Not part of M12, still open: `sysc-2056` (game frame pacing against Niri); hardware qualification
(gamma on a physical panel, explicit sync on NVIDIA, a physical tablet and pad); the owner's choice
on libinput device settings (tablet `left-handed`/`calibration-matrix` warn until then).

## VM state and how to drive it

The VM is running in virgl mode with a logged-in session (started 10:14 UTC on 2026-10-29) holding
Chromium, OBS and the probe server. Its `/usr/local/bin/sysc-compositor` was built from
`~/sysc-compositor-gate` in the VM: #68's tree plus #69's `spawn.go` and #70's `pipewire.go`, which
together equal main's code. The portal permission store holds "allow" for non-interactive
screenshots. To start clean, quit with `msg action quit --skip-confirmation` and log in again.

Assets are in `assets/2026-10-29-m12-vm-gate/`. In the VM, `~/portal_probe.py`, `~/cast/` and
`~/.config/obs-studio/` match them; on the laptop, `qmpclick.py` and `qmpsweep.py` sit beside
`run.sh`.

| Asset | Use |
|---|---|
| `portal_probe.py screenshot` / `pickcolor` / `screencast TYPES CURSOR FRAMES [RAW]` | drives the portal as an app; screencast pulls frames through `gst-launch-1.0 pipewiresrc` and prints each caps change, writing them to RAW as RGB when given (`castrun.sh` wraps a monitor cast). Needs `python-gobject`, `gst-plugins-base`, `gst-plugin-pipewire` (installed). Its output is buffered: read it after it exits. |
| `cast.html`, `server.py` | WebRTC probe page on 127.0.0.1:8765; the page POSTs frames, size and mean colour to `~/cast/results.jsonl` |
| `qmpclick.py X Y` / `qmpsweep.py SECONDS` | click on, or sweep the pointer over, the 1280×800 console through QMP's virtio tablet |
| `obs/` | OBS 33 profile with a `pipewire-screen-capture-source` and first-run flags |

Commands run in the session through `./vssh` with `export DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus`
and `NIRI_SOCKET=$(ls /run/user/1000/niri.wayland-0.*.sock)`. Start apps with
`sysc-compositor msg action spawn -- …` so they get the session environment.

Lessons from this gate:

- Never `pgrep -f` / `pkill -f` over `vssh`: the pattern matches your own shell's command line. One
  such `pkill` killed the shell before a quit ran, and the password typed for the "relogin" went into
  the live session. Use `pkill -x NAME`, a bracketed pattern such as `cast/[s]erver.py`, or a PID.
- The startup hotkey overlay covers dialogs; one click elsewhere dismisses it.
- GNOME's chooser preselects a lone monitor; clicking the tile deselects it. Press Share directly.
- Firefox's `media.navigator.permission.disabled` breaks Wayland screen sharing (`NotFoundError`
  before the portal); leave it out.
- First portal screenshot from a host app shows GNOME's permission dialog; the request waits on it.
- `/tmp` on the desktop is an 8 GB tmpfs near its quota; build with `GOTMPDIR`/`TMPDIR` under the
  worktree, and keep the scratchpad small.
- On the laptop, one named test per command, never a loop (broken twice on 2026-10-29).

## Local worktrees

`.recovery/{gamma,tearing,syncobj,tablet,portal-shot,spawn-env,cast-rate}` all belong to merged
branches and can be removed with `git worktree remove`. `.recovery/handover` holds this document's
branch.
