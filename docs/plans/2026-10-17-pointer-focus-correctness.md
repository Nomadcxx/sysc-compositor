# Pointer focus correctness (TDD plan)

**Kind:** plan (Tier 3 fix, gates M4).
**Inputs:** `2026-10-16-input-pattern-comparison-report.md` P1–P4 and M4.
**bd:** `sysc-1296`, which blocks M4 `sysc-1269`.
**Baseline:** `origin/main` `edbdf27`.

## Exit gate

Each behaviour has a nested test that drives the **user seat** through an in-process pointer and a
real client (weston-terminal), and fails on `edbdf27`:

| # | Behaviour | Test |
|---|---|---|
| P1 | While any button is held, pointer focus stays on the surface that had it; the release reaches that surface, and the surface under the pointer gets focus after the last release. | `TestPointerFocusStaysOnPressedSurface` |
| P2 | Over empty space the drawn image is the theme's `default`, not the last client image. | `TestEmptySpaceShowsDefaultImage` |
| P3 | A hit on a popup or subsurface resolves to the window it belongs to. | `TestPopupResolvesToItsWindow` |
| P4 | Unmap clears every seat's keyboard and pointer focus on the window, the focused view and any compositor grab on it. | `TestUnmapReleasesFocusAndGrab` |
| M4 | Starting a compositor move or resize clears pointer focus; ending it gives focus back to the surface under the pointer. | `TestCompositorGrabClearsPointerFocus` |

Plus the existing package tests, and a laptop live check: the real binary nested, kitty inside, a
selection dragged out of the window, and a `grim` shot of the image over empty space.

## Decisions

| Item | Choice | Why |
|---|---|---|
| Implicit grab | Held while **any** user-seat button is down; ends on the last release, which re-targets at once | Smithay's click grab, which Niri uses: it ends when `current_pressed()` is empty and restores focus. Mango ends on any release (`input/pointer.c` button handler), which drops the grab under a two-button chord. |
| Press over empty space | Focus stays empty until the last release, even if the pointer crosses a window | Niri's click grab has no focus in that case. Mango re-targets because its grab only holds a toplevel. |
| Surface-local coordinates during the grab | Recorded origin of the focused surface (`cursor − (sx, sy)` at the last ordinary motion) | Works the same for toplevels, subsurfaces and popups. Mango subtracts the window geometry, which is wrong for a subsurface. A window moved by layout while a button is held (M6 animations) would need the origin recomputed; nothing moves one today. |
| Drag-and-drop | Not handled | No `start_drag` request is honoured yet. When it is, the drag's own seat grab replaces this. |
| Injected seats | Unchanged: their logical pointer re-targets on every motion | The issue and the report are about the user's pointer. |
| Image over empty space | `default`, set on every motion with no surface under the pointer; the startup image moves from `left_ptr` to `default` too | Mango `pointer.c:1389`, tinywl. `wlr_cursor_set_xcursor` returns at once when the name is unchanged (`types/wlr_cursor.c:657`, 0.20.0), so per-motion calls cost nothing. |
| Observing the image | The compositor records the name it set, or that a client surface replaced it | `struct wlr_cursor_state` is private. The pixels are the live check's job. |
| Root lookup | `viewForSurface` walks to the root surface, then up popup parents to the window, then to the toplevel; `viewAtCursor` uses it | One helper for every seat. `wlr_surface_get_root_surface` stops at a popup, because a popup is not a subsurface. |
| Unmap | A `surface.events.unmap` listener per view: clear any seat's keyboard and pointer focus that resolves to the view, `focusedView`, and the grab | wlroots clears a seat's pointer focus only on surface destroy (`types/seat/wlr_seat_pointer.c:136`). Choosing the next focused window is layout policy, M5. |
| Driving unmap in the test | `wlr_surface_unmap` on the mapped terminal | It emits the same signal a null-buffer commit does. No installed client unmaps without destroying its role. |
| Compositor grab | `wlr_seat_pointer_notify_clear_focus` when it starts, re-target after the last release | Niri starts its grabs with `Focus::Clear` and restores focus on release. |
| Test pointer | `fakePointer`: `wlr_pointer_init` in C memory, attached through `handleNewInput` | The `fakeKeyboard` pattern. The nested backend's one pointer is the host's. |

## Tasks

1. **Test pointer.** `internal/wlr/fakepointer.go`: create, absolute move in layout coordinates
   (normalised against `layoutBox`), button through `wlr_pointer_notify_button`, frame, remove. Helpers
   for tests: the user seat's pointer focus, the recorded image name.
2. **Failing tests.** `internal/wlr/pointer_focus_test.go` with the five tests above. Run them against
   the baseline on the laptop and record that each fails for the stated reason.
3. **P3.** `viewForSurface` resolves popups; `viewAtCursor` calls it.
4. **P1 and M4.** Implicit grab in `processCursorMotion`; clear focus in `startPointerGrab`; re-target
   after the last release in `handleCursorButton`.
5. **P2.** Set `default` over empty space and at startup; record the image.
6. **P4.** Unmap listener in `handleNewToplevel`, sharing the clearing code with destroy.
7. **Gate.** Package tests on the laptop, the live check with a `grim` shot, then merge and close
   `sysc-1296`.

## Execution record (2026-10-17)

Branch `fix/pointer-focus` from `edbdf27`. Tasks 1–6 landed as planned.

- **Baseline failures**, laptop, nested: P1 focus `0x0` while dragging out and the release never reached
  the client; P2 image `""` over empty space; P3 popup hit resolved to no view; P4 user keyboard, injected
  keyboard and pointer focus and the grab all survived the unmap; M4 pointer focus kept during the grab.
- **After:** all five pass, and the whole `internal/wlr` package passes nested on the laptop (the two
  `foot` scale subtests skip there, as before).
- **One test-harness fix during execution:** the scene ended the run right after its last step, so the
  client had not read the `leave` the release produced. `pointerScene` now pauses once more before it
  ends.
- **Subsurfaces** are covered by `wlr_surface_get_root_surface`, which `viewForSurface` already used; the
  nested test exercises the popup walk, which is the new code.

**Live check, laptop, real input** (ydotool with the bench's flat-acceleration calibration, kitty inside
the nested binary, a press on kitty's text, 20 motions out into empty nested space, release). Kitty's
`WAYLAND_DEBUG`:

| Binary | press | motions | release | leave |
|---|---|---|---|---|
| `edbdf27` | 1 | 9 | **never** | after the 9th motion, with the button still down |
| this branch | 1 | 21 | 1 | after the release |

`grim -c` shots: with the button held over empty space the image stays kitty's I-beam (the client still
owns the pointer); after the release it is the theme's `default` arrow.

**Not shown live:** kitty drew no selection highlight and no typed text. The compositor never sends
`wl_surface.frame` done events (`wlr_scene_output_send_frame_done` is never called), so a client that
paces itself on frame callbacks stops repainting after its first frames. Kitty received 9 `done` events
in a minute while the host frame loop ran 3,750 times. That bug predates this branch and is tracked on
its own as `sysc-1299`.
