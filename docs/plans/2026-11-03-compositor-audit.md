# Compositor audit and bug hunt — 2026-11-03

Owner decision on 2026-11-03: the packaging tranche stays deferred until the compositor has a
working full-window path and has been audited and bug-hunted. The physical touchpad gate passed
before this audit (three-finger swipe, two-finger scroll and tap-to-focus all work on archThink).

This records what was checked, what was wrong, what was changed, and what is still open. It is an
assessment, not a milestone plan, so it is not scheduled work.

## 1. Full window with Ctrl+F

`Ctrl+F` was never bound. Niri binds `Mod+F` to `maximize-column` and `Mod+Shift+F` to
`fullscreen-window`; it binds `Ctrl+F` to nothing. The owner's muscle memory came from another
compositor, so the key did nothing.

The action itself was always there and always worked:

- `internal/bind/bind.go` names the bind action `fullscreen-window`.
- `internal/wlr/keyboard.go` maps it to `c.ToggleFullscreen()`.
- `internal/wlr/workspaces.go` implements it as `SetFullscreen(window.ID, !window.Fullscreen)`, a
  real toggle.
- `internal/wlr/keyboard.go` runs `dispatchKeyBinding` **before** `forwardKeyboardKey`, so a bind
  wins over the focused client and the client cannot swallow the key.

Proof on archThink, over the real key path rather than the IPC path:

```
XDG_RUNTIME_DIR=/run/user/1000 WAYLAND_DISPLAY=wayland-0 wtype -M ctrl f -m ctrl
```

The focused kitty window went from `window_size [1920,1080]` (fullscreen) to `[953,514]` (tiled),
and the `tile_size` never changed, which is what a client-driven fullscreen looks like.

`Ctrl+F { fullscreen-window; }` was added to the owner's own `~/.config/sysc-compositor/config.kdl`
and `~/.config/niri/config.kdl` on archThink. Both validate and both reload live.

It was deliberately **not** added to the shipped `internal/config/default-config.kdl`: `Ctrl+F` is
a common terminal binding and the compositor would swallow it for every user. That is an owner
decision to revisit.

## 2. The biggest finding: binds that are accepted and then ignored

`sysc-compositor validate -c internal/config/default-config.kdl` printed:

```
warning: bind actions are accepted but not applied yet: switch-preset-column-width,
switch-preset-column-width-back, switch-preset-window-height, reset-window-height,
maximize-column, maximize-window-to-edges, expand-column-to-available-width, center-column,
center-visible-columns, set-column-width, set-window-height
```

The mechanism is `internal/config/config.go`: when a bind names an action that
`bind.IsSupportedAction` does not know, the action is recorded in `UnappliedActions` and the bind
is dropped. Nothing else happens.

**Thirteen of the binds in the shipped default config did nothing at all, with no message:**

| Bind | Action | Niri's action |
|---|---|---|
| `Mod+R` | `switch-preset-column-width` | same |
| `Mod+Shift+R` | `switch-preset-column-width-back` | same |
| `Mod+Ctrl+Shift+R` | `switch-preset-window-height` | same |
| `Mod+Ctrl+R` | `reset-window-height` | same |
| `Mod+F` | `maximize-column` | same |
| `Mod+M` | `maximize-window-to-edges` | same |
| `Mod+Ctrl+F` | `expand-column-to-available-width` | same |
| `Mod+C` | `center-column` | same |
| `Mod+Ctrl+C` | `center-visible-columns` | same |
| `Mod+Minus` | `set-column-width "-10%"` | same |
| `Mod+Equal` | `set-column-width "+10%"` | same |
| `Mod+Shift+Minus` | `set-window-height "-10%"` | same |
| `Mod+Shift+Equal` | `set-window-height "+10%"` | same |

This is the family the owner means by "full window" in Niri: `Mod+F` maximizes a column, `Mod+M`
maximizes a window to the output edges, `Mod+C` centres a column. They are accepted, they are
listed in the default config as if they work, and they do nothing.

The reason is structural: `internal/layout/layout.go` keeps a per-column width
(`columnState.width`, `w.columns[i].width`) and a default `w.columnWidth`, but exposes no public
setter — the public layout API is `SetGeometry(bounds, columnWidth, gap)`. A `set-column-width`
action needs a layout-level column width setter plus the bind and IPC wiring. The window-height
family needs more than that, because the layout is column-oriented and has no vertical sizing
model.

The same applies to config **sections**. `workspace.layout`, `window-rule`, `window-rule.match`,
`layer-rule` and `layer-rule.match` are parsed and then ignored. The shipped default config
contains a `window-rule` block that opens a Firefox picture-in-picture window floating; it does
nothing today.

> **Correction, 2026-11-03 (after the touch tranche).** The paragraph above is wrong about
> `window-rule` and `layer-rule`, and it was checked rather than believed.
> `sysc-compositor validate -c internal/config/default-config.kdl` prints **no warning** for the
> shipped default, and the Firefox picture-in-picture rule **is applied**: `internal/wlr/windows.go`
> resolves the rules when a window maps, and `open-floating` puts that window in the floating layer.
> The warning names are per unsupported **sub-key**, not per section: a config with
> `workspace { layout { gaps 8 } }` and `window-rule { match is-active=true }` warns exactly
> `workspace.layout` and `window-rule.match`, and a `layer-rule` with `place-within-backdrop` warns
> nothing and is applied (`internal/wlr/layer.go`).
> What is genuinely accepted and ignored is `workspace.layout`, Niri's per-workspace layout
> override. The rest of the gap is the Niri keys sysc has no counterpart for, which warn.

## 3. The silence was the bug

The bind path was silent where the IPC path is honest:

- An IPC action the compositor does not implement returns `{"Err":"unimplemented: <name>"}`
  (`internal/ipc/wire.go`, `internal/server/niri.go`).
- A **bind** action the compositor does not implement was dropped at parse time and nothing was
  said, at startup, on reload, or ever.

Two changes close the silence:

1. `internal/cli/cli.go` gained `reportConfigGaps`, and `run` calls it right after the config is
   loaded, so the compositor prints the accepted-but-not-applied sections and bind actions at
   startup. `validate` prints the same list through the same helper, so the wording cannot drift.
2. `internal/server/config_reload.go` gained the same report, so a live reload says it too. Before
   this, a reload printed only the `niri` command binds.

The compositor's own journal was two lines long — the `WAYLAND_DISPLAY` and `NIRI_SOCKET` lines —
so on the laptop there was no way to see any of this.

## 4. The hotkey overlay advertised dead binds

`internal/ui/hotkey_overlay.go` lists Niri's "Important Hotkeys" rows, and two of them are actions
this compositor does not apply: `Mod+R` "Switch Preset Column Widths" and the "Maximize Column"
row. The overlay promised a result the compositor could not give.

The overlay now skips any action the compositor does not apply, using the same
`bind.IsSupportedAction` list the config uses.

## 5. What the audit did **not** find

Two cross-checks came back clean, which is worth recording:

- **The supported-action list and the runtime dispatcher are exactly in sync.** `IsSupportedAction`
  names 112 actions; the compositor's `executeBindAction` switch, the navigation table and the
  early-return actions cover the same 112. No action is marked supported and then ignored at
  runtime, and no action is handled at runtime while the config warns about it. The gap in §2 is a
  gap between Niri and this compositor, not a drift between the config and the compositor.
- **The IPC action path is complete.** Every action the IPC handler accepts is handled by the
  compositor: either in the server's own switch, or through the navigation table that the
  compositor's `NavigationAction` resolves by name. The actions that look missing from a naive
  comparison (`FocusColumn`, `SwapWindowLeft`, `MoveWorkspaceToIndex` and the rest) are all in the
  navigation table.

## 6. The wlroots listener sweep

The crash in [the first-login record](2026-11-03-first-login-crash-record.md) was a wlroots
assertion: a surface was destroyed while a commit listener was still attached to it, and wlroots
requires that list to be empty. So every place the compositor attaches a listener to a surface that
can be destroyed was read by hand and checked for a matching removal.

There are six such places, and all six are sound:

| Site | Commit listener | What removes it |
|---|---|---|
| `background_effect.go` | the effect surface's commit | the same surface's destroy listener, which drops the commit listener and the root's |
| `window_capture.go` | the captured surface's commit | the surface's destroy listener, which drops the commit, unmap and destroy listeners together |
| `input.go` (toplevel) | the toplevel surface's initial commit | the commit callback removes it, and the view's teardown drops it again |
| `input.go` (pointer image) | the client's pointer surface's commit | the pointer surface's destroy listener, and the request handler that replaces it |
| `cursor_shape.go` | the client's pointer surface's commit | the named-shape handler drops it, then the destroy listener that would have dropped it |
| `xdg.go` (popup) | the popup surface's commit and damage | the popup's destroy listener, which drops the commit, damage, reposition and destroy listeners together |
| `overview.go` | the overview surface's commit and unmap | the surface's destroy listener, which calls the overview's own drop helper |

Two points make the remaining cases safe. `Listener.Destroy` is idempotent — it returns at once
when the listener has already been removed — so a listener that is dropped twice, as the toplevel's
initial commit is, cannot be freed twice. And the pointer surface's destroy listener only clears
the compositor's pointers when the surface it watches is still the current one, so a surface that
was replaced leaves the newer one alone.

The one leak was the named-shape path, and it is fixed and merged. No other listener site in
`internal/wlr` attaches to a surface that can go away without the matching removal.

## 7. Test and CI state

- Every package except `internal/wlr` and `internal/server` passes when run one at a time. Those
  two need a nested compositor and are covered by the CI `compositor` lane.
- Real CI failures in the recent history, both in the `compositor` lane:
  - `TestAnimateValuePreservesSameTargetAndRetargetsSample` failed on a velocity difference of
    `1.7e-4` against a `1e-4` tolerance. The test sampled the old spring at its own `now`, while
    `animateValue` creates the new spring a few microseconds later, so the two velocities differ
    by the spring's acceleration. A timing flake, not a product fault. The test now samples the
    old spring at the retarget instant.
  - `TestPortalColorPickerReturnsPixelAndCancels` failed once with a black pixel. The colour picker
    reads the rendered buffer at the pointer position, so a stale or empty buffer under `vkms`
    gives black. Intermittent; the same branch passed later.
- The overview scroll fixture failure in the same lane is the known one tracked as `sysc-2219`.

## 8. Changes made by this audit

| Change | File |
|---|---|
| Report the accepted-but-not-applied config at startup | `internal/cli/cli.go` |
| Report it on a live reload too | `internal/server/config_reload.go` |
| Tests for both reports | `internal/cli/cli_test.go`, `internal/server/config_reload_test.go` |
| Sample the old spring at the retarget instant | `internal/wlr/animation_test.go` |
| Keep dead actions out of the hotkey overlay | `internal/ui/hotkey_overlay.go`, `internal/ui/hotkey_overlay_test.go` |

## 9. Still open

1. **The Niri width and height family** — `maximize-column`, `maximize-window-to-edges`,
   `center-column`, `center-visible-columns`, `expand-column-to-available-width`,
   `set-column-width`, `switch-preset-column-width`, `set-window-height`, `reset-window-height`.
   Nine of the thirteen dead binds. This needs a layout-level column width setter and, for the
   height family, a vertical sizing model. It is the natural next milestone.
   **Done on 2026-11-03** — see
   [the sizing plan](2026-11-03-column-width-and-window-height.md); the family applies now and the
   shipped default config leaves no accepted-but-unapplied action.
2. **Config sections that are parsed and ignored** — `window-rule` and `layer-rule` effects, and
   `workspace.layout`. The `window-rule` block in the shipped default config is dead today.
   **Corrected 2026-11-03**: `window-rule` and `layer-rule` are applied for the keys sysc supports
   (see the correction above); the shipped default's picture-in-picture rule is live. What remains
   is `workspace.layout` and the Niri keys with no counterpart, which warn.
3. **Touchscreen input** — `sysc-2227`. The compositor skips touch devices and never advertises
   `WL_SEAT_CAPABILITY_TOUCH`. **Done on 2026-11-03** — the touch tranche is on PR #91, and the
   evidence route for the gate is built (`sysc-compositor touch-probe` plus the compositor's touch
   log lines); the physical laptop gate is the owner's.
4. **The two known flakes** — the portal colour picker and the overview scroll fixture
   (`sysc-2219`). Both need the nested `compositor` lane under `vkms` to hunt properly.
5. **The physical touchpad and packaging gates** still hold PR #89 and the shell's PR #612 in
   draft, and the packaging tranche is deferred by the owner.
