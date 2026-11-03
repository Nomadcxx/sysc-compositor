# Touchscreen input — plan and record

Milestone: native touch routing. Date: 2026-11-03. Status: implemented, awaiting the laptop gate.

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
- **The window under the finger is activated**, as niri activates it. The pointer's focus is
  cleared at the same time, so the two input paths do not fight over the same client.
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

The compositor packages are covered by CI lane 2, which runs them under a virtual KMS.

## What is still open

- The laptop gate: the touchscreen on the archThink has to answer. The ELAN panel reports
  `ID_INPUT_TOUCHSCREEN=1` on event4, so the device must now reach the compositor as a touch
  device; the owner has to confirm that a finger lands where it lands under Niri.
- Multi-finger gestures are not implemented. Each finger is tracked separately and delivered
  correctly, but pinch, rotate and swipe are Niri's gestures, not this milestone's.
- Touch does not drive the compositor's own UI yet — the overview, the switcher and the colour
  picker take touch only in the sense that a modal UI claims it first.
- `input.touch`'s `calibration-matrix` still warns.
