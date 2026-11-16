# Commission: stale ghosts on screen (`sysc-2294`)

Hunt down and fix whatever makes sysc-compositor leave stale images of windows
and the bar on screen. This is a beta blocker. The owner's observations are
actionable even though the bug is not reproducing right now (just after a
reboot and a Niri ↔ sysc switch): build the evidence, then fix the root cause.

Read `AGENTS.md`, [the docs index](../README.md) and this document. The rules in
[the continuation handover](2026-11-14-handover.md) apply, notably: one package
and named tests at a time, compositor tests only in the headless laptop VM or
with the owner's per-run consent, `runtime.LockOSThread`, Go + cgo only, **no AI
attribution in commits or PRs**, and the commit hook's banned substrings.

## The report

On archThink (ThinkPad T490s, Intel UHD 620, eDP-1 1920×1080, InfoVision
R140NWF5 IPS) under sysc-compositor, across builds (at least `86aac7c` and
`d6357d6`/`3d86b14`):

1. After a terminal has been open for 20–30 minutes, moving or closing it leaves
   a faint ghost of the window and its contents where it was.
2. The ghost does not fade. A restart clears it. It never happens under Niri or
   other compositors on the same panel, so it is not panel image retention.
3. Mod+O (overview) shows pronounced ghosts of the bar and of terminal window
   borders. After the bar is turned off in the shell's bar display settings, its
   ghost still shows in the overview.
4. Outside the overview the ghost is subtle: zooming into Zen shows terminal
   borders behind it. It gets worse after workspace switches, stays fixed to the
   screen across workspaces, and is sometimes intense.
5. It is most visible with the void theme and a black background, and when the
   wallpaper is not running (see `sysc-2298`).

Read these as constraints. Borders are SceneFX rects/borders, not client
buffers; a hidden bar has no mapped surface; "fixed to the screen" means
output-space, not workspace-space; "worse after switches" means large moves.

## Owner configuration that matters

From archThink `~/.config/sysc-compositor/config.kdl` (+ `sysc-shell.kdl`):

- `window-rule { match is-active=false; opacity 0.9 }` — every inactive window
  is translucent.
- `layer-rule { match namespace="^quickshell$"; place-within-backdrop true }`.
- `overview { workspace-shadow { off } }`; no `blur` block (defaults).
- sysc-shell theme: `bar-opacity 95`, `panel-opacity 92`, `blur-radius 49`,
  palette seed `void`. The bar is a 1920×49 top-layer buffer with **15 blur
  nodes** (corner strips). An overlay-layer surface 404×1080 sits at x=1516.
- Lock is `sysc-lock`; idle lock after 9h30m (no idle blank near 20–30 min).

## Ruled out, with evidence (2026-11-13/14)

| Candidate | Why not |
|---|---|
| Panel image retention / PSR as the sole cause | Only under sysc; does not fade; borders and hidden bar. (PSR interplay with sysc's damage clips is not fully excluded; see lead 5.) |
| Overview copying hidden layers | `projectOverviewLayer` requires `surface.mapped` (`internal/wlr/overview.go`); borders are not buffers, so projection cannot copy them. |
| Background-effect blur nodes leaking | Rebuilt on every commit, placed in the surface's own tree, destroyed on surface destroy (`background_effect.go`). |
| SceneFX optimized-blur cache | sysc never creates `WLR_SCENE_NODE_OPTIMIZED_BLUR`. |
| Window opacity bypassing damage/opaque logic | Applied with `wlr_scene_buffer_set_opacity`; SceneFX 0.5 `scene_node_opaque_region` returns empty for opacity ≠ 1 and subtracts corners. |
| Buffer clipping misreporting opaque regions | `sceneFXClipTree` only clips fresh copies (overview, switcher); `clip-to-geometry` uses `wlr_scene_subsurface_tree_set_clip`. |
| Capture renders corrupting the damage ring | Captures call `wlr_scene_output_build_state` with a temporary swapchain; wlroots 0.20.2 `wlr_damage_ring_rotate_buffer` plus `buffer_handle_destroy` squash the temporary entry's damage back into `ring->current`. |
| sysc's custom repaint | `sysc_debug_repaint` (`debug.go`) equals `wlr_scene_output_commit` when the debug toggles are off. |
| Screen transitions | Nothing calls `do-screen-transition` (sysc-shell, owner configs). |
| Idle/DPMS round trips | Idle lock is 9h30m. |

Static reading is exhausted. Do not repeat it; get runtime evidence.

## Leads, most likely first

1. **Animation cancellation strands scene nodes (`sysc-2295`, confirmed
   defect).** `cancelAnimation` and `cancelViewAnimations` delete an animation
   without running `done`. Close-fade snapshots are destroyed only in `done`
   (`startCloseSnapshot`, `animations.go`). Snapshot owners come from
   `nextAnimationID`, window ids from `nextViewID`; both start at 1, so
   `cancelViewAnimations(view)` can cancel another window's close fade and leave
   its copy in `windowAnimationTree` at partial opacity, fixed to the screen.
   Audit every animation whose `done` destroys or hides a node (close snapshot,
   `finishResizeIfSettled`, shader effects, switcher, screenshot UI) and every
   path that cancels without `done`. A stranded copy explains a faint persistent
   window ghost but not, alone, a ghost after a plain move or a hidden bar.
2. **Missing damage.** A visual change that never damages its old region leaves
   the previous pixels in the swapchain buffers, and later blends over them show
   through translucent surfaces (inactive windows at 0.9). Suspects: direct
   field writes or SceneFX setters that do not damage, the workspace-switch
   offset and lower-layer projection from `sysc-2280` (`workspace_gesture.go`,
   `overview.go`), focus ring/border/shadow updates on move, the switch backdrop
   when the overview is closed. "Worse after workspace switches" points here.
3. **Opaque culling.** SceneFX skips clearing and drawing under regions it
   believes opaque (`scene_node_opaque_region`, render-list culling). Anything
   drawn translucent but reported opaque leaves stale pixels beneath. Check
   rects (borders, focus ring, backdrop, background-color) and buffers whose
   `buffer_is_opaque` disagrees with what is drawn.
4. **SceneFX blur artifact compensation.** With 15 bar blur nodes, SceneFX's
   `blur_padding_region` copy/paste around damaged blur regions reuses pixels
   from the target buffer. Wrong padding around moving content could reintroduce
   old pixels.
5. **i915 PSR2 selective update with sysc's damage clips** (only if 1–4 come up
   empty): sysc's per-frame damage differs from Niri's. Test once with
   `i915.enable_psr=0` (owner runs it).

## Tools already built

- **Scene dump:** branch `diag/scene-dump-20261113` (`3d86b14`, pushed; not for
  main). `SIGUSR1` writes every scene node with owner label, position, size and
  opacity to the journal (`journalctl --user -u sysc-compositor.service`). Armed
  on archThink at `~/.cache/sysc-compositor-gate-20261113/3d86b14` via
  `~/.config/systemd/user/sysc-compositor.service.d/10-touch-gate.conf`;
  `rollback.sh` there restores `86aac7c`. A clean baseline dump (session start)
  showed no stranded nodes.
- **Full redraw:** `sysc-compositor msg action debug-toggle-damage` (twice)
  damages every output whole. If a visible ghost vanishes, it was stale pixels
  (lead 2/4); if it stays and the dump has no stray node, suspect lead 3.
  `debug-toggle-opaque-regions` paints what sysc treats as opaque. Note:
  `sysc_debug_scene` overwrites `debug_damage_option`, so `WLR_SCENE_DEBUG_DAMAGE`
  from the environment has no effect; add a rerender toggle to the diagnostic
  build if you need one.
- **Fresh render:** `grim` or `msg action screenshot-screen` render the scene
  fresh (`renderOutputForTarget`), so they never show stale swapchain pixels. A
  ghost on screen but absent from them is stale pixels.

Capture protocol when the owner reports a visible ghost: dump → `grim` → damage
toggle on/off (owner says whether the ghost vanished) → opaque-regions on/off.

## Reproduce without the owner

The bug needs reproduction you can run. Pixel tests that capture through
`renderOutputForTarget` cannot see it. Read back the buffer actually committed
to the output instead: add a test helper that copies the scene output's last
committed buffer (keep a locked reference in the commit listener) and compare
it with a fresh render after a scripted sequence — two translucent windows on a
black background, close-fade of one while another closes, moves between columns,
workspace switches up/down, overview open/close, a layer surface unmapping.
Run in the headless laptop VM (`bash run.sh lane2` in `~/vm/sysc-session`, SSH
in ~14 s; wrapper `~/lane2-m15-closeout2.sh`, guest tree `~/sysc-m15-closeout2`;
sync and checksum the tree before trusting a run). Nested runs on the NVIDIA
desktop do not work (`sysc-2296`).

## Deliverable

1. Root cause stated with runtime evidence (dump, committed-buffer readback,
   or the damage-toggle result).
2. A failing regression that reproduces it, then the fix at the owning layer.
   Fix `sysc-2295` regardless, with its own regression.
3. `internal/wlr` regression in two named batches in the VM (last: 348 pass, 4
   skip of 352 on `feat/output-config-resume-20261113`).
4. A PR (no AI attribution). Arming a fixed build on archThink and the owner's
   long-session verification need the owner's approval; the owner closes
   `sysc-2294`.

Coordinate with the continuation agent: this commission owns `internal/wlr`
rendering, animation and damage code until it lands.
