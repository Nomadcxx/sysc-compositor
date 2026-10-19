# M4: mapped windows and Niri window actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Publish mapped xdg toplevels through the Niri 26.4.0 IPC contract and support the window and workspace operations sysc-shell already sends.

**Architecture:** Reuse `toplevelView.id` as the stable identity from compositor lifecycle through IPC and actions. The compositor thread builds plain Go snapshots; IPC clients only read snapshots and post requested operations back to that thread. M4 reports all windows as floating, assigns each to the output under its geometry until M5 adds workspace placement, and keeps the current one-workspace-per-output model.

**Tech Stack:** Go 1.26, cgo, wlroots 0.20, Niri 26.04 / `niri-ipc` 26.4.0.

---

## Constraints and references

- Niri 26.04 defines wire fields, event semantics, focus behavior, and action references.
- Mango 0.18.0 is the wlroots 0.20 reference for xdg lifecycle, seat focus, and close requests. Do not copy its layout or tags.
- Hyprland is a secondary behavior/rendering reference; it uses Aquamarine rather than wlroots, so it does not replace wlroots or Niri evidence for this milestone.
- Go and cgo only. No standalone `.c` or `.h` files. Listener trampolines stay in Go cgo preambles.
- Call wlroots only on the `runtime.LockOSThread` compositor thread. Snapshot publication must not perform socket I/O.
- A window ID remains stable for the toplevel's lifetime, including an unmap/remap, and is never reused during a compositor run. This is also the key used by M12's approved `ext_foreign_toplevel_image_capture_source_v1` join.
- Keep Mango as the greetd host. Do not install this build, restart greetd, or change greetd configuration.
- Run named tests in one package at a time. Never run `go test ./...` or `-race`.

## Task 1: mapped-window lifecycle and snapshots

**Files:** `internal/wlr/windows.go`, `internal/wlr/input.go`, `internal/wlr/compositor.go`, `internal/wlr/window_snapshot_test.go`.

1. Add a nested-client test proving that only mapped xdg toplevels appear in `Compositor.Windows`, each with a nonzero stable ID, app ID/title/PID, logical geometry, output, and actual keyboard-focus state.
2. Run `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run '^TestWindowsReportMappedToplevelState$'`; confirm the lifecycle assertion fails because xdg map, unmap, and destroy do not currently notify the state callback and `Windows` does not expose mapped/focus/output state.
3. Add lifecycle state to the existing `toplevelView`; keep its ID across unmap/remap, omit it while unmapped, and remove it on destroy. Notify the state callback only when mapped-window metadata, geometry, output, focus, or lifecycle changes.
4. Track actual keyboard focus separately from the retained focus candidate used when a layer surface has keyboard focus. Report `is_focused` only for the window receiving keyboard focus.
5. Add Niri-compatible monotonic focus timestamps: first focus is immediate; subsequent focus commits after the Niri default 750 ms debounce. Keep the debounce configurable only when the existing config model gains `recent-windows` support in its owning milestone.
6. Run the named window and focus tests, plus `TestNewWindowTakesFocus` and `TestFocusingNewWindowDeactivatesPrevious` in `internal/wlr`.

## Task 2: Niri window and workspace wire state

**Files:** `internal/ipc/wire.go`, `internal/ipc/wire_test.go`, `internal/server/niri.go`, `internal/server/niri_test.go`, `internal/server/server.go`.

1. Add exact JSON tests for `FocusedWindow`, populated `Windows`, null optional metadata, floating state, and focus timestamps. Include `CloseWindow` with and without an ID, `FocusWindow` by ID, and `FocusWorkspace` references encoded as `Id`, `Index`, and `Name`.
2. Run the focused `internal/ipc` and `internal/server` tests and confirm the new action fields or query are rejected/unimplemented.
3. Convert mapped `wlr.WindowInfo` values to Niri `Window` values: nullable title/app ID/PID, stable ID, workspace ID, actual focus state, floating=true, logical window geometry, and monotonic timestamp.
4. Associate each window with the current workspace for the output under its geometry. Set each workspace's `active_window_id` to its most recently focused mapped window on that output, even while a layer surface receives keyboard focus.
5. Implement `FocusedWindow` as the keyboard-focused window, or null when keyboard focus belongs to a layer surface or no window.
6. Keep `FocusWorkspace` references wire-valid; resolve stable IDs to their output. Until M5 adds dynamic workspaces, an index resolves against the current output and unmatched names/indices are accepted as Niri-style no-ops.
7. Run named IPC and server conversion tests and compare the serialized fields with the pinned Niri `niri-ipc` definitions.

## Task 3: incremental Niri window events

**Files:** `internal/server/eventstream.go`, `internal/server/eventstream_test.go`, `internal/server/niri_test.go`.

1. Add state-diff tests for map, title/app-ID change, close, focus-to-window, focus-clear, focus-timestamp commit, and workspace active-window changes. Verify exact event names, payloads, and order.
2. Run `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/server -run '^TestEventStreamWindow'`; confirm the current `WindowsChanged`-for-every-mutation behavior fails the incremental Niri event assertions.
3. Keep `WindowsChanged` as the initial full snapshot. Publish `WindowOpenedOrChanged`, `WindowClosed`, `WindowFocusChanged`, `WindowFocusTimestampChanged`, and `WorkspaceActiveWindowChanged` for later changes, matching Niri 26.04 semantics.
4. Ensure window lifecycle updates continue through the existing bounded per-client queue. A slow client must still disconnect and receive a full current snapshot on reconnect.
5. Run the named EventStream tests, including the existing handshake, bounded-client, workspace focus, and destroy cases.

## Task 4: window and workspace actions

**Files:** `internal/ipc/wire.go`, `internal/ipc/wire_test.go`, `internal/server/niri.go`, `internal/server/niri_test.go`, `internal/server/server.go`, `internal/server/control.go`, `internal/wlr/input.go`, `internal/wlr/windows.go`, and focused tests.

1. Add handler tests proving valid action requests decode, reach the compositor-thread operation, return `Handled`, and unknown mapped IDs are harmless no-ops as in Niri.
2. Implement `FocusWindow`, `CloseWindow` (explicit ID or retained Niri layout focus, including when a layer owns keyboard focus), and `FocusWorkspace` through `Compositor.Post`; no IPC goroutine may call wlroots directly.
3. Focus through the existing `focusView` path so activation, layer priority, active output, and state events remain consistent. Close with `wlr_xdg_toplevel_send_close`; do not destroy client-owned surfaces directly.
4. Focus the output that owns an ID-targeted workspace while M4 has one active workspace per output. Validate the output still exists when the posted operation runs.
5. Add real-socket action tests and focused wlroots tests showing the target window receives focus or a close request and the resulting snapshot/events update.
6. Run the named IPC, server, and wlroots action tests.

## Task 5: sysc-shell and laptop gate

1. Build sysc-shell from a clean worktree of current `origin/main`; do not modify or deploy the dirty primary sysc-shell checkout.
2. Build the compositor and run it nested in the laptop's existing Niri user session with an isolated runtime directory. Keep Mango as greetd host and do not replace the installed binary.
3. Confirm `niri msg -j windows` reports mapped windows with stable IDs and `niri msg action focus-window --id ...`, `close-window --id ...`, and `focus-workspace ...` return successfully and cause the expected state changes.
4. Run sysc-shell on the nested compositor. Confirm its running-app window list receives incremental map/close/focus changes and its existing FocusWindow, CloseWindow, and FocusWorkspace actions work against the same IDs.
5. Reconnect EventStream after a client disconnect and confirm the initial `WindowsChanged` list matches the current `Windows` query.
6. Record laptop evidence, unavailable hardware behaviors, and rendered observations here; do not claim the full compositor roadmap complete until later milestone gates pass.

## Completion checks

- Update the `FocusedWindow`, `Windows`, `WorkspaceActiveWindowChanged`, `WindowOpenedOrChanged`, `WindowClosed`, `WindowFocusChanged`, `WindowFocusTimestampChanged`, and M4 action rows in `docs/plans/niri-parity-matrix.md`.
- Run `gofmt -w .`, `test -z "$(gofmt -l .)"`, `go vet -p 2 ./...`, build both `cmd/sysc-compositor` and `cmd/niri`, and verify `git diff --exit-code -- go.mod go.sum`.
- Run `go generate ./internal/wlr` once in the fresh worktree before its first build. Generated protocol headers remain gitignored.
- Push the finished M4 branch. M5 must be able to use the M4 window IDs and snapshots without introducing a second identity model.

## Execution record

- Named `internal/wlr`, `internal/server`, and `internal/ipc` checks pass, including nested-client and real Niri-socket actions on the current host.
- `go vet -p 2 ./...` and builds for `cmd/sysc-compositor` and `cmd/niri` pass.
- sysc-shell built from a clean worktree at `origin/main` commit `2e81653b`; focused Niri stream and running-app tests pass.
- Review follow-up: changing the retained layout focus now refreshes active-window state while a layer surface owns keyboard focus; ID-less `CloseWindow` targets that retained Niri focus; windows whose centers leave every output stay in IPC on the focused output; focus timestamp commits reject stale timer callbacks.
- Focused regressions for those cases pass. The stable-ID lifecycle test still verifies identity through unmap; no existing nested test client remaps an xdg toplevel, and the ID remains on the retained `toplevelView` until destroy.
- Laptop runtime qualification completed on 2026-10-19 after SSH to the saved `laptop` target (`192.168.0.64:7777`) recovered. The compositor ran nested under the existing Niri session with an isolated runtime directory and a `WL-1` output at 760×814; the test processes and temporary laptop files were removed afterward.
- `niri msg -j windows` reported mapped windows with stable IDs and metadata. `focus-window --id 1`, `focus-workspace 1`, and `close-window --id 1` returned successfully and produced the expected state changes. EventStream reconnect delivered a full `WindowsChanged` snapshot matching the current windows query; live updates included map/title, active-window, focus, timestamp, and close events.
- With sysc-shell's panel holding keyboard focus, focusing window 4 changed `active_window_id` while the window remained `is_focused: false`; ID-less `close-window` then closed window 4, matching Niri's retained layout-focus behavior.
- A sysc-shell build from the clean `origin/main` worktree, configured with the Running Apps bar, ran on the nested compositor. Its bar, toast, shield, and panel layers mapped, and the process stayed alive through window updates. The host already owned the ScreenSaver bus name; the nested shell continued running.
- Nested `grim` capture was unavailable because M12's screen-capture protocol is not implemented yet. An outer-session screenshot showed the nested compositor window. The host Niri session and sysc-shell remained running; no install, greetd change, or reboot occurred.
