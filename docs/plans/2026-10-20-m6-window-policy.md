# M6 Window Policy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the Niri 26.04 M6 window and workspace behavior: tabbed columns, consume/expel, floating, fullscreen variants, named workspaces, monitor moves, and the M6 window/layer rule behavior.

**Architecture:** Keep layout and workspace policy in pure Go `internal/layout` and `internal/wm`; keep config parsing and matching in Go; route wlroots scene and xdg state changes through `internal/wlr` on its locked compositor thread. Preserve Niri KDL, IPC, snapshot, and action semantics. Mango is consulted only for wlroots call patterns.

**Tech Stack:** Go 1.26, cgo, wlroots 0.20, niri-ipc 26.4.0, Niri v26.04 reference at `8ed0da44d974c32c6877d2f4630c314da0717ecb`, Mango 0.18.0 reference at `a21b10fcbe6156b76ea96178d1941de5cb36137f`.

---

## Reference decisions

- Niri v26.04 is the behavior source for layout, rules, KDL, actions, snapshots, and events.
- Mango 0.18.0 is limited to wlroots 0.20 API and lifecycle patterns. Do not use Mango's layout or WM policy.
- Hyprland's current rule documentation was checked on 2026-10-20. Its separation of match conditions from effects is useful internally; its rule ordering, dynamic re-evaluation, Lua configuration, and behavior are not part of this contract. See [window rules](https://wiki.hypr.land/configuring/core/rules/window-rules/) and [layer rules](https://wiki.hypr.land/configuring/core/rules/layer-rules/).
- Keep M7 overview, M8 visual effects, M9 live reload, and M12 capture/privacy behavior out of this milestone. M6 may retain the `place-within-backdrop` layer classification for M7, but must not introduce overview UI.
- Relative window and column moves up or down use adjacent workspace IDs on the focused output. Focus follows by default; a column move while floating moves only the focused floating window, matching Niri.

## Task 1: Add tabbed columns and consume/expel to the pure layout model

**Files:**
- Modify `internal/layout/layout.go`.
- Modify `internal/layout/layout_test.go`.

**Step 1: Write failing cases** in `layout_test.go` for consuming the adjacent column's window into the focused column, consume-or-expel at the workspace edge, expelling an active tab into an adjacent column, preserving focus, removing a tab without losing the remaining column, tabbed display geometry, and per-window column/tile indices.

**Step 2: Run one named test** with `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/layout -run TestConsumeAdjacentWindowIntoFocusedColumn` and confirm the missing behavior fails.

**Step 3: Implement the minimum model change.** Replace the one-window column payload with an ordered tile group and active tile index; represent normal/tabbed display explicitly. Keep column order as the source of truth. `Placements` and focus/move/remove operations must account for every tile. Avoid wlroots or IPC types in this package.

**Step 4: Run named layout tests** for consume, expel, display, removal, focus, and the existing resize behavior.

## Task 2: Add floating and fullscreen state to layout and WM policy

**Files:**
- Modify `internal/layout/layout.go` and `internal/layout/layout_test.go`.
- Modify `internal/wm/model.go` and `internal/wm/model_test.go`.

**Step 1:** Add failing pure Go cases for toggling a window between tiled and floating placement, retaining floating geometry and resize state, keeping floating windows attached to their workspace, restoring the original placement after fullscreen, and separating client fullscreen from windowed fullscreen.

**Step 2:** Implement the smallest explicit state needed for tiled, floating, fullscreen, and windowed-fullscreen. Keep fullscreen as a window/client state with output geometry supplied by the WM; do not encode scene nodes in the model.

**Step 3:** Verify each named layout and model test. Include transitions for unmap/remap and duplicate or unknown IDs.

## Task 3: Add named workspaces and cross-output window/column moves

**Files:**
- Modify `internal/wm/model.go` and `internal/wm/model_test.go`.
- Modify `internal/server/niri.go` and `internal/server/niri_test.go`.
- Modify `internal/server/eventstream.go` and `internal/server/eventstream_test.go` where snapshots/events require a change.

**Step 1:** Write failing cases for unique workspace names, name and ID/index lookup, focus by name, stable IDs while moving a window or column between workspaces/outputs, focus-follow behavior, and output removal/reconnect.

**Step 2:** Implement name ownership and move operations in `wm.Model`; preserve the existing per-output dynamic workspace ordering and stable IDs. Resolve name references against the same snapshot used by the Niri handler.

**Step 3:** Add failing socket and event-stream cases for changed workspace membership, active workspace, and window layout, then update serialization to match niri-ipc 26.4.0.

**Step 4:** Run the named `wm`, `server`, and `ipc` tests covering each transition.

## Task 4: Parse and resolve M6 workspace, window, and layer rules

**Files:**
- Modify `internal/config/config.go` and `internal/config/config_test.go`.
- Add the smallest pure Go rule evaluator under `internal/rules/` only if it keeps matching independent of the WLR package; otherwise keep it in `internal/config`.
- Add focused rule tests in the owning package.

**Step 1:** Add failing KDL cases for named `workspace` entries, `window-rule` match/exclude using app-id and title, ordered field overrides, `open-on-output`, `open-on-workspace`, `open-floating`, `open-fullscreen`, default column width/window height, layer namespace rules, duplicate workspace names, malformed values, and invalid regexes.

**Step 2:** Parse only M6-owned state and placement fields. Validate regexes at the config boundary. Use Go's RE2 semantics and unanchored substring matching, as Niri's regex matcher does; explicit `^`/`$` anchors remain the config author's choice. Resolve matches separately from applying the resulting state so later milestones can add dynamic visual effects without coupling to wlroots.

**Step 3:** Add rule evaluation cases for matching, exclusion, and later matching rules overriding only fields they specify. Apply initial placement rules when a new toplevel is assigned. Retain layer namespace/`place-within-backdrop` classification for M7 without adding overview behavior.

**Step 4:** Run named config and rule tests, plus `niri validate` against the laptop config and the pinned Niri default config fixture when available.

## Task 5: Integrate model state with wlroots scene and xdg requests

**Files:**
- Modify `internal/wlr/windows.go`, `internal/wlr/workspaces.go`, `internal/wlr/xdg.go`, and `internal/wlr/layer.go` as required.
- Modify the corresponding focused tests, including `internal/wlr/layout_test.go`, `internal/wlr/window_snapshot_test.go`, and `internal/wlr/scale_test.go`.

**Step 1:** Add failing nested tests for inactive tab visibility/focus, floating geometry, tiled/fullscreen configure requests, windowed-fullscreen client state, workspace/output scene placement, and layer rule classification.

**Step 2:** Keep all WLR calls on the locked compositor thread. Configure clients from model placements, retain restore geometry, and update snapshots after acknowledged client commits where geometry is client-dependent. Reuse existing scene trees and listeners.

**Step 3:** Run one named WLR test at a time with the repository's 90-second command. Confirm listeners and scene trees still clean up on unmap/destroy.

## Task 6: Wire M6 actions, binds, snapshots, and urgency events

**Files:**
- Modify `internal/bind/bind.go` and `internal/bind/bind_test.go`.
- Modify `internal/wlr/keyboard.go` and focused keyboard tests.
- Modify `internal/server/niri.go`, `internal/server/eventstream.go`, and focused server tests.
- Modify `internal/ipc/names.go` or `internal/ipc/wire.go` only if the already-decoded niri-ipc 26.4.0 action shapes require it.

**Step 1:** Add failing cases for M6 action dispatch and argument handling: consume/expel, tab display, floating focus/toggle/move, fullscreen/windowed-fullscreen, named and adjacent workspace moves with focus-follow, workspace name, and monitor moves by direction or output name. Preserve stable error responses for unsupported later-milestone actions.

**Step 2:** Implement actions by posting the policy mutation to the compositor thread. Keep parsing and resolution pure Go. Apply the M6 actions used by the laptop KDL and the Niri action subset assigned to M6 in the parity matrix.

**Step 3:** Add urgency state conversion and `WindowUrgencyChanged` / `WorkspaceUrgencyChanged` events, including unchanged and cleared state cases.

**Step 4:** Run the named bind, IPC, keyboard, snapshot, and event tests.

## Task 7: Laptop nested gate and milestone record

**Files:**
- Update `docs/plans/niri-parity-matrix.md`.
- Update this plan with actual execution evidence.

Run the built branch binary nested in the laptop's currently active Niri session. Discover the live `NIRI_SOCKET`, `WAYLAND_DISPLAY`, and `XDG_RUNTIME_DIR` at run time. Validate the laptop config, start ordinary clients, verify tab/floating/fullscreen/workspace/rule behavior through the UI and `niri msg`, and confirm sysc-shell can still read the IPC snapshots and EventStream. Do not install the build, change greetd, reboot, or stop the parent compositor. The parent Niri session currently reports DP-1 and DP-3. Nested Wayland tests do not exercise sysc-compositor across both physical outputs, so physical multi-output behavior remains unrun.

Before the milestone is closed, run the repository-required formatting check, `go vet -p 2 ./...`, both command builds, and `git diff --exit-code -- go.mod go.sum`. Do not run `go test ./...` or `-race`.

**M6 stop condition:** Pure model and rule cases pass; named WLR and IPC tests pass; the laptop nested gate proves the available behavior on the child compositor's Wayland output; sysc-shell reads its snapshots and EventStream; the M6 config/action/event rows in the parity matrix reflect only behavior actually proved. The parent Niri currently has two physical outputs, but the nested child exposes its own Wayland output, so physical multi-output behavior remains unproven. Stop before M7 overview work.

## Verification note (2026-10-20)

These focused nested WLR tests passed individually under the active parent Niri session (`WAYLAND_DISPLAY=wayland-1`, `XDG_RUNTIME_DIR=/run/user/1000`):

- `TestWindowRulePlacesNewWindowOnNamedWorkspaceAndFloating`
- `TestWindowRulesApplyDefaultSizesAndFullscreen`
- `TestWindowRuleFallsBackWhenNamedWorkspaceIsUnavailable`
- `TestWindowedFullscreenUsesOutputBoundsAndRestoresTile`
- `TestTabbedColumnHitTestIgnoresInactiveWindow`
- `TestNestedKeyboardBindNamesAndMovesWindowToNamedWorkspace`
- `TestFloatingBindsMoveMappedWindowBetweenLayers`

Each test owns a nested compositor and registers `Compositor.Destroy` with `t.Cleanup`, so its short-lived test session closes when the assertions finish. This is expected; the separate persistent run below exercised the branch binary directly.

A persistent nested run used `/tmp/sysc-m6-window-policy-20261020/sysc-compositor-m6-final` with a temporary config declaring `Work` and `Chat`. Its config validated, two `weston-terminal` clients mapped, and IPC actions changed floating state, moved a window between named workspaces, consumed the adjacent window into a tabbed column, and toggled client fullscreen and windowed-fullscreen. `Windows` snapshots reflected the floating/workspace changes, both tabs shared column position 1 with tile indices 1 and 2, fullscreen requests resized the client to output bounds, and leaving fullscreen restored the tiled geometry. The EventStream emitted its initial snapshot and live `WindowOpenedOrChanged` / `WindowLayoutsChanged` updates. The Niri 26.04 CLI also queried and dispatched actions through the child socket; its version query warned that the running compositor is sysc-compositor, as expected.

The parent Niri output query returned DP-1 and DP-3. The nested child reported one synthetic output, WL-1 at 1696x1368 and 60 Hz, with VRR unsupported. This proves output reporting, not physical output mode or VRR control. `sysc-compositor validate` accepted the laptop config but warned that `output` is not applied yet and listed remaining unapplied config sections/actions. A second sysc-shell process could not be started because the active parent session already has a sysc-shell instance; that instance was left untouched, so child-session shell compatibility remains unverified.

Follow-up M6 coverage found that `MoveFloatingWindow` decoded but still returned `unimplemented`. The action is now routed through IPC, the WM model, and the WLR scene. Focused layout, WM, WLR, and server tests pass for fixed/proportional movement, explicit and focused-window targets, saved position restoration, and unmap/remap of a floating window. The saved floating position also takes precedence over the current tiled rectangle when the window is floated again.

Later follow-up supersedes the earlier note that child-session shell compatibility was unverified. I built the current M6 branch into `/tmp`, started it nested under the active parent Niri with an isolated runtime and empty config, and started the installed sysc-shell with its own temporary XDG config/cache/data/state directories and the child `WAYLAND_DISPLAY`/`NIRI_SOCKET`. `niri msg -j outputs` returned the child WL-1 snapshot; `niri msg -j layers` showed sysc-shell bar, toast, shield, and panel surfaces; `sysc-shell ipc status` returned `ok:true`; after a weston-terminal mapped, `niri msg -j windows` returned its window snapshot and the shell remained running. The original parent Niri and shell remained running throughout. The second shell logged a nonfatal ScreenSaver D-Bus name ownership warning because the parent shell was still active on the same session bus. The child compositor, test client, and second shell were stopped afterward.

### Pointer panic follow-up (2026-10-20)

A later shell run on the same nested compositor logged `dispatch: panic handling opcode=0 senderID=17` and exited after pointer interaction. `wayland-info` confirmed that the child does not advertise `wp_cursor_shape_manager_v1`. In sysc-shell, pointer enter calls `syncCursor`; that passed a nil `*WpCursorShapeDeviceV1` through the `cursorShaper` interface, whose nil check did not catch the typed nil. A focused regression test reproduced the panic in `WpCursorShapeDeviceV1.SetShape`; an owner-side nil-device guard was added in the separate sysc-shell worktree.

The fixed sysc-shell build was then run in the laptop's nested session. After the reported trackpad interaction, PID 404839 remained alive; its log had no dispatch panic or fatal error. `niri msg -j layers`, `windows`, and `outputs` returned valid child-session state, and `sysc-shell ipc status` returned `ok:true`. The parent Niri and installed shell remained running. The nested shell, terminals, and compositor were stopped after the check. The sysc-shell change remains uncommitted and uninstalled, so this proves the fix only in the test build.

After code review, these focused checks passed individually on the current M6 tree: `TestFullscreenWindowDoesNotOverrideAnotherWorkspace`, `TestFullscreenRequestMovesWindowToRequestedOutput`, `TestNestedModPointerGrabsUseEdgeResizeAndCursorImages`, `TestMoveFocusedColumnToWorkspaceMovesOnlyFocusedFloatingWindow`, `TestVerticalResizePreservesConstrainedColumnWidth`, and `TestVerticalResizePreservesConstrainedWidthAcrossRemap`. The reported review conditions are covered by the current implementation and tests; no additional compositor code change was needed.

The laptop's physical output is eDP-1 at 1920x1080, 60.010 Hz, scale 1.25, with VRR unsupported. The nested child still exposes one synthetic WL-1 output; mode, refresh, scale, position, transform, power, and VRR application remain M15 work. The physical monitor configuration was not changed.
