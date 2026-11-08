# Config surface handover — 2026-11-04, after slice 4b items 1–3

Historical snapshot. The later commits `069a2fc` and `4e757a7` completed the
ordering fix, struts and per-window scroll factor; `7043d17` records the close-out.
Continue from [the 2026-11-05 plan](2026-11-05-handover.md), not the open-work
list below.

This picks up from [2026-11-03-config-surface-handover.md](2026-11-03-config-surface-handover.md).
It records what landed on `feat/config-surface-20261103` after that handover, the
config-wide ordering gap to close first, the settled design for the rest of slice 4b
(struts, scroll-factor), and the working notes that bite.

## The commit map

| Commit | What it applies |
|---|---|
| `75271b8` | docs: the previous handover (slices 1–4a). |
| `f471fc7` | `preset-column-widths` and `preset-window-heights` (per workspace, mixed fixed and proportion presets) and `default-column-display` (per workspace). CI `41793716106` green. |
| `a78b4c2` | `center-focused-column` (`never` / `always` / `on-overflow`). CI `41803145551` green. |
| this commit | docs: this handover. |

Every run green on the two lanes (job `check` with gofmt, vet, build and lane 1; job
`compositor` with lane 2 under vkms). Working tree clean except untracked `.commandcode/`
(never add it). Base `main`, head `feat/config-surface-20261103`, draft PR #94. Never
merge `main` into the branch; cherry-pick.

## What the two commits apply, and where

### `f471fc7` — the preset sizes and the default column display

- `internal/layout`: `PresetSize{Fixed bool; Value float64}` (fixed is pixels, otherwise
  `Value` is a share of the working size) with `defaultPresetSizes = [1/3, 1/2, 2/3]`.
  `Workspace` carries `presetColumnWidths`, `presetWindowHeights []PresetSize` and
  `defaultDisplay ColumnDisplay`, set through `SetPresetSizes(widths, heights)` and
  `SetDefaultColumnDisplay(display)`. An empty slice keeps the Niri default presets.
  `presetPixels(presets[i], working)` resolves one preset to pixels and `presetIndex`
  compares in pixels, so mixed fixed and proportion lists cycle correctly.
  `SwitchPresetColumnWidth` / `SwitchPresetWindowHeight` read the workspace's own lists.
  Every fresh `columnState{…}` literal sets `display` from the workspace default
  (the cross-workspace `MoveWindowTo` literal uses the target's default); a whole-column
  move keeps the column's own display.
- `internal/config`: `Layout` and `WorkspaceLayout` gain `PresetColumnWidths`,
  `PresetWindowHeights []PresetSize` and `DefaultColumnDisplay`. In a workspace layout a
  nil preset list keeps the config's list and a non-nil empty one resets to the defaults;
  a nil display keeps the config's. Parsed in both `layout {…}` and `workspace.layout {…}`
  as `preset-column-widths { fixed 600; proportion 0.25; }` style children and
  `default-column-display "tabbed"`. These keys warn no more.
- `internal/wm`: `WorkspaceLayout` mirrors the fields; `applyWorkspaceLayout` pushes
  geometry, preset sizes and the default display into every workspace's strip;
  `SetPresetSizes` / `SetDefaultColumnDisplay` reapply across all outputs.
- `internal/server` + `internal/wlr`: the values reach the model through
  `wlr.LayoutOptions` and the startup/Reconfigure setters.

### `a78b4c2` — the center-focused-column policy

- `internal/layout`: `CenterFocusedColumn{Never, Always, OnOverflow}` on the workspace,
  set through `SetCenterFocusedColumn`. `revealFocused()` is `revealFocusedFrom(-1)`:
  `always` centers the focused column on every reveal (window add, moves, geometry),
  `on-overflow` centers only when a *focus change* lands on a column that does not fit
  on screen next to the previously focused one (the neighbor-fit check from Niri's
  `compute_new_view_offset_for_column`), `never` keeps the minimal-reveal behavior.
  A column wider than the view is left-aligned in every mode.
  `FocusLeft`, `FocusRight`, `FocusFirst`, `FocusLast`, `FocusColumn` and the click-focus
  path capture the previously focused index and call `revealFocusedFrom(previous)`; all
  other reveals pass no previous index (Niri passes `prev_idx = None` there).
- `internal/config`, `internal/wm`, `internal/server`, `internal/wlr`: the same
  parse-merge-apply plumbing as above; `center-focused-column "on-overflow"` parses in
  both the `layout` and `workspace.layout` sections, a workspace's mode overrides the
  config's, and the key warns no more.

## Open work, in order

### 0. Close the config-wide ordering gap (small; do this first)

`wm.Model.newWorkspace` only builds `layout.NewWorkspace(out.bounds, out.columnWidth,
out.gap)`, and the plain workspace `AddOutput` creates never passes through
`applyWorkspaceLayout` — only the named workspaces `addConfiguredWorkspaces` creates do.
The config-wide setters run at compositor startup before outputs exist, so a workspace
created afterwards keeps the layout defaults instead of the config's preset sizes,
default display and centering policy. Fix: `newWorkspace` applies the config-wide values
to the fresh strip (for an unnamed workspace the merged values *are* the config-wide
values; named ones are corrected right after by `addConfiguredWorkspaces`). Regression
test to add in `internal/wm`:

```go
m := New()
m.SetPresetSizes([]layout.PresetSize{{Fixed: true, Value: 300}}, nil)
m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
// map a window to the plain workspace, then:
m.SwitchPresetColumnWidth(false)
// assert Tile.Width == 300
```

Item 4 needs the same hook for struts, so folding this into the struts commit is fine —
but it must not be dropped.

### 1. Item 4 — `struts` (design settled; no code written)

`layout { struts { left 10; right 10; top 0; bottom 0; } }`, also per workspace, values
-65535..65535 (negative expands the area). Niri applies struts at the layout level
(working area for columns), *in addition to* the layer-shell exclusive zones already
tracked per output in `internal/wlr` (`c.usable`). Design:

1. `layout.Struts{Left, Right, Top, Bottom int}` with `Inset(bounds Bounds) Bounds`
   (X += Left, Y += Top, Width -= Left+Right, Height -= Top+Bottom, clamped to zero).
   `config.Struts` mirrors it; `config.Layout.Struts Struts` (value) and
   `config.WorkspaceLayout.Struts *Struts` (nil keeps the config's; Niri merges with
   `merge_clone`, so a workspace's struct replaces the whole thing, not per edge).
2. Parse `struts { left N; … }` in *both* `c.layout` (config.go) and `workspaceLayout`
   (rules.go): each child is `left|right|top|bottom` with `intArg(child, -65535, 65535)`,
   unknown child errors. The key leaves the warn paths, so
   `TestWorkspaceLayoutWarnsForTheKeysTheCompositorCannotApply` must be re-targeted at a
   still-unapplied key (e.g. `always-center-single-column`), expecting only that key.
3. `internal/wm`: `WorkspaceLayout.Struts *layout.Struts`, model field `struts`, setter
   `SetStruts` (unchanged value is a no-op, then `reapplyWorkspaceLayout`), merge helper
   `strutsFor(workspace)`. New `workspaceBounds(out, workspace) = strutsFor(...).Inset(out.bounds)`;
   `applyWorkspaceLayout` passes that to `strip.SetGeometry`. `MoveWorkspaceToOutput`
   currently sets geometry directly with the target's raw values — route it through
   `applyWorkspaceLayout(target, workspace)` (this also makes slice-4a overrides apply on
   output moves; say so in the commit body). `workspaceGeometry`'s proportion case should
   use the strutted width (Niri computes proportions on the working area); the
   output-derived default column width keeps the unstrutted value — known difference,
   record it in the plan doc.
4. `internal/server` + `internal/wlr`: `struts`/`strutsPtr` converters, `LayoutOptions.Struts`,
   `SetStruts` beside the other startup/Reconfigure setters, and `syncWorkspaceOutput`
   computes the config-wide default width from `bounds.Width - struts.Left - struts.Right`.
   Struts apply only to the strip's bounds; fullscreen and overview keep the full usable
   area.
5. Tests: config parse + rejects (`left 70000`, `west 3`, `left "ten"`, `right -70000`);
   wm `TestWorkspaceStrutsFollowTheConfig` — `SetStruts({Left: 100})` after `AddOutput`
   with a "wide" workspace overriding `{Left: 200}`: plain window `Tile.X == 116`
   (inset X 100 + fit padding 16), wide `Tile.X == 216`; server assertion in
   `TestWorkspaceLayoutReachesTheModelOptions`; lane-2 `TestWorkspaceStrutsReachTheLayout`
   with `Layout{Gaps: 16, FixedColumnWidth: 300, Struts: {Left: 100}}`, asserting
   `Tile.X == bounds.x + 100 + pad` with `pad = min(16, (bounds.w-400)/2)` from
   `c.layoutBox()` (output-size independent).

### 2. Item 5 — per-window `scroll-factor`

Window-rule key, `scroll-factor 0.5`, range 0..100 (Niri `FloatOrInt<0, 100>`). Add
`WindowRule.ScrollFactor *float64` and a result field with `applyRuleValue`, parse in
`internal/config/rules.go`, and scale the scroll the pointer sends to the surface:
`handleCursorAxis` (internal/wlr/input.go, the `wlr_seat_pointer_notify_axis` call)
multiplies `event.delta` and `event.delta_discrete` by the focused window's factor
(`view.appearance`, default 1). Niri multiplies by the device's scroll factor too
(input.mouse / input.touchpad); those keys are outside this tranche — record the
difference.

### 3. The slice-4b close-out

One docs commit for the slice (same rhythm as 4a): "what actually shipped" rows in
[2026-11-03-niri-config-surface.md](2026-11-03-niri-config-surface.md) for preset sizes,
default column display, center-focused-column, struts and scroll-factor, the known
differences listed above, and matching rows in
[niri-parity-matrix.md](niri-parity-matrix.md). Then a note on `sysc-2231` from
`/home/nomadx/sysc-shell` (`bd comments add sysc-2231 -f <file>`) recording the slice.

## How to verify a slice

Never `go test ./...`, never `-race`. One package, one named test:

```
timeout 90s env GOMAXPROCS=2 CGO_CFLAGS_ALLOW=-fno-strict-overflow \
  GOTMPDIR=/home/nomadx/.cache/go-tmp TMPDIR=/home/nomadx/.cache/go-tmp \
  go test -count=1 ./<pkg> -run <Name>
```

The `./` prefix is required (bare `internal/layout` is read as a std path). `go vet` and
`go build ./...` need the same env. Lane 1 (internal/config, internal/layout,
internal/wm, internal/cli) runs locally; internal/wlr and internal/server lane-2 tests
run only in CI under vkms — a skipped lane-2 test without a `.github/ci/skips.txt` entry
fails the lane, and never add a skip to make red green. Nested tests need
`SYSC_NESTED_TESTS=1` and the owner's consent for that run; never in a loop, a script or
a background job. Then `gofmt -l internal/`, commit, push and
`gh run watch <id> --exit-status` on the new run.

## Working notes that bite

- `/tmp` is a full tmpfs: build and test into `/home/nomadx/.cache` (GOTMPDIR/TMPDIR as
  above). Heredocs of any kind (`cat <<EOF`, `python3 <<PY`, `git commit -F -`) fail with
  `write failed: disk quota exceeded` — write the text to `/home/nomadx/.cache/tmp-*.txt`
  with the write tool and use `git commit -F <file>`.
- Never `git add -A`; add named files only. `.commandcode/` is untracked and must stay so.
- Push with `TMPDIR=/home/nomadx/.cache/go-tmp`. Watch CI via
  `gh run list --branch feat/config-surface-20261103`, then a background
  `gh run watch <id> --exit-status`.
- Attribution guard (AGENTS.md): commit messages and author must not contain `bot`,
  `bard`, `agent`, `cursor`, `llm`, `codex`, `gemini`, `claude`, `openai`, `gpt-[0-9]`
  or "ai assistant". `bot` hides inside "both" — write "the two"; `bard` hides inside
  "keyboard" — rephrase. The guard applies to commits; prose docs may quote the list.
- `bd` runs only from `/home/nomadx/sysc-shell` (DB `.beads` there), never a worktree.
- Go + cgo only, no `.c`/`.h` in the tree; `runtime.LockOSThread()` before any wlroots
  call. No bar, lock screen, wallpaper engine, notification server or tray here.
  Niri's semantics win; document differences in the plan doc.

## The one-line summary for the next session

Close the config-wide ordering gap in `newWorkspace`, then finish slice 4b with `struts`
(design above) and the per-window `scroll-factor`, then the slice docs commit and the
`sysc-2231` note; every slice lands the same way: parse in `internal/config`, apply
through `internal/wm` and `internal/layout`, lane-1 test, lane-2 test in `internal/wlr`,
docs commit in the same slice.
