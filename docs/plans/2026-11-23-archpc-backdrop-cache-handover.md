# archPC backdrop cache handover

Start at `92804a3` on `fix/archpc-backdrop-blur`. This adds Niri-style cached
blur on top of the snapshot and modeset candidate. On stock `672850b` with
matched NVIDIA 620.73.04, archPC's compositor thread used 92% of one core with
blur on, 34% with it off and 93% after restoration. SceneFX's live blur pass
accounted for 77% of sampled userspace cycles. [Baseline and source review](2026-11-23-archpc-nvidia-handover.md)

## Implementation and proof

- Windows and layers with blur sample a shared SceneFX backdrop for each
  output. Popups default to live blur and `xray false` selects live blur.
  Sysc also uses live blur inside the wallpaper layer to avoid sampling
  the cache while drawing its source. This change implements xray for
  blur. Niri also supports see-through xray without blur.
  [Rule resolution and effect projection](../../internal/wlr/background_effect.go)
- A cache is created on demand for a visible effect. It rebuilds after
  backdrop damage, output changes, graphics recovery, or blur settings
  changes. SceneFX exposes pixels beneath occluders on rebuild. Sysc
  scopes a full clear to that pass, then disables the scene marker to
  restore occlusion. The cached framebuffer remains available to draws.
  The ordinary repaint path counts rebuilds in `blur_cache_rebuilds`.
  [Cache lifecycle](../../internal/wlr/backdrop_blur.go) ·
  [Repaint and instrumentation](../../internal/wlr/debug.go)
- Capture invalidates the shared cache when it masks a background layer,
  and invalidates it again after restoring the normal scene. Output
  readback discards the frame if a required rebuild fails. The privacy
  path still needs device-backed coverage for a blocked layer and a
  translucent foreground over it. [Capture masking](../../internal/wlr/capture_privacy.go) ·
  [Output readback](../../internal/wlr/screenshot.go)
- Seven named wlroots tests, three config tests and the diagnostics test
  passed. The cache regression failed with cache creation removed. The
  standalone offscreen probe used the RTX 4060 render node, a headless
  output and an explicitly supplied GBM swapchain. It read green through
  the initial cache, green again after moving an opaque foreground,
  blue after changing the backdrop, and black after removing the backdrop.
  It measured pixels from the renderer without creating a desktop
  compositor or submitting to physical KMS.
- The binary and the offscreen probe are staged at
  `~/.local/state/sysc-compositor-archpc-20261123/92804a3/`.
  The build validated the archPC config. The active desktop and next
  sign-in use stock. To repeat the renderer probe on archPC, run
  `WLR_RENDER_DRM_DEVICE=/dev/dri/renderD128 go run cache-probe.go`
  from the staging directory. The probe uses the installed wlroots and
  SceneFX libraries.

## Commission for the receiving developer

Audit the patch before deploying it for native acceptance. Read the
SceneFX 0.5 implementation alongside the sysc diff and trace which
backdrop each draw can sample. Check both outputs after rotation, scaling,
format changes, power cycling and renderer loss. Include background
subsurfaces and popups that cross an output edge. Source damage currently
uses the layer's assigned output. Verify that this covers each output that
can sample the layer, and widen invalidation if it does not. Verify
capture redaction and failed cache rebuilds against actual pixels. Check
that stopping cache rebuilds also restores the expected damage and
direct-scanout behavior.

Do an independent research sweep. Start with current SceneFX and Umbriel
cache invalidation and buffer lifetime, then compare Niri/Smithay and
Hyprland on NVIDIA. Check driver and renderer changes since the profiled
versions. SceneFX 0.5 still rebinds a 2D EGL image when sampling its cached
texture. The stock profile put 28% of userspace cycles under
`fx_texture_from_buffer` and did not resolve the kernel callers.
Measure whether cached drawing still incurs that cost before proposing
a renderer texture-lifetime change. Review alternative explanations for
residual input stalls, including GPU synchronization and KMS submission.
Record anything the original source sweep missed. [SceneFX cached sample](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_pass.c#L1114) ·
[SceneFX EGL rebind](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_texture.c#L350)

Run the desktop comparison on archPC with the 120 Hz Acer and 165 Hz
Samsung, a fixed workload, and a matched NVIDIA stack. Start the
candidate with `SYSC_BENCH_DIAGNOSTICS` pointing at a private Unix socket
in a mode-0700 directory. Record deltas for `blur_cache_rebuilds`,
`output_frames`, `scene_commits`, `scene_work_ns` and `presented`.
Compare cached blur, an explicit live-blur rule for the same clients,
and blur off. Record compositor main-thread CPU and input delay during
cursor motion, terminal typing and window drag. With static backgrounds,
foreground interaction should leave the rebuild counter flat while blur
stays visible. Repeat with an animated wallpaper and verify that it
updates on the relevant output. Use the blur-off CPU and latency as the
reference. The offscreen proof leaves this performance gate open.

Track performance under `sysc-2417`, Acer re-enable under `sysc-2416`,
and the separate SceneFX visibility defect under `sysc-2303`. Follow
`AGENTS.md` for named tests, compositor-thread ownership and explicit
consent for any nested desktop run.
