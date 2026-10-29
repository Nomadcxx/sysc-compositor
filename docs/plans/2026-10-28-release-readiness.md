# sysc-compositor release readiness

Assessment on 2026-10-28, starting from `8797cef` (PR #60). This is a gate assessment, not a plan
for M13–M15. The product contract and [parity matrix](niri-parity-matrix.md) define scope; bd in
`/home/nomadx/sysc-shell` owns issue status.

## Current baseline

The native Wayland session runs. Existing gates cover the greeter and session lifecycle, shell
startup, scrolling layout, workspaces, floating/fullscreen, overview, spring animations, config
reload, idle/lock protocols, screenshots and the window switcher. The session VM record includes
login through sysc-greet, clean shutdown, compositor crash and subsequent login.

PRs #59 and #60 add virtual input/IME-grab routing and privacy-filtered live output/window capture.
Their local reviews and static/compositor CI lanes passed. Named laptop checks cover 15 virtual
input regressions, 12 capture runtime checks, five capture logic checks and eight combined
integration checks. These establish the capture/input foundation; portal consumers still need
their own acceptance checks.

The current change adds the session-only portal discovery/screenshot services. Nine named
private-bus regressions cover wire signatures, concurrent replies, socket ownership, malformed
requests, caller departure and bus/shutdown cancellation. Baremetal bench checks cover native
service clients, known color-picker pixels and cancellation, screenshot write errors, private
concurrent fallback PNGs, clipboard preservation, multi-output rejection and nested startup
leaving session service names alone. Local review and CI still gate its merge.

Descriptor rejection also exposed a bug in the pinned godbus decoder. The reviewed local
backport retains v5.2.2 and adds no Go dependency; eight raw-transport cases and the portal
descriptor EOF check prove the correction. CI runs that named dependency regression.

The baremetal `TestNativeIBusComposesInGTK` passes with packaged IBus 1.5.34 and GTK 4.24.1:
text-input-v3/input-method-v2 activation, Unicode preedit and an `é` commit, then an unconsumed
Backspace returned through the IME's virtual keyboard exactly once down/up. GTK receives the
release and the final expected text. The test leaves a runnable acceptance check; CI records
its optional-tool skip because the CI image does not include this packaged client stack.

The project supports an experimental native session. A release that can replace Niri on this
machine still needs the gates below. Counting merged milestones would hide their size: PipeWire,
X11 applications, accessibility and output management remain substantial work.

At the assessed baseline, the server dispatches 86 of the 141 Niri action names; 55 still return
`unimplemented`. Four are the M12 cast actions. This counts routed actions, not configuration,
hardware or integration completeness.

## Remaining release gates

| Gate | Work and acceptance evidence still needed | Owner |
|---|---|---|
| Portal services | Review/merge the tested session-only services, then prove GNOME/GTK portal discovery in an isolated session. DisplayConfig reports current outputs; output mutation remains explicit unsupported behavior until M15. | M12 Task 5, `sysc-1277` |
| Screen sharing | PipeWire monitor/window frames, pointer modes, privacy, resize, daemon loss, dynamic targets and cast IPC/events; actual Firefox, Chromium and OBS consumers. | M12 Task 6/8 |
| Remaining client protocols | Gamma apply/restore, tearing policy/fallback, explicit-sync acquire/release and tablet tool/pad routing. Each needs behavioral proof; supporting hardware must qualify capabilities that nested/vkms cannot exercise. | M12 Task 7 |
| X11 applications | xwayland-satellite lifecycle, DISPLAY publication, native/X11 focus, and Steam/Discord in nested and real sessions. | M13, `sysc-1278` |
| Accessibility | KeyboardMonitor/announcements and a real Orca session. | M14, `sysc-1279` |
| Output and action parity | Runtime/persistent output configuration, mixed DPI, VRR, GPU reset, missing IPC/actions/binds, easing, remaining rules and config effects; default and owner configs must use no unimplemented actions. | M15, `sysc-1280`; matrix |
| Session integration | Retest production sysc-lock lock/unlock after its compatibility fix; prove shell idle blank/resume and injection rejection. Complete suite install/start/stop integration without replacing a host session during testing. | `sysc-1643`, `sysc-1283`; sibling owners |
| First-use behavior and release proof | Default config/binds, mouse/scroll binds, remaining protocol consumer checks, multi-output and physical-input qualification, installation/rollback and the final release CI/packaging checks. Track each change in its responsible milestone/repository. | Matrix, current handover, suite release standard |

M15 also owns the screenshot UI's easing and size binds, switcher preview fade-in and missing
opened-switcher actions. Background/bottom layer projection and remaining layout/rule effects
remain partial in the matrix. `sysc-1470` tracks repeated IPC snapshot rebuilding during motion;
measure it on the final session before deciding whether it gates the intended release.

## Close-out versus implementation

`sysc-1631` still awaits the owner's closure despite its recorded session evidence. M11
(`sysc-1276`) likewise awaits a scope/closure decision with its documented M15 follow-ups. Neither
requires rebuilding its completed functionality.

`sysc-1652` is closed as of this assessment: the sibling locker compatibility fix is no longer
an implementation blocker. Its production-name acceptance remains part of `sysc-1643`.
The lane-2 quarantine contains no tests; the old overview/remap quarantine is not remaining work.

## Distance to release

A runnable native preview exists today. A supported Niri replacement still needs the remainder
of M12 and M13–M15, followed by suite and hardware acceptance. This is several feature/integration
stages, not just a packaging pass. A date or percentage would require estimating those stages
after the portal/PipeWire consumer gate; current test counts cannot supply that estimate.

The next stage is merging M12 Task 5, then implementing Task 6. Keep each protocol/lifecycle regression with
its implementation and require local review plus both CI lanes before merge. Do not mark M12
complete merely because services advertise their names.

## Test environment

The owner has assigned today's runtime checks to the baremetal laptop bench; the desktop is
physically unavailable. Use the standing named-test consent in the testing handover. Keep builds
uninstalled and host greetd, lock and graphical-session.target untouched. Session services must
use a private bus for nested checks. A real same-user `--session` run would overwrite the host
activation environment and is not an acceptable substitute for an isolated session gate.

The bench currently has PipeWire 1.6.10, IBus 1.5.34, GTK 3.24.52/4.24.1, Chromium
156.0.8078.47, and GNOME/GTK portals. Its IBus Wayland component implements input-method-v2 and
virtual-keyboard. Firefox and OBS are absent; no consumer evidence for them is claimed.
