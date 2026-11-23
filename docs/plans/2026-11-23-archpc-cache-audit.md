# archPC backdrop cache audit

Start at `4b436bd` on `fix/archpc-backdrop-blur`. It sits on top of the cached
blur candidate `92804a3` and fixes four defects found by tracing the patch
through SceneFX 0.5. An offscreen measurement on the RTX 4060 brings the CPU
cost of cached blur under typing damage down to blur-off level. Under full
damage, each cached draw still costs about 90 µs, mostly in the kernel. The
native desktop comparison is still open. Its kit is staged and needs the owner
to sign in. [Commission](2026-11-23-archpc-backdrop-cache-handover.md)

## Fixed in 4b436bd

- **Invalidation damaged outputs that showed no blur.** It enabled the
  optimized node and marked it dirty. SceneFX then lifted occlusion below
  the node and damaged the whole output, even when no xray effect was
  visible. The next repaint disabled the node and damaged the output again.
  Under a fullscreen window, an animated wallpaper cost two whole-output
  commits per wallpaper frame and broke direct scanout on those frames.
  Invalidation now only marks the cache stale and schedules a frame. The
  repaint enables the node and marks it dirty only when a consumer is
  visible on that output. The damage then flows through the scene into
  `pending_commit_damage`, where the earlier `damage_ring`-only call never
  reached. [Cache lifecycle](../../internal/wlr/backdrop_blur.go)
- **Background-layer changes invalidated only the layer's assigned output.**
  Popups of background layers live in the layer's tree, so they belong to
  the backdrop and can cross onto another output. Layer shadows also live in
  the tree and extend past the surface. Changes are now queued and measured
  at the next repaint, after the scene has applied the commit. Every cache
  that the tree's old or new bounds overlap is invalidated. Bounds include
  every enabled node type, not only buffers. Teardown uses the last
  measured box, because every rebuild flushes the queue first.
- **A blur offset that rounds to 0 left the cache dirty.** The config
  allows an offset of 0. SceneFX's `is_scene_blur_enabled` then skips the
  rebuild, so every later repaint redrew the whole output with culling off.
  Screenshots also failed, because `renderOutputForTarget` refuses a dirty
  cache. Effects now take the live path whenever the radius is 0.
- **A render-format commit counted as a geometry change.** It ran the full
  output relayout, state refresh and IPC publish. The cached pixels do not
  depend on the output format, because SceneFX allocates the optimized
  buffers as RGBA8 regardless. The change is reverted.

The popup damage hook in `xdg.go` was redundant with the capture surface
observer and is removed.

## Verified against SceneFX 0.5

- SceneFX keeps the optimized buffers per `wlr_output` as an addon, sized
  to the render buffer. It reallocates them only when the buffer size
  changes. Mode, scale and transform commits invalidate the cache. Layout
  moves invalidate it on the next repaint. Power cycling invalidates it
  through the enable commit.
- The rebuild uses a normal transform, and cached draws sample the buffer at
  output-buffer coordinates. Rotated outputs therefore sample correctly.
  SceneFX `1d5ce45` fixes a 90°/270° transform bug in the live blur pass for
  transparency-masked layers. That fix is not in 0.5 but concerns only live
  blur. [SceneFX 1d5ce45](https://github.com/wlrfx/scenefx/commit/1d5ce45)
- Renderer loss destroys every offscreen buffer when the old `fx_renderer`
  goes, and recovery invalidates every cache. Failed recovery terminates the
  compositor, so the ordering inside `recoverRenderer` is safe.
- Screenshots render at the output's swapchain size, so they share the
  per-output buffer without reallocating it. A rebuild that does not finish
  leaves the node dirty, and output readback discards that frame.
- A windowed cached consumer samples the buffer of whichever output is
  rendering it. A window that spans two outputs builds a cache on each.
- After a rebuild, disabling the node restores occlusion, direct-scanout
  eligibility and normal damage. It also damages the output once more, so
  each rebuild costs two whole-output repaints. The second repaint draws
  cached textures only. The public API offers no way to recompute visibility
  without that damage.
- SceneFX `06fb6d8` fixes negative capture damage from live blur nodes that
  straddle two outputs. Sysc's capture builds its own whole-frame damage and
  uses no wlroots-managed capture protocol, so that crash does not apply.
  [SceneFX 06fb6d8](https://github.com/wlrfx/scenefx/commit/06fb6d8)

## Offscreen measurement

`cost-probe.go` renders a 2560×1440 headless output on the RTX 4060 render
node. The scene has striped backdrop rects and translucent windows, each with
a blur node, plus a 12×22 rectangle that changes every frame. It measures
thread CPU with `getrusage(RUSAGE_THREAD)`. It runs no compositor and
submits nothing to KMS.

| Per frame | Off | Live | Cached |
| --- | ---: | ---: | ---: |
| Typing-sized damage, 6 consumers: wall time | 0.08 ms | 1.13 ms | 0.09 ms |
| Typing-sized damage, 6 consumers: kernel time | 0.02 ms | 0.63 ms | 0.01 ms |
| Full damage, 1 consumer: wall time | 0.16 ms | 1.25 ms | 0.25 ms |
| Full damage, 12 consumers: wall time | 0.19 ms | 10.64 ms | 1.26 ms |
| Full damage, 12 consumers: kernel time | 0.01 ms | 6.28 ms | 0.69 ms |

Under typing damage, live blur reproduces the kernel-heavy pattern of the
native profile, and cached blur matches blur off. Under full damage, each
cached draw adds about 90 µs. `strace -c` counts about 4 driver ioctls per
additional cached draw, about 32 per live draw and none for rect draws. This
fits SceneFX's per-draw `fx_texture_from_buffer` for the cache. That call
allocates a texture, makes the EGL context current, rebinds the EGL image
with `glEGLImageTargetTexture2DOES` and destroys the texture afterwards.
Umbriel's fork, umbrielfx, does the same. Niri keeps a `GlesTexture` per
effect buffer. A window drag repaints the whole output, so with several blur
consumers this residual cost is worth fixing upstream. Keep the optimized
buffer's `wlr_texture` in `fx_offscreen_buffers` until the buffer is
reallocated. The renderer writes that buffer in the same context, so no
rebind is needed. That change belongs in SceneFX, not in this tree.
[SceneFX cached draw](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_pass.c#L1134) ·
[SceneFX DMA-BUF import](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_texture.c#L394)

`cache-probe.go` repeats the earlier pixel proof in the candidate's node
order: the cache starts disabled, and a rebuild enables it and then marks it
dirty. It read green, green after the opaque foreground moved, blue after
the backdrop changed and black after the backdrop was removed.

## Research sweep

- **SceneFX main after 0.5** has four relevant commits, all missing from
  archPC's `scenefx 0.5-1`. They are `1d5ce45` and `06fb6d8` above,
  `3606f3d`, which detects full blur damage by region coverage instead of
  extents, and `0070ae5`, which extrapolates screen edges instead of reading
  stale buffer contents during blur. Until `0070ae5` ships in a release,
  blur near a screen edge, cached or live, can mix in stale texels. None of these commits change the texture
  lifetime.
- **Umbriel** keeps one optimized node per output inside its
  background-layer tree. It marks the node dirty from layer commits, fades,
  maps and output changes, and scopes each layer to its assigned output.
  Its fork adds `fx_renderer_clear_output_effect_buffers` to free buffers
  when blur is off. Sysc relies on SceneFX's existing release instead.
  [Umbriel output cache](https://github.com/noctalia-dev/umbriel/blob/a909081/src/output/output.cpp#L1745)
- **Niri 26.04** blurs the backdrop once per output and shares it across
  xray effects. It recomputes the blur only when that output's backdrop
  damage changes. It keeps a separate buffer for each render target (output,
  screencast and screen capture) so that screencasting does not force
  rebuilds. Sysc shares one buffer per output, so a capture that masks a
  background layer rebuilds twice per captured frame: once masked for the
  capture, then once unmasked for the display. A screencast with a blocked
  background layer therefore runs at roughly live-blur cost.
  [Niri xray](https://github.com/niri-wm/niri/blob/2fbc6f3/src/render_helpers/xray.rs) ·
  [Window effects](https://niri-wm.github.io/niri/Window-Effects.html)
- **Hyprland** keeps blur state per monitor, as the earlier review found.
  Its forum notes that changing wallpapers and floating windows force a blur
  every frame. That is the same invalidation boundary as here. Current
  reports of NVIDIA lag on Hyprland are anecdotal and do not separate blur
  from driver issues.
  [Hyprland per-monitor blur state](https://github.com/hyprwm/Hyprland/blob/8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd/src/render/SceneResources.cpp#L75) ·
  [Hyprland blur cost](https://forum.hypr.land/t/understanding-blur-different-ways-to-set-and-performance-hit/112)
- **NVIDIA**: archPC runs 620.73.04, later than the 620.70.06 release on
  2026-11-04, which improved explicit-sync performance on Wayland. This
  sweep found no newer driver and no EGL-image change in the release notes.
  [NVIDIA 620.70.06](https://www.phoronix.com/news/NVIDIA-620.70.06-Linux-Driver)

The original source sweep missed three points. Umbriel vendors a SceneFX
fork. Niri keeps separate buffers per render target. And the post-0.5
SceneFX fixes above are not in the Arch package.

## Native gate

This is still open. Everything below runs on archPC in the owner's session.
The staged kit is in `~/.local/state/sysc-compositor-archpc-20261123/4b436bd/`.

1. Run `./arm.sh`. It validates the config, refuses to proceed if another
   archPC override is armed, and installs a drop-in. The drop-in starts the
   candidate with `SYSC_BENCH_DIAGNOSTICS` on a socket in a mode-0700
   runtime directory. Then sign out and back in.
2. For each mode, run `./bench.sh cached 20`, `./bench.sh live 20` and
   `./bench.sh off 20`. During each 20-second window, repeat the same
   cursor motion, Ghostty typing and window drag. Each run loads the mode
   over IPC (`live` forces `xray false` and `off` sets `blur { off; }`, both
   including the owner's file unchanged). It then prints deltas for
   `blur_cache_rebuilds`, `output_frames`, `scene_commits`,
   `scene_work_ns`, `presented` and `zero_copy_presents`, main-thread CPU,
   and any libinput delay reports. Finish with `./bench.sh cached 5` to
   restore the owner's config.
3. Pass criteria: with a static wallpaper, `blur_cache_rebuilds` stays flat
   during foreground interaction, the blur stays visible, and CPU and input
   delay approach the blur-off run. Repeat with an animated wallpaper and
   confirm that the blur updates on that output only. Also check a
   background-layer popup that crosses DP-1/DP-3, a rotated or scaled
   output, a power cycle and a screenshot.
4. Run `./rollback.sh` to return the next sign-in to the installed build.

If cached CPU still sits well above blur off, profile before changing the
cache. Blur off still used 33.7% of a core on stock, so the remaining cost
lies outside blur. Candidates include the snapshot acquire fences,
`wait-for-frame-completion-before-queueing`, and the 120/165 Hz VRR pacing.

## Proof and residual risks

- `go vet ./internal/wlr` passed. Three named tests passed:
  `TestBackdropBlurReusesCacheUntilItsOutputChanges`,
  `TestBackdropTreeBoundsCoverShadowsAndSkipDisabledNodes` and
  `TestBackgroundEffectReusesNodesAcrossBufferCommits`. Each of the two new
  tests failed when its fix was reverted. `TestBackgroundEffectDrawsFrostUnderTheRegion`
  skipped, because it needs a nested compositor and that run had no
  consent.
- The binary built at `4b436bd` validates the archPC config.
- The offscreen probes cover SceneFX's renderer behavior, not sysc's frame
  loop. Popups that cross outputs, capture redaction against real pixels,
  and failed rebuilds on a live output still need the nested or native run.
- Scheduling a frame on invalidation costs an idle repaint on outputs with
  no consumer. That repaint finds no damage and commits nothing.
- If allocating the optimized node fails, every cached effect toggles to
  live blur and back each frame, which repeats the damage. This only
  happens under allocation failure.

Track the performance gate under `sysc-2417`, the Acer re-enable under
`sysc-2416` and the SceneFX visibility defect under `sysc-2303`. Follow
`AGENTS.md`.
