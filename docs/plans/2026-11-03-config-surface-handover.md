# Handover: 2026-11-03, after the config surface slices 1–4a

Supersedes nothing: this sits beside `2026-11-03-touch-handover.md`, which is
still the state of the touch and pointer work. **bd is authoritative for what
is open.** The bd DB lives at `/home/nomadx/sysc-shell/.beads`; run `bd` from
`/home/nomadx/sysc-shell`, never from a worktree.

## State in one paragraph

The Niri config surface tranche (`sysc-2231`) is half done. Slice 1 applies the
`window-rule` keys that map onto machinery the compositor already had
(`open-maximized`, `open-maximized-to-edges`, `open-focused`, `min`/`max`
`width`/`height`, `default-column-display`, `default-floating-position`). Slice 2
made the state-dependent match properties work (`is-focused`, `is-urgent`,
`is-active-in-column`, `is-floating`, `at-startup`, with a 60-second startup
clock). Slice 3 finished the match properties (`is-active`,
`is-window-cast-target`), so **every match property Niri has is now parsed and
evaluated** and no rule is dropped for an unevaluatable match. Slice 4a gave a
workspace its own `gaps` and `default-column-width` through
`workspace { layout { … } }`. Everything is on draft PR #94, and CI run
`41625019080` is green on both lanes. The remaining work is exactly the keys in
slices 4b, 5 and 6 — chiefly the appearance properties, which is why the
archThink's own config still prints fourteen warnings at every start.

## The branch

`feat/config-surface-20261103`, cut from `main` at `1b03199` (the sizing
milestone and the bar work). It is **deliberately not** cut from the input,
touch or gate branches: the config surface does not depend on them, and this way
it lands independently of the physical gates those branches wait on.

| Commit | What |
|---|---|
| `ae1cd2b` | docs: the plan for the tranche |
| `3bd4e41` | feat: slice 1, the window-rule open keys |
| `5480773` | docs: slice 1 landed |
| `79b939d` | feat: slice 2, the state match properties |
| `c1b59c3` | docs: slice 2 landed |
| `99a6c7e` | feat: slice 3, is-active and is-window-cast-target |
| `fcc27ee` | docs: slice 3 landed |
| `c817344` | feat: slice 4a, the per-workspace layout |
| `225aac1` | docs: slice 4a landed |

Draft PR **#94**: https://github.com/Nomadcxx/sysc-compositor/pull/94 — base
`main`, head `feat/config-surface-20261103`. The body is at
`/home/nomadx/.cache/gate/pr-body-config-surface.md` and describes slice 1 only;
rewrite it when the tranche is done. Diff against `main`: 24 files,
+2070/−64.

**CI run `41625019080` is green on both lanes** — job `check` at 11:10:57Z and
job `compositor` at 11:12:30Z, where the lane-2 tests run under vkms. Earlier
green runs: `41623581678` (slice 3) and `41622434062` (slice 2).

The working tree is clean except the untracked `.commandcode/` directory, which
is never added.

## The plan document

`docs/plans/2026-11-03-niri-config-surface.md` is the design of record for the
tranche: the corrected premise, the key-by-key gap, the six slices, and a
"what actually shipped" section per landed slice. Read it before touching a key.
`docs/plans/niri-parity-matrix.md` carries the per-section rows that name what
each slice added.

### The premise the tranche corrected

The earlier handover and the audit said `window-rule`, `layer-rule` and
`workspace.layout` are "parsed and then ignored". **That was wrong.** The
compositor applies what it supports and warns per block, so the shipped
`internal/config/default-config.kdl` prints no warning at all. The real gap is
the Niri keys with no counterpart, and it is visible on a real machine: the
archThink's config printed fifteen warnings at every start, and prints fourteen
today (`window-rule.match` is gone).

## What shipped, slice by slice

### Slice 1 — the window-rule keys that map onto existing machinery

Parsed in `internal/config/rules.go`, applied in
`internal/wlr/windows.go` `mapWindowWithRules` when the window maps.

- `open-maximized` → `layout.MaximizeColumn` / `model.MaximizeColumnFor`: the
  column takes the whole working width and keeps the gaps on either side.
- `open-maximized-to-edges` → `layout.MaximizeWindowToEdges` /
  `model.MaximizeWindowToEdgesFor`: the window fills the height of its column
  and the column the working width.
- `open-focused false` → the window does not take the focus. The layout focuses
  a new column when the window is added, so the map path in
  `internal/wlr/input.go` reads the focused window **before** the map and gives
  the focus back afterwards. `true` keeps the default behaviour.
- `min-width`, `max-width`, `min-height`, `max-height` →
  `internal/wlr/workspaces.go` `windowSizeHints`. Niri's rule: a rule's minimum
  **raises** the client's minimum, a rule's maximum **lowers** the client's
  maximum, and a rule's maximum replaces an absent client maximum. The
  compositor never calls `xdg_toplevel_set_min_size`, so these are the layout's
  own bounds, not a configure the client has to honour.
- `default-column-display` (`normal`/`tabbed`) → `model.SetColumnDisplayFor`
  sets the display mode of the column **the window created**, not of the
  focused column.
- `default-floating-position` (`x`, `y`, `relative-to`) →
  `internal/layout.FloatingPosition.resolve` with Niri's maths: a corner
  offsets from the corner, a side centres the window on that side, and the
  working area's origin is added last. A window that is tiled when the rule
  applies keeps the position for the day it floats.

### Slice 2 — the match properties readable at map time

The rules are matched against a `config.WindowState`. A window that is not on
screen yet has Niri's unmapped state — not focused, not urgent, not floating,
**`is-active-in-column` true** — which is why `match is-active-in-column=true`
applies to a window that opens and `match is-focused=true` does not.

- `internal/wlr/window_rules.go` `ruleState` reads a mapped window's state: the
  urgency and floating state from the model's snapshot, the focus from the
  seat, the startup flag from the compositor.
- The rules are read again when the state moves: `focusView`,
  `SetWindowUrgent`, `ToggleWindowUrgent`, `SetWindowFloating`, the focused
  output change, and the unmap and destroy paths all call
  `reapplyWindowRules`.
- `internal/wlr/startup.go` arms a `wl_event_loop` timer for the first sixty
  seconds and ends the window when it runs out, then reads the rules again, so
  an `at-startup` rule stops matching. `startupWindowMS` is a `var` so a test
  can shorten it.

### Slice 3 — the match properties that need new state

- `is-active` is Niri's "window with the active border": every workspace on the
  focused monitor has one. `ruleState` compares the view's id with
  `WorkspaceSnapshot.ActiveWindowID` for every workspace whose output is the
  focused one.
- `is-window-cast-target` is the window an ongoing **individual** window
  screencast shows. sysc has pipewire and the portal but no
  `zwlr_screencast_manager_v1`, so no window is ever the target: the property
  is evaluated and is always false. There is a comment saying so; do not
  "fix" it without adding the protocol.
- The `Unsupported` flag in `RuleMatch` survives as a guard for a property that
  cannot be evaluated yet, but **nothing sets it now**. An unknown match
  property is a hard parse error.

### Slice 4a — the per-workspace layout

`workspace "name" { layout { … } }` is Niri's per-workspace `LayoutPart`,
merged over the output's layout. Shipped: `gaps` and `default-column-width`
(`proportion` or `fixed`).

- `config.WorkspaceLayout` and `wm.WorkspaceLayout` hold `*int` / `*float64`
  fields, so a nil field means "keep the output's value".
- `wm.Model.workspaceGeometry` does the merge; it runs when the workspace is
  created, when the config changes, and whenever the output is resized.
- `layout.ColumnWidthPixels(available, gap, proportion)` is now the one place
  that turns a share into pixels; the compositor and the model both use it.
- Every other key inside `workspace.layout` warns **by name**
  (`workspace.layout.<key>`), so `workspace.layout` itself no longer warns.

**Known difference from Niri, documented in the plan:** a workspace that sets
only the gaps keeps the column width in pixels the output computed with the
output's gaps. Niri recomputes the width from the share and the new gaps.
Making it exact needs the output's share, not only its pixels, which would
touch every caller of `model.AddOutput` and `model.SetOutputGeometry`.

## What remains

### Slice 4b — the global layout keys that map onto existing machinery

1. **`preset-column-widths` and `preset-window-heights`.** Today they are the
   hardcoded `var presetColumnWidths = []float64{1/3, 0.5, 2/3}` and the same
   for heights in `internal/layout/sizing.go`. The layout needs them per
   workspace (Niri merges them like the rest of the layout part), which means
   `Workspace` carries its own slice and `SwitchPresetColumnWidth` /
   `SwitchPresetWindowHeight` read it. Remember `presetIndex` compares against
   the current pixel width, so mixed fixed and proportional presets need care.
2. **`default-column-display`.** `layout.ColumnDisplay` already has
   `ColumnDisplayNormal` and `ColumnDisplayTabbed`, but the display mode is a
   zero value at the ~8 `columnState{…}` **creation points** in
   `internal/layout/layout.go` (roughly lines 164, 166, 432, 434, 659, 660,
   714, 936, 1014, 1016, 1084, 1086). A per-workspace default means every one
   of them reads the workspace's default instead of leaving the zero value.
   Enumerate them with `grep -n 'columnState{' internal/layout/*.go` — do not
   trust the line numbers above, they move.
3. **`center-focused-column`** (`never` / `always` / `on-overflow`) — the
   layout has `CenterColumn` and `CenterVisibleColumns` actions already, and
   `revealFocused` does an implicit overflow-centring; this key makes it a
   policy.
4. **`struts`** — the bar's exclusive zone is already tracked per output in
   `internal/wlr` (`c.usable`), so this is a question of whether the layout
   should also reserve space.
5. **`scroll-factor` per window** — a window-rule key, parsed in `rules.go`,
   applied where pointer scroll reaches a surface.

### Slice 5 — the appearance properties

`opacity`, `border`, `focus-ring`, `shadow`, `tab-indicator`,
`draw-border-with-background`, `background-effect`, and `layer-rule`'s
`opacity`, `shadow`, `geometry-corner-radius`, `background-effect`,
`block-out-from`. These are per-window overrides of what is one global setting
today (`c.layoutOptions` in `internal/wlr/appearance.go`), so they touch the
decoration and the renderer. **This is the slice that removes the last
`window-rule` warning on the archThink**, which comes from `opacity` and
`draw-border-with-background`.

### Slice 6 — the leftovers

`popups`, `variable-refresh-rate`, `baba-is-float`, `tiled-state`, and
`layer-rule.match`'s `at-startup` and `layer`. Some of these are better
rejected with a clear message than accepted and ignored.

## The measured warnings

`sysc-compositor validate -c CONFIG` is the fastest measure of the gap.

- **archPC** (`~/.config/sysc-compositor/config.kdl`): 8 warnings — `output`,
  `layout.center-focused-column`, `layout.preset-column-widths`,
  `layout.border`, `layout.tab-indicator`, `config-notification`, `debug`,
  `window-rule`.
- **archThink** (461-line config fetched to
  `/home/nomadx/.cache/gate/laptop-config.kdl`, with its `sysc-shell.kdl` and
  `sysc.kdl` includes beside it so the include resolves): **14** warnings —
  `config-notification`, `output`, `layout.background-color`,
  `layout.center-focused-column`, `layout.preset-column-widths`,
  `layout.border`, `layout.struts`, `overview.workspace-shadow`, `debug`,
  `animations.window-open.easing`, `animations.window-close.easing`,
  `animations.config-notification-open-close`, `animations.screenshot-ui-open`,
  `window-rule`.
  It was 15; `window-rule.match` is gone. `window-rule` remains for `opacity`
  and `draw-border-with-background` (slice 5).

Note that `output`, `config-notification`, `debug`, `overview` and
`animations` are **outside this tranche** — they are their own keys with their
own machinery. The tranche's own remainder is exactly `layout.*`,
`workspace.layout.*`, `window-rule` and `layer-rule`.

## How to verify a slice

Never `go test ./...`, never `-race`. One package, one named test:

```
timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run <Name>
```

with

```
CGO_CFLAGS_ALLOW=-fno-strict-overflow \
GOTMPDIR=/home/nomadx/.cache/go-tmp TMPDIR=/home/nomadx/.cache/go-tmp
```

- Lane 1 (`internal/config`, `internal/layout`, `internal/wm`, `internal/cli`)
  runs locally.
- `internal/wlr` and `internal/server` build a compositor and **are excluded
  from the local test policy**. CI lane 2 under vkms is their gate. A nested
  test locally needs `SYSC_NESTED_TESTS=1` **and** the owner's explicit consent
  for that run — it opens a window on their desktop.
- A skipped lane-2 test with no entry in `.github/ci/skips.txt` fails the lane.
  Do not add a skip to make a red run green.

Then `gofmt -l internal/`, `go build ./...`, `go vet ./internal/...`, commit,
push with `TMPDIR=/home/nomadx/.cache/go-tmp` (the pre-push hook's `mktemp`
hits the full `/tmp`), and watch CI:

```
gh run watch <id> --exit-status
```

## Working notes that bite every time

- `/tmp` is a full tmpfs. Build and test into `/home/nomadx/.cache`. A heredoc
  (`cat <<EOF`, `python3 <<PY`) fails with "write failed: disk quota exceeded" —
  write a file with the editor or a script file and run that.
- `git commit -F -` with a heredoc fails the same way; `git commit -F <file>`
  works.
- Never `git add -A`: `.commandcode/` is untracked and must stay out.
- Never merge `main` into a feature branch; cherry-pick instead.
- Commit messages and author metadata avoid the attribution-guard words:
  `bot` (so "both" is spelled "the two"), `bard` (inside "keyboard"), agent,
  cursor, llm, codex, gemini, claude, openai, `gpt-[0-9]`, ai assistant.
- Go + cgo only: no `.c` or `.h` files in the tree, listener trampolines live in
  a `.go` cgo preamble, and `runtime.LockOSThread()` comes before any wlroots
  call.
- Do not implement a bar, lock screen, wallpaper engine, notification server or
  tray here.
- Niri's semantics win. When sysc's model cannot express one, say so in the
  plan rather than quietly choosing sysc's behaviour.

## Open work outside this tranche

These are not yours unless the owner says so.

- **Physical gates**, all the owner's, on the archThink: the sizing family
  (`sysc-2228`) and the bar auto-hide (`sysc-2229`, shell PR #616). The touch
  gate (`sysc-2227`) passed on 2026-11-03 and is closed.
- **Draft PRs**: #89 (input), #91 (touch), #93 (the union tree that ran the
  touch gate — still conflicts with `main` in `docs/README.md` and
  `docs/plans/2026-11-03-compositor-audit.md` only, and CI never ran on it),
  and #92 (performance research). #94 is this tranche and stays draft until
  the owner has looked at it.
- **Flakes**: the portal colour picker under vkms, and the overview scroll
  fixture (`sysc-2219`).
- **Packaging** (`sysc-2200` plus children), deferred by the owner.
- The installed `/usr/local/bin/sysc-compositor` on the archThink is still the
  pre-touch build, and the touch gate's systemd drop-in is disarmed.

## The one-line summary for the next agent

Slices 1–4a are green on PR #94; open
`docs/plans/2026-11-03-niri-config-surface.md`, pick up slice 4b (the preset
widths and heights, then the per-workspace column display), and follow the same
rhythm: parse in `internal/config/rules.go`, apply through `internal/wm` and
`internal/layout`, a lane-1 test, a lane-2 test in `internal/wlr`, a docs commit
in the same slice, and a bd note — `bd` from `/home/nomadx/sysc-shell` only.
