# sysc-compositor release readiness

Updated on 2026-10-30, after the M12 session VM gate (PRs #68–#70 and #72). This is a gate assessment, not a plan
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

PR #61 merged the session-only portal discovery/screenshot services. Nine named
private-bus regressions cover wire signatures, concurrent replies, socket ownership, malformed
requests, caller departure and bus/shutdown cancellation. Baremetal bench checks cover native
service clients, known color-picker pixels and cancellation, screenshot write errors, private
concurrent fallback PNGs, clipboard preservation, multi-output rejection and nested startup
leaving session service names alone. Local review and the static/compositor CI lanes passed.

Descriptor rejection also exposed a bug in the pinned godbus decoder. The reviewed local
backport retains v5.2.2 and adds no Go dependency; eight raw-transport cases and the portal
descriptor EOF check prove the correction. CI runs that named dependency regression.

The baremetal `TestNativeIBusComposesInGTK` passes with packaged IBus 1.5.34 and GTK 4.24.1:
text-input-v3/input-method-v2 activation, Unicode preedit and an `é` commit, then an unconsumed
Backspace returned through the IME's virtual keyboard exactly once down/up. GTK receives the
release and the final expected text. The test leaves a runnable acceptance check; CI records
its optional-tool skip because the CI image does not include this packaged client stack.

The PipeWire change adds compositor-owned casts and a session-only Mutter ScreenCast Version 4
service. Baremetal checks prove known window and monitor pixels, live size/rate negotiation,
dynamic retargeting and clearing, protected/locked black frames, pointer isolation, stop and
daemon loss. The complete private-bus-to-IPC-to-PipeWire path passes on the laptop. Private-bus
regressions cover caller ownership, property types, cancellation, stale readiness after loss
and forged departure signals. Independent review passes; merges require static and compositor CI.
Cast IPC also tracks legacy damage-copy managers,
with shared process-lifetime IDs, peer PID, queued output selection and ten-second inactivity
expiry. A mixed-producer bench check proves the IPC baseline and stop events, and confirms
PipeWire daemon loss preserves legacy activity. Ordinary screenshots create no cast record.

The M12 VM gate proved the consumers in a real greetd session with xdg-desktop-portal-gnome 51 and
-gtk 1.15.3: portal discovery, Screenshot and PickColor; monitor, window and dynamic ScreenCast;
cursor modes; `block-out-from` for casts and screenshots; Firefox 159, Chromium 156 and OBS 33
with a restore token. It found four bugs, all fixed with regression checks: the portal screenshot
directory (#68), the spawned programs' session environment (#69), WebRTC negotiation and link
failure (#70), and a config created after startup never loading (#72). The
[execution record](2026-10-24-m12-capture-portals.md#execution-record-vm-gate-2026-10-2930) lists
versions and evidence.

The project supports an experimental native session. A release that can replace Niri on this
machine still needs the gates below. Counting merged milestones would hide their size: X11
applications, accessibility and output management remain substantial work.

With the PipeWire change, the server dispatches 90 of the 141 Niri action names; 51 still return
`unimplemented`. This counts routed actions, not configuration,
hardware or integration completeness.

## Remaining release gates

| Gate | Work and acceptance evidence still needed | Owner |
|---|---|---|
| Client protocol hardware | Task 7's protocols have behavioral proof. Hardware qualification remains: gamma on a physical panel, explicit sync on NVIDIA, and a physical tablet and pad. Gamma control is proved on vkms. Explicit sync is proved with Vulkan on Intel; NVIDIA is not qualified. Tearing control records hints and never tears, as Niri; game frame pacing against Niri is `sysc-2056`. | M12 Task 7 |
| X11 applications | M13's VM checks prove on-demand satellite lifecycle/restart, DISPLAY, native/X11 focus, clipboard and Steam's sign-in window in nested and real sessions. Discord's updater maps, but its application page reports a network error on X11 and native Wayland; finish that usable-app gate. | M13, `sysc-1278` |
| Accessibility | KeyboardMonitor/announcements and a real Orca session. | M14, `sysc-1279` |
| Output and action parity | Runtime/persistent output configuration, mixed DPI, VRR, GPU reset, missing IPC/actions/binds, easing, remaining rules and config effects; default and owner configs must use no unimplemented actions. | M15, `sysc-1280`; matrix |
| Session integration | Retest production sysc-lock lock/unlock after its compatibility fix; prove shell idle blank/resume and injection rejection. Complete suite install/start/stop integration without replacing a host session during testing. | `sysc-1643`, `sysc-1283`; sibling owners |
| First-use behavior and release proof | Owner decision 2026-10-30/31: keep in-memory defaults and do not write a first-use config. Default binds, mouse/scroll binds, remaining protocol consumer checks, multi-output and physical-input qualification, installation/rollback and final CI/packaging checks remain. Track each change in its responsible milestone/repository. | Matrix, current handover, suite release standard |

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

A runnable native preview exists today. A supported Niri replacement still needs M12's hardware
qualification and M13–M15, followed by suite and hardware acceptance. This is several feature/integration
stages, not just a packaging pass. A date or percentage would require estimating those stages;
current test counts cannot supply that estimate.

M12's VM and CI gates have passed. Closing `sysc-1277` with hardware qualification still open is
the owner's decision; M13 (X11 applications) follows. Keep each protocol/lifecycle regression
with its implementation and require local review plus both CI lanes before merge.

## Mango and Pleamar capability work

The [approved capability design](2026-10-15-tier3-wm-scene-equivalent-design.md#owner-update-2026-10-20-selected-capabilities-under-the-niri-contract)
keeps Niri's KDL/IPC contract while selecting implementations per subsystem. Mango supplies
wlroots lifecycle, input and protocol patterns; each change must verify those against the
installed wlroots API. Task 7's protocol work is complete.

Pleamar-derived seat isolation, the local control socket, compositor-enforced Stop, lock/blank
gating and private live window capture already have implementations and regression checks.
The remaining selected work is physical-user-only re-arm (`sysc-1261`) and shell presence
(`sysc-1193`, owned by sysc-shell under suite integration). Worker containment (`sysc-1255`)
still gates the assistant consumer. Browser remote access (`sysc-1213`) needs its separate
authentication/consent/revocation design after capture; the capture implementation does not
authorize remote input. Further layout or authoring changes need a named workflow under the
existing Go+cgo compatibility boundary.

## Test environment

The owner has assigned today's runtime checks to the baremetal laptop bench; the desktop is
physically unavailable. Use the standing named-test consent in the testing handover. Keep builds
uninstalled and host greetd, lock and graphical-session.target untouched. Session services must
use a private bus for nested checks. A real same-user `--session` run would overwrite the host
activation environment and is not an acceptable substitute for an isolated session gate.

The bench currently has PipeWire 1.6.10, IBus 1.5.34, GTK 3.24.52/4.24.1, Chromium
156.0.8078.47, and GNOME/GTK portals. Its IBus Wayland component implements input-method-v2 and
virtual-keyboard. Firefox and OBS are absent; no consumer evidence for them is claimed.
