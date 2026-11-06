# Niri config surface: `window-rule`, `layer-rule` and `workspace.layout`

- Date: 2026-11-03
- Bead: `sysc-2231`
- Branch: `feat/config-surface-20261103`
- Execution snapshot: slices 1–5 are implemented on draft PR #94 (5b and 5c
  on 2026-11-06); slice 6 remains. See the execution records below and the
  [2026-11-05 continuation plan](2026-11-05-handover.md). bd `sysc-2231`
  owns current status; implemented on this branch does not mean merged.
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
| `match is-focused`, `is-urgent`, `is-active-in-column`, `is-floating`, `at-startup` | `rules.go` `parseWindowMatch` | `internal/wlr/window_rules.go` `ruleState` → `ResolveWindowRules` | slice 2; the rules are matched against the state of the window they apply to |
| `match is-active` | `rules.go` `parseWindowMatch` | `internal/wlr/window_rules.go` `ruleState` | slice 3; the active window of every workspace on the focused output |
| `match is-window-cast-target` | `rules.go` `parseWindowMatch` | `internal/wlr/window_rules.go` `ruleState` | slice 3; the compositor has no window screencast protocol, so no window is ever the target |

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

### 2.3 `window-rule` — the match properties

Slices 2 and 3 took every match property Niri has out of this table:
`is-focused`, `is-urgent`, `is-active-in-column`, `is-floating`,
`at-startup`, `is-active` and `is-window-cast-target` are parsed and
evaluated now (see 2.1). No window rule warns about a match property any
more, and an unknown match property is a hard error.

### 2.4 `layer-rule`

Supported today: `match` / `exclude` on `namespace`, and
`place-within-backdrop` (applied in `internal/wlr/layer.go`). Missing:
`opacity`, `block-out-from`, `shadow`, `geometry-corner-radius`,
`background-effect`, `popups`, `baba-is-float`, and the match properties
`at-startup` and `layer`. Slice 5.

### 2.5 `workspace` and the `layout` section

`workspace` supports `open-on-output`. `workspace.layout` is Niri's
per-workspace layout override (a `LayoutPart`). Slice 4a made `gaps` and
`default-column-width` work per workspace; the other keys of the layout part
warn by name (`workspace.layout.<key>`).

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

**Landed on 2026-11-03** (commit `79b939d`, draft PR #94). What actually
shipped:

- The rules are matched against the state of the window they apply to, the way
  Niri does. `internal/config/rules.go` gained a `WindowState` and
  `UnmappedWindowState(atStartup)`: a window that is not on screen yet is not
  focused, not urgent and not floating, and it is the active window of its
  column. That is what Niri reads for a window that is still unmapped, so
  `match is-active-in-column=true` applies to a window that opens and
  `match is-focused=true` does not.
- `internal/wlr/window_rules.go` reads the state of a mapped window: the
  urgency and the floating state from the model's snapshot, the focus from the
  seat, and the startup flag from the compositor.
- The rules are read again when the state moves: `focusView`, `SetWindowUrgent`,
  `ToggleWindowUrgent` and `SetWindowFloating` call `reapplyWindowRules`, which
  resolves the rules for every view against the current state.
- The startup window is a clock of its own: `internal/wlr/startup.go` arms a
  timer for the first sixty seconds and ends the window when it runs out, so
  an `at-startup` rule stops matching and the rules are read again.
- `is-active` and `is-window-cast-target` still warn `window-rule.match` and
  still drop the whole rule: they need the state of slice 3.

### Slice 3 — the match properties that need a new state

`is-active` (one active window per workspace on the focused monitor) and
`is-window-cast-target`.

**Landed on 2026-11-03** (commit `99a6c7e`, draft PR #94). What actually
shipped:

- Niri's `is-active` is the window that has the active border: every
  workspace on the focused monitor has one active window. The model's
  snapshot carries the active window of every workspace
  (`WorkspaceSnapshot.ActiveWindowID`), so `ruleState` compares it with the
  focused output.
- The rules are read again when the focused output moves
  (`setActiveOutputName`) and when a window leaves its workspace (the unmap
  and destroy paths), because both change which windows are active.
- Niri's `is-window-cast-target` is the window an ongoing window screencast
  shows. The compositor has no window screencast protocol, so no window is
  ever the target of one: the property is evaluated, and it is always false.
- With this, every match property Niri has is parsed and evaluated. The
  `Unsupported` flag in `RuleMatch` stays as the guard for a property that
  cannot be evaluated yet, but nothing sets it now; an unknown match
  property is a hard error.

### Slice 4 — `workspace.layout` and the missing `layout` keys

`workspace.layout` as a per-workspace `LayoutPart`: `gaps`,
`default-column-width`, `preset-column-widths`, `preset-window-heights`,
`default-column-display`, `center-focused-column`, `struts`, `border`,
`tab-indicator`, `focus-ring`, `shadow`, `background-color`. The layout has
to read the gaps and the column width from the workspace instead of from the
output. `scroll-factor` per window rides along here.

#### Slice 4a — the gaps and the column width per workspace (landed)

**Landed on 2026-11-03** (commit `c817344`, draft PR #94).

- `internal/config/rules.go` gained `WorkspaceLayout{Gaps, ColumnWidthProportion,
  FixedColumnWidth}` and the parser for the `layout` child of a workspace. The
  keys the compositor cannot apply warn by name (`workspace.layout.<key>`), so
  `workspace.layout` itself no longer warns.
- `internal/wm/model.go` gained the same layout on `WorkspaceConfig`. The model
  merges it over the values of the output in `workspaceGeometry`: a workspace
  that sets the gaps uses them, and a workspace that sets the column width
  gets it from its proportion or its fixed size. The merge happens when the
  workspace is created, when the config changes, and whenever the output is
  resized.
- `internal/layout/sizing.go` gained `ColumnWidthPixels`, the one place that
  turns a share of the available width into pixels; the compositor and the
  model use it.
- `internal/server/config_reload.go` carries the layout from the config into
  the model options.

**Where sysc differs from Niri:** a workspace that sets only the gaps keeps
the column width in pixels that the output computed with the output's gaps.
Niri recomputes the width from the share and the new gaps. Making that exact
would mean the model would need the share of the output, not only its pixels;
the model takes the width in pixels today, and the change would touch every
caller of `model.AddOutput` and `model.SetOutputGeometry`.

**Still open in slice 4:** `preset-column-widths`, `preset-window-heights` and
`default-column-display` (the layout has the presets and the column display
as fixed values today), `center-focused-column`, `struts`, and the appearance
keys of the layout part, which belong to slice 5. `scroll-factor` per window
is still open.

#### Slice 4b — the preset sizes, the column display, the centering, the struts and the scroll factor (landed)

**Landed on 2026-11-04** (commits `f471fc7`, `a78b4c2`, `069a2fc`, `4e757a7`,
draft PR #94). This closes everything slice 4 listed as still open except the
appearance keys, which remain slice 5's.

- `preset-column-widths`, `preset-window-heights` and `default-column-display`
  (`f471fc7`) parse in the `layout` section and in a workspace's `layout` part.
  A nil preset list keeps the config's list and an empty one resets to Niri's
  default thirds, per workspace over config-wide; the same for the display.
  Preset cycles mix `fixed` and `proportion` entries: each preset resolves to
  pixels before the cycle compares it with the current size. A column a
  workspace creates fresh starts in that workspace's default display; columns
  that move whole keep their own.
- `center-focused-column` (`a78b4c2`) parses in both places. `never` keeps the
  scroll-into-view, `always` centers the focused column on every reveal, and
  `on-overflow` centers it only when the focused column and the one focused
  before it do not fit on screen together — Niri's rule. Focus changes carry
  the previous column into the reveal; window adds, moves and geometry changes
  keep the fit behavior, matching the places in Niri that pass no previous
  column.
- `struts` (`069a2fc`) parse in both places as a block with `left`, `right`,
  `top` and `bottom`, each from -65535 to 65535; a negative edge widens the
  area past the output. `layout.Struts.Inset` cuts the strip's bounds inside
  the usable area — the area layer-shell exclusive zones already left — and
  the model re-applies it when the config-wide value changes or a workspace's
  override is merged. A workspace without a `struts` block keeps the config's;
  one with a block replaces the whole struct, as Niri's `merge_clone` does.
  `MoveWorkspaceToOutput` now routes through `applyWorkspaceLayout`, so a
  workspace that moves to another output gets its gaps, its column width and
  its struts there too — slice 4a's overrides now also apply on output moves.
  The same commit closes the ordering gap the handover named: a workspace
  created after the config was read — a hotplug, a new workspace — starts from
  the config-wide preset sizes, display, centering and struts, not from the
  layout's own defaults.
- `scroll-factor` (`4e757a7`) parses as a window rule, greater than zero and
  at most 100 like Niri's `FloatOrInt<0,100>`. The resolved factor rides on
  the window's appearance record, re-resolved on reconfigure, and the pointer
  axis handler multiplies the continuous and the discrete delta by the focused
  window's factor before they reach the client. Scroll binds and the overview
  gesture see the device's own delta, as in Niri.

**Where sysc differs from Niri:** the column width in pixels that an output
computes when the config gives a proportion uses the config-wide struts; a
workspace that overrides the struts keeps that width, the same pixels-over-
share gap slice 4a records for the gaps. Niri also multiplies a per-device
scroll factor from `input.mouse` and `input.touchpad`; sysc scales only the
per-window factor. The discrete delta rounds toward zero after scaling, a
detail Niri's fixed-point path hides. Struts cut the strip's bounds; a
fullscreen window and the overview keep the full usable area, as in Niri.

**Still open in slice 4:** the appearance keys of the layout part — `border`,
`tab-indicator`, `focus-ring`, `shadow`, `background-color` — which belong to
slice 5, and `always-center-single-column`, which still warns.

### Slice 5 — the appearance properties

`opacity`, `border`, `focus-ring`, `shadow`, `tab-indicator`,
`draw-border-with-background`, `background-effect`, and `layer-rule`'s
`opacity`, `shadow`, `geometry-corner-radius`, `background-effect`,
`block-out-from`. These are per-window overrides of what is one global
setting today, so they touch the decoration and the renderer.

#### Slice 5a — window opacity

The window-rule `opacity` key parses as a finite number and resolves with the
last matching rule. Rendering clamps it to 0–1, multiplies it by the opening
animation opacity, and forces fullscreen windows opaque. `syncViewOpacity`
applies the value to every buffer under the window tree; newly mapped xdg popup
trees receive it when wlroots creates them. Resize snapshots copy the buffer
alpha and keep it while the live tree stays enabled for hit testing and frame
callbacks.

Tests: `internal/config/config_test.go`
`TestNiriWindowRuleOpacityIsParsedAndResolved`; `internal/wlr/window_rules_test.go`
`TestWindowRuleOpacityComposesWithOpeningAndResizeSnapshots` and
`TestWindowRuleOpacityTracksUrgencyAndFullscreen` and
`TestWindowRuleOpacityChangesRenderedPixels`; and
`internal/wlr/popup_test.go` `TestPopupDrawsBesideItsParent` also checks popup
buffer opacity. The rendered-pixel test captures the compositor output and
checks the 50% blend, transparent urgency, fullscreen entry and exit, and
rule-removal restoration. The named config test passes locally. Hosted CI run
`42006897511` on `0d4416b` passed both `check` and `compositor`; the compositor
lane ran under vkms. No nested compositor test ran locally.

Known differences: sysc applies full opacity as soon as fullscreen state lands;
Niri interpolates the window-rule opacity toward one during its fullscreen
transition. `popups.opacity` remains unsupported and belongs to slice 6.

#### Slice 5b — decorations

Niri 26.04 parses `focus-ring` and `border` as one partial type (`BorderRule`)
and `shadow` as another (`ShadowRule`), the same in `layout`,
`workspace.layout` and `window-rule`. A part merges field by field: `off` and
`on` override each other, a set colour or number replaces the inherited one,
and an unset field inherits. `layout` parts merge onto the defaults, the
workspace part onto the layout, and every matching window rule onto the
workspace's result, in rule order. So a rule that sets only `active-color`
keeps the width and the other colours.

What the renderer must do, from `src/layout/tile.rs` and `focus_ring.rs`:

- The colour is urgent, then active, then inactive. Active means the active
  window of a workspace on the focused output, for border and ring alike.
- The focus ring is drawn around the active window of each workspace and does
  not take layout space. The border is drawn around every window and does:
  the tile is the window plus the border on each side, so the window is
  configured smaller, its min and max sizes grow by the border, and
  proportional and fixed column widths are tile widths. Fullscreen and
  maximized-to-edges windows have no border. A nonzero width rounds to whole
  physical pixels, at least one.
- `draw-border-with-background` defaults to true for a client-side decorated
  window: the border fills the window's rectangle as well as its edge. The
  focus ring fills only when the border is off.
- The shadow uses `inactive-color` for an inactive window when set. It is cut
  out from under the window unless `draw-behind-window` is true. A shadow
  `offset` sets both axes; a missing axis is zero.

Gradients stay warnings, by key, until implemented.

Parts, each with its own regression:

1. **Config.** One parser per partial type for the three places; the new
   `layout.border`, `urgent-color` and the shadow's `inactive-color` and
   `draw-behind-window`; window-rule `focus-ring`, `border`, `shadow` and
   `draw-border-with-background`; `workspace.layout` `focus-ring`, `border`
   and `shadow`.
2. **Render and layout.** Resolve each window's decorations from the layout,
   its workspace and its rules; draw the border; shrink the window inside its
   tile; apply urgency, inactive shadow colour, background fill, reload and
   removal. Prove the colours and the configured size from rendered pixels and
   client configures.
3. **Tab indicator.** Draw it for tabbed columns, with the `layout`,
   `workspace.layout` and window-rule colour overrides and the space Niri
   reserves for it with `place-within-column`.

**Parts 1 and 2 landed** on 2026-11-06 (`751d161`, `80401a6`). Tests:
`internal/config/config_test.go`
`TestNiriDecorationPartsMergeAcrossLayoutWorkspaceAndRules` (parsing, merge
order, a rule's one colour keeping the inherited width, on/off, gradient
warnings, bad values) passes locally; `internal/layout/sizing_test.go`
`TestMaximizeWindowToEdgesFillsTheColumnAndHidesTheRest` checks the placement's
maximized flag; `internal/wlr/decorations_test.go`
`TestBorderDrawsAroundTheWindowAndShrinksIt` reads rendered pixels for the
inactive rule colour, urgency, the active colour, fullscreen and reload
removal, checks the configured size against the tile less the border, the
background fill and the shadow cut-out. It first ran on 2026-11-07, in lane 2
in the laptop VM, and failed: the pixel past the border lies under the shadow,
which the test turns on to check the cut-out. The test now gives the shadow
a clear colour, so the cut-out is still checked and the pixel reads the
background. It passes.

The same run showed that the shadow, an M8 mapping, did not match Niri.
Niri's shadow box is the tile grown by `spread`, blurred with sigma
`softness / 2` and drawn `ceil(1.5 * softness)` past the box
(`src/layout/shadow.rs`). SceneFX 0.5's box-shadow shader takes the node
inset by `blur_sigma` as the box and blurs it with sigma `blur_sigma / 2`.
sysc passed `softness / 2` as `blur_sigma` on a node the size of the box,
so its shadow was half as soft and its box lay `softness / 2 - spread` px
inside the tile, mostly under the window. The node is now the box grown by
`ceil(softness)`, and `blur_sigma` is the softness, for windows and layers
alike. Laptop configs with a shadow `on` will see a larger, softer shadow,
as in Niri.

Known differences: a floating window keeps its rectangle and draws the border
outside it, where Niri sizes the floating tile to hold the border. Widths
round in logical pixels. Borders sit in one tree under all windows, so a
floating window's border can fall under a tiled window it overlaps. The
background fill is checked from the scene, not pixels, because the test
client's buffers are opaque. A workspace's decoration part is merged by the
config test; no compositor test places a window on a named workspace.
SceneFX draws the shadow only inside its node, `softness` (two sigmas) past
the box, where Niri draws to three sigmas, so the outermost tail, under 3 %
of the shadow's alpha, is cut off.

**Part 3 landed** on 2026-11-06 (`8d46adb`). `tab-indicator` parses in
`layout`, `workspace.layout` and window rules, where only the colours are
allowed, as in Niri's TabIndicatorRule. A tabbed column draws one tab per
window beside its active tile. The tab takes the rule's colour, then the tab
indicator's, then the focus ring's (the border's when the ring is off); an
inactive column halves it. The indicator is hidden for a fullscreen or
maximized column and, with `hide-when-single-tab`, for one tab.
`place-within-column` keeps the width and the gap inside the column. A click
on a tab activates its window. Tests: `TestNiriTabIndicatorIsParsedAndMerged`
(config, local), `TestTabbedColumnKeepsRoomForItsTabIndicator` (layout,
local), `TestTabIndicatorRectsFollowNiri` (Niri's tab geometry, pure, local)
and `TestTabIndicatorDrawsTabbedColumnsAndActivatesTabs` (rendered colours,
rule precedence and click activation; passes in lane 2, 2026-11-07).

Known differences: no open animation; the room kept within the column comes
out of the column's width where Niri adds it to a fixed width; a touch on a
tab does not activate it; the click test calls the hit directly rather than
driving a pointer device. Gradients still warn.

#### Slice 5c — background effects and layer appearance (landed)

Landed on 2026-11-06 (`a59aeef`). Niri's `background-effect` part parses in
window and layer rules. Its `blur` applies: true blurs the whole surface when
the client asked for no ext-background-effect region, false never blurs, and
unset follows the region. A forced blur gets an effect even when the client
never bound the protocol. `xray`, `noise` and `saturation` are checked and
warn: SceneFX takes noise and saturation for the whole scene, and xray needs
an optimized blur pass that tracks the wallpaper, including a video one.

Layer rules apply `opacity` (clamped), `geometry-corner-radius`, `shadow` and
`block-out-from`. The shadow starts from the layout's shadow turned off, as in
Niri, and is cut out from under the surface unless drawn behind it. A blocked
layer's buffers are masked in the captures the rule names, through the
window-privacy mask. The border and tab rects no longer take pointer input,
so the surface under them keeps the pointer.

Tests: `TestNiriLayerRuleAppearanceIsParsedAndResolved` (config, local);
`TestWindowRuleBlurForcesTheBackgroundEffect` and
`TestLayerRuleAppearanceAndBlockOut` (a swaybg wallpaper at half opacity in
the displayed pixels and black in a screen capture). Both first ran on
2026-11-07, in lane 2 in the laptop VM, and failed. Both now pass.

- The forced blur never appeared because the window's rules did not match:
  the test client maps before it sets its app-id, and sysc resolved window
  rules again only on focus, state and reload. Niri resolves them again on
  every title and app-id change (`app_id_changed` and `title_changed` in
  `src/handlers/xdg_shell.rs`); sysc now does too, for a mapped window.
- The wallpaper kept full opacity although its buffer's opacity was 0.5.
  `swaybg -c` draws with a single-pixel buffer. wlroots 0.20.2 and SceneFX 0.5
  (both still so on their main branches) draw a single-pixel buffer as a
  rect, multiplying only its alpha by the opacity. The rect colour is
  premultiplied, so the colour stays at full strength. That is an upstream
  bug, not reported yet. The test now gives swaybg a solid PNG, which
  arrives as an ordinary shm buffer.

Known differences: sysc's blur is not xray, so it shows the windows behind;
Niri's default is xray. Opacity and block-out do not reach a layer's popups.
A layer newly blocked from screen capture does not close an open screenshot
UI, as a window does. A window or layer drawn with a single-pixel buffer
ignores rule opacity (the upstream bug above).

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
  `TestNiriWindowRuleOpenKeysRejectBadValues`. Slice 2 added
  `TestWindowRuleStatePropertiesAreParsed`,
  `TestWindowRuleStatePropertiesMatchTheWindowState` and
  `TestWindowRuleUnsupportedMatchPropertiesWarnAndDropTheRule`. Slice 3
  replaced the last one with `TestWindowRuleActiveAndCastTargetPropertiesAreParsed`
  and `TestWindowRuleUnknownMatchPropertyIsRejected`.
- `internal/layout` and `internal/wm`: one test per set-variant, next to the
  toggle tests. Slice 1 added `TestMaximizeColumnFillsTheWorkingWidth`,
  `TestMaximizeWindowToEdgesFillsTheColumnHeight`,
  `TestSetColumnDisplayForSetsTheColumnOfTheWindow`,
  `TestSetFloatingPositionPlacesTheWindow` and
  `TestSetFloatingPositionKeepsThePositionForALaterFloat`. Slice 4a added
  `internal/wm/workspace_layout_test.go` with
  `TestWorkspaceLayoutOverridesTheOutputLayout` and
  `TestWorkspaceLayoutFixedColumnWidth`. Slice 4b added
  `TestSwitchPresetColumnWidthMixesFixedAndProportionalPresets`,
  `TestSwitchPresetWindowHeightMixesFixedAndProportionalPresets`,
  `TestNewColumnsUseTheDefaultColumnDisplay`,
  `TestCenterFocusedColumnAlwaysCentersTheFocusedColumn`,
  `TestCenterFocusedColumnOnOverflowCentersWhenTheColumnsOverflow`,
  `TestCenterFocusedColumnOnOverflowFitsWhenTheColumnsFit`,
  `TestStrutsInset`, `TestWorkspacePresetsFollowTheConfig`,
  `TestWorkspaceDefaultColumnDisplayFollowsTheConfig`,
  `TestWorkspaceCenterFocusedColumnFollowsTheConfig`,
  `TestWorkspaceStrutsFollowsTheConfig` and
  `TestConfigWideLayoutReachesLaterWorkspaces`, the last one pinning the
  ordering gap the 2026-11-04 handover named.
- `internal/config`: slice 4a added `internal/config/workspace_layout_test.go`
  with `TestWorkspaceLayoutIsParsed`,
  `TestWorkspaceLayoutWarnsForTheKeysTheCompositorCannotApply` and
  `TestWorkspaceLayoutRejectsBadValues`. Slice 4b added
  `TestLayoutPresetsAreParsed`, `TestLayoutRejectsBadPresetValues`,
  `TestCenterFocusedColumnIsParsed` and `TestStrutsAreParsed`, re-targeted the
  warning test at `always-center-single-column`, and taught
  `TestNiriWindowRuleOpenKeysAreParsedAndResolved` and its reject twin the
  `scroll-factor` key.
- `internal/wlr`: one test per key that the compositor applies it, next to the
  existing rule tests. These need a compositor, so they run in CI lane 2
  under vkms, not locally. Slice 1 added `internal/wlr/window_rules_test.go`
  with `TestWindowRuleOpenMaximizedFillsTheWorkingWidth`,
  `TestWindowRuleOpenMaximizedToEdgesFillsTheColumn`,
  `TestWindowRuleOpenFocusedFalseKeepsTheFocus` and
  `TestWindowRuleMinWidthAmendsTheSizeHints`. Slice 2 added
  `TestWindowRuleIsActiveInColumnMatchesTheWindowThatOpens`,
  `TestWindowRuleIsFocusedDoesNotMatchAWindowThatOpens`,
  `TestWindowRuleIsUrgentAppliesWhenTheWindowBecomesUrgent`,
  `TestWindowRuleAtStartupMatchesInsideTheStartupWindow` and
  `TestWindowRuleStartupTimerEndsTheStartupWindow`. Slice 3 added
  `TestWindowRuleIsActiveAppliesToTheActiveWindow` and
  `TestWindowRuleIsWindowCastTargetIsNeverTrue`. Slice 4a added
  `internal/wlr/workspace_layout_test.go` with
  `TestWorkspaceLayoutOverridesTheOutputLayout`. Slice 4b added
  `TestWorkspacePresetsReachTheLayout`,
  `TestCenterFocusedColumnReachesTheLayout` and
  `TestWorkspaceStrutsReachTheLayout`. The scroll factor multiplies the axis
  delta before it reaches `wlr_seat_pointer_notify_axis`; no vkms test drives
  a synthetic axis event, so that path rests on the config parse tests and
  code reading for now.
- `internal/server`: the config has to carry the layout of a workspace into the
  model options. Slice 4a added `TestWorkspaceLayoutReachesTheModelOptions`;
  slice 4b extended it with the presets, the column display, the centering
  and the struts.
- `internal/cli`: `validate` on a config that uses the new keys must print no
  warning. Slice 1 covered this in the config test instead: a config that
  uses the new keys parses without a warning.

## 6. Docs

- This plan gets a row in `docs/README.md` in the same commit.
- `docs/plans/niri-parity-matrix.md` gets the rows for the keys that land.
- `docs/plans/2026-11-03-touch-handover.md` keeps pointing at this plan for
  the config-surface item until the tranche is done.
