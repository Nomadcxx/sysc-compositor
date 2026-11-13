# Remaining compositor parity

Goal: implement `sysc-2280`–`sysc-2286` in the owner's listed order against Niri 26.04.
Use the existing wlroots/SceneFX, config and session paths; add no dependencies.
The owner approved continuation on 2026-11-12. Physical acceptance remains separate.

For each slice: add a named failing regression, run it, extend the responsible path,
prove it passes, run the affected existing checks, update the matrix and bd. Use
`timeout 90s env GOMAXPROCS=2 go test -count=1 <package> -run '^TestName$'` locally;
run compositor tests only in the pre-approved headless laptop VM. Review locally.

1. **Workspace switches (`sysc-2280`).** Extend `internal/wm/overview.go`'s frame
   geometry to support full-scale switching. Reuse the layer projection in
   `internal/wlr/overview.go` below windows during switches. Consolidate keyboard
   switches and swipes onto `workspace_gesture.go`'s output offset, replacing the
   per-window switch animations in `workspaces.go`. Prove the 10% gap, moving
   wallpaper/bottom layers, and restoration with pixel and gesture checks.
2. **Overview shadow (`sysc-2281`).** Parse `Overview.WorkspaceShadow` through the
   existing shadow parser, with Niri's normalized defaults. Draw SceneFX shadows
   behind workspace projections, scaled by output height/1080 and zoom; prove
   defaults, off/removal and pixels. Stop the unapplied warning.
3. **Insert-hint gradient (`sysc-2282`).** Extend `InsertHint` through the existing
   gradient parser and renderer. Prove from/to, angle, relative-to and restoration
   to solid color; remove the gradient warning.
4. **DND workspace switching (`sysc-2283`).** Extend the existing DND edge tracker
   and timer, using the overview's workspace geometry and gesture offset. Parse
   trigger-height/delay-ms/max-speed; prove dwell, edge speed, drop/removal and
   pointer/touch routing. Stop the warning.
5. **Non-session D-Bus (`sysc-2284`).** Extend the server's existing service-start
   condition for the debug flag. Keep session environment import conditional on
   session mode. Prove names on a private bus and stop the warning.
6. **PipeWire modifier (`sysc-2285`).** Trace the existing cast buffer negotiation
   and Niri's flag before changing it. Apply the flag where formats originate;
   prove the advertised format and default behavior, then stop the warning.
7. **Lid policy (`sysc-2286`).** Trace switch events and output policy first.
   Implement Niri's default panel-off policy and debug override in that owner;
   prove synthetic close/open and external-output preservation. Physical lid
   acceptance stays open until the owner runs the prepared build.

After the slices: named regression batches, build/vet/format checks, local review,
and a handover with the remaining owner gates. Packaging and benchmarking stay
deferred. Do not deploy or restart the owner's running session for these changes.

## Execution record

- `sysc-2280`: shared per-output switch/swipe offset, 10% gap and moving lower-layer
  projection. Gap, transformed/cached-buffer crop, projected input, and stacked-output
  isolation checks passed in the headless VM.
- `sysc-2281`: Niri defaults through the existing shadow parser, normalized SceneFX
  workspace shadows, output isolation and empty extreme-negative-spread handling.
  Config checks and on/off pixel checks passed.
- `sysc-2282`: validated insert-hint gradients, premultiplied interpolation and a
  bounded visible-hint image using the existing image node. Pure interpolation and
  VM render/solid-restoration checks passed.
- `sysc-2283`: DND workspace dwell and edge speed in the existing timer, 1500 pixels
  per workspace, clamped reversal, native serial validation, touch grab preservation,
  drag icon/focus routing and lock cancellation. Five focused VM regressions passed
  (1.305 s), including safe release of a NULL-source touch drag over empty overview.
- `sysc-2284`: debug non-session D-Bus through the existing server path; activation
  environment and ServiceChannel stay session-only. Private-bus tests and the VM
  server/accessibility batch passed (0.447 s).
- `sysc-2285`: source review found a fixed MemFd transport, with no DMA-BUF format
  list or DRM modifier to filter. Keep the flag as a documented no-op for that
  backend and remove its unapplied warning. The config warning regression failed
  before removal and passes afterward. The cast regression now runs with the flag
  off and on and checks modifier-free negotiation and MemFd buffers. Both modes
  passed in the headless VM (0.334 s), including resize and daemon loss. Focused
  review found no issues.
- `sysc-2286`: the compositor owns the lid state from the switch and login1.
  One `outputOff` predicate, config off or Niri's derived panel policy, feeds
  reload, strict IPC/output-management preparation and apply, hotplug and
  unplug restore; the debug flag's warning is gone. An output adopted while
  PowerOffMonitors holds stays dark, as Niri keeps a connector. The nested
  regression failed with the policy neutralised and passes with it (0.32 s).
- Whole-change review found a `sysc-2283` touch-drag defect. Installed wlroots
  0.20.2 dereferences the drag source on every touch up, not only over an empty
  target, so lifting a source-less drag over a client crashed; and a drag
  finger whose surface went lost its point, so its up never ended the grab.
  Both are fixed in `touch.go`: the drag finger outlives its surface, and every
  source-less touch drag ends without a drop, as wlroots' pointer path ends one.
  `TestNativeTouchDragEndsAfterItsSurfaceIsGone` reproduced both (stuck grab,
  then SIGSEGV at `drag_handle_touch_up`) and passes.
- Final regression (headless VM, 2026-11-13): `internal/wlr` in two named
  batches, 347 pass/4 skip of 351. The first run found
  `TestOverviewTreeStacksAboveWindowsAndBelowTopLayers` still asserting the
  pre-`sysc-2280` closed order; it now checks both the closed switch backdrop
  beneath windows and the open overview above them. server 63, config 110/3
  skip (reference tests pass locally; the owner-config test needs
  `SYSC_NIRI_CONFIG`), wm 85, ui 13, session 24, cli 24, ipc 27 locally.

Source review corrected earlier assumptions: ring/border gradients had no renderer
for insert hints to reuse. Niri's lid policy disables internal panels only while
an external connector exists.

The owner requested a handover after the PipeWire slice on 2026-11-13. Continue from
[the 2026-11-13 handover](2026-11-13-handover.md); final regression and integration
remain after the lid slice.
