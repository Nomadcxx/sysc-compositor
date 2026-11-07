# Touchscreen input — plan and record

Milestone: native touch routing. Date: 2026-11-03. Status: implemented, the laptop gate passed.

Bead: `sysc-2227`. Depends on: `sysc-2199`.

## Why

The laptop has an ELAN touchscreen. It does not work at all: the compositor never looked at a touch
device.

`internal/wlr/input.go` handled the keyboard, the pointer, the tablet, the tablet pad and the
switch, and everything else fell through to `default: return`. A `WLR_INPUT_DEVICE_TOUCH` device
was therefore never attached, never listened to, and never counted. `refreshCapabilities`
advertised `POINTER` and `KEYBOARD` and nothing else, so the seat never offered
`WL_SEAT_CAPABILITY_TOUCH` and no client could ask for a `wl_touch`.

wlroots 0.20 has the whole touch path — `wlr_touch`, the seat's touch points, the grab rules, the
`wl_touch` protocol — but it is a substrate, not a policy: it delivers the events, and the
compositor decides where a finger lands, which window it belongs to, and whether it is allowed to
go anywhere at all. That decision is the work.

This milestone gives the compositor a real touch path: native touch routing, the touch
capability, and the focus rules that make a finger behave like a click on a window.

## What niri does

Niri 26.04 is the behaviour target. Its rules, in the order it applies them:

1. **Where the finger is.** `compute_touch_location` resolves the device's normalised position
   into layout coordinates. The output is chosen by `output_for_touch`: the configured
   `input.touch.map-to-output` if it names an output, otherwise the first output. The position is
   then rotated by the output's transform and offset by the output's place in the layout.
2. **What is under the finger.** `contents_under` finds the surface, the window and the output.
3. **What happens.** Niri activates the window under the finger, focuses the output when the finger
   is over one, and hands the touch to the surface. The pointer is hidden while touch is used.
4. **The protocol.** Niri does not check `wl_surface_accepts_touch`; it delivers the touch to the
   surface it found.

Two details of niri's mapping are worth stating because they are not obvious:

- Niri's `evt.device().output(state)` is **always `None`** for the libinput backend
  (`src/input/backend_ext.rs`: `impl NiriInputDevice for libinput::Device { fn output(...) ->
  Option<Output> { None } }`). So niri always falls back to the configured output or the first
  output, whatever the device claims.
- The transform happens in the **unit square**, not in pixels. The device position is rotated as a
  fraction of the output, then scaled to the output's transformed logical size. This is the same
  arithmetic wlroots' `apply_output_transform` does.

## What sysc-compositor does now

The compositor handles the touch device itself, at the device level, and owns the mapping. It
does **not** attach the touch device to the pointer's cursor: the cursor path needs the device
mapped to an output before the event arrives, because wlroots applies the output transform inside
the cursor's own device listener. Owning the mapping in one place is easier to prove than
relying on the cursor's state at the moment an event fires.

### The mapping

| Question | Answer |
|---|---|
| Which output? | The device's own `output_name` when it names an enabled output; else `input.touch.map-to-output` when it names one; else the first output in the layout. This is niri's order, with niri's device claim kept for the backends that make one. |
| Which box? | The output's transformed logical box from `wlr_output_layout_get_box`: the size the picture is drawn at, with the sides already swapped for a 90° or 270° transform. |
| Which rotation? | The output's `wl_output.transform`, applied in the unit square, exactly as wlroots' `apply_output_transform` does. |
| Where does it land? | `lx = box.x + box.width * tx`, `ly = box.y + box.height * ty`, with `(tx, ty)` the rotated normalised position. |

The eight transforms, in the unit square:

| Transform | `(tx, ty)` from `(x, y)` |
|---|---|
| `normal` | `(x, y)` |
| `90` | `(1 - y, x)` |
| `180` | `(1 - x, 1 - y)` |
| `270` | `(y, 1 - x)` |
| `flipped` | `(1 - x, y)` |
| `flipped 90` | `(y, x)` |
| `flipped 180` | `(x, 1 - y)` |
| `flipped 270` | `(1 - y, 1 - x)` |

`internal/wlr/touch_map.go` holds this as a pure function, so it can be proved against wlroots
without a compositor.

### The rules

- **A touch reaches a client that asked for one.** The compositor checks
  `wlr_surface_accepts_touch` before delivering, which is true only for a client that bound
  `wl_seat.get_touch`. A client that never asked for touch is never touched.
- **Touch is never emulated as a mouse.** There is no mouse-emulation fallback. The bead for this
  work forbids it, and a compositor that fakes a pointer would break the client that *did* ask
  for touch. A finger therefore produces pointer buttons on no client at all.
- **The window under the finger is activated**, as niri activates it, before the capability check:
  a tile whose client never bound `wl_touch` still takes focus from a tap, and gets no event. The
  pointer's focus is cleared at the same time, so the two input paths do not fight over the same
  client.
- **The finger keeps the surface it went down on.** The layout-to-surface offset is recorded when
  the finger lands, and every later motion is reported relative to it. This is the implicit grab,
  and it is what lets a drag leave the window it started in.
- **A sealed session gets nothing.** While the session is locked, the lock tree sits above
  everything, so the hit test finds the lock surface and never a client window. The compositor
  also refuses touch outright while locked.
- **A modal UI owns the finger.** The switcher, the screenshot UI, the colour picker, the exit
  dialog and the overview take touch before the windows do.
- **The surface that goes away releases the finger.** Each finger holds a listener on its
  surface's destroy signal, so a window that dies mid-gesture does not leave a dangling touch
  point.
- **The device that goes away ends its fingers.** A touch device that is destroyed lifts every
  finger it held and drops its listeners.

### One wlroots caveat worth writing down

wlroots 0.20's `wlr_seat_touch_notify_cancel` takes a seat client and does not check it:
`wlr_seat_touch_send_cancel` walks `seat_client->touches` straight away, so a nil seat client
faults. A nil client also clears nothing, because the point loop matches on `point->client == NULL`,
which no real point does. Mango calls it that way; on 0.20 it cannot be called that way.

So the compositor ends each finger on its own with `wlr_seat_touch_notify_up`, which is a no-op
when the point is already gone. The device going away, a cancel from the device and `input.touch`
going off all go through one helper, `endTouchPoints`.

### The config surface

`input.touch` follows niri's:

| Node | Meaning |
|---|---|
| `off` | Touch does not reach the seat at all: the capability is not advertised and no finger is delivered. |
| `map-to-output "<name>"` | The output a finger belongs to. The name matches the connector, or `make model serial`, as niri's output names do. |
| `calibration-matrix` | Accepted and warned about, as `input.tablet`'s is: it is a libinput device setting that no device class applies yet. |

`off` and `map-to-output` are applied. `calibration-matrix` warns. A node niri does not have is an
error, so `map-to-focused-output` — which is a tablet node, not a touch node — is rejected.

## The files

- `internal/wlr/touch.go` — new. The touch device, the seat routing and the rules above.
- `internal/wlr/touch_map.go` — new. The transform, as a pure function.
- `internal/wlr/faketouch.go` — new. A synthetic touch device for the tests, built like the
  synthetic pointer and the synthetic tablet.
- `internal/wlr/input.go` — the touch case in `handleNewInput`, the cleanup in
  `handleInputDestroy`, the touch capability in `refreshCapabilities`, and `modalUIOpen`, which
  the tablet now shares.
- `internal/wlr/compositor.go` — the touch devices, the touch points, the touch config and the
  `Options.Touch` field.
- `internal/wlr/tablet.go` — `tabletModalUI` now asks the shared `modalUIOpen`.
- `internal/config/config.go` — the `input.touch` section and `config.Touch`.
- `internal/server/config_reload.go` — the touch config reaches the compositor on reload.
- `internal/wlr/testclient/toplevel.go` — the test client can bind `wl_touch` and report what it
  received. It does not bind automatically, so a test can have a client that never asks.
- `internal/touchprobe/probe.go` — new. The probe: a small Wayland client that asks the seat for
  touch and prints what it receives.
- `internal/cli/touchprobe.go` — new. The `sysc-compositor touch-probe` command that runs it.

## The tests

- `internal/config/config_test.go` — `TestParseTouch`: the section parses, `calibration-matrix`
  warns, and a node niri does not have is an error.
- `internal/wlr/touch_map_test.go` — `TestTouchTransformPointFollowsWlroots` and
  `TestTouchInverseTransformPointUndoesTheTransform`: the eight transforms, worked by hand, and
  the inverse round-trip. Pure Go, no compositor.
- `internal/wlr/touch_test.go` — `TestTouchReachesTheClientThatAskedForIt`,
  `TestTouchOffKeepsTheSeatSilent` and `TestTouchReachesNothingWhileTheSessionIsLocked`: a finger
  reaches the client that asked for touch and no other, no finger reaches anything while the
  session is locked, and `input.touch off` keeps the seat silent.
  `TestTouchFocusesWindowThatDidNotRequestTouch`: a tap focuses a tile whose client never bound
  `wl_touch`, with no touch event and no pointer button. `TestSessionLockEndsAnActiveTouch` and
  `TestModalOpeningEndsAnActiveTouch`: a finger already down is lifted when the session locks or a
  modal UI opens, so it sends nothing further to the client.
- `internal/wlr/touch_probe_test.go` — `TestTouchProbeSeesTheFinger`: the probe client, the same
  client the laptop gate uses, sees the finger the compositor sends.
- `internal/touchprobe/probe_test.go` — `TestCapabilityNames`, `TestSummaryCountsTheFinger` and
  `TestHasTouchCapability`: the probe's own reporting, pure Go, no compositor.

The compositor packages are covered by CI lane 2, which runs them under a virtual KMS.

## The evidence route

The physical gate needs evidence, and the ordinary tools cannot give it:
`wayland-info` and `wlr-randr` hang against the compositor's socket and print nothing, the seat's
capabilities live in `internal/wlr` only and are not exposed over the IPC, and the compositor did
not log them. So the gate has its own route, built in two halves.

**The compositor side logs what it does.** `internal/wlr/touch.go` writes a line to stderr — the
journal — when a touch device arrives or goes, and for every finger: whether it was delivered to a
client or dropped, and why it was dropped (`input.touch is off`, `the session is locked`, `a modal
surface is open`, `no surface under the finger`, `the client did not ask for touch`).
`internal/wlr/input.go` logs the seat's capabilities whenever they change, spelled out as
`pointer|keyboard|touch`.

**The client side asks for touch and reports it.** `internal/touchprobe` is a small Wayland client
that binds `wl_seat.get_touch` — only when the seat advertises the capability, since asking
without it is a protocol error — maps a toplevel so a finger has a surface to land on, and prints
every event. `sysc-compositor touch-probe` runs it:

```
sysc-compositor touch-probe [--display NAME] [--timeout DURATION] [--no-window]
```

It exits 0 when a finger arrived, 1 when the run ended with no finger, 2 on a usage error, 3 when
the seat does not advertise touch, and 4 when the probe could not run at all. The lines it prints
are stable: `touch-probe: seat capabilities=pointer|keyboard|touch`, `touch-probe: touch bound`,
`touch-probe: window mapped`, `touch-probe: down id=1 x=… y=… surface=probe`, and a summary line
with the counts.

The probe is the same client the CI lane-2 test runs against a nested compositor, so the route is
proved before it is used on the laptop.

## What is still open

Touch is not finished across the desktop. The gates below prove the compositor's routing only.

- **Shell surfaces (`sysc-2260`, fixed in sysc-shell).** The sysc-shell bar and panels bound only
  `wl_pointer`, and the compositor, like Niri 26.04, delivers a finger only to a client that bound
  `wl_touch`, so every tap on the bar was dropped. Mango has pointer emulation for such clients but
  ships it off. The owner chose the client-side fix on 2026-11-07: sysc-shell PR #622 binds
  `wl_touch` and turns the first finger into its pointer events. The owner accepted it on
  archThink (tapping bar items and using a panel). Dragging a panel's content to scroll is open
  (`sysc-2262`).
- **Wake and idle (`sysc-2254`).** Touch down and motion, swipe begins, and pinch and hold now count as
  input activity, and a release no longer wakes monitors that PowerOffMonitors turned off, as in
  Niri's `should_activate_monitors`. Pointer motion already woke them; the owner confirmed it with
  the trackpad on 2026-11-07.
- **The laptop gate passed on 2026-11-03.** The seat advertised `pointer|keyboard|touch`, the ELAN
  panel was picked up as a touch device, and a finger on the panel reached the probe's own window as
  a whole stroke — down, motion, up — with the compositor's log naming the same finger. The record
  is in `~/.cache/sysc-compositor-gate-20261103/touch-gate-capture.txt` on the laptop. The probe
  grew a `--until-stroke` flag for this, so the record holds the finger's lift as well as its
  landing; the exit code says which half was missing (5 = the finger arrived but never lifted).
- **The tile-focus gate passed on 2026-11-06** with build `a76110f` (main at `f902729` plus PR #89).
  Before PR #95 a tap on a tile whose client had not bound `wl_touch` was dropped before the
  activation, so a finger could focus nothing but a touch-aware client. On the archThink two kitty
  tiles were tapped in turn: the journal logged each touch-down as `the client did not ask for
  touch`, and IPC focus timestamps show focus moving to each tapped tile within about 80 ms of its
  touch-down. The owner confirmed the focus ring moved. The probe then received a whole stroke on
  the same build (down, four motions, up, no cancel); the record is
  `~/.cache/sysc-compositor-gate-20261103/touch-gate-capture-a76110f.txt` on the laptop. Chromium
  handled tap, pinch zoom and scrolling on the previous candidate (`03f9274`).
- Multi-finger gestures are not implemented. Each finger is tracked separately and delivered
  correctly, but pinch, rotate and swipe are Niri's gestures, not this milestone's.
- **The compositor's own UI takes touch (2026-11-07)**, as Niri's `on_touch_down` routes it. The
  first finger drives the screenshot UI, the window switcher, the exit dialog and the overview
  through the pointer: the cursor goes to the finger and the finger is the left button, so a tap
  confirms a switcher preview or cancels the switcher, selects an overview window, closes the exit
  dialog, and a drag draws a screenshot selection. In the overview a finger over a top or overlay
  layer surface goes to that surface instead. A touch down hides the hotkey overlay and is still
  delivered. The colour picker takes no touch, as Niri's does not. Tests:
  `internal/wlr/touch_ui_test.go`. Known differences: the cursor jumps to the finger, where Niri
  hides it; the overview takes taps only, not Niri's touch drags; a second finger does not move a
  screenshot selection; Mod with a finger does not start a move grab; a tab indicator tap waits for
  the config branch, which owns the tab indicator.
- Pinch and hold gestures count only as activity; they are not handled or forwarded to clients.
- `input.touch`'s `calibration-matrix` still warns.
