# Ghosting findings (`sysc-2294`, `sysc-2295`), 2026-11-14

Record of the work on [the ghosting commission](2026-11-14-ghosting-commission.md).
Branch `fix/sysc-2294-ghosting-20261114`, from `main` at `05f17da`.

## Summary

- **Root cause of the window ghost, proven on archThink: `sysc-2295`.** On
  2026-11-14 the owner killed a kitty and its contents stayed on screen, plain
  through the kitty that took its place. A scene dump of the running `3d86b14`
  session at that moment showed a stranded close fade:

  ```
  scene dump: views=3 closingNodes=1 animations=0
    tree <windowAnimationTree>
      tree at=5,42 <closing snapshot owner=4 animating=false>
        buffer at=5,42 size=953x1033 opacity=1.000
    tree <layer:windows>
      view 7 kitty (opacity 0.900), view 8 kitty (0.900), view 9 kitty (1.000)
  ```

  The owner's region screenshot from the same session (`screenshot_2026-11-14_11-39-56.png`
  on archThink) shows it too: the killed kitty's `cp -rv` output, faint, behind
  the live kitty's text. A screenshot renders the scene afresh, so the ghost was
  in the scene, not in stale buffer or panel pixels.

  The killed window's close fade (owner 4) was cancelled the moment it began,
  by the teardown of window 4, so its copy stayed at full opacity in the
  animation tree, which sits under the windows. kitty draws its background at
  0.75 and inactive windows at 0.9, so the copy shows through the windows
  above it. It is a scene node, not a stale pixel: it never fades, sits in
  output space across workspaces, and goes away only on restart.
- **`sysc-2295` is fixed.** A window's teardown cancelled any close fade whose
  owner id equalled the window's id. Close-fade owners now come from the
  window id counter, so they never equal a live window's id. Regression
  `TestCloseFadeSurvivesWindowTeardown` failed before the fix (`tearing down
  window 1 cancelled close fade 1`, and the copy outlived its fade) and passes
  after it.
- **The compositor's committed buffers hold no stale pixels in archThink's
  arrangement.** A new test hook keeps each output's last committed buffer;
  `TestCommittedFramesMatchTheScene` compares it with a fresh render after
  each step of the owner's report, on archThink's drawing settings. No step
  leaves a stale pixel, which rules out leads 2–4 for the scripted sequences on
  vkms.
- **SceneFX 0.5 does leave stale blur pixels when a frosted surface lies
  under an opaque node** (`sysc-2303`, see below). archThink's only blur is
  the bar's, with nothing opaque above it, so this is not the owner's ghost.
- **Still open:** the bar ghost in the overview is not explained by a
  stranded window copy. The fixed build runs on archThink from 2026-11-14 for
  the owner's long-session check.

## Method

`internal/wlr/committed.go` adds `committedBuffers`: while a test sets it, the
output commit listener keeps a locked reference to the buffer each commit put
on screen, and `committedImage` reads it back. `captureOutputForTarget`
renders the scene fresh into a buffer of its own, so it can never show stale
swapchain pixels. A difference between the two, once nothing animates and no
output needs a frame, is a stale pixel.

`TestCommittedFramesMatchTheScene` uses archThink's settings where they touch
drawing: `background-color "transparent"` with no wallpaper, active windows at
0.75 (kitty's `background_opacity`), inactive at 0.9, corner radius 12 with
`clip-to-geometry`, the white 2 px focus ring, no border or shadow, the owner's
springs and easings, and a 30 px top-layer bar frosted by a layer rule (the
test asserts it got a blur node). Steps: open three windows; focus left twice;
move a column right and left; close two windows together; reopen; move a window
to the workspace below and back; switch workspaces down and up; open and close
the overview; map the bar; overview with the bar; unmap the bar; overview after
it. `SYSC_GHOST_DUMP=<dir>` writes both images for a failing step.

All runs were in the laptop session VM, lane 2 (DRM on vkms, gles2 on
llvmpipe), guest tree `~/sysc-ghost` checksummed against the worktree.

## The SceneFX blur defect (`sysc-2303`)

With blur regions on the test windows (frosted, translucent when inactive), the
committed buffer kept a 26–41 px strip of stale pixels at a window edge after
every later step (5,000–6,400 pixels; for example `(467,184)` shown
`{117 42 42}`, scene `{79 13 13}`). Two toggles isolate it:

- `WLR_SCENE_DISABLE_VISIBILITY=1`: no stale pixels.
- Every window translucent (nothing opaque but the focus ring): 42 stale
  pixels, all under the opaque ring.

SceneFX culls the black background clear under opaque nodes
(`wlr_scene_output_build_state`, "Cull areas of the background"), and nodes
below an opaque node do not draw there either. A blur node below the opaque
node samples those pixels before the opaque node covers them, so the blur
mixes in whatever that swapchain buffer last held there, and the result stays
in pixels nothing damages again. A fresh buffer holds zeros there, which is
why fresh renders differ. SceneFX `main` (`54563ad`) culls the same way.
Fixing it belongs in SceneFX: keep the clear within a blur's sampling distance
of its visible region, or stop culling while a blur is drawn. sysc can only
turn visibility off scene-wide, which costs occlusion culling, direct
scan-out under fullscreen and frame-callback throttling.

Two related fixes landed in SceneFX after 0.5 and are in no release yet:
`3606f3d` (#210; 0.5 takes damage whose extents span the output for full
damage and skips blur compensation) and `0070ae5` (#221; blur samples past a
downscaled level's valid area read stale offscreen contents). A test with two
floating windows changing at opposite corners beside a frosted window, which
should trip #210, matched the scene on 0.5, so it is not in the suite.

## archThink evidence (read-only)

- A scene dump of the live `3d86b14` session (37 minutes in) showed one kitty,
  the bar with its 15 blur nodes, gslapper's background surface, the toast
  overlay, and no stranded nodes.
- `msg layers`: `sysc-shell:bar` (top), `sysc-shell-toast` (overlay),
  `slapper` (background). The bar declares no opaque region (translucent at
  95%), and kitty requests no blur, so nothing opaque sits above any blur node.
- The i915 PSR parameters are root-only; the kernel command line does not set
  them.
- The fixed build (`43b0593`) came up clean on 2026-11-15: two dumps 12 and 16
  minutes in showed `closingNodes=0` and an empty `windowAnimationTree`, with
  window ids 2, 7 and 8 live, so several close fades had already run to the end.
- eDP-1 logs `Atomic commit failed: Device or resource busy` every few minutes
  to hours, under every build, and the kernel logs i915 `Atomic update failure
  on pipe A` (vblank evasion) under Niri and sysc alike. A failed frame is not
  retried: SceneFX has already moved its damage into the ring, and sysc asks for
  no frame after a commit (`output.go`, sysc-1469). The panel keeps the frame
  before it until the next damage or cursor move schedules a frame
  (`types/output/cursor.c`), and that commit is complete, since the ring and
  `pending_commit_damage` still cover the lost frame. So it can leave a ghost
  for a few seconds, never one that lasts to a restart.

## Regression

`internal/wlr` in two named batches in the VM: 175 pass, 2 skip, then 174 pass,
2 skip, so 349 pass and 4 skip of 353 (main's 351 plus the two new tests). The
skips are the usual environment ones.

## Next

1. Owner: the fixed build (`43b0593`: main at `cc77dcd`, PR #107, the scene
   dump and this PR) is armed on archThink in
   `~/.cache/sysc-compositor-gate-20261114` (`rollback.sh` returns to
   `3d86b14`); a long session, then merge. Screenshots in that session need the
   shell's `org.sysc.screenshot` plugin, which the shell installed on
   2026-11-15 does not have; a phone photo serves.
2. If a ghost appears: the commission's capture protocol, plus the scene dump
   (any `closing snapshot … animating=false` line would mean a cancel path the
   fix missed). A diagnostic build can arm `committedBuffers` and dump
   committed vs fresh images on a signal; that tells stale compositor pixels
   (shown in the committed buffer) from the panel (absent from it).
3. If the committed buffer is clean while the panel shows the ghost: boot once
   with `i915.enable_psr=0` (lead 5).
