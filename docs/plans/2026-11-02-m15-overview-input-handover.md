# Handover: M15 part 1 follow-up (overview input), 2026-11-02

Read this first when continuing. Tracker: `sysc-1280` in `/home/nomadx/sysc-shell`.

## State

- `main` is `68dac8f`: M15 part 1 merged as #85. The record is in
  [the part 1 plan](2026-10-31-m15-navigation-binds.md).
- Branch `feat/sysc-1280-m15-overview-input-20261101` is pushed with no PR yet.
  - Worktree: `~/.config/superpowers/worktrees/sysc-compositor/m15-overview-input-20261101`.
  - It has one code commit, `ef3afe9` (item 1 below), and this file.

## Why this branch exists

After part 1, the owner asked for two deferred items: Niri's overview touchpad scroll gesture, and
layer-shell popups that work in the overview. The owner also asked for part 1's rulings to be
checked against Mango and Hyprland.

## Rulings checked against Mango and Hyprland (sources in `/home/nomadx/references`)

| Ruling | Niri | Mango 0.18.0 | Hyprland | Outcome |
|---|---|---|---|---|
| Cooldown clock | calloop timer per bind (`handle_bind`) | event `time_msec`, one shared `axis_bind_apply_timeout` (100 ms) | one wall-clock timer, `binds:scroll_event_delay` (300 ms); within it the scroll passes to the client | **Changed** to the monotonic clock (item 1) |
| A scroll with no matching bind reaches the client | yes | yes (`handle_cursor_axis`) | yes (`passEvent`) | stands |
| Top/overlay layers keep the pointer in the overview | yes (`contents_under`) | yes: `pointer_focus` refuses only windows in its overview; layers and their popups still get focus | no core overview (hyprexpo is a plugin) | stands; popups missing (item 2) |
| The overview column wheel acts on the hovered workspace row | `workspace_under_cursor(true)` | no per-workspace overview rows | none | stands (Niri only) |
| The open switcher takes or-focus binds | `make_dynamic_opened_binds` | the switcher applies ordinary binds, with no remap | no core switcher | stands (Niri only) |
| Composites report "changed", not "output changed" | internal | n/a | `movefocus` with `binds:window_direction_monitor_fallback` crosses monitors at the edge, the same policy | stands |

## Work items

1. **Done (`ef3afe9`).** Bind cooldowns run on the compositor's monotonic clock: one clock for
   keys, wheels and mouse binds (`bindCoolingDown` in `internal/wlr/pointer_binds.go`).
   - Tests set `c.pointerBinds.now` to drive the clock.
   - New test `TestWheelCooldownIgnoresEventTimestamps`: RED, then PASS on the laptop.
   - `TestWheelBindsInterceptOnlyBoundModifiers`, `TestOverviewWheelSwitchesWorkspaceUnderPointer`,
     `TestMouseBindSuppressesItsRelease` and `TestNavigationBindsDriveTheCompositor` pass.

2. **Next: layer popups in the overview.**
   - Cause: `layerSurfaceForSurface` (`internal/wlr/layer.go:204`) stops at
     `wlr_surface_get_root_surface`, so a popup never maps to its layer.
   - Fix: use `sysc_window_surface` (the C helper in `internal/wlr/xdg.go`, which walks popup
     parents; it is static there, so copy it into the `layer.go` preamble or move it).
   - The same lookup sits behind `focusSurfaceAtCursor` (`input.go`). Today, clicking a layer's popup
     clears that layer's on-demand keyboard focus. The fix corrects that too, as Niri's
     `find_root_shell_surface` does.
   - Test first. Add `internal/wlr/testclient/layer.go`: a hand-written client like `lock.go` and
     `gamma.go`, with these pieces:
     - `zwlr_layer_shell_v1` v1, `zwlr_layer_surface_v1` (`get_popup` "o"), `xdg_wm_base` v2,
       `xdg_positioner`, `xdg_surface` (`get_popup` "n?oo") and `xdg_popup` v2;
     - `wl_seat` **v1**, so the `wl_pointer` listener only needs enter/leave/motion/button/axis
       (libwayland crashes on a NULL handler);
     - a 30 px top bar, then a popup: positioner size 120x80, anchor rect 0,0,40,30, anchor
       bottom_left (6), gravity bottom_right (8). It should land near layout (0..120, 30..110);
     - counters for pointer enters, axes and buttons on the popup surface.
   - Nested test: add the fake pointer **before** the client starts. Open the overview, put the
     pointer on the popup, then send a wheel and a click. Expect the popup to get both, the overview
     to stay open, and the workspace not to change.

3. **Next: Niri's overview touchpad scroll gesture** (`src/input/mod.rs` ≈3264,
   `src/input/scroll_swipe_gesture.rs`).
   - In the overview, with no modifiers and the pointer not on a layer surface, a `FINGER` or
     `CONTINUOUS` axis event drives a gesture and never reaches binds or clients.
   - Reuse sysc's 3-finger swipe path rather than a second implementation:
     - factor the 3-finger branch of `handlePointerSwipeUpdate` (`input.go`) into a helper;
     - a vertical swipe switches the output's workspace once per gesture;
     - a horizontal swipe calls `ScrollWorkspaceView` on `workspaceUnderGesture`.
   - Translating wlroots events (one orientation each):
     - the first non-zero event begins a gesture: output under the pointer, workspace under it;
     - feed vertical events as (0, d) and horizontal ones as (d, 0) through the existing
       `SwipeAxisSelector`;
     - a zero-delta event (libinput's scroll stop) ends the gesture;
     - so does any event outside these conditions; then fall through to the normal path.
   - Rulings to record: axis choice by the selector's 16 px threshold instead of Niri's "first event
     decides". The vertical switch is discrete, as sysc's 3-finger swipe is; Niri's
     `workspace_switch_gesture` follows the fingers continuously. That gap predates this work in the
     3-finger path.
   - Also make finger-scroll *binds* accept `CONTINUOUS`, as Niri's do. Today `handleCursorAxis`
     passes only `FINGER`.
   - Tests: add a horizontal variant of `fakePointer.fingerScrollAt`. Cover:
     - a vertical gesture switches once, and only again after a zero-delta end;
     - a horizontal gesture moves the hovered row's view (`windowPlacementX`);
     - with Shift held the gesture does not run;
     - no axis reaches a window.

4. **Then:**
   - Rerun the nested overview and pointer tests touched, one per command. The part 1 record lists
     them.
   - Add a VM check:
     - a two-finger scroll can't come from QMP's tablet, so prove item 3 by test only and say so;
     - for the popup, waybar with a `menu` module clicked in the session VM, if practical.
   - Add the record to the part 1 plan as an addendum. Update the matrix `overview` row: drop the
     two "Not yet" items and name the tests.
   - Open the PR; merge on green CI under the standing approval.
   - Then write the **M15 part 2 (sizing)** plan.

## Environment notes

- **Laptop:** `~/sysc-m15` is a non-git copy. Sync it with
  `tar --exclude=.git --exclude=.superpowers --exclude=internal/wlr/protocols -cf - . | ssh laptop 'tar -xf - -C ~/sysc-m15'`
  (the laptop has no rsync). Run one nested test per command with `ssh laptop '~/lt-m15.sh <Test> [pkg]'`.
- **Local builds** need `CGO_CFLAGS_ALLOW=-fno-strict-overflow`. Never `go test ./...`; never set
  `SYSC_NESTED_TESTS` on the desktop without consent.
- **Session VM** (`~/vm/sysc-session` on the laptop): powered off.
  - It has the part 1 build installed; the previous binary is `sysc-compositor.pre-m15-20261031`.
  - It has a copy of the owner's niri config (`config.kdl`, `sysc-shell.kdl`, `sysc.kdl`).
  - `qmpwheel.py` holds keys and sends wheel clicks. The virtio tablet has no horizontal wheel.
- **Commit messages:** no "both", "agent" or "cursor", and no Co-Authored-By trailer (the hook
  rejects them).
- **Still the owner's:**
  - desktop session `sysc-2122`: `desktop-gate.sh` and `desktop-discord-gate.sh`;
  - closing M13 and M14;
  - `sysc-2081` (tablet) and `sysc-2056` (game benchmark).
