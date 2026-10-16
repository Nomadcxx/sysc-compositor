# Input, seat, focus and move/resize: Niri and Mango compared with M0

Kind: report. bd `sysc-1247`. Written 2026-10-16 against sysc-compositor `origin/main` `1eb9c2c`.

The question is which input patterns in the two pinned references this compositor should adopt, which do
not apply, and which wlroots or our code already covers. Each finding names the milestone that owns it.
Any code work is tracked in bd, not here.

## Sources

| Source | Pin | Read for |
|---|---|---|
| Niri | `v26.04`, `8ed0da4` | Behaviour: what a user sees (`src/input/mod.rs`, `src/backend/mod.rs`, `niri-config/src/input.rs`, `src/cursor.rs`) |
| mangowm | `0.18.0`, `a21b10f` | wlroots 0.20 calls, the same substrate as ours (`src/input/`, `src/manage/`, `src/main.c`) |
| wlroots | `0.20.0` tag, installed `0.20.2` | Behaviour of the nested Wayland backend (`backend/wayland/seat.c`) |
| Ours | `internal/wlr/input.go`, `internal/wlr/xdg.go`, `internal/wlr/seat.go` | M0 as merged |

Hyprland was not consulted. Mango already answers every wlroots-level question here on our exact wlroots
version, and Niri is the behaviour target. Hyprland would add a third opinion, not evidence.

## Findings

Status: **apply** (adopt; the milestone named owns it), **covered** (wlroots or our code already does it),
or **n/a** (does not fit this product).

### Keyboard

**K1. User-seat keyboards have no xkb keymap.** Status: apply, before M2.
- wlroots never gives a backend keyboard a keymap. The nested backend closes the host's keymap fd unread
  (`backend/wayland/seat.c:21`), and libinput keyboards start with none.
- `handleNewInput` (`input.go:174`) hands the keyboard to the seat without a keymap, so clients on the user
  seat receive keycodes they cannot translate.
- The injected seat already compiles a fixed US keymap (`seat.go:31`), so the pattern exists in-tree.
- Mango compiles one keymap from its config and sets it on every keyboard (`input/keyboard.c:138`).
- Niri takes `input { keyboard { xkb {…} repeat-delay 600 repeat-rate 25 } }` (`niri-config/src/input.rs:101`).
- M2's gate types a password into sysc-greet, so this must land first. Use the default rules (or
  `XKB_DEFAULT_*`) until M5's KDL supplies `xkb`.
- This is read from source; M0's gate never typed into a client.

**K2. Only the most recently attached keyboard is heard.** Status: apply, before M2.
- `handleKeyboardKey` and `handleKeyboardModifiers` return early for any keyboard other than
  `activeKeyboard` (`input.go:267`, `:280`).
- `activeKeyboard` is the last one added. On a VT, libinput presents several keyboard devices (power button,
  video bus, consumer control, the keyboard itself), so the real keyboard can be the one ignored.
- Mango puts every keyboard into one `wlr_keyboard_group` (`main.c:869`, `input/keyboard.c:143`). Smithay
  gives Niri one logical keyboard per seat.
- Adopt the group for the user seat. The injected seat keeps its own in-process keyboard (D2.4.1), outside
  the group.

**K3. Escape quits the compositor.** Status: apply, before M2.
- This is an M0 test affordance (`input.go:271`). On a greetd VT, Escape would end the greeter session.
- M2's Quit IPC replaces it.

**K4. Key repeat for compositor binds.** Status: apply at M10 (design §15 "niri-like key repeat").
- Mango repeats bound actions on its own timer (`input/keyboard.c`, `keyboard_repeat`). Clients repeat
  for themselves from `repeat_info`.
- The design forbids compositor auto-repeat to layer-shell clients (§7, "Keyboard to layer-shell").
- K1's `wlr_keyboard_set_repeat_info` is the only part needed earlier.

**K5. Per-device keymap rules.** Status: n/a. Mango has standalone device rules; Niri has none.

### Pointer

**P1. Pointer focus does not stay on the pressed surface.** Status: apply, before M4.
- `processCursorMotion` re-targets focus to whatever surface is under the cursor on every motion
  (`input.go:301`), even with a button held. A text selection dragged out of a window loses its motion
  events, and the release reaches another client.
- Mango keeps focus on the pressed surface while `CurPressed` (`input/pointer.c:1295`). Smithay gives
  Niri the same implicit grab.

**P2. The cursor image is not reset over empty space.** Status: apply, before M4.
- With no surface under the pointer we clear focus (`input.go:315`) but keep the client's last image, so
  a text I-beam can linger over the desktop.
- Mango sets `default` there (`input/pointer.c:1389`), as tinywl does.

**P3. Clicks on a subsurface or popup do not resolve to their window.** Status: apply, before M4.
- `viewAtCursor` calls `wlr_xdg_toplevel_try_from_wlr_surface` on the hit surface (`input.go:411`).
- A click on a subsurface (video, GTK popovers) or a popup therefore neither focuses nor starts Mod+drag.
- Mango walks to `wlr_surface_get_root_surface` and then up popup parents (`manage/client.c:70`).

**P4. Unmap releases nothing.** Status: apply, before M4.
- Only `destroy` clears focus and the grab (`input.go:583`). A window that unmaps and stays alive (a
  client hiding itself) keeps keyboard focus and any grab.
- Niri and Mango refocus on unmap. Which window receives focus is layout policy (M5); clearing is not.

**P5. Relative pointer, pointer constraints, cursor-shape.** Status: apply at their milestone.
- All are in the design's global set (§7).
- Mango sends relative motion beside every `wlr_cursor_move` (`input/pointer.c:1192`) and confines in the
  motion handler.
- cursor-shape is M10. Constraints and relative pointer have no milestone in §15; they are noted on M10's
  issue, beside cursor-shape, for its plan to place.

**P6. Pointer-device ownership.** Status: covered.
- `wlr_cursor_attach_input_device` with the seat fed through `wlr_seat_pointer_notify_*` is exactly
  Mango's shape.
- The asymmetry with keyboards is already documented in `handleNewInput`.

### Move and resize

**M1. Mod key: Super on a TTY, Alt when nested.** Status: apply at M5.
- Niri chooses by backend (`src/backend/mod.rs:99`) and honours `mod-key` / `mod-key-nested`.
- We hard-code Super (`input.go:354`). Nested under the host's Niri, Super+drag belongs to the host first.
- M5 brings binds from KDL; the backend-dependent default belongs there.

**M2. Mod+RMB resizes from the edges under the pointer.** Status: apply at M5/M6.
- Niri picks edges from where in the window the press lands (`src/input/mod.rs:2923`).
- We always resize right+bottom (`input.go:363`). Niri's double-click full-width and height reset are
  layout actions (M6).

**M3. Grab cursor images.** Status: apply at M5.
- Niri shows `grabbing` for a move and the edge icon for a resize (`src/input/mod.rs:2880`). Mango does the
  same through `resize_cursor_name`.

**M4. Grab start clears pointer focus.** Status: apply with P1.
- Niri starts compositor grabs with `Focus::Clear`, so the window under a Mod+drag gets a leave rather than
  a frozen enter. We stop sending motion but leave focus set.

**M5. Top/left resize is not synchronised with the client's commit.** Status: apply at M6 (floating).
- During a resize we move the tree on every motion (`input.go:304`) before the client commits the matching
  size, so the far edge jitters.
- Mango throttles floating resize (`drag_floating_refresh_interval`). Niri positions on the commit that
  acknowledges the configure.

**M6. Client move/resize serial validation.** Status: covered.
- `handleToplevelMove` / `Resize` validate the grab serial against a still-pressed button. This is
  stricter than tinywl and matches what both references require.

### xdg-shell popups

**X1. Popups are created at the scene root and never unconstrained.** Status: apply at M1.
- `ensureSceneTree` creates every xdg surface, popups included, as a child of the scene root
  (`xdg.go:37`). A menu therefore draws at the layout origin, not beside its window, and can run off the
  output.
- Mango creates the popup tree under its parent's tree on the popup's initial commit (`manage/layer.c:448`).
  It then calls `wlr_xdg_popup_unconstrain_from_box` against the usable area of the output (`:418`).
- M1's xdg-shell exit gate is where this belongs; layer-shell popups need the same parenting.

### Cursor theme and scale

**C1. Cursor theme and size are fixed.** Status: apply at M5 (config) and M15 (mixed DPI).
- `wlr_xcursor_manager_create(NULL, 24)` with scale 1 (`input.go:19`, `:33`).
- Niri reads `cursor { xcursor-theme xcursor-size }` and exports `XCURSOR_THEME` / `XCURSOR_SIZE` to
  children (`src/cursor.rs:191`).
- Per-output scale needs `wlr_xcursor_manager_load` at each output scale. That arrives with fractional
  scale (M1) for correctness and mixed DPI (M15) for completeness.

### Not taken from Mango

- Tags, layouts, scroller focus locks, hot areas, group bars and `mmsg` are Mango product, not input
  substrate (orchestration "What not to schedule").
- Niri's overview grabs (Mod+MMB view-offset, RMB in the overview) belong to M7.

## Follow-up

The items above that need code before a later milestone are filed in bd from `sysc-1247`, gating the
milestone named:
- K1–K3: keyboard group and keymap, Escape removed: `sysc-1295`, gates M2 (`sysc-1267`).
- P1–P4 plus M4: pointer focus correctness: `sysc-1296`, gates M4 (`sysc-1269`).
- X1: popup parenting and unconstraining: `sysc-1297`, gates M1 (`sysc-1266`).

Items owned by a milestone's own scope (K4, P5, M1–M3, M5, C1) are noted on that milestone's issue (M1 `sysc-1266`,
M5 `sysc-1270`, M6 `sysc-1271`, M10 `sysc-1275`), so its plan picks them up when it is written.
