# M15 Part 1: navigation actions, workspace moves, and mouse and scroll binds

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Commission: the owner's go-ahead to continue the plan, 2026-10-31. Tracker: `sysc-1280` in
`/home/nomadx/sysc-shell`.

**Goal:** sysc runs every navigation and workspace action in niri-ipc 26.4.0, plus the mouse and
scroll binds that Niri's default config and the owner's config use.

**Architecture:**
- Strip-level actions go on `layout.Workspace`, routed between floating and tiling as Niri's
  `src/layout/workspace.rs` routes them.
- Actions that cross a workspace or monitor boundary go on `wm.Model`.
- One wiring task connects them to the bind parser, the IPC dispatch and the compositor.
- Scroll and mouse triggers resolve through the existing key-bind lookup and execution path.

**Tech Stack:** Go and cgo on wlroots 0.20.2 and SceneFX 0.5.

**Spec:** `docs/plans/2026-10-02-sysc-compositor-design.md` §8 (IPC: "implement the full enum"), §15
(M15: "zero unimplemented used by default niri config and by this machine's config.kdl"), and
`docs/plans/niri-parity-matrix.md`.

## M15 split

The full table has 141 actions. 51 of them reply `unimplemented` (counted on 2026-10-31 against
`internal/server/niri.go`). `validate` on Niri's default config and on the owner's config lists:
- these unapplied actions: workspace up/down/previous and moves, column sizing, centering, and
  every bind on a `WheelScroll*` trigger;
- these unapplied sections: `output`, `layout.center-focused-column`, `preset-column-widths`,
  `border`, `struts`, `tab-indicator`, `config-notification` and `debug`, plus some `window-rule`
  effects.

That is four subsystems, so M15 gets four plans. Each is written when the previous one merges:

| Part | Scope | Status |
|---|---|---|
| 1 (this plan) | 30 navigation and workspace actions, wheel/touchpad/mouse bind triggers with `cooldown-ms`, Niri's overview wheel | this plan |
| 2 Sizing | Niri's `ColumnWidth` (proportion/fixed) in the layout, `preset-column-widths`/`preset-window-heights`, switch-preset, set/reset width and height, maximize-column, maximize-window-to-edges, expand-column-to-available-width, center-column/window/visible-columns, `center-focused-column`, `always-center-single-column`, `struts`, foreign-toplevel maximize | next |
| 3 Outputs | `output` config (mode, scale, transform, position, off, VRR incl. on-demand rule), mixed-DPI cursor, `zwlr_output_manager_v1`, wiring `zwlr_output_power_manager_v1`, the `Output` request, DisplayConfig apply, GPU reset on renderer loss | after 2 |
| 4 Close-out | DoScreenTransition, ToggleDebugTint, DebugToggleOpaqueRegions, DebugToggleDamage, ToggleWindowRuleOpacity and rule `opacity`, PickWindow, PickColor, `border`, `tab-indicator`, `config-notification`, `debug`, `input.workspace-auto-back-and-forth`, `msg action` subcommands for all 141, the final `validate` and matrix tick | last |

## Reference comparison

Every item here is a Niri action or trigger with defined behaviour, so Niri 26.04 is the contract
and no policy is chosen. Mango 0.18.0 and Hyprland have different action vocabularies, and neither
has scrolling columns, so they offer no competing policy. The one implementation choice (cooldown
timing) is a ruling below.

Niri sources the tasks cite (pinned checkout `/home/nomadx/references/niri`):
- `src/input/mod.rs`:
  - action handlers: `Action::FocusColumnRightOrFirst` (≈1094) through `Action::SwapWindowLeft`
    (≈1554);
  - `handle_bind` with cooldown (≈616);
  - `on_pointer_button` mouse binds (≈2736);
  - `on_pointer_axis` wheel and finger-scroll binds (≈3058–3420);
  - `mods_with_wheel_binds`, `mods_with_finger_scroll_binds` and `mods_with_mouse_binds` (≈4988).
- `src/input/scroll_tracker.rs`: `ScrollTracker::accumulate`. Ticks are 120 for the wheel and 10
  for the finger trackers (`src/niri.rs` ≈2585).
- `src/layout/workspace.rs`: floating/tiling routing for each strip action (≈940–1140).
- `src/layout/scrolling.rs`: `focus_column` (1580), the `focus_*_or_*` combinations (1612–1660),
  `move_column_to_index` (1672) and `swap_window_in_direction` (2032).
- `src/layout/monitor.rs`:
  - `previous_workspace_id`, set in `activate_workspace_with_anim_config` (437) only when the index
    changes;
  - `switch_workspace_previous` (1027);
  - `move_workspace_down`/`up`, which keep `previous_workspace_id` (1242);
  - `move_workspace_to_idx` (1294);
  - the `*_or_to_workspace_*` and `focus_window_or_workspace_*` helpers (761–783).

Rulings:
- Cooldown is measured on input event timestamps (msec), not on a wall-clock timer. For real
  devices the two are the same, and tests become deterministic. If wrong, a device with skewed
  timestamps rate-limits differently from Niri.
- The `*UnderMouse` actions and Niri's built-in overview wheel binds act on the output under the
  cursor, as Niri does. They are not exposed as config names, because Niri marks them
  `knuffel(skip)`.

## Global Constraints

- Never `go test ./...`. Never `-race`. Run one package and one named test:
  `timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run <Name>`.
- Go 1.x builds here need `CGO_CFLAGS_ALLOW=-fno-strict-overflow`.
- Tests that build a compositor skip unless `SYSC_NESTED_TESTS=1`. Run them on the laptop
  (`ssh laptop`, pre-approved, one test per command), with
  `WLR_BACKENDS=headless WLR_HEADLESS_OUTPUTS=1 SYSC_NESTED_TESTS=1`. Never run them on the desktop
  without consent.
- `runtime.LockOSThread()` before any wlroots call. Write no `.c` or `.h` files.
- IPC pin: niri-ipc 26.4.0. Each action's JSON shape is in `niri-ipc/src/lib.rs`. `FocusColumn`,
  `MoveColumnToIndex` and `MoveWorkspaceToIndex` carry `index: usize`, and `MoveWorkspaceToIndex`
  also carries `reference: Option<WorkspaceReferenceArg>`.
- Commit messages must avoid the hook substrings ("both", "agent", "cursor", etc.) and carry no
  Co-Authored-By trailer.

## Review Focus

1. A scroll with modifiers that have no wheel bind must still reach the client, and its accumulator
   must reset. If Mod+scroll is swallowed in an app with no Mod wheel bind, scrolling breaks.
   Covered in Task 5.
2. Several workspaces must not share a single previous-workspace record. A
   `focus-workspace-previous` after a monitor focus change must use the focused monitor's own
   previous, as Niri's per-`Monitor` field does. Covered in Task 1.
3. A mouse bind's release must not reach the client. A press that ran a bind is suppressed with its
   release, otherwise the client sees a release with no press. Covered in Task 5.
4. Cross-boundary composites at an edge with no neighbour (a single output, or the last workspace)
   must fall back to the plain action, never no-op silently. Covered in Task 3.
5. Index actions must take index 0 and indices beyond the end as Niri's `saturating_sub(1)` and
   clamp do. They must not panic or reject. Covered in Tasks 1 and 2.

---

### Task 1: workspace navigation and moves in `wm.Model`

**Files:**
- Modify: `internal/wm/model.go`
- Test: `internal/wm/model_test.go`

**Interfaces:**
- Produces: `(*Model).FocusWorkspaceDown() bool`, `FocusWorkspaceUp() bool`,
  `FocusWorkspacePrevious() bool`, `MoveWorkspaceDown() bool`, `MoveWorkspaceUp() bool`, and
  `MoveWorkspaceToIndex(index int, workspaceID uint64) bool`. A `workspaceID` of 0 means the focused
  output's active workspace. Each returns whether anything changed.

- [ ] **Step 1: Write the failing tests**

`TestFocusWorkspaceUpDownAndPreviousFollowNiri`:
- Set up one output with workspaces 1, 2 and 3 and a trailing empty workspace.
- `FocusWorkspace("",1)`, then `FocusWorkspaceDown()` → active index 1.
- `FocusWorkspaceDown()` again → index 2.
- `FocusWorkspacePrevious()` → index 1, and a second call → index 2.
- `FocusWorkspaceUp()` at index 0 returns false.
- Add a second output and focus it: its `FocusWorkspacePrevious()` with no history returns false,
  and the first output's previous is unchanged.

`TestMoveWorkspaceDownKeepsPreviousAndTrailingEmpty`:
- Give workspaces A (index 0, active, with a window) and B (with a window), plus the empty
  trailing workspace. Set the previous workspace to B by focusing B and then A.
- `MoveWorkspaceDown()` → order B, A, empty. A stays active (by ID) and previous is still B.
- A further `MoveWorkspaceDown()` puts A last before a new trailing empty workspace, so reconcile
  keeps exactly one trailing empty.
- `MoveWorkspaceUp()` reverses it.
- On the trailing empty workspace, `MoveWorkspaceDown()` returns false.

`TestMoveWorkspaceToIndexClampsAndResolvesReference`:
- With 3 used workspaces, `MoveWorkspaceToIndex(0, 0)` moves the active workspace to the first
  position (`saturating_sub`), and `MoveWorkspaceToIndex(99, 0)` clamps to the last position.
- Given a non-active workspace's ID, it moves that workspace within its own output and leaves which
  workspace is active as Niri's `Monitor::move_workspace_to_idx` does. Read monitor.rs:1294 and
  assert the active ID it yields.

- [ ] **Step 2: Run them to verify they fail**

Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wm -run 'TestFocusWorkspaceUpDownAndPreviousFollowNiri|TestMoveWorkspaceDownKeepsPreviousAndTrailingEmpty|TestMoveWorkspaceToIndexClampsAndResolvesReference'`
Expected: build failure. `FocusWorkspaceDown` and the others are undefined.

- [ ] **Step 3: Implement**

- Add `previousWorkspace uint64` to `outputState`.
- Route every change of `out.active` through one helper that records the old active ID when the
  index changes, mirroring Niri's `activate_workspace_with_anim_config`. "Every" covers the
  `FocusWorkspace*` paths, follow-moves, overview selection and gesture ends. Find them with
  `grep -n '\.active = ' internal/wm/*.go`.
- `MoveWorkspaceDown` and `MoveWorkspaceUp` swap with the neighbour, keep the moved workspace
  active, and restore `previousWorkspace` afterwards, as Niri does. Then `reconcile`.
- `FocusWorkspaceDown` and `FocusWorkspaceUp` are `FocusWorkspaceByDelta(m.focusedOutput, ±1)`.

- [ ] **Step 4: Run the tests to verify they pass, plus the package's existing workspace tests**

Run the Step 2 command, then
`timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wm -run 'Workspace|Overview|Gesture'`.
Expected: PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat: workspace up, down, previous and moves in the model"`

### Task 2: strip navigation in `layout.Workspace`

**Files:**
- Modify: `internal/layout/layout.go`
- Test: `internal/layout/layout_test.go`

**Interfaces:**
- Produces: these methods on `*layout.Workspace`, each returning whether focus or order changed:
  - `FocusColumn(index int) bool`
  - `FocusColumnRightOrFirst() bool`, `FocusColumnLeftOrLast() bool`
  - `FocusWindowTop() bool`, `FocusWindowBottom() bool`
  - `FocusWindowDownOrTop() bool`, `FocusWindowUpOrBottom() bool`
  - `FocusWindowDownOrColumnLeft() bool`, `FocusWindowDownOrColumnRight() bool`
  - `FocusWindowUpOrColumnLeft() bool`, `FocusWindowUpOrColumnRight() bool`
  - `MoveColumnToIndex(index int) bool`
  - `SwapWindowLeft() bool`, `SwapWindowRight() bool`

  `index` is 1-based. Index 0 is treated as 1 and an index past the end clamps to the last column.
- Consumes: the existing `FocusLeft`, `FocusRight`, `FocusFirst`, `FocusLast`, `FocusWindowDown`,
  `FocusWindowUp` and `FocusTiling`.

- [ ] **Step 1: Write the failing tests**

`TestStripIndexActionsClampLikeNiri`:
- Use columns A, B and C, with focus on C.
- `FocusColumn(1)` focuses A, `FocusColumn(0)` focuses A, and `FocusColumn(9)` focuses C.
- With focus on A, `MoveColumnToIndex(3)` gives the order B, C, A with A still focused.
- `MoveColumnToIndex(0)` gives A, B, C.
- With a floating window focused, `FocusColumn(2)` focuses tiling column B (workspace.rs:952), and
  `MoveColumnToIndex` returns false (workspace.rs:1074).

`TestStripWrapAndOrFallbacks`:
- `FocusColumnRightOrFirst` on the last column focuses the first column. `FocusColumnLeftOrLast`
  on the first column focuses the last.
- In a column of three tiles: `FocusWindowBottom`/`Top` jump to the bottom and top tiles;
  `FocusWindowDownOrTop` wraps from the bottom tile to the top one; `FocusWindowUpOrBottom` wraps
  from the top tile to the bottom one.
- `FocusWindowDownOrColumnRight` on a column's bottom tile focuses the next column. In the middle
  of a column it moves down. Repeat this check for the three sibling methods.
- With floating active, `FocusWindowTop` focuses the topmost floating window (workspace.rs:1014).

`TestSwapWindowMatchesNiri`:
- With two single-tile columns, `SwapWindowRight` swaps the columns, which is a plain column move.
- With column A of two tiles (a1 active) and a single-tile column B, `SwapWindowRight` gives A =
  [B's window, a2] and B = [a1], with focus following a1, as scrolling.rs:2032 does.
- On the last column `SwapWindowRight` returns false. With floating active, it returns false.

- [ ] **Step 2: Run them to verify they fail**

Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/layout -run 'TestStripIndexActionsClampLikeNiri|TestStripWrapAndOrFallbacks|TestSwapWindowMatchesNiri'`
Expected: build failure. `FocusColumn` and the others are undefined.

- [ ] **Step 3: Implement each method from the cited Niri function**

Route floating against tiling per `src/layout/workspace.rs`. For the swap, port
`swap_window_in_direction`, including its column-width and active-tile bookkeeping. Keep
`revealFocused()` after every focus change.

- [ ] **Step 4: Run the tests and the whole layout package's named tests**

Run the Step 2 command, then `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/layout -run 'Test'`.
`./internal/layout` builds no compositor, so running it whole is allowed.
Expected: PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat: column index, wrap, swap and or-fallback focus in the strip"`

### Task 3: cross-boundary composites in `wm.Model`

**Files:**
- Modify: `internal/wm/model.go`
- Test: `internal/wm/model_test.go`

**Interfaces:**
- Consumes: Task 1's `FocusWorkspaceDown`/`Up`, Task 2's methods (through `activeStripAction`),
  and the existing `FocusOutputDirection`, `OutputInDirection`,
  `MoveFocusedColumnToMonitorDirection` and `moveActiveWindowToAdjacentWorkspace`.
- Produces: these `*Model` methods, each returning whether the active output changed (Niri's
  `bool` from the `*_or_output` helpers):
  - `FocusWindowOrMonitorUp() bool`, `FocusWindowOrMonitorDown() bool`
  - `FocusColumnOrMonitorLeft() bool`, `FocusColumnOrMonitorRight() bool`
  - `MoveColumnLeftOrToMonitorLeft() bool`, `MoveColumnRightOrToMonitorRight() bool`

  And these `*Model` methods, each returning whether anything changed:
  - `FocusWindowOrWorkspaceDown() bool`, `FocusWindowOrWorkspaceUp() bool`
  - `MoveWindowDownOrToWorkspaceDown() bool`, `MoveWindowUpOrToWorkspaceUp() bool`

  Task 2's strip methods are exposed on `*Model` the same way as `FocusColumnLeft`:
  `FocusColumn(index int)`, `MoveColumnToIndex(index int)`, `SwapWindowLeft()` and the others.

- [ ] **Step 1: Write the failing tests**

`TestOrMonitorCompositesCrossOnlyAtTheEdge`:
- Set up outputs L and R side by side, each with two columns.
- On L's last column, `FocusColumnOrMonitorRight()` focuses R and returns true.
- On L's first column it focuses L's second column and returns false.
- With only one output connected it behaves as plain `FocusColumnRight`.
- Check `MoveColumnRightOrToMonitorRight` the same way: at the edge the column lands on R and
  focus follows it.
- `FocusWindowOrMonitorDown` uses vertical outputs; set up outputs T and B.

`TestOrWorkspaceCompositesFallBackToWorkspace`:
- On a column's bottom tile, `FocusWindowOrWorkspaceDown()` switches to the next workspace.
- In the middle of the column it focuses the next tile.
- On a bottom tile, `MoveWindowDownOrToWorkspaceDown()` moves the window to the workspace below
  and follows it (Niri passes `focus=true`, monitor.rs:761).
- In the middle of a column it is a plain move down.

- [ ] **Step 2: Run them to verify they fail**

Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wm -run 'TestOrMonitorCompositesCrossOnlyAtTheEdge|TestOrWorkspaceCompositesFallBackToWorkspace'`
Expected: build failure. The methods are undefined.

- [ ] **Step 3: Implement as Niri composes them**

The `or_output` forms check `OutputInDirection` first and fall back to the plain action when there
is no output (input/mod.rs ≈1115–1200). The `or_workspace` forms compose as in monitor.rs:761–783.

- [ ] **Step 4: Run the tests and the Task 1 tests**

Expected: PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat: or-monitor and or-workspace composite actions"`

### Task 4: wire the 30 actions to binds, IPC and the compositor

**Files:**
- Modify:
  - `internal/bind/bind.go` (names, `ParseAction` arguments, `IsSupportedAction`)
  - `internal/ipc/wire.go` (an `Index *uint64` field; decode `FocusColumn`, `MoveColumnToIndex`
    and `MoveWorkspaceToIndex`)
  - `internal/server/niri.go` (dispatch lists)
  - `internal/server/server.go` (the post switch)
  - `internal/wlr/windows.go` (compositor wrappers)
  - `internal/wlr/keyboard.go` (`executeBindAction`)
  - `internal/wlr/screenshot_ui.go` (`runShotBinding`: the `*OrToMonitor*` moves move the
    selection, as Niri's `Action::MoveColumnLeftOrToMonitorLeft` checks `screenshot_ui.is_open()`
    first; the `*OrToWorkspace*` window moves move it vertically)
- Test:
  - `internal/bind/bind_test.go`
  - `internal/ipc/wire_test.go`
  - `internal/server/niri_test.go`
  - `internal/config/config_test.go`
  - `internal/wlr/layout_test.go`

**Interfaces:**
- Consumes: the Task 1–3 `*Model` methods.
- Produces:
  - Bind names, which are Niri's kebab-case: `focus-workspace-down`, `focus-workspace-up`,
    `focus-workspace-previous`, `move-workspace-down`, `move-workspace-up`,
    `move-workspace-to-index N`, `focus-column N`, `focus-column-right-or-first`,
    `focus-column-left-or-last`, `focus-window-or-monitor-up/down`,
    `focus-column-or-monitor-left/right`, `focus-window-down-or-column-left/right`,
    `focus-window-up-or-column-left/right`, `focus-window-or-workspace-down/up`,
    `focus-window-top/bottom`, `focus-window-down-or-top`, `focus-window-up-or-bottom`,
    `move-column-left-or-to-monitor-left`, `move-column-right-or-to-monitor-right`,
    `move-column-to-index N`, `move-window-down-or-to-workspace-down`,
    `move-window-up-or-to-workspace-up`, `swap-window-left/right`.
  - A `bind.Action.Index int` field for the three index actions.
  - `ipc.Action.Index *uint64`.

- [ ] **Step 1: Write the failing tests**

- In `wire_test.go`, `TestIndexActionsDecodeNiriShapes`:
  - `{"Action":{"FocusColumn":{"index":2}}}` gives `Index` 2.
  - `{"Action":{"MoveColumnToIndex":{"index":1}}}` decodes.
  - `{"Action":{"MoveWorkspaceToIndex":{"index":3,"reference":null}}}` gives `Index` 3 and a nil
    `Workspace`; with `"reference":{"Name":"web"}` it sets `Workspace`.
  - A missing `index` is `ErrParse`.
- In `niri_test.go`, `TestNavigationActionsAreHandled`:
  - For each of the 30 Pascal names, the handler with a recording `postAction` replies
    `{"Ok":"Handled"}` and posts the action.
  - The existing `ToggleDebugTint` row stays `unimplemented`.
- In `config_test.go`, `TestDefaultConfigNavigationBindsApply`:
  - Parse `/home/nomadx/references/niri/resources/default-config.kdl`, read in the test, and
    skip if absent.
  - Assert that `UnappliedActions` contains none of the Task 4 names. The `WheelScroll` binds
    still count until Task 5, so assert only on names bound to key triggers.
- In `bind_test.go`, `TestNavigationActionArguments`:
  - `focus-column 2` parses `Index` 2.
  - `focus-column` with no argument errors.
  - `move-workspace-to-index 1` parses.
  - `focus-workspace-previous x` errors with "takes no arguments".
- In `layout_test.go` (nested), `TestNavigationBindsDriveTheCompositor`:
  - Bind `focus-workspace-down` and `swap-window-right` through `executeBindAction`.
  - Assert that the active workspace ID and the window order in `c.model.Snapshot()` change, and
    that the IPC state notifier fired.

- [ ] **Step 2: Run them to verify they fail**

Run each package's named test, for example
`timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/ipc -run TestIndexActionsDecodeNiriShapes`.
Run the nested test on the laptop.
Expected: FAIL. The decode yields no `Index`, the replies are `unimplemented`, and the binds are
unapplied.

- [ ] **Step 3: Implement the wiring**

Follow the existing per-action pattern in each file. Each compositor wrapper calls
`applyWorkspaceFocusAction` or `applyWorkspaceAction`, the same split as its neighbours: focus
actions focus, moves do not. Wrappers that change output also update cursor-output focus, as
`FocusMonitorLeft` does.

- [ ] **Step 4: Run every Step 1 test, then the existing `TestNiriHandler`, `TestBindsAreNotReportedUnapplied` and `TestScreenshotUIMovesTheSelectionByBinds` (nested, laptop)**

Expected: PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat: wire navigation and workspace actions to binds and IPC"`

### Task 5: wheel, touchpad-scroll and mouse bind triggers with `cooldown-ms`

**Files:**
- Modify:
  - `internal/bind/bind.go` (`Binding.Cooldown time.Duration`)
  - `internal/config/config.go` (parse `cooldown-ms`, a non-negative integer; apply
    non-key-trigger binds instead of marking them unapplied)
  - `internal/wlr/input.go` (`handleCursorAxis` and `handleCursorButton`)
  - `internal/wlr/keyboard.go` (cooldown in `executeKeyBinding`, and a lookup by chord shared with
    `dispatchKeyBinding`)
  - `internal/wlr/fakepointer.go` (an `axis(orientation, delta float64, v120 int32, source)`
    helper for tests)
- Create: `internal/wlr/scroll_tracker.go` (Niri's `ScrollTracker`)
- Test:
  - `internal/wlr/scroll_tracker_test.go` (pure)
  - `internal/config/config_test.go`
  - `internal/wlr/pointer_bind_test.go` (nested)

**Interfaces:**
- Produces:
  - `type scrollTracker struct{ tick, last, acc float64 }`, built with
    `newScrollTracker(tick float64)`.
  - `(*scrollTracker).accumulate(amount float64) int8` and `reset()`.
  - `bind.Binding.Cooldown`.

- [ ] **Step 1: Write the failing tests**

`TestScrollTrackerMatchesNiri` (pure):
- With tick 120: `accumulate(60)` gives 0, then `accumulate(60)` gives 1.
- `accumulate(-30)` after positive input resets the accumulator for the direction change and
  gives 0.
- `accumulate(-240)` gives -2.
- The result clamps at ±127 ticks.

`TestScrollBindsParseWithCooldown`:
- `Mod+WheelScrollDown cooldown-ms=150 { focus-workspace-down; }` gives a binding with a scroll
  trigger, a 150 ms cooldown, and an applied action.
- `cooldown-ms=-1` errors.
- Niri's default config now has an empty `UnappliedActions` for every Part 1 name.

`TestWheelBindsInterceptOnlyBoundModifiers` (nested):
- Bind `Mod+WheelScrollDown cooldown-ms=150 { focus-workspace-down; }` and
  `Mod+WheelScrollRight { focus-column-right; }`.
- With Mod held, send a wheel event with v120=120 at t=1000. The workspace switches and the client
  receives no axis event.
- Another at t=1100 is inside the cooldown: no switch.
- At t=1200 it switches again.
- Without Mod, the axis reaches the client's `wl_pointer`. Use testclient's pointer axis counter,
  adding it if absent.
- With Shift only (no Shift wheel bind), it also reaches the client.

`TestMouseBindSuppressesItsRelease` (nested):
- Bind `Mod+MouseMiddle { close-window; }`. A Mod+middle press closes the focused window.
- Neither the press nor the release reaches the client.
- A plain middle press reaches the client.

`TestTouchpadScrollBindsUseFingerTicks` (nested):
- Bind `Mod+TouchpadScrollDown { focus-workspace-down; }`.
- Finger-source axis deltas of 4 and 4 trigger nothing; a further 4 triggers one action (tick 10).

- [ ] **Step 2: Run them to verify they fail**

Expected: build failure (`newScrollTracker`), or a FAIL where the binds are unapplied and the axis
is delivered to the client.

- [ ] **Step 3: Implement as `on_pointer_axis` and `on_pointer_button` do**

- Intercept wheel input only when the current modifiers are in `modsWithWheelBinds`. Do the same
  for finger scroll with `modsWithFingerScrollBinds` and for buttons with `modsWithMouseBinds`.
  Recompute these sets on config load and reload.
- Otherwise reset both trackers and deliver the input as now.
- Run the bind once per tick.
- Keep cooldowns per chord in a map of the last run's event time. Check lock gating before the
  cooldown, as `handle_bind` does.
- Suppress the release by button code.

- [ ] **Step 4: Run the tests, then `TestNestedModPointerGrabsUseEdgeResizeAndCursorImages` and `TestModDragPersistsFloatingWindowPositionOnRelease` (Mod+drag must still work when no mouse bind uses Mod+Left)**

Expected: PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat: wheel, touchpad-scroll and mouse binds with cooldown"`

### Task 6: Niri's overview wheel

**Files:**
- Modify:
  - `internal/wlr/input.go` (the axis path)
  - `internal/wm/model.go` (under-mouse variants by output name)
  - `internal/wlr/overview.go`
- Test: `internal/wlr/overview_test.go` (nested)

**Interfaces:**
- Consumes: Task 5's trackers and cooldown.
- Produces: `(*Model).FocusWorkspaceDownOn(output string) bool`, `FocusWorkspaceUpOn(output string)
  bool`, `FocusColumnLeftOn(output string) bool` and `FocusColumnRightOn(output string) bool`, which
  act on the given output's active workspace without changing the focused output.

- [ ] **Step 1: Write the failing test**

`TestOverviewWheelSwitchesWorkspaceUnderPointer`:
- Open the overview and put the pointer over output A's backdrop (not over a layer surface).
- A wheel down with no modifier switches A's workspace down. A second within 50 ms does not.
- Shift+wheel down focuses the next column on A.
- A horizontal wheel right focuses the next column.
- Over a top-layer surface the axis reaches that surface instead.
- With the overview closed and no modifiers, the axis reaches the window.

- [ ] **Step 2: Run it to verify it fails** (on the laptop, nested)

Expected: FAIL. The workspace does not change in the overview.

- [ ] **Step 3: Implement Niri's `should_handle_in_overview` branch**

It handles the overview only while the pointer is not over a top or overlay layer surface. Bind the
built-in wheel actions: `FocusWorkspaceUp/DownUnderMouse` with a 50 ms cooldown,
`FocusColumnLeft/RightUnderMouse` for Shift with a 50 ms cooldown, and the horizontal binds with no
cooldown. These run ahead of configured binds, but only when there are no modifiers or only Shift,
as `on_pointer_axis` does.

- [ ] **Step 4: Run it and the existing overview tests**

Run `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run 'Overview'` on the laptop,
nested.
Expected: PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat: overview wheel switches workspaces and columns under the pointer"`

### Task 7: gate and records

**Files:**
- Modify:
  - `docs/plans/niri-parity-matrix.md`
  - `docs/README.md` (an index row)
  - this plan (the execution record)
- Evidence: `docs/plans/assets/2026-10-31-m15-navigation/` (VM screenshots, `validate` output)

- [ ] **Step 1: Run `validate` on both configs**

Build `sysc-compositor` and run `validate -c` on Niri's default config and on
`/home/nomadx/.config/niri/config.kdl`.
Expected: the unapplied-action warning lists only Part 2 sizing names
(`switch-preset-*`, `set-column-width`, `set-window-height`, `reset-window-height`,
`maximize-*`, `expand-column-to-available-width`, `center-*`).

- [ ] **Step 2: Run the VM gate**

Boot `~/vm/sysc-session` (`./run.sh`) with this build installed, and log in through sysc-greet.
Drive each check with `qmp.py`:
- Mod+Page_Down and Mod+Page_Up switch workspaces.
- Mod+wheel down switches workspaces once per 150 ms.
- Mod+Ctrl+wheel moves the column to another workspace.
- The overview wheel works.
- `focus-workspace-previous` returns to the previous workspace. It is bound in the owner's
  `config.kdl`.

Capture `gvnccapture` screenshots and save them under the evidence directory.

- [ ] **Step 3: Update the parity matrix**

- Add one matrix row group "Navigation and workspace (M15 part 1)", ticking the 30 actions with
  their test names.
- Mark `binds` as covering mouse and scroll triggers and `cooldown-ms`.
- Mark the overview wheel in the `overview` row.
- Update the `recent-windows` row's "opened keys for … or-first … not built".

- [ ] **Step 4: Commit, push and open the PR. Merge on green CI under the standing approval.**

## Execution record, 2026-10-31

All seven tasks ran inline (executing-plans), with nested tests on the laptop, one test per
command. The whole-branch review was a self-review, because this session does not start subagents
unless the owner asks.

**Tests.** Unit tests in `wm`, `layout`, `bind`, `config`, `ipc` and `server` pass. Nested tests
on the laptop: `TestNavigationBindsDriveTheCompositor`, `TestScreenshotUIMovesTheSelectionByOrActions`,
`TestWheelBindsInterceptOnlyBoundModifiers`, `TestMouseBindSuppressesItsRelease`,
`TestTouchpadScrollBindsUseFingerTicks`, `TestOverviewWheelSwitchesWorkspaceUnderPointer`,
`TestOverviewWheelOverTopLayerReachesTheLayer` and `TestOverviewHeldButtonKeepsWindowsUnfocused`
each failed before their code, and pass. The existing overview, pointer-focus, DND and
hot-corner tests still pass.

**`validate`** ([validate.txt](assets/2026-10-31-m15-navigation/validate.txt)). Niri's default
config and the owner's `config.kdl` now list only Part 2 sizing actions as unapplied
(`switch-preset-*`, `set-column-width`, `set-window-height`, `reset-window-height`, `maximize-*`,
`expand-column-to-available-width`, `center-*`).

**VM gate.** The session VM ran virgl and a real greetd login through sysc-greet. The branch at `269ceb7`, before
the final review's fix, was installed as `/usr/local/bin/sysc-compositor` (the previous binary is
`sysc-compositor.pre-m15-20261031`). The VM used a copy of the owner's `config.kdl`, with
`sysc-shell.kdl` and `sysc.kdl`. Three foot windows were opened over IPC. Keys went through QMP, and
wheels through [qmpwheel.py](assets/2026-10-31-m15-navigation/qmpwheel.py), which holds keys
and sends wheel clicks a set time apart. State was read over the Niri IPC socket.

| Check | Result |
|---|---|
| Mod+Page_Down, Mod+Page_Up | workspace 1 → 2 → 1 ([ws2](assets/2026-10-31-m15-navigation/m15-ws2.png), [ws1](assets/2026-10-31-m15-navigation/m15-ws1.png)) |
| Mod+Tab (`focus-workspace-previous` in the owner's config) | 1 → 2 → 1 |
| Mod+wheel down, `cooldown-ms=150` | three clicks 50 ms apart moved one workspace; two clicks 250 ms apart moved two |
| Mod+Ctrl+wheel down | the focused column moved to workspace 2 and focus followed ([moved](assets/2026-10-31-m15-navigation/m15-moved.png)) |
| Overview wheel, on the backdrop | a click down switched workspace 2 → 3 ([overview](assets/2026-10-31-m15-navigation/m15-overview-wheel.png)); three clicks up 15 ms apart moved one workspace (50 ms cooldown) |
| Overview Shift+wheel up | column focus moved left (window 3 → 4) ([shift](assets/2026-10-31-m15-navigation/m15-overview-shift.png)) |

The VM's virtio tablet reports only `REL_WHEEL`, so it cannot send a horizontal wheel.
`TestOverviewWheelSwitchesWorkspaceUnderPointer` covers the horizontal overview binds.

**Rulings.**
- The or-composites return whether anything changed, not Niri's output-changed bool. sysc has no
  cursor warp for that bool to drive.
- `TestNavigationBindsDriveTheCompositor` is a nested test, because focus actions need a real seat.
- `TestParseReportsUnappliedBindActions` now uses `toggle-debug-tint` (Part 4) as its unapplied
  example. `TestParseKeepsSupportedActionsOnUnsupportedTriggersVisible` is removed, because it
  pinned wheel binds as unapplied. `TestScrollBindsParseWithCooldown` pins the new contract.
- The open window switcher takes Niri's opened binds for the or-first, or-monitor and
  window-or-column focus actions, on every trigger (`mru.rs` `make_dynamic_opened_binds`).
- Key-bind cooldowns use a wall clock in milliseconds; pointer-bind cooldowns use the event's
  `time_msec`. Each chord only ever sees one clock.
- The overview's column wheel acts on the workspace under the pointer, which may be an inactive
  row. That is Niri's `workspace_under_cursor(true)`; the brief had said the output's active
  workspace.
- In the overview, top and overlay layer surfaces now keep the pointer, scrolls and clicks, as
  Niri's `contents_under` gives them. Before, the overview cleared pointer focus everywhere. A click
  on a bar in the overview therefore no longer closes the overview.
- The final review found that this change let a button held on the overview backdrop carry motion
  to a window with stale pointer focus. It is fixed, and
  `TestOverviewHeldButtonKeepsWindowsUnfocused` covers it.

**Deferred.**
- Niri's overview touchpad scroll gesture (`overview_scroll_swipe_gesture`) is not built. A finger
  scroll in the overview reaches nothing.
- Popups of top and overlay layer surfaces do not take the pointer in the overview.

Both are built in the addendum below.

## Addendum, 2026-11-02: overview input follow-up

The owner asked for the two deferred items, and for part 1's rulings to be checked against Mango
and Hyprland. Branch `feat/sysc-1280-m15-overview-input-20261101`; the steps are in
[the handover](2026-11-02-m15-overview-input-handover.md).

**Rulings against Mango and Hyprland.** One changed. Bind cooldowns now run on the compositor's
monotonic clock for keys, wheels and mouse binds, not on event timestamps (`ef3afe9`). Niri times a
cooldown with its own timer, and Hyprland's `binds:scroll_event_delay` is a wall-clock timer too. A
virtual pointer that stamps every event alike left a bind stuck in its cooldown
(`TestWheelCooldownIgnoresEventTimestamps`). The other rulings stand; the handover has the table.

**Layer popups in the overview** (`e6b1e2a`). A popup is not a subsurface, so the root-surface
lookup never mapped a bar's menu to its layer. In the overview the menu lost the pointer. Outside
it, a click on the menu cleared the bar's on-demand keyboard focus. `layerSurfaceForSurface` now
walks popup parents through `shellSurface`, which the window lookup already used. That is Niri's
`find_root_shell_surface`. `TestOverviewLayerPopupTakesThePointer` uses a new hand-written client,
`testclient.LayerPopupClient`: a top-layer bar with on-demand keyboard and an xdg popup. In the
overview, a wheel and a click on the popup reach it. The overview stays open, the workspace does
not change, and the bar takes the keyboard. The client binds layer-shell v4, not v1, because
on-demand keyboard needs v4.

**Overview touchpad scroll** (`208581f`). This is Niri's `overview_scroll_swipe_gesture`. In the
overview, with no modifiers held and the pointer off layer surfaces, a finger or continuous scroll
is a gesture and reaches no bind or client. sysc feeds it through the 3-finger swipe; the swipe's
update is now `workspaceSwipe`, and its aim is `aimWorkspaceSwipe`. A horizontal scroll moves the
view of the row under the pointer. A vertical scroll switches the workspace. A zero delta,
libinput's scroll stop, ends the gesture. So does any scroll outside those conditions, and that
scroll then goes on to binds and clients. Continuous scrolls now also count as touchpad scrolls
for `TouchpadScroll*` binds, as in Niri. `TestOverviewTouchpadScrollSwipes` covers the horizontal
view move, one switch per vertical gesture, a second switch after the scroll stop (from a
continuous source), no gesture with Shift held, and no axis reaching the window.

**Rulings.**
- The axis is chosen by the swipe's 16 px threshold, not by Niri's "first event decides".
- The vertical switch is discrete, once per gesture, as sysc's 3-finger swipe is. Niri's
  `workspace_switch_gesture` follows the fingers. That gap predates this work.
- Niri reads a positive scroll as down or right. The 3-finger swipe reads a positive dx as right
  but a positive dy as up, so the vertical scroll delta is flipped before it reaches the swipe.
- Found, not fixed: sysc applies no libinput settings, so touchpad `natural-scroll` (on in Niri's
  default config) is never set. The 3-finger swipe's two axes also assume opposite settings. With
  natural scrolling on, Niri's horizontal swipe moves the view the other way from sysc's.

**Tests.** `TestOverviewLayerPopupTakesThePointer` and `TestOverviewTouchpadScrollSwipes` failed
before their code, and pass. The touchpad test's RED run had the gesture call disabled in the
laptop copy. These nested tests also pass on the laptop, one per command:
- `TestTouchpadScrollBindsUseFingerTicks` and `TestPointerGesturesAndHotCornerDriveOverviewOnCompositorThread`;
- `TestOverviewWheelOverTopLayerReachesTheLayer` and `TestOverviewWheelSwitchesWorkspaceUnderPointer`;
- `TestOverviewHeldButtonKeepsWindowsUnfocused` and `TestLayerKeyboardFocusKeepsWindowActionsAndSnapshot`;
- `TestWheelBindsInterceptOnlyBoundModifiers`, `TestMouseBindSuppressesItsRelease` and `TestWheelCooldownIgnoresEventTimestamps`;
- `TestExclusiveZoneShrinksUsableArea` and `TestNavigationBindsDriveTheCompositor`.

**VM check.** The session VM ran this branch at `208581f`, installed as
`/usr/local/bin/sysc-compositor`, with a greetd login through sysc-greet. The part 1 build it
replaced was not kept; `sysc-compositor.pre-m15-20261031` is still there. waybar 0.15 ran a
custom module with `"menu": "on-click"`; its first item touches a marker file
([config](assets/2026-11-02-m15-overview-input/waybar-menu-config.txt)). A foot window was open,
and the overview was opened over IPC. A QMP click on the module opened the menu in the overview
([menu](assets/2026-11-02-m15-overview-input/overview-layer-menu.png)). A click on the first item
created the file, and `overview-state` stayed open
([after](assets/2026-11-02-m15-overview-input/overview-after-menu-item.png)). In an earlier
attempt, on an empty workspace, the first click on the module after Escape had closed the menu
outside the overview showed nothing, and the second opened it; the item click passed the same
way. QMP's tablet cannot send a finger scroll, so the touchpad gesture is proved by test only.

Part 2 (sizing) is next.
