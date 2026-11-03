# Niri config surface: `window-rule`, `layer-rule` and `workspace.layout`

- Date: 2026-11-03
- Bead: `sysc-2231`
- Branch: `feat/config-surface-20261103`
- Status: slice 1 landed on 2026-11-03 (commit `3bd4e41`, draft PR #94). Slices
  2 to 6 are not started.
- Reference: niri v26.04 in `/home/nomadx/references/niri`
  (`niri-config/src/window_rule.rs`, `layer_rule.rs`, `workspace.rs`, `layout.rs`,
  `docs/wiki/Configuration:-Window-Rules.md`, `Configuration:-Layer-Rules.md`,
  `Fullscreen-and-Maximize.md`)

## 1. The premise, corrected

The handover and the audit said that `workspace.layout`, `window-rule`,
`window-rule.match`, `layer-rule` and `layer-rule.match` are "parsed and then
ignored", and that the shipped default's Firefox picture-in-picture rule "does
nothing". **That was wrong.** Checked on 2026-11-03:

- `sysc-compositor validate -c internal/config/default-config.kdl` prints
  **no warning at all**, so the shipped default's `window-rule` is parsed and
  applied — that rule opens the Firefox picture-in-picture window floating
  (`internal/wlr/windows.go`, `mapWindowWithRules`).
- A config with `workspace { layout { gaps 8 } }` warns exactly
  `workspace.layout`. A `window-rule { match is-active=true }` warns exactly
  `window-rule.match`. A `layer-rule` warns nothing, because the whole
  `layer-rule` block is supported.

So the real gap is not the sections. It is:

1. the Niri keys that sysc has **no counterpart for**, and
2. the match properties that sysc **cannot evaluate**, so the whole rule is
   dropped (an unsupported property inside one `match` line makes that line
   fail, and a rule with a failing match never applies).

This is user-visible: on the archThink the owner's own config (the Niri
default, 461 lines) makes the compositor print **fifteen** warnings at every
start, one per block that holds something unsupported — `config-notification`,
`output`, `layout.background-color`, `layout.center-focused-column`,
`layout.preset-column-widths`, `layout.border`, `layout.struts`,
`overview.workspace-shadow`, `debug`, `animations.window-open.easing`,
`animations.window-close.easing`, `animations.config-notification-open-close`,
`animations.screenshot-ui-open`, `window-rule` and `window-rule.match`. The
warning names the block, not the whole feature: the supported keys inside
`output` or `window-rule` still apply.

This plan covers the rule and layout surface only. No bar, no lock screen, no
wallpaper engine, no notification server, no tray.

## 2. What sysc does today

### 2.1 `window-rule` — supported and applied

| key | parsed in | applied in | notes |
|---|---|---|---|
| `match` / `exclude` | `internal/config/rules.go` `parseWindowMatch` | `ResolveWindowRules` | `app-id` and `title` only; several `match` lines in one block are ORed |
| `open-on-output` | `rules.go` | `internal/wlr/windows.go` `mapWindowWithRules` | |
| `open-on-workspace` | `rules.go` | `mapWindowWithRules` | by name |
| `open-floating` | `rules.go` | `mapWindowWithRules` → `model.SetWindowFloating` | |
| `open-fullscreen` | `rules.go` | `mapWindowWithRules` → `c.SetFullscreen` | |
| `default-column-width` | `rules.go` `parsePresetSize` | `mapWindowWithRules` → `model.ResizeWindow` | `fixed`, `proportion` or empty |
| `default-window-height` | `rules.go` `parsePresetSize` | `mapWindowWithRules` → `model.ResizeWindow` | `fixed`, `proportion` or empty |
| `geometry-corner-radius` | `rules.go` `parseCornerRadius` | `internal/wlr/appearance.go` | |
| `clip-to-geometry` | `rules.go` | `internal/wlr/appearance.go` | |
| `block-out-from` | `rules.go` | `internal/wlr/capture_privacy.go` | `screen-capture` or `screencast` |
| `open-maximized` | `rules.go` | `internal/wlr/windows.go` `mapWindowWithRules` → `model.MaximizeColumnFor` | slice 1 |
| `open-maximized-to-edges` | `rules.go` | `mapWindowWithRules` → `model.MaximizeWindowToEdgesFor` | slice 1 |
| `open-focused` | `rules.go` | `internal/wlr/input.go` (the map path) | slice 1; `false` keeps the focus |
| `min-width`, `max-width`, `min-height`, `max-height` | `rules.go` | `internal/wlr/workspaces.go` `windowSizeHints` | slice 1; amend the client's hints |
| `default-column-display` | `rules.go` | `mapWindowWithRules` → `model.SetColumnDisplayFor` | slice 1; `normal` or `tabbed` |
| `default-floating-position` | `rules.go` `parseFloatingPosition` | `mapWindowWithRules` → `model.SetFloatingPositionFor` | slice 1; `x`, `y`, `relative-to` |

### 2.2 `window-rule` — the keys sysc has no counterpart for

| Niri key | Niri's meaning | what sysc needs | slice |
|---|---|---|---|
| `scroll-factor` | a scroll factor for the events sent to that window | the scroll path has to look up the window under the pointer | 4 |
| `opacity` | the window is drawn with an alpha | per-window alpha in the renderer | 5 |
| `border`, `focus-ring`, `shadow`, `tab-indicator` | per-window decoration overrides | the decoration code reads one global `layoutOptions` | 5 |
| `draw-border-with-background` | draw the border over the background | decoration code | 5 |
| `background-effect` | a per-window background effect (blur) | the effect is global today | 5 |
| `popups` | rules for the popup surfaces of the window | sysc does not model popups | 6 |
| `variable-refresh-rate` | the output runs at a variable refresh rate | output-level, needs the wlroots commit path | 6 |
| `baba-is-float`, `tiled-state` | the Baba Is You game's quirks | niche, not for this tranche | 6 |

### 2.3 `window-rule` — the match properties sysc cannot evaluate

| property | Niri's meaning | the state sysc would need | slice |
|---|---|---|---|
| `is-focused` | the one window with the keyboard focus | the model has exactly one focused window, but the rule has to be re-read whenever the focus moves | 2 |
| `is-floating` | matches floating windows; Niri applies it only after the window is open | the tile's floating state, read right after the window opens | 2 |
| `is-urgent` | the window asked for attention | the model already carries the urgency (`model.SetWindowUrgent`) | 2 |
| `is-active-in-column` | the window last focused in its column; true while the window opens | the column's active tile, read right after the window opens | 2 |
| `at-startup` | matches during the first 60 seconds after the compositor started | the compositor's start time | 2 |
| `is-active` | every workspace on the focused monitor has one active window | sysc only models one focused window; the active window per workspace does not exist yet | 3 |
| `is-window-cast-target` | the window is the target of an individual screencast | sysc knows about screencasts but does not track the target window | 3 |

### 2.4 `layer-rule`

Supported today: `match` / `exclude` on `namespace`, and
`place-within-backdrop` (applied in `internal/wlr/layer.go`). Missing:
`opacity`, `block-out-from`, `shadow`, `geometry-corner-radius`,
`background-effect`, `popups`, `baba-is-float`, and the match properties
`at-startup` and `layer`. Slice 5.

### 2.5 `workspace` and the `layout` section

`workspace` supports `open-on-output`. `workspace.layout` is Niri's
per-workspace layout override (a `LayoutPart`) and is the one whole section
that warns and does nothing.

The `layout` section supports `gaps`, `default-column-width`, `insert-hint`,
`focus-ring` and `shadow`. Missing from Niri's `LayoutPart`: `border`,
`tab-indicator`, `preset-column-widths`, `preset-window-heights`,
`center-focused-column`, `always-center-single-column`,
`empty-workspace-above-first`, `default-column-display`, `struts` and
`background-color`. The layout is per output today
(`model.SetOutputGeometry(name, bounds, columnWidth, gap)`), so a
per-workspace override needs new plumbing. Slice 4.

## 3. The slices

The tranche is milestone-sized, so it is cut into slices that each land on
their own. A slice is done when its keys parse, are applied, are covered by
tests and no longer warn.

### Slice 1 — the window-rule keys that map onto machinery that exists

`open-maximized`, `open-maximized-to-edges`, `open-focused`,
`min-width`/`max-width`/`min-height`/`max-height`,
`default-column-display`, `default-floating-position`.

**Landed on 2026-11-03** (commit `3bd4e41`, draft PR #94). What actually
shipped, against the sketch below:

- The layout gained set-variants, named for what they do rather than for the
  toggle they replace: `MaximizeColumn(id)`, `MaximizeWindowToEdges(id)`,
  `SetColumnDisplayFor(id, display)` and `SetFloatingPosition(id, position)`
  in `internal/layout`, wrapped by the model as `MaximizeColumnFor`,
  `MaximizeWindowToEdgesFor`, `SetColumnDisplayFor` and
  `SetFloatingPositionFor`. The toggles stay as they are.
- `open-focused` changes the map path in `internal/wlr/input.go`, where a new
  window is focused unconditionally ("Niri activates newly mapped windows").
  A rule with `open-focused false` keeps the focus where it was; `true`
  focuses the new window even when the client did not ask for it. Because the
  layout focuses the new column when the window is added, the map path takes
  the focused window before the map and gives it back when the rule says the
  new window should not have the focus.
- The size hints are amended in `syncWorkspaceWindowSizeHints` through the new
  `windowSizeHints(view)` helper: the rule's minimum raises the client's
  minimum, the rule's maximum lowers the client's maximum, and a rule's
  maximum replaces an absent client maximum (Niri's rule).
- `default-column-display` applies the display mode to the column the window
  created, not to the focused column.
- `default-floating-position` positions a floating window relative to the
  working area (top-left, top-right, bottom-left, bottom-right, top, bottom,
  left, right), with Niri's maths: a corner offsets from the corner, a side
  centres the window on that side. A window that is tiled when the rule
  applies keeps the position for the day it floats.

### Slice 2 — the match properties that can be read when a window maps

`is-focused`, `is-floating`, `is-urgent`, `is-active-in-column`, `at-startup`.

These need the resolved rule to be evaluated against the window's state at
map time, and the rules that depend on that state to be re-read when the
state moves (a window that becomes urgent, a column that changes focus).

### Slice 3 — the match properties that need a new state

`is-active` (one active window per workspace on the focused monitor) and
`is-window-cast-target`. Both need state sysc does not keep yet.

### Slice 4 — `workspace.layout` and the missing `layout` keys

`workspace.layout` as a per-workspace `LayoutPart`: `gaps`,
`default-column-width`, `preset-column-widths`, `preset-window-heights`,
`default-column-display`, `center-focused-column`, `struts`, `border`,
`tab-indicator`, `focus-ring`, `shadow`, `background-color`. The layout has
to read the gaps and the column width from the workspace instead of from the
output. `scroll-factor` per window rides along here.

### Slice 5 — the appearance properties

`opacity`, `border`, `focus-ring`, `shadow`, `tab-indicator`,
`draw-border-with-background`, `background-effect`, and `layer-rule`'s
`opacity`, `shadow`, `geometry-corner-radius`, `background-effect`,
`block-out-from`. These are per-window overrides of what is one global
setting today, so they touch the decoration and the renderer.

### Slice 6 — the leftovers

`popups`, `variable-refresh-rate`, `baba-is-float`, `tiled-state`,
`layer-rule.match`'s `at-startup` and `layer`. Some of these may end up
rejected with a clear message rather than accepted and ignored.

## 4. Rules

- The compositor never ignores a rule silently. A key that is parsed but not
  applied keeps its warning, and the warning names the key.
- A rule that cannot be evaluated is still a rule that is parsed: the warning
  names the block, as it does today.
- Niri's semantics win. Where sysc's model differs from Niri's, the plan says
  so in the table above rather than quietly picking one.
- Nothing in this tranche touches the bar, the lock screen or the shell.

## 5. Tests

- `internal/config`: one test per key that it parses and lands in the
  resolved rule result, and one test that an unsupported match property still
  drops the rule and still warns. Slice 1 added
  `TestNiriWindowRuleOpenKeysAreParsedAndResolved` and
  `TestNiriWindowRuleOpenKeysRejectBadValues`.
- `internal/layout` and `internal/wm`: one test per set-variant, next to the
  toggle tests. Slice 1 added `TestMaximizeColumnFillsTheWorkingWidth`,
  `TestMaximizeWindowToEdgesFillsTheColumnHeight`,
  `TestSetColumnDisplayForSetsTheColumnOfTheWindow`,
  `TestSetFloatingPositionPlacesTheWindow` and
  `TestSetFloatingPositionKeepsThePositionForALaterFloat`.
- `internal/wlr`: one test per key that the compositor applies it, next to the
  existing rule tests. These need a compositor, so they run in CI lane 2
  under vkms, not locally. Slice 1 added `internal/wlr/window_rules_test.go`
  with `TestWindowRuleOpenMaximizedFillsTheWorkingWidth`,
  `TestWindowRuleOpenMaximizedToEdgesFillsTheColumn`,
  `TestWindowRuleOpenFocusedFalseKeepsTheFocus` and
  `TestWindowRuleMinWidthAmendsTheSizeHints`.
- `internal/cli`: `validate` on a config that uses the new keys must print no
  warning. Slice 1 covered this in the config test instead: a config that
  uses the new keys parses without a warning.

## 6. Docs

- This plan gets a row in `docs/README.md` in the same commit.
- `docs/plans/niri-parity-matrix.md` gets the rows for the keys that land.
- `docs/plans/2026-11-03-touch-handover.md` keeps pointing at this plan for
  the config-surface item until the tranche is done.
