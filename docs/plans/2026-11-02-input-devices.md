# Input devices (compositor side) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** sysc-compositor parses and applies Niri's pointing-device sections (touchpad, mouse,
trackpoint and trackball) through libinput, with `scroll-factor` included. Both swipe gestures
follow natural scrolling.

**Architecture:**
- **Config.** `internal/config` decodes four device sections into one `PointerDevice` type each,
  with Niri's fields and with replace-not-merge semantics across includes.
- **Applying settings.** `internal/wlr` gains `libinput.go`, the one cgo file that includes
  `<libinput.h>`. A pure-Go `pointerSettings` plan type decides which config calls to make, and
  is unit-tested; the cgo layer only performs them. Settings apply on device add and on reload.
- **Device kind.** Read in pure Go from the udev database.

**Tech Stack:** Go and cgo, wlroots 0.20 (`wlr/backend/libinput.h`), libinput (`pkg-config: libinput`).

**Spec:** [2026-11-02-input-devices-design.md](2026-11-02-input-devices-design.md). The sysc-shell
half is `/home/nomadx/sysc-shell/docs/plans/2026-11-02-sysc-compositor-native-support.md`,
phase B.

## Global Constraints

- **C code.**
  - libinput is approved (owner, 2026-11-02), for `libinput_device_config_*` and
    `libinput_device_get_sysname` only.
  - `libinput_device_get_udev_device` must **not** be used, because it pulls in libudev, which is
    not approved.
  - No `.c` or `.h` files: the preamble lives in `internal/wlr/libinput.go`.
- **Threading.** Make libinput calls only on the compositor thread, which holds
  `runtime.LockOSThread`.
- **Field ranges** follow Niri 26.04 `niri-config/src/input.rs`: `accel-speed` −1…1, and the enum
  spellings exactly as Niri's.
- **Include merging.** A device section in a later include replaces the earlier one entirely.
- **Running tests.** Never run `go test ./...` or `-race`. Run one package with a named test, with
  `CGO_CFLAGS_ALLOW=-fno-strict-overflow`. Nested tests run only on the laptop, one per command.
- **Commit messages** are hook-screened, with no trailer.

## Review Focus

1. **A config reload that removes a touchpad section.** Every setting goes back to libinput's
   device default (`libinput_device_config_*_get_default_*`), not the previous value. Task 3 tests
   this with the plan type.
2. **A device that doesn't support a setting** (`UNSUPPORTED`), for example tap on a mouse. It is
   skipped silently, and the remaining settings still apply. Task 3.
3. **An unreadable `/run/udev/data` entry,** such as in a container with no udev. The device is a
   mouse, unless it has tap fingers (touchpad). Task 2.
4. **`scroll-factor` with a v120 wheel value that is not a multiple of 120 after scaling.** The
   v120 value is rounded and the delta scaled exactly, so wheel binds still accumulate correctly.
   Task 4.
5. **A virtual pointer, or the nested backend's pointer.** `wlr_input_device_is_libinput` is
   false: no settings and no crash. `scroll-factor` uses the mouse factor. Task 3, Task 4.

---

### Task 1: Config for the four pointing-device sections

**Files:**
- Modify: `internal/config/config.go` (`Input` at `:87`, and `input()` at `:468`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces, in `internal/config`:
  ```go
  type ScrollFactor struct{ Vertical, Horizontal float64 } // both 1 when unset
  type PointerDevice struct {
      Set                     bool // the section appeared at all
      Off, NaturalScroll      bool
      AccelSpeed              float64 // −1…1
      AccelProfile            string  // "", "adaptive", "flat"
      ScrollMethod            string  // "", "no-scroll", "two-finger", "edge", "on-button-down"
      ScrollButton            *uint32
      ScrollButtonLock        bool
      LeftHanded              bool
      MiddleEmulation         bool
      // Touchpad only
      Tap, DWT, DWTP, DragLock bool
      Drag                    *bool
      ClickMethod             string // "", "button-areas", "clickfinger"
      TapButtonMap            string // "", "left-right-middle", "left-middle-right"
      DisabledOnExternalMouse bool
      ScrollFactor            ScrollFactor // touchpad and mouse
  }
  // Input gains: Touchpad, Mouse, Trackpoint, Trackball PointerDevice
  ```

- [ ] **Step 1: Write the failing tests.**
  - **`TestPointerDeviceSectionsDecode`.** The touchpad section is
    `tap; dwt; natural-scroll; accel-speed 0.2; accel-profile "flat"; scroll-method "two-finger"; click-method "clickfinger"; tap-button-map "left-middle-right"; drag false; disabled-on-external-mouse; scroll-factor 1.5`.
    The mouse section is `left-handed; scroll-factor vertical=2.0 horizontal=-1.0`. Assert:
    - every touchpad field is set as written;
    - `Drag` points at false;
    - `ScrollFactor{1.5,1.5}` for the touchpad and `{2,-1}` for the mouse;
    - `Trackpoint.Set == false`.
  - **`TestPointerDeviceSectionsReplaceAcrossIncludes`.** The main file has
    `input { touchpad { tap; natural-scroll } }` and then includes `t.kdl` holding
    `input { touchpad { dwt } }`. The result has `Tap == false`, `NaturalScroll == false` and
    `DWT == true`.
  - **`TestPointerDeviceSectionsRejectBadValues`.** The cases are `accel-speed 1.5`,
    `accel-profile "fast"`, `scroll-method "three"`, and `tap 1` (an argument on a flag). Each
    returns an error naming the field.
  - Also assert that `Warnings` no longer lists `input` children for these sections.
- [ ] **Step 2: Run them to see them fail.**
  Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/config -run 'TestPointerDeviceSections'`.
  Expected: FAIL.
- [ ] **Step 3: Implement** `func pointerDevice(n *document.Node, touchpad bool) (PointerDevice, error)`.
  - Decode only the fields of the device kind; touchpad-only fields on a mouse are an error, as
    in Niri.
  - `input()` assigns the result. A new section therefore replaces an old one, and per-field
    merging does not apply.
- [ ] **Step 4: Run the tests again.** Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: decode touchpad, mouse, trackpoint and trackball sections"`

### Task 2: Device kind from libinput and the udev database

**Files:**
- Create: `internal/wlr/devicekind.go` (pure Go) and `internal/wlr/devicekind_test.go`

**Interfaces:**
- Produces:
  ```go
  type pointerKind int // kindMouse, kindTouchpad, kindTrackpoint, kindTrackball
  func classifyPointer(tapFingers int, udev map[string]string) pointerKind
  func udevProperties(root, sysname string) map[string]string // root is "/" in production
  ```

- [ ] **Step 1: Write the failing tests.**
  - **`TestClassifyPointerFollowsNiri`.**
    - `tapFingers` above 0 is a touchpad, even if `ID_INPUT_POINTINGSTICK=1`.
    - `ID_INPUT_POINTINGSTICK=1` is a trackpoint.
    - `ID_INPUT_TRACKBALL=1` is a trackball.
    - Otherwise it is a mouse.
  - **`TestUdevPropertiesReadsTheDatabase`.** The temp root holds `sys/class/input/event7/dev`
    containing `13:71`, and `run/udev/data/c13:71` holding the lines `E:ID_INPUT_POINTINGSTICK=1`
    and `G:seat`. The result is `{"ID_INPUT_POINTINGSTICK":"1"}`. A missing file gives an empty
    map, not an error.
- [ ] **Step 2: Run them to see them fail.**
  Run: `timeout 90s env GOMAXPROCS=2 CGO_CFLAGS_ALLOW=-fno-strict-overflow go test -count=1 ./internal/wlr -run 'TestClassifyPointer|TestUdevProperties'`.
  Expected: FAIL. These are pure tests with no compositor; the nested guard skips nothing here.
- [ ] **Step 3: Implement.** Only `E:` lines are read.
- [ ] **Step 4: Run the tests again.** Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: classify pointer devices the way Niri does"`

### Task 3: Apply settings through libinput, on add and on reload

**Files:**
- Create:
  - `internal/wlr/pointer_settings.go`: pure Go, the plan type;
  - `internal/wlr/libinput.go`: cgo, the `#cgo pkg-config: libinput` line and the preamble with
    `<libinput.h>` and `<wlr/backend/libinput.h>`;
  - `internal/wlr/pointer_settings_test.go`.
- Modify:
  - `internal/wlr/compositor.go`: `Options` gains `Pointers config.Pointers` (a struct holding the
    four `PointerDevice`s), and the apply function near `:702` calls `c.setPointerConfig`;
  - `internal/wlr/input.go:293-297`: on a pointer add, call `c.applyPointerSettings(device)`;
  - `internal/server/config_reload.go:41`: pass `Pointers`;
  - `.github/workflows/ci.yml`: add `libinput` to the pacman line.

**Interfaces:**
- Consumes: `classifyPointer` and `udevProperties` (Task 2), and `config.PointerDevice` (Task 1).
- Produces:
  ```go
  type deviceCaps struct { TapFingers int; HasAccel, HasNatural, HasLeftHanded, HasDWT, HasDWTP, HasMiddleEmulation bool; ScrollMethods, ClickMethods uint32 }
  type pointerSetting struct { Name string; Value any } // Name is the libinput config call, e.g. "tap_set_enabled"
  func planPointerSettings(kind pointerKind, cfg config.PointerDevice, caps deviceCaps) []pointerSetting
  func (c *Compositor) setPointerConfig(p config.Pointers) // stores the config, reapplies to every libinput device
  func (c *Compositor) applyPointerSettings(device *C.struct_wlr_input_device)
  func (c *Compositor) touchpadNaturalScroll(device unsafe.Pointer) bool // for Task 5
  ```

- [ ] **Step 1: Write the failing tests** for `planPointerSettings`, table-driven:
  - **`TestPlanTouchpadSettings`.** With all capabilities, a touchpad with
    `Tap, DWT, NaturalScroll, AccelSpeed 0.2, ClickMethod "clickfinger"` gives, in Niri's order of
    `apply_libinput_settings`:
    - `send_events_set_mode=enabled`;
    - `tap_set_enabled=true`;
    - `dwt_set_enabled=true`;
    - `scroll_set_natural_scroll_enabled=true`;
    - `accel_set_speed=0.2`;
    - `click_set_method=clickfinger`.

    Copy the exact list and order from `/home/nomadx/references/niri/src/input/mod.rs:4674`
    into the test.
  - **`TestPlanUnsetSectionUsesDefaults`.** With `cfg.Set == false`, each call carries the value
    `"default"`. The cgo layer turns `"default"` into the matching `get_default_*`.
  - **`TestPlanSkipsUnsupported`.** A mouse with `HasDWT == false` and a config with `DWT` set
    gets no `dwt_set_enabled` call, but still gets `accel_set_speed`.
  - **`TestPlanOffAndExternalMouse`.** `Off` gives `send_events_set_mode=disabled`, and
    `DisabledOnExternalMouse` gives `disabled-on-external-mouse`.
- [ ] **Step 2: Run them to see them fail.**
  Run: `timeout 90s env GOMAXPROCS=2 CGO_CFLAGS_ALLOW=-fno-strict-overflow go test -count=1 ./internal/wlr -run 'TestPlan'`.
  Expected: FAIL.
- [ ] **Step 3: Implement `planPointerSettings`.**
- [ ] **Step 4: Implement the cgo layer.** `libinput.go` reads capabilities from the handle and
  performs each `pointerSetting` with one switch over `Name`. It skips non-libinput devices
  (`wlr_input_device_is_libinput`). `setPointerConfig` reapplies to every key in
  `c.pointerDevices`.
- [ ] **Step 5: Run Step 2's tests again**, and confirm the package builds:
  `go vet ./internal/wlr`. Expected: PASS, and vet is clean.
- [ ] **Step 6: Check on the laptop** (real libinput):
  - `TestNavigationBindsDriveTheCompositor`, nested and once, shows the nested backend is
    unaffected.
  - The real proof is Task 6's session gate.
- [ ] **Step 7: Commit.** `git commit -m "feat: apply pointing-device settings through libinput"`

### Task 4: scroll-factor

**Files:**
- Modify: `internal/wlr/input.go`, `handleCursorAxis` (after `pointerInputAllowed`, before
  `overviewScrollGesture`)
- Test: `internal/wlr/pointer_settings_test.go`

**Interfaces:**
- Produces: `func scaleAxis(f config.ScrollFactor, horizontal bool, delta float64, v120 int32) (float64, int32)`

- [ ] **Step 1: Write the failing test.** `TestScaleAxis`:
  - factor `{2, -1}` vertical turns `(15, 120)` into `(30, 240)`;
  - horizontal turns it into `(-15, -120)`;
  - factor 1.5 turns `(15, 120)` into `(22.5, 180)`;
  - factor 0.3 turns `(15, 120)` into `(4.5, 36)` (rounded).
- [ ] **Step 2: Run it to see it fail.**
  Run: `timeout 90s env GOMAXPROCS=2 CGO_CFLAGS_ALLOW=-fno-strict-overflow go test -count=1 ./internal/wlr -run TestScaleAxis`.
  Expected: FAIL.
- [ ] **Step 3: Implement it and call it.** `handleCursorAxis` picks the touchpad factor for
  finger and continuous sources on a touchpad, and the mouse factor otherwise. The scaled values
  feed the gesture, binds and `wlr_seat_pointer_notify_axis`.
- [ ] **Step 4: Run it again.** Expected: PASS. Then on the laptop, one per command, run
  `TestWheelBindsInterceptOnlyBoundModifiers` and `TestOverviewTouchpadScrollSwipes` with factor 1
  as the default. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: apply scroll-factor to pointer axes"`

### Task 5: Swipe gestures follow natural scrolling

**Files:**
- Modify: `internal/wlr/input.go`:
  - `handlePointerSwipeUpdate`: negate dx and dy when `touchpadNaturalScroll(device)`;
  - `workspaceSwipe`: positive dy now means down;
  - `overviewScrollGesture`: drop the `-delta` flip and its comment.
- Test: `internal/wlr/overview_test.go`

- [ ] **Step 1: Change the tests first.** Update
  `TestPointerGesturesAndHotCornerDriveOverviewOnCompositorThread` so the 3-finger vertical swipe
  that switches down uses `dy = +16`. That is Niri's rule with natural scrolling off; the fake
  pointer is not libinput, so natural scrolling is off.
  - `TestOverviewTouchpadScrollSwipes` keeps its expectations: a positive scroll still goes down.
  - Add `TestSwipeDirectionFollowsNaturalScroll`, pure, using a helper
    `swipeDeltas(natural bool, dx, dy float64) (float64, float64)`. It returns `(-dx,-dy)` when
    `natural` is true, and the deltas unchanged otherwise.
- [ ] **Step 2: Run them on the laptop to see them fail** (nested, one per command), plus the pure
  test locally. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them again.** Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "fix: swipes follow natural scrolling as in Niri"`

### Task 6: Gate and records

- [ ] **Step 1: Session VM gate.** The VM's virtio tablet is not a touchpad, so gate the mouse
  path there:
  - `input { mouse { left-handed } }` swaps the buttons. Prove it by right-clicking with QMP on a
    foot window, which then receives BTN_LEFT; check with `wev`, or log with `WAYLAND_DEBUG`.
  - `scroll-factor 2` doubles the wheel steps in foot.
- [ ] **Step 2: Laptop gate** (a real touchpad), on a spare TTY with `--session`; this is
  pre-approved. With `tap`, `natural-scroll` and `accel-speed 0.3`:
  - tap clicks;
  - a two-finger scroll in foot follows natural scrolling;
  - a 3-finger swipe up switches the workspace down;
  - removing `tap` from the config and saving turns tap off without a restart.

  Record what was seen.
- [ ] **Step 3: Update the parity matrix.** In `niri-parity-matrix.md`, `input` lists the four
  sections, and the M15 addendum's "found, not fixed" item points here as fixed.
- [ ] **Step 4: Write the execution record** in this plan, then open the PR and merge it on green
  CI.
