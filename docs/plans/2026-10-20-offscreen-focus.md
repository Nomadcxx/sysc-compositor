# Offscreen Focus Fallback Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Focus mapped windows whose centers fall outside every output and report the same output in compositor and Niri state.

**Architecture:** Add `outputNameForView` beside `outputNameAt` in `internal/wlr/output.go`. It returns the output under the view center, falling back to `FocusedOutputName()` when the center has no output. Use it for focus routing, keyboard-focus eligibility, window snapshots, and workspace window selection.

**Tech Stack:** Go, cgo, wlroots 0.20, nested `weston-terminal` clients.

---

### Task 1: Add the failing offscreen focus regression

**Files:**
- Modify: `internal/wlr/window_snapshot_test.go`

**Step 1: Add `TestFocusWindowOffscreenCenterUsesFocusedOutput`**

Use `newNested` and two `weston-terminal` clients. In the second `viewMapped` callback, move the first view with `c.placeView(target, 100000, 100000)`. Save `c.FocusedOutputName()`, call `c.FocusWindow(target.id)`, and record whether `c.keyboardFocusedView() == target` and the target's `WindowInfo.Output` equals the saved output. Terminate the nested compositor after the assertions.

**Step 2: Run the test and confirm the expected failure**

Run:

```bash
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run '^TestFocusWindowOffscreenCenterUsesFocusedOutput$'
```

Expected: the test fails because `FocusWindow` accepts the ID while `keyboardFocusedView()` remains on the other view.

### Task 2: Share the view-to-output fallback

**Files:**
- Modify: `internal/wlr/output.go`
- Modify: `internal/wlr/input.go`
- Modify: `internal/wlr/windows.go`

**Step 1: Add the resolver**

Add `outputNameForView(view)` beside `outputNameAt`. Return `outputNameAt(viewCenter)` when present; otherwise return `FocusedOutputName()`.

**Step 2: Use the resolver in every M4 view-output decision**

Replace the center-only checks in `focusView`, `updateKeyboardFocus`, `Windows`, and `FocusWorkspaceOutput`. Keep layer selection and geometry unchanged.

**Step 3: Run the focused wlroots checks**

Run:

```bash
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run '^(TestFocusWindowOffscreenCenterUsesFocusedOutput|TestFocusWindowSelectsStableID|TestLayerKeyboardFocusKeepsWindowActionsAndSnapshot)$'
```

Expected: all selected tests pass. The nested tests may skip when their client programs are unavailable.

### Task 3: Verify Niri output fallback

**Files:**
- Test: `internal/server/niri_test.go`

Run the existing offscreen window-output fallback assertion:

```bash
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/server -run '^TestWindowOutputFallsBackToFocusedOutputWhenCenterIsOffscreen$'
```

Expected: the server continues assigning an offscreen window to the connected focused output.

### Task 4: Run the laptop nested check

Build the compositor in this worktree after running `go generate ./internal/wlr` once. Start it nested in the laptop's current Niri session with an isolated runtime directory. Map two test windows, move one center offscreen, call `FocusWindow`, and confirm `niri msg -j windows` reports the target focused on the connected focused output. Leave the installed binary, greetd, and Mango session unchanged.

### Task 5: Complete focused repository checks

Run `gofmt -w internal/wlr/output.go internal/wlr/input.go internal/wlr/windows.go internal/wlr/window_snapshot_test.go`, then confirm `test -z "$(gofmt -l .)"`. Run `go vet -p 2 ./...`, build `cmd/sysc-compositor` and `cmd/niri`, and confirm `git diff --exit-code -- go.mod go.sum`. Do not run `go test ./...` or `-race`.

Commit the implementation and tests after the focused checks pass.
