# Column width and window height — plan and record

Milestone: the niri sizing family. Date: 2026-11-03. Status: implemented, awaiting the laptop gate.

## Why

The shipped default config binds thirteen actions that the compositor accepted but never applied:
`maximize-column`, `maximize-window-to-edges`, `center-column`, `center-visible-columns`,
`expand-column-to-available-width`, `set-column-width`, `switch-preset-column-width`,
`switch-preset-column-width-back`, `set-window-height`, `reset-window-height`,
`switch-preset-window-height`, `switch-preset-window-height-back`. A bind whose action is not
applied does nothing at all, so `Mod+F` — Niri's *maximize column* — did nothing in sysc-compositor,
and so did the rest of the family. The hotkey overlay even advertised *Maximize Column* and
*Switch Preset Column Widths*.

The cause was narrow: `internal/layout` kept a width per column and a height per window, but
exposed no way to change either. The layout's public API offered `SetGeometry(bounds, columnWidth,
gap)` and nothing else.

This milestone gives the layout that API and wires the family through the bind, IPC and CLI
paths, so the thirteen binds do what Niri's do.

## What niri does, and what sysc-compositor does now

Niri stores sizes as proportions of the working area; sysc-compositor stores pixels. The
percentage arguments are resolved against the workspace bounds, and the presets are
`1/3`, `1/2` and `2/3` of the working width or height, as Niri's are.

| Action | Niri | sysc-compositor |
|---|---|---|
| `maximize-column` | toggles the column's full-width flag, keeping the stored width | `ToggleMaximizeColumn` — same toggle, same restore |
| `maximize-window-to-edges` | toggles: the column takes the full width and the window fills the column height | `ToggleMaximizeWindowToEdges` — the column goes full width, the window fills the height, the other windows of the column are hidden |
| `center-column` | scrolls the view so the focused column is centred | `CenterColumn` — the view offset is the column's left edge minus half the free space |
| `center-visible-columns` | centres the group of columns that are fully visible | `CenterVisibleColumns` — the same set, the same arithmetic |
| `expand-column-to-available-width` | grows the focused column into the space the other visible columns do not use; toggles full width when it is the only visible column | `ExpandColumnToAvailableWidth` — same |
| `set-column-width` | `SetProportion`, `AdjustProportion`, `SetFixed`, `AdjustFixed` | `SetColumnWidth` — same four shapes, parsed from one argument |
| `switch-preset-column-width` / `-back` | cycles the presets, forward or back | `SwitchPresetColumnWidth` — same cycle, with Niri's one-pixel slack |
| `set-window-height` | the same four shapes, against the working height | `SetWindowHeight` — same |
| `reset-window-height` | the window goes back to its automatic height | `ResetWindowHeight` — same |
| `switch-preset-window-height` / `-back` | cycles the height presets | `SwitchPresetWindowHeight` — same |

A size argument reads as Niri reads it: a leading `+` or `-` makes the change relative, a number
with a `%` is a percentage of the working area, and a bare number is a pixel size. So `50%` sets
half the working width, `-10%` takes ten points off it, and `400` sets 400 pixels.

### Deliberate differences

- **Proportions are a plain share of the working area.** Niri computes a column width as
  `(working_w - gap) * p - gap`, because its layout keeps the gap inside the column. sysc-compositor
  keeps the gap outside, so the share is taken of the whole working width. The two agree to within
  one gap.
- **`set-column-width` with a fixed size gives the focused window that width**, as in Niri, where a
  fixed size applies to the active window of the column. The column follows the window.
- **Setting a width clears the full-width flag, the preset index and the maximized window**, as
  Niri's `set_column_width` does.

## The code

- `internal/layout/sizing.go` — new. `SizeChange`, `ParseSizeChange`, the preset lists and the ten
  `Workspace` methods above, with the helpers `resolveSize`, `presetIndex`, `presetPixels`,
  `focusedTile` and `visibleColumns`.
- `internal/layout/layout.go` — the column keeps `fullWidth`, `presetWidth` and `presetWidthIdx`;
  the window keeps `maximized`, `presetHeight` and `presetHeightIdx`. `columnWidthAt` returns the
  full working width for a full-width or maximized column, `defaultTileHeight` gives a maximized
  window the whole column height, `tileBaseY` pushes the other windows of a maximized column out
  of the working area, and `placementAt` hides them.
- `internal/bind/bind.go` — the twelve new action names, their argument parsing and their entry in
  `IsSupportedAction`. `Action` carries the parsed `SizeChange`.
- `internal/wm/model.go` — the wrappers that run an action on the focused output's active workspace
  strip, and the window-targeted ones, which find the strip that holds a given window.
- `internal/wlr/sizing.go` — new. The compositor-level methods the IPC path calls, each of which
  applies the model and tells the clients that the window state changed.
- `internal/wlr/keyboard.go` — the bind cases.
- `internal/ipc/wire.go` — the IPC actions. Niri's `SizeChange` has no serde rename, so the wire
  names are `SetFixed`, `SetProportion`, `AdjustFixed` and `AdjustProportion`, and the actions are
  `{"change": …}`, `{"id": …}` or `{}` as niri-ipc has them.
- `internal/server/niri.go` and `internal/server/server.go` — the action router, which no longer
  answers these names with *unimplemented*.
- `internal/config/default-config.kdl` — `Ctrl+F { fullscreen-window; }` now sits beside
  `Mod+Shift+F`, so the fullscreen shortcut works out of the box.

## The tests

- `internal/layout/sizing_test.go` — the parsing, the presets, the toggles, the centring and the
  height split, plus `TestScrollingUsesContentCoordinatesNotOutputCoordinates`, which holds the
  scrolling arithmetic to the content coordinates the layout actually uses.
- `internal/bind/sizing_test.go` — the action names parse, the two size actions reject a missing
  size, and all of them are supported.
- `internal/ipc/sizing_test.go` — the wire shapes round-trip, and a malformed field is rejected.
- `internal/config/firstrun_test.go` — the shipped default config must leave no unapplied action.

The compositor packages are covered by CI lane 2, which runs them under a virtual KMS.

## What is still open

- The laptop gate: the fixed binary has to be built from the input branch with these changes and
  installed on the laptop, where the owner checks the family against Niri on the same hardware.
- `set-window-width`, `center-window`, `pick-window`, `pick-color` and the debug actions are still
  unimplemented, and the default config binds none of them.
- The window height family does not save a height per window across a remap yet.
