# Handover: 2026-11-03, after the touch tranche

Supersedes the 2026-11-03 sizing-and-bar handover for planning. **bd is
authoritative for what is open.** The bd DB lives at
`/home/nomadx/sysc-shell/.beads`; run `bd` from `/home/nomadx/sysc-shell`,
never from a worktree.

## State in one paragraph

The touchscreen now works in the compositor. `sysc-2227` is implemented,
tested, documented and pushed: the compositor handles a touch device itself,
advertises the seat's touch capability, and delivers a finger to the client
that asked for touch, as Niri does. The tranche is on draft PR #91 with green
CI on both lanes. **The evidence route the gate needs now exists**: a
`sysc-compositor touch-probe` client that asks the seat for touch and prints
every event it receives, plus the compositor's own log lines for the seat's
capabilities and for every finger. The physical gate on the archThink is
staged and **armed, but not yet run**: the new binary is on the laptop, a
systemd user drop-in that runs it without sudo is in place, and the gate is a
login cycle plus `capture-touch-gate.sh` and a finger on the panel. The
sizing family (`sysc-2228`) and the bar auto-hide (`sysc-2229`) gates are
still the owner's to run on the same machine. The packaging tranche stays
deferred, and PR #89 and PR #91 stay draft until the physical gates pass.

## Repositories and their state

### sysc-compositor (/home/nomadx/sysc-compositor)

`main` is at `1b03199` ("docs: record the state after the sizing milestone and
the bar work"). The commit map on `main`: `12c42e5` test(server) focused-window
golden JSON; `1ac7ed4` feat(ipc) report `is_fullscreen`; `5232ebc` fix(layout)
keep the gaps around a maximised column; `3effbcd` docs milestone; `aaf6b64`
feat(layout) niri column width and window height actions.

Three branches matter. The **code** commits are listed; each branch also carries
doc commits on top of them, which is why the tip is ahead of the last commit
named here.

| Branch | Code commits | What it is |
|---|---|---|
| `main` | `1b03199` | the merged sizing and bar work |
| `feat/sysc-input-20261102` | `c082c44` | the input branch: PR #89, draft |
| `feat/touch-input-20261103` | `0b1a5e9`, `699a97e`, `db64bcc`, `6226600` | main plus the touch tranche and the evidence route: PR #91, draft |
| `feat/laptop-touch-gate-20261103` | `02e3335`, `72ce0d6`, `59f135f`, `aec02f4` | the input branch plus the touch tranche and the evidence route: what the laptop runs |

**`main` does not carry the pointing-device work.** The pointer settings
(`internal/config/pointer.go`, `internal/wlr/pointer_settings.go`,
`internal/wlr/libinput.go`, `internal/wlr/devicekind.go`) live only on
`feat/sysc-input-20261102`. A plain `main` build would regress mouse
handedness, scroll factor and tap-to-click on the laptop, so a laptop build
always comes from the input branch. The gate branch
`feat/laptop-touch-gate-20261103` is the input branch with the three touch
commits cherry-picked on top (`02e3335`, `72ce0d6`, `59f135f`) and the
evidence route on top of those (`aec02f4`, "feat(input): add the touch
evidence route"). The
cherry-picks needed five conflicts, all resolved by keeping both sides: the
POINTER case in `internal/wlr/input.go` keeps `c.applyPointerSettings(device)`
and gains the TOUCH case; `internal/server/config_reload.go` keeps
`Pointers:` and gains `Touch:`; `internal/wlr/compositor.go` keeps
`Pointers config.Pointers` and gains `Touch config.Touch` in three places;
the `input` row of the parity matrix keeps the pointing-device sentence and
gains the touch sentence.

Open PRs: **#91** (touch, draft), **#89** (input, draft), **#92** (the
performance research plan, draft, not ours). #89 and #91 stay draft until the
physical gates pass. PR #91 carries the touch tranche (`0b1a5e9`, `699a97e`,
`db64bcc`) and then the evidence route (`6226600`, "feat(input): add the touch
evidence route"), with its docs commits after it; the gate branch carries the
same work under its own hashes, because the two branches have different
ancestors.

### sysc-shell (/home/nomadx/sysc-shell)

PR **#616** `feat/bar-autohide-fullscreen` (head `e1c02dc0`) hides the bar of
the output whose focused window is fullscreen. PR **#612** (mouse and
touchpad settings) stays draft for the same gates as #89. The shell's primary
checkout is on branch `audit/toasthost-wayland-crash` with **20 modified
files, +1370/-989 uncommitted** — **do not touch it**. The bar work was done
in the worktree `/home/nomadx/.config/superpowers/worktrees/sysc-shell/bar-autohide-20261103`.

## The touch tranche

### What was built

`internal/wlr/touch.go` handles a `wlr_touch` device at the **device** level.
It is deliberately not attached to the pointer's cursor: the cursor path needs
the device mapped to an output before the event arrives, and wlroots applies
the output transform inside its own cursor-side listener. sysc owns the
mapping instead, in pure Go, so it is unit-testable.

The rules, all of them:

- The seat advertises `WL_SEAT_CAPABILITY_TOUCH` when touch is on and a
  device exists.
- A finger's position is normalised over the device; it is mapped through the
  output's transformed logical box, with the output transform applied in the
  unit square (`internal/wlr/touch_map.go`, the eight `wl_output_transform`
  cases, mirroring wlroots' own `apply_output_transform`).
- The output is the one `input.touch map-to-output` names, else the first
  output. Niri's `evt.device().output(state)` is always `None` for the
  libinput backend, so niri always uses the fallback too.
- The touch reaches **only the client that bound `wl_seat.get_touch`**. A
  client that never asked for touch gets nothing, and the compositor does
  **not** fall back to emulating a mouse. Mango's mouse-emulation fallback is
  deliberately not copied.
- The window under the finger is activated, as Niri's `activate_window` does.
- The finger keeps its surface for the rest of the stroke (the implicit
  grab), so a drag that leaves the window still reports to it.
- A sealed session, a modal UI (the overview, the switcher, the colour
  picker, the screenshot UI, the exit dialog), a destroyed surface and a
  destroyed device each end the touch.

**The evidence route** is a second piece of work on top of the tranche:
`internal/touchprobe` (a self-contained Wayland client), the
`sysc-compositor touch-probe` command that runs it, the compositor's log
lines for the seat's capabilities and for every finger, and the lane-2 test
that proves the probe against a nested compositor. See "How to see whether the
gate passed" below.

### sysc-wayland needs no change

The sibling library already carries the whole `wl_touch` surface, so nothing
has to change there for this tranche:
`/home/nomadx/sysc-wayland/client/client.go` has `Seat.GetTouch()` (line
4761), `SeatCapabilityTouch` (line 4806), the `wl_touch` interface (line
6276 onwards) and `Touch` with `SetDownHandler`, `SetUpHandler`,
`SetMotionHandler`, `SetFrameHandler`, `SetCancelHandler`,
`SetShapeHandler` and `SetOrientationHandler`. It is generated from the
vendored `protocols/wayland.xml`.

No consumer calls `GetTouch` yet, and that is correct rather than a gap: the
compositor hands a finger only to the client that asked for touch, so a shell
client has to opt in deliberately. Because there is no mouse emulation, a
finger over a surface whose client never bound touch does nothing at all — so
the shell panels stay finger-dead until a shell client binds touch. That is
the shell repo's decision, and it does not gate this tranche.

### One wlroots caveat worth writing down

`wlr_seat_touch_notify_cancel(seat, NULL)` is a NULL dereference in wlroots
0.20.2: `wlr_seat_touch_send_cancel` walks `seat_client->touches` with no
NULL check. With a nil seat client it also clears no points, because the
point-destroy loop compares `point->client == seat_client`. So the
compositor ends each finger on its own with `wlr_seat_touch_notify_up`
(`internal/wlr/touch.go`, `endTouchPoints`). The first CI run of PR #91
faulted exactly here, on the device-removal path.

### The config surface

`input.touch` takes `off` and `map-to-output`, as Niri's `input.touch` does.
`calibration-matrix` is accepted and warns: no device class applies a
calibration matrix yet. `map-to-focused-output` is tablet-only and is
rejected here, as it is in Niri.

### The tests

- `internal/config/config_test.go`, `TestParseTouch` — lane 1.
- `internal/wlr/touch_map_test.go`, `TestTouchTransformPointFollowsWlroots`
  and `TestTouchInverseTransformPointUndoesTheTransform` — pure, no
  compositor.
- `internal/wlr/touch_test.go`, `TestTouchReachesTheClientThatAskedForIt`,
  `TestTouchOffKeepsTheSeatSilent` and
  `TestTouchReachesNothingWhileTheSessionIsLocked` — lane 2, under vkms.
  The test client gained a `wl_touch` listener and a `BindTouch` call
  (`internal/wlr/testclient/toplevel.go`); the touch is **not** bound
  automatically, so a test can have a client that never asks for touch.
- `internal/wlr/touch_probe_test.go`, `TestTouchProbeSeesTheFinger` — lane 2,
  under vkms. The probe client — the same one the laptop gate uses — sees the
  finger the compositor sends.
- `internal/touchprobe/probe_test.go`, `TestCapabilityNames`,
  `TestSummaryCountsTheFinger` and `TestHasTouchCapability` — lane 1, pure.

CI run `41604096155` is green on both lanes: lane 1 `check` and lane 2
`compositor` under vkms. Every touch test ran and passed, including
`TestTouchProbeSeesTheFinger`, which is the evidence route. The earlier run
`41588290485` was green for the tranche alone.

## The laptop (archThink)

`ssh laptop` (192.168.0.64, port 7777, user nomadx). `hostname` is not on
PATH; use `cat /etc/hostname`, which says `archThink`. The clock is UTC.
`/home/nomadx` is **not** shared with archPC: the gate directory exists
separately on each machine, so anything the laptop needs has to be copied
over.

**The touchscreen is there.** `/proc/bus/input/devices` shows `ELAN
Touchscreen` on `usb-0000:00:14.0-6/input0` with handlers `event4 mouse0`
and `PROP=2`. Two further `ELAN Touchscreen UNKNOWN` devices appear as
event5 and event6. The running compositor (pid 1021) has `/dev/input/event4`
open, so libinput already sees the panel: the old build simply had no touch
branch, so the seat never advertised touch. That matches the owner's
observation that the deployed compositor does not use the touchscreen.

**The staged install.** The gate directory is
`~/.cache/sysc-compositor-gate-20261103/` and now holds:

- `bin/sysc-compositor.touch` — the new build, 17478872 bytes, sha256
  `c6c9e6a810d0e27613459fdb59706c5ae185aff07e93968fbb71d0870f58b11f`. It was
  built on archPC from `feat/laptop-touch-gate-20261103` at `84dc7a0` and
  copied over; its version string reads `84dc7a0+dirty` because the checkout
  carries the untracked `.commandcode/` directory. It carries the touch tranche
  **and** the `touch-probe` evidence route. Two earlier staged builds were
  replaced: the touch tranche alone (17385096 bytes, sha256
  `86a6b52918464d3bc51f68050f5745cf12f3acb7f4a759d60e8a84e57e036bb5`) and the
  tranche plus the probe built from a dirty tree (17478904 bytes, sha256
  `a8b2293e0d07e35a3e7b69417d45fabc9cde58957a1da54d77ca1f500dca45e3`).
- `install-touch-compositor.sh` — the installer, following the pattern of
  `install-gapfix-compositor.sh`: it refuses to run as root, refuses to run
  anywhere but archThink, refuses to run twice, backs the current binary up
  to `before-touch-install/`, installs to a temporary name and moves it over
  `/usr/local/bin/sysc-compositor` (so the running process keeps its inode),
  and writes `before-touch-install/restore.sh`.

**The owner runs this, because sudo needs a password:**

```
sh ~/.cache/sysc-compositor-gate-20261103/install-touch-compositor.sh
```

**A unit restart ends the graphical session.** `systemctl --user restart
sysc-compositor.service` is not a quiet restart here.
`/usr/local/bin/sysc-compositor-session` runs `systemctl --user --wait start
sysc-compositor.service` and then forces `sysc-compositor-shutdown.target`,
so when the compositor exits the whole graphical session is torn down with it:
the shell, the panels and Xwayland go, and the environment the compositor
published is unset. A login cycle is the way in, not a service restart.

**A drop-in deploys the build without sudo.** A user drop-in overrides
`ExecStart`, so the staged build can run without touching
`/usr/local/bin` and without a password. Three scripts are staged next to the
build:

```
sh ~/.cache/sysc-compositor-gate-20261103/arm-touch-gate.sh      # writes the drop-in
sh ~/.cache/sysc-compositor-gate-20261103/capture-touch-gate.sh  # runs the probe, records the finger
sh ~/.cache/sysc-compositor-gate-20261103/disarm-touch-gate.sh    # removes the drop-in
```

`arm-touch-gate.sh` writes
`~/.config/systemd/user/sysc-compositor.service.d/10-touch-gate.conf` with
an `ExecStart=` override pointing at
`~/.cache/sysc-compositor-gate-20261103/bin/sysc-compositor.touch`,
reloads the user daemon and prints the unit's new `ExecStart`. It changes
nothing that is running. The drop-in is **armed** on the laptop now;
`disarm-touch-gate.sh` puts the installed build back, also without sudo, and
ssh keeps working if the session dies.

**The gate, step by step.** Log out of the graphical session and log back
in; the staged build starts. Then:

```
sh ~/.cache/sysc-compositor-gate-20261103/capture-touch-gate.sh
```

It runs the probe against the session (`WAYLAND_DISPLAY=wayland-0`) and writes
what the finger did to `touch-gate-capture.txt`. The probe maps a small
200x150 window and takes the focus: **touch inside that window**. A finger
anywhere else belongs to a client that has not asked for touch, and the
compositor drops it with the reason `the client did not ask for touch`.

### What was checked before the gate

- The running old build advertises no touch. The probe against the live
  session prints `touch-probe: seat capabilities=pointer|keyboard` and exits
  3, so the evidence route's negative path works on the real session.
- udev classifies the panel as a touchscreen:
  `/dev/input/event4` has `ID_INPUT_TOUCHSCREEN=1`, `ID_INPUT_WIDTH_MM=309`,
  `ID_INPUT_HEIGHT_MM=170` and
  `ID_PATH=pci-0000:00:14.0-usb-0:6:1.0`, so libinput makes it a touch
  device and the new build will advertise `WL_SEAT_CAPABILITY_TOUCH`. The
  other two `ELAN Touchscreen UNKNOWN` devices (event5, event6) carry no
  `ID_INPUT_TOUCHSCREEN` and no ABS axes; libinput ignores them.
- The staged build runs on the laptop: a headless run of it reached
  `WAYLAND_DISPLAY=wayland-1` with the owner's config, so the binary itself
  is not broken. The libinput backend cannot be used for the same check,
  because libseat cannot open a seat the live session already owns.
- The panel's output is `eDP-1`, 1920x1080, scale 1, and the owner is in the
  `input` group.
- The compositor reads the owner's own config,
  `~/.config/sysc-compositor/config.kdl` (461 lines, the Niri default). It
  uses window-rule keys sysc has no counterpart for (`open-maximized`,
  `draw-border-with-background`, `opacity`) and the unsupported match
  property `match is-active=false`, so the compositor prints "config section
  `window-rule` is accepted but not applied yet" at every start. The
  installed build prints the same warnings today, so this is the
  config-surface gap (open work item 4), not a gate failure. The rest of
  that config parses: `default-column-width { proportion 0.5; }` is
  supported, several `match` lines in one block are supported, and the
  layer-rule applies.

**How to see whether the gate passed.** `wayland-info` and `wlr-randr` both
hang against the compositor's socket
`/run/user/1000/sysc-compositor.wayland-0.1021.sock` and print nothing, so
they cannot be used as evidence. The route that works is built now, in two
halves.

*The compositor logs what it does.* The journal carries a line for every
change of the seat's capabilities (`sysc-compositor: seat capabilities:
pointer|keyboard|touch`), a line when a touch device arrives or goes, and a
line for every finger: `sysc-compositor: touch down id=1 delivered to the
client`, or why it was dropped (`input.touch is off`, `the session is locked`,
`a modal surface is open`, `no surface under the finger`, `the client did not
ask for touch`), and `touch up id=1`.

*The client asks for touch and reports it.* The new binary carries
`sysc-compositor touch-probe`: a small Wayland client that binds
`wl_seat.get_touch`, maps a window so a finger has a surface to land on, and
prints every event.

```
sysc-compositor touch-probe --timeout 60s
```

It exits 0 when a finger arrived, 1 when the run ended with no finger, 3 when
the seat does not advertise touch, and 4 when the probe could not run. The
lines are stable: `touch-probe: seat capabilities=pointer|keyboard|touch`,
`touch-probe: touch bound`, `touch-probe: down id=1 x=… y=… surface=probe`,
and a summary with the counts. The probe maps a window, so the owner runs it
on the laptop's session where the finger is.

The same client is proved in CI lane 2 against a nested compositor
(`TestTouchProbeSeesTheFinger`), so the route is known to work before the
laptop is touched.

**Other laptop notes.** The systemd user unit is a system file at
`/usr/lib/systemd/user/sysc-compositor.service`; a user drop-in under
`~/.config/systemd/user/sysc-compositor.service.d/` can override `ExecStart`
without sudo. The journal shows repeated `[ERROR] [backend/drm/atomic.c:82]
connector eDP-1: Atomic commit failed: Device or resource busy` every few
minutes, and one `failed to read client connection (pid 2351)`. These predate
the touch work.

## The audits

`docs/plans/2026-11-03-compositor-audit.md` holds the audit: the Ctrl+F
proof, the 13 default binds that did nothing, the two silent config
warnings, the overlay advertising unapplied actions, the two CI flakes and
the wlroots listener sweep.

Two real CI failures remain. `TestAnimateValuePreservesSameTargetAndRetargetsSample`
was a timing flake and is fixed. `TestPortalColorPickerReturnsPixelAndCancels`
is still open and intermittent: a stale or empty render buffer under vkms
gives a black pixel. Bead `sysc-2219` covers the overview scroll fixture
flake.

## Owner decisions in force

- **The attribution guard is not to be edited or bypassed.**
  `/home/nomadx/.git-hooks/` holds `commit-msg` and `pre-push` hooks that
  reject any commit message or author metadata matching `cursor`, `claude`,
  `anthropic`, `chatgpt`, `openai`, `copilot`, `cody`, `tabnine`, `codex`,
  `gemini`, `bard`, `gpt-[0-9]`, `llm`, `ai assistant`, `bot`, `agent`.
  **"bot" matches inside "both"** — commit subjects must say "the two",
  never "both". Use "pointer image" and "pointer surface" for the wlroots
  pointer API. The hooks only read commit messages, never file contents, so
  upstream identifiers in code are fine.
- **Never merge `main` into a feature branch.** The PR #90 merge commit
  carried the branch name and the guard refused every push containing it.
  Cherry-pick instead; that merge commit was rebuilt locally with the same
  tree and the same two parents under a clean message, and `main` was
  force-pushed with `--force-with-lease`.
- Bar and fullscreen window: keep Niri parity. The compositor does not change
  fullscreen geometry; the shell hides its own bar, and the compositor
  reports `is_fullscreen`.
- The packaging tranche is deferred until the sizing-family laptop gate and
  the audits are done.
- There is no local nested compositor lane. CI lane 2 under vkms is the
  gate.

## Merging to main when the gates pass

The owner's decision on 2026-11-03: PR #89 and PR #91 stay draft until the
physical gates pass. Nothing was merged. What was checked, so the next
session does not check it again:

- `main` is `origin/main` is `1b03199`; nothing was left unpushed.
- PR #89 (pointer settings) and PR #91 (touch) are green on both CI lanes
  and both `MERGEABLE`/`CLEAN` against `main`.
- **Do not merge #91 alone.** `main` has no `internal/config/pointer.go`, no
  `internal/wlr/libinput.go` and no `internal/wlr/pointer_settings.go`; those
  are #89's work. A `main` build after #91 alone would still regress mouse
  handedness, scroll factor and tap-to-click.
- The merge mechanics, checked with `git merge-tree` (dry, no worktree
  touched): #89 into `main` is clean and #91 into `main` is clean (`main` is
  an ancestor of the touch branch), but the two together conflict, in either
  order, in `internal/config/config.go`, `internal/server/config_reload.go`,
  `internal/wlr/compositor.go`, `internal/wlr/input.go` and
  `docs/plans/niri-parity-matrix.md` — plus `docs/README.md` and
  `docs/plans/2026-11-03-compositor-audit.md` in the other order.
- The gate branch `feat/laptop-touch-gate-20261103` already holds the
  resolved union: its `internal/wlr/touch.go`, `internal/wlr/touch_map.go`,
  `internal/touchprobe/probe.go`, `internal/cli/touchprobe.go` and
  `internal/wlr/touch_probe_test.go` are byte-identical to #91's, and the
  four shared files carry the pointer work and the touch work together.
- **The union tree has never been through CI.** CI runs on pull requests and
  on `main` only, and there is no run for the gate branch. Opening a pull
  request from the gate branch is how CI gets to see the exact tree the
  laptop runs.
- Never merge `main` into a feature branch; cherry-pick instead.

## Open work, in order

1. **The touch gate on the laptop** (`sysc-2227`, P2). The build is staged
   and the no-sudo drop-in is armed, so the gate is: log out, log back in,
   then `sh ~/.cache/sysc-compositor-gate-20261103/capture-touch-gate.sh`
   and touch inside the probe's 200x150 window. The evidence route is built:
   `sysc-compositor touch-probe` on the laptop's session, and the journal's
   touch lines (see "How to see whether the gate passed" and "What was
   checked before the gate"). The session's new build is the staged one
   until `disarm-touch-gate.sh` is run.
2. **The sizing family gate on the laptop** (`sysc-2228`, P1). The owner
   compares against Niri on the same hardware: maximise-column,
   maximise-window-to-edges, centre-column, centre-visible-columns,
   expand-column-to-available-width, set-column-width,
   switch-preset-column-width(-back), set-window-height, reset-window-height,
   switch-preset-window-height(-back). Close the bead when it passes.
3. **Bar auto-hide** (`sysc-2229`, P1). PR #616 needs a look on the laptop
   once a shell build carrying it is installed; the release path is the shell
   repo's business.
4. **The config surface that is really not applied** (`sysc-2231`, P2). The
   handover's earlier
   claim was wrong and was checked on 2026-11-03: sysc does **not** ignore
   `window-rule` and `layer-rule` as a whole. `validate` warns per unsupported
   **sub-key**, and the compositor applies the supported ones. `window-rule`
   supports `match`/`exclude` (on `app-id` and `title`), `open-on-output`,
   `open-on-workspace`, `open-floating`, `open-fullscreen`,
   `default-column-width`, `default-window-height`, `geometry-corner-radius`,
   `clip-to-geometry` and `block-out-from`; `layer-rule` supports
   `match`/`exclude` (on `namespace`) and `place-within-backdrop`. The shipped
   default's Firefox picture-in-picture rule is applied: the rules resolve when
   a window maps (`internal/wlr/windows.go`), so the rule opens that window
   floating. `validate -c internal/config/default-config.kdl` prints no warning
   at all.
   What is accepted and ignored is `workspace.layout`, Niri's per-workspace
   layout override — the only whole section that warns and does nothing. The
   rest of the gap is the Niri keys sysc has no counterpart for
   (`open-maximized`, `open-maximized-to-edges`, `open-focused`, `min`/`max`
   `width`/`height`, `focus-ring`, `border`, `shadow`, `opacity`,
   `default-column-display`, `default-floating-position`, `scroll-factor`,
   `background-effect`, `popups`, `variable-refresh-rate` and more) and the
   unsupported match properties (`is-active`, `is-focused`,
   `is-active-in-column`, `is-floating`, `is-window-cast-target`, `is-urgent`,
   `at-startup`, plus `layer-rule.match`'s `at-startup` and `layer`). Those
   warn, so the user is told. A tranche here is milestone-sized and wants its
   own plan, the way the touch tranche did. The archThink's own config shows
   the gap on a real machine: it uses `open-maximized`,
   `draw-border-with-background`, `opacity` and `match is-active=false`, so
   the compositor prints the `window-rule` warning at every start.
5. **The two flakes**: the portal colour picker under vkms, and the overview
   scroll fixture (`sysc-2219`).
6. **Packaging** (`sysc-2200` plus the children `sysc-2201`–`2213`) once the
   owner lifts the deferral. Accounts, pushes and upstream submissions are
   owner gates.
7. PR #89, PR #91 and shell PR #612 stay draft until the physical touchpad
   and touch gates and the packaging gate pass. What to do when they do is
   written down in "Merging to main when the gates pass".

## Working notes

- Build and test:
  `CGO_CFLAGS_ALLOW=-fno-strict-overflow GOTMPDIR=/home/nomadx/.cache/go-tmp TMPDIR=/home/nomadx/.cache/go-tmp timeout 120s env GOMAXPROCS=2 go test -count=1 <pkg> -run <Name>`.
  The `-fno-strict-overflow` comes from `pkg-config wlroots-0.20`; cgo refuses
  it without the allowance. archPC's `/tmp` is a nearly-full tmpfs, so both
  `GOTMPDIR` and `TMPDIR` must point at the cache directory or the linker
  fails with "disk quota exceeded".
- **One package and one named test at a time. Never `go test ./...`, never
  `-race`.** `internal/wlr` and `internal/server` are excluded from the local
  test policy because they need a compositor; CI lane 2 runs them under vkms.
  A nested compositor test needs `SYSC_NESTED_TESTS=1` **and** the owner's
  explicit consent for that run. Never in a loop, a script or a background
  job.
- `runtime.LockOSThread()` before any wlroots call. Never call wlroots off the
  compositor thread.
- Go and cgo only. No `.c` or `.h` in this tree. Listener trampolines go in
  the cgo preamble of a `.go` file.
- After a fresh clone, run `go generate ./internal/wlr` once: it writes the
  protocol headers wlroots does not install (from `protocols/*.xml`) into the
  gitignored `internal/wlr/protocols/`. Never commit them.
- sysc-wayland is a **client**. Do not turn it into the server.
- Do not implement a bar, lock screen, wallpaper engine, notification server
  or tray here.
- The Niri reference checkout is `/home/nomadx/references/niri` at tag
  `v26.04` (commit `8ed0da44d974c32c6877d2f4630c314da0717ecb`). Prefer it over
  Niri `main`. The closest wlroots-0.20 implementation reference for touch is
  `/home/nomadx/references/mango` at 0.18.0.
- **Never `git add -A` in the compositor checkout** — it sweeps in the
  untracked `.commandcode/` directory.
