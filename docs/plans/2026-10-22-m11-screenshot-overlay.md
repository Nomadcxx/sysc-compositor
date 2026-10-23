# M11 Screenshots, Capture Globals, Hotkey Overlay and Recent Windows Plan

**Goal:** Complete the M11 portion of the Niri 26.04 contract: the capture and clipboard globals assigned by owner decision Q9; the screenshot actions and the interactive screenshot UI; and the hotkey overlay and recent-windows switcher.

**Architecture:** `internal/wlr` owns every Wayland global, the scene, capture and the selection, all on the locked compositor thread. Screenshots are read back from SceneFX's renderer into Go memory, encoded with the standard library's `image/png`, then written to `screenshot-path` and offered as the clipboard selection by a compositor-owned `wlr_data_source`. `internal/server` turns the screenshot actions into `ScreenshotCaptured` events. No new Go module.

**Tech stack:** Go, cgo, wlroots 0.20, SceneFX 0.5, niri-ipc 26.4.0. Behaviour reference: Niri `v26.04` (`8ed0da4`, `src/ui/screenshot_ui.rs`, `src/ui/hotkey_overlay.rs`, `src/ui/mru.rs`). wlroots-call reference: mango `0.18.0` (`a21b10f`).

## Contract and decisions

**Issue and sources.** Issue `sysc-1276` covers design §15 M11, the M11 rows of `niri-parity-matrix.md`, and the Q9 globals.

**Globals (Q9).**
- **Data control:** `ext_data_control_manager_v1` and the `zwlr_data_control_manager_v1` fallback. Design §7 makes ext-data-control session-hard: sysc-clipboard depends on it.
- **Screencopy:** `zwlr_screencopy_manager_v1` v3, for grim and the sysc-shell frost fallback. wlroots refuses a buffer that is not exactly the frame's size and stride, which matches Niri's exact-sized SHM rule (design §7).
- **Image copy capture:** `ext_image_copy_capture_manager_v1` with the output capture source.
- **Scene-handled:** `wp_presentation`, `wp_single_pixel_buffer_manager_v1` and `wp_alpha_modifier_v1`. SceneFX's scene implements presentation feedback and alpha-modifier itself once the globals exist.
- **Security:** Niri's security model restricts none of these, so neither does this milestone.

**Screenshot actions.**
- `ScreenshotScreen` and `ScreenshotWindow` take `write-to-disk` and `show-pointer`; `Screenshot` takes `show-pointer`; all take the optional path, as in Niri's `binds.rs`.
- Each screenshot is copied to the clipboard as `image/png`. It is written to `screenshot-path` (a strftime pattern, default `~/Pictures/Screenshots/Screenshot from %Y-%m-%d %H-%M-%S.png`) unless `write-to-disk=false` or the section sets `null`.
- The `ScreenshotCaptured { path }` event goes out on the event stream.

**Interactive screenshot UI** (`Screenshot`).
- It freezes every output's current frame and dims it outside a selection.
- The selection is moved and resized with the pointer and with Niri's keys. Space, Enter and the bound action confirm; Escape cancels; `P` toggles the pointer.
- The selection persists between openings, as in Niri.

**Hotkey overlay and recent windows.**
- `ShowHotkeyOverlay` and the `hotkey-overlay` section (`skip-at-startup`, `hide-not-bound`).
- The `recent-windows` section and its switcher, Niri's MRU UI.

**Owner decision needed: text rendering.**
- **What needs it:** the hotkey overlay, the recent-windows switcher, the screenshot UI's hint and the exit confirmation dialog all draw text. Niri uses pango and cairo.
- **The gap:** this tree has no text renderer. The standard library rasterizes no fonts.
- **Options:**
  - (a) cgo to the system pango and cairo through pkg-config: no Go module, matches Niri's look, and adds two C library dependencies.
  - (b) A Go font module such as `golang.org/x/image`: a `go.mod` change, which needs approval.
  - (c) Ship these UIs without text first.
- **Until answered:** the text-free parts land first (globals, screenshot actions, the UI's selection and dimming), and the text parts wait.
- **Answered 2026-10-23:** no new C. A hybrid of the Go options: `github.com/go-text/typesetting` finds system fonts, falls back per rune and shapes (its HarfBuzz port), and `golang.org/x/image` rasterizes into a Go RGBA image that the scene shows through a compositor-owned `wlr_buffer`. Both modules are approved for `go.mod`. bd `sysc-1646` separately investigates replacing SceneFX and wlroots with Go.

**Out of scope.** Screencast and the portal are M12. Foreign-toplevel capture sources are M12 (Q9).

## Task 1: Capture and clipboard globals

**Files:** `internal/wlr` (a new `capture.go`, `compositor.go`), `internal/wlr/client_test.go`, `.github/ci/compositor-tests.sh` (install `wl-clipboard` and `grim`).

1. **Create the managers** in `NewWith` after the scene and the output layout:
   - data control (ext and wlr);
   - screencopy;
   - ext-image-copy-capture with the output image-capture source;
   - presentation-time (with the backend);
   - single-pixel-buffer;
   - alpha-modifier.
2. **Globals test:** list them in the M11 globals test.
3. **Prove data control:**
   - A nested test sets a selection from one client (`wl-copy`).
   - A second client with no keyboard focus reads it back through data control (`wl-paste`).
4. **Prove screencopy:** grim captures an output to PNG at exactly the output's pixel size. A test client with a wrong-sized SHM buffer gets `invalid_buffer`.

## Task 2: Screenshot actions without UI

**Files:** `internal/wlr` (`screenshot.go`), `internal/bind`, `internal/config` (`screenshot-path`, the action properties), `internal/ipc`, `internal/server`.

1. **Read-back:**
   - Render an output's scene, or one window's tree, into a buffer through SceneFX's renderer.
   - Read it back as RGBA, with the pointer if asked.
   - Encode PNG on a goroutine, never on the compositor thread.
2. **Write and copy:**
   - Write to the expanded `screenshot-path`, creating the directory.
   - Set the clipboard selection to a compositor-owned `image/png` source that serves the bytes on request.
3. **Event:** emit `ScreenshotCaptured`.
4. **Tests:**
   - The PNG's size and a known pixel of a solid client buffer.
   - The written path.
   - The clipboard offer, read back over data control.
   - The event.

## Task 3: Interactive screenshot UI (text-free part)

1. **Open:**
   - Freeze per-output frames into scene buffers above all content.
   - Dim outside the selection, and draw the selection border with SceneFX rects.
2. **Input routing:** while open, the pointer and Niri's keys go to the UI only. The session lock closes it.
3. **Confirm:** Space, Enter or the bound action captures the selection from the frozen frame through the Task 2 path. Escape cancels.
4. **Tests:** open, move the selection with keys, confirm. The captured PNG has the selection's size.

## Task 4: Hotkey overlay, recent windows, text (after the owner's text decision)

1. Text rendering per the decision.
2. **Hotkey overlay:** `ShowHotkeyOverlay`, with Niri's list of shown actions and its `hotkey-overlay` options.
3. **Recent windows:** the switcher with Niri's MRU order, binds and `recent-windows` options.
4. **Text-dependent pieces:**
   - the screenshot UI's hint text;
   - the exit confirmation dialog, which also completes `Quit` without `skip-confirmation` (matrix row `Quit`).

### Task 4, recent windows (written 2026-10-23)

Niri sources: `src/ui/mru.rs`, `src/ui/mru/tests.rs`, `niri-config/src/recent_windows.rs`,
`src/niri.rs` (`PendingMruCommit`, `mru_apply_keyboard_commit`), `src/input/mod.rs`
(`FocusWindowPrevious`, `MruAdvance`, `make_binds_iter`). Matrix rows: `recent-windows` and the
`FocusWindowPrevious` action. Each step below is its own PR.

1. **Config, focus timestamps and `FocusWindowPrevious`.**
   - The `recent-windows` section: `on`/`off`, `debounce-ms` (750), `open-delay-ms` (150);
     `highlight` (`active-color` 0.6 grey, `urgent-color` (1, .6, .6), `padding` 30,
     `corner-radius` 0); `previews` (`max-height` 480, `max-scale` 0.5).
   - Its `binds` block: keyboard keys with a modifier, one `next-window` or `previous-window` each,
     with `scope=` (`all`, `output`, `workspace`) and `filter=` (`app-id`), and `allow-inhibiting`
     and `hotkey-overlay-title`. A duplicate key is an error. The first `binds` block clears the
     defaults (Alt and Mod with Tab, and with grave filtered by app id; Shift reverses); a later one
     replaces binds by key.
   - `animations.recent-windows-close`: a spring, 1.0/800/0.001.
   - The existing focus timestamps take `debounce-ms`; a debounce of 0 stamps at once.
   - A key that reaches the focused client commits the pending stamp at once.
   - `FocusWindowPrevious` (IPC and the `focus-window-previous` bind) focuses the window with the
     newest timestamp other than the focused one, committing the pending stamp first.
2. **The MRU model** in `internal/wm`, pure Go: every window newest first, with its output,
   workspace and app id; scopes All, Output and Workspace; the app-id filter; forward, backward,
   first, last, set-current and remove. The selection is always in the filtered list (Niri's
   `verify_invariants`), proved by a randomized test like Niri's proptest.
3. **The switcher.**
   - The recent-windows binds run when no general bind matches. The first opens the switcher on the
     active output after committing the pending stamp, and selects the next window unless nothing
     is focused; later ones advance.
   - It shows after `open-delay-ms`: the 0.8 black backdrop, previews by `sceneFXCloneView` as the
     overview does, the highlight and 2px border, titles and the scope panel through
     `internal/text`. Preview sizing follows `Thumbnail::preview_size`.
   - While open: Niri's preset keys (Escape cancels; Return and Space confirm; a, o, w set the
     scope; s cycles it; Home, End, Left, Right) and the keys of the general binds that Niri maps
     (`make_dynamic_opened_binds`); releasing every modifier confirms; the pointer selects; a left
     click on a preview confirms, and anywhere else cancels.
   - Confirm focuses the selection; cancel leaves focus; a closing window leaves the list, and the
     last one closes the switcher. `recent-windows-close` fades it out.
4. **Hotkey overlay.** Niri's `collect_actions` reads the general binds only, so the overlay does
   not list the recent-windows binds. Nothing to add.

## Task 5: Close M11

1. Tick only proven rows in the parity matrix: requests, events, actions, config, and the Wayland globals table.
2. Run named tests one at a time with the required command form, plus the static checks, and confirm `go.mod`/`go.sum` are unchanged.
3. Nested gate with consent: grim, wl-copy/wl-paste against sysc-clipboard, the screenshot UI.

**M11 stop condition:**
- Every M11 global, action, event and config row listed above has working behaviour and a focused proof.
- Data control serves sysc-clipboard.
- Screenshots match Niri's paths, clipboard and event.
- The text-dependent UIs land per the owner's text decision, or are recorded as waiting on it.

## Execution record (2026-10-22)

PR #32 delivers Tasks 1–3. Task 4 waits for the owner's text-rendering decision. The nested tests
ran in CI lane 2; the pure ones also ran locally, one named test per command.

| Item | Proof |
|---|---|
| Capture and clipboard globals (Task 1) | `TestM11CaptureGlobalsAdvertised`, `TestDataControlServesTheSelectionWithoutFocus`, `TestScreencopyCapturesTheOutputAtItsSize` |
| Output read-back, PNG clipboard | `TestScreenshotReadsBackAnOutputAndOffersItAsPNG` |
| `screenshot-path`, `ScreenshotScreen`, `ScreenshotCaptured` | `TestScreenshotPathDefaultsToNiriAndTakesNull`, `TestScreenshotPathExpandsStrftimeAndHome`, `TestScreenshotScreenWritesThePNGAndReportsIt`, `TestScreenshotScreenActionRoundTrips`, `TestScreenshotScreenBindProperties` |
| `ScreenshotWindow` | `TestScreenshotWindowCapturesTheWindowAlone`, `TestScreenshotWindowActionRoundTrips`, `TestScreenshotWindowBindDefaultsHideThePointer` |
| Interactive UI, text-free (Task 3) | `TestScreenshotUISelectsAndCapturesARegion`, `TestScreenshotActionRoundTrips`, `TestScreenshotBindTakesShowPointerOnly` |

**Found along the way:**
- **No client could set the clipboard** (`sysc-1645`): the user seat ignored selection requests.
  Fixed and merged on its own as #33.
- **`runClient` lost a fast client's output** by closing the pipe before draining it.
- **The data-control test hung lane 2**: `wl-copy` forked and held the test's pipes.

**Open:**
- **The pointer is not drawn into screenshots** (`show-pointer`), because the pointer is not part of
  the scene.
- **The UI has no keyboard selection moves yet.**
- **Untested here:** presentation-time, single-pixel-buffer and alpha-modifier are advertised and
  scene-handled. Ext-image-copy-capture likewise has no capture test of its own.
- **Task 4:** the hotkey overlay, recent windows, the screenshot hint and the exit dialog wait for the
  text-rendering decision.
- **Nested gate** with grim and sysc-clipboard on the laptop.

## Execution record (2026-10-23): text and the exit dialog

The first Task 4 PR. The owner chose Go text rendering (decision above).

| Item | Proof |
|---|---|
| `internal/text`: the Pango markup subset Niri's UIs and `hotkey-overlay-title` use, system fonts with per-rune fallback, shaping, letter spacing, span colours and backgrounds, centred lines | `TestParseMarkupTakesNiriStrings`, `TestLayoutDrawsTextAtTheFontSizeAndScale`, `TestLayoutBreaksLinesAndCentresThem`, `TestSpanBackgroundAndColourAreDrawn`, `TestFallbackDrawsARuneTheFamilyLacks` (skips where no installed font has the CJK probe rune) |
| `internal/ui`: Niri's exit dialog panel (sans 14px, 16px padding, 8px red stroke, mono key span) per scale | `TestExitDialogIsNiriPanel` |
| A read-only `wlr_buffer` over Go-drawn pixels, as a scene buffer node | used by the dialog tests below |
| `Quit` with the exit dialog: IPC and the `quit` bind with `skip-confirmation`; over every output and the lock screen; no client focus while open; Enter quits, any other key, button or scroll closes it; `exit-confirmation-open-close` animation | `TestExitDialogAsksBeforeQuitting`, `TestExitDialogEnterQuits`, `TestQuitSkipConfirmationQuitsAtOnce`, `TestQuitBindRunsWhileLocked`, `TestQuitBindTakesSkipConfirmation`, `TestExitConfirmationAnimationDefaultsToNiri`, `TestNiriHandler` |

CI lanes 1 and 2 install `ttf-dejavu`, so text renders in both.

**Still open in Task 4:** the screenshot hint (with the `P` pointer toggle, which needs the pointer drawn
into screenshots), the hotkey overlay and recent windows.

## Execution record (2026-10-23): the hotkey overlay

| Item | Proof |
|---|---|
| `hotkey-overlay` (`skip-at-startup`, `hide-not-bound`) and every bind's `hotkey-overlay-title`, kept for binds whose action is not applied here too, so the overlay shows their keys as Niri does | `TestHotkeyOverlaySectionAndBindTitles`, `TestGreeterConfig` (the greeter skips it) |
| Niri's `collect_actions`, `format_bind`, `action_name` and `key_name`, with xkb's canonical key names | `TestHotkeyFormatBindMatchesNiri` (Niri's snapshot cases), `TestHotkeyLinesFollowNiriOrderAndFilters`, `TestHotkeyKeyNameMatchesNiri`, `TestKeyNameIsXKBCanonical` |
| The panel: bold title, monospace keys on grey, markup titles, 2px blue frame, drawn at 0.9 opacity per output scale | `TestHotkeyOverlayPanelLaysOutKeysAndActions` |
| `ShowHotkeyOverlay` (IPC and bind), shown at startup; over windows and layers, under the screenshot UI and the lock; it takes no input, and the next key, button or scroll closes it after being handled | `TestHotkeyOverlayShowsAtStartupWithoutTakingInput` (lane 2) |

Spawn commands in titles are markup-escaped; Niri interpolates them raw.

## Execution record (2026-10-23): the pointer and the screenshot UI's panel

| Item | Proof |
|---|---|
| The pointer in screenshots: the theme image by name or the client's cursor surface, redrawn since wlroots keeps the cursor's image private; `show_pointer` for `ScreenshotScreen`, `ScreenshotWindow` (where it is over the window) and `Screenshot` | `TestScreenshotScreenDrawsThePointerWhenAsked` (lane 2) |
| Niri's help panel (capture button, Space and P keys, 2px line spacing), bottom centre on every output at 0.9, 0.3 while dragging | `TestScreenshotHintIsNiriPanel` |
| P toggles the frozen pointer and the panel text; Ctrl+C confirms to the clipboard only; Alt or Shift make keys inert; a press and release on the capture button saves; a click selects 32×32 | `TestScreenshotUIHintTogglesThePointerAndCaptures` (lane 2) |

**Still open:** selection moves by the move, monitor and size binds, and the `screenshot-ui-open` animation; then recent windows.

## Execution record (2026-10-23): selection moves

While the screenshot UI is open, keys it does not take run their binds as Niri's
`allowed_during_screenshot`: quit and monitor power as usual; `move-column-left/right` and
`move-window-up/down` move the selection 50px within its output; the column and window
monitor moves carry it to that output keeping its relative place and logical size. Held binds
repeat. Proof: `TestScreenshotUIMovesTheSelectionByBinds` (lane 2). The size binds wait for
`set-window-width/height` and `set-column-width`, which are not built yet.

## Execution record (2026-10-23): recent-windows config and focus-window-previous

Step 1 of the recent-windows section above.

| Item | Proof |
|---|---|
| `recent-windows`: Niri's defaults, every option, the binds block (keyboard keys with a modifier, one `next-window` or `previous-window` with `scope` and `filter`), the first binds block clearing the defaults and later ones replacing by key, `recent-windows-close` | `TestRecentWindowsDefaultsToNiri`, `TestRecentWindowsSectionMergesAsNiri` |
| `FocusWindowPrevious` (IPC and `focus-window-previous`), committing the stamp of the window it leaves; `debounce-ms`, including 0; a key sent to the focused window commits its stamp | `TestFocusWindowPreviousCommitsTheWindowItLeaves` (lane 2), `TestParseAction`, `TestNiriM6ActionsArePosted` |

## Execution record (2026-10-23): the MRU model

Step 2. `internal/wm/mru.go` ports Niri's `WindowMru`: windows newest stamp first (unstamped last),
scopes, the app-id filter, forward/backward with wrap, first, last, set-current, Niri's scope cycle
(All, Workspace, Output) and removal. A scope or filter change keeps the selection, or takes the
nearest listed window before it, else the first.

| Item | Proof |
|---|---|
| Order, wrap-around | `TestMRUOrdersByFocusStampNewestFirst` |
| Niri's `remove_last_window_out_of_two` | `TestMRURemovingTheLastOfTwoKeepsASelection` |
| Scope, filter and cycle selection rules | `TestMRUScopeAndFilterKeepTheNearestSelection` |
| Niri's `random_operations_dont_panic` with `verify_invariants` (2,000 seeded runs) | `TestMRURandomOperationsKeepTheSelectionListed` |

## Execution record (2026-10-23): the window switcher

Step 3. `internal/wlr/switcher.go` is Niri's `WindowMruUi`; `internal/wm/mru_layout.go` is its strip
layout; `internal/ui/mru.go` draws the titles and the scope panel.

| Item | Proof |
|---|---|
| Strip layout: Niri's `preview_size`, the gap from the highlight padding and border, centring a strip that fits, `compute_view_pos` scrolling with the least motion, the highlight box with its title | `TestMRUPreviewSizeFollowsNiri`, `TestMRUStripCentresOrScrollsToTheSelection` |
| Titles (sans 14px, one line, cut to the preview with Niri's 18px fade) and the scope panel (key letters on the key background, the scope in white) | `TestMRUTitleFadesOutWhenCut`, `TestMRUScopePanelIsNiriPanel` |
| Niri's `make_dynamic_opened_binds` for the actions built here, the fewest modifiers winning per key | `TestSwitcherDynamicBindsFollowNiri` (lane 2) |
| A recent-windows bind opens the switcher on the next window (the newest when nothing is focused), committing the pending stamp; it holds the keyboard and keeps the pointer from clients; it shows after `open-delay-ms`; Tab, the preset keys and the general binds' keys move the selection; releasing every modifier focuses it; Escape cancels; the pointer selects and a left click focuses (anywhere else cancels) | `TestWindowSwitcherSelectsConfirmsAndCancels` (lane 2) |

Also: a closing window leaves the list and the last one closes the switcher; the lock, `Screenshot`
and losing the output cancel it; `recent-windows-close` fades it out; it sits under the hotkey
overlay, the screenshot UI and the lock.

**Not yet like Niri:**
- the view and the previews jump instead of animating (Niri moves them with `window-movement`
  and fades new previews in with `window-open`);
- previews are not clipped to the window geometry or its corner radius;
- the close fade has no offscreen, so overlapping previews show through each other;
- mouse and scroll binds do not reach the open switcher, since only key binds are applied here;
- the opened keys come only from the actions built here (Niri also maps
  `focus-column-right-or-first`, `focus-column-or-monitor-right`,
  `focus-window-down-or-column-right` and their left forms).
