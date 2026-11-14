# Ghosting findings (`sysc-2294`, `sysc-2295`), 2026-11-14

Record of the work on [the ghosting commission](2026-11-14-ghosting-commission.md).
Branch `fix/sysc-2294-ghosting-20261114`, from `main` at `05f17da`.

## Summary

- **`sysc-2295` is fixed.** A window's teardown cancelled any close fade whose
  owner id equalled the window's id, which stranded the fading copy in
  `windowAnimationTree`: output space, at the opacity it had reached, never
  destroyed. Close-fade owners now come from the window id counter, so they
  never equal a live window's id. Regression `TestCloseFadeSurvivesWindowTeardown`
  failed before the fix (`tearing down window 1 cancelled close fade 1`, and
  the copy outlived its fade) and passes after it.
- **The compositor's committed buffers hold no stale pixels in archThink's
  arrangement.** A new test hook keeps each output's last committed buffer;
  `TestCommittedFramesMatchTheScene` compares it with a fresh render after
  each step of the owner's report, on archThink's drawing settings. No step
  leaves a stale pixel. That rules out leads 2–4 (missing damage, opaque
  culling, blur compensation) for the scripted sequences on vkms.
- **SceneFX 0.5 does leave stale blur pixels when a frosted surface lies
  under an opaque node** (`sysc-2303`, see below). archThink's only blur is the
  bar's, with nothing opaque above it, so this is probably not the owner's
  ghost.
- **The owner's ghost is not yet proven.** A stranded close fade
  (`sysc-2295`) fits the persistent window ghost: output space, fixed across
  workspaces, faint because kitty draws at `background_opacity 0.75`, intense
  when copies stack. It does not explain the bar ghost in the overview. Lead 5
  (i915 PSR2 selective update) is the next candidate; the evidence must come
  from archThink.

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

## Regression

`internal/wlr` in two named batches in the VM: 175 pass, 2 skip, then 174 pass,
2 skip, so 349 pass and 4 skip of 353 (main's 351 plus the two new tests). The
skips are the usual environment ones.

## Next

1. Owner: merge, then arm the fixed build on archThink for a long session.
2. If a ghost appears: the commission's capture protocol, plus the scene dump
   (any `closing snapshot … animating=false` line would mean a cancel path the
   fix missed). A diagnostic build can arm `committedBuffers` and dump
   committed vs fresh images on a signal; that tells stale compositor pixels
   (shown in the committed buffer) from the panel (absent from it).
3. If the committed buffer is clean while the panel shows the ghost: boot once
   with `i915.enable_psr=0` (lead 5).
