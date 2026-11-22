# archPC NVIDIA lag handover

The next implementation should move default blur to a shared per-output
backdrop cache. The live archPC experiment locates the lag in the current
blur path. A native test still has to show that cached blur preserves
the effect and gets CPU and input latency close to the blur-off result.
Track this in `sysc-2417`. [Earlier source review](2026-11-23-archpc-nvidia-review.md)

## Native evidence

archPC rebooted into sysc-compositor `672850b`, with NVIDIA 620.73.04 loaded
in the kernel and userspace. The RTX 4060 drove the Acer on DP-1 at 120 Hz
with VRR and the Samsung on DP-3 at 165 Hz. The owner reproduced cursor,
drag, and typing lag. Libinput logged mouse delays up to 143 ms and keyboard
delays up to 49 ms. Libinput rate-limited subsequent reports.

I loaded a temporary KDL file through the compositor IPC. It included the
owner's config and added `blur { off; }`. I waited for `ConfigLoaded`,
sampled PID 4769's main thread for about six seconds, then reloaded the
original file and repeated the sample. Reported output modes and VRR
settings matched throughout. The experiment made no edits to the owner's
config files.

| Stock `672850b` | User CPU | Kernel CPU | Total, percent of one core |
| --- | ---: | ---: | ---: |
| Blur on | 33.8% | 58.3% | 92.1% |
| Blur off | 31.0% | 2.7% | 33.7% |
| Original config restored | 34.1% | 59.0% | 93.1% |

The thread was runnable in 311 of 330 baseline samples and 306 after
restoration. With blur off it waited in epoll in 207 samples. NVIDIA kernel
lock waits also fell from 18 samples to one. System GPU utilization
overlapped across the short windows. An earlier static-wallpaper run and
single-monitor run still showed load. [Earlier native evidence](2026-11-23-archpc-nvidia-review.md#native-evidence)

The post-reboot blur-on profile recorded 586 samples of `cycles:u`. The
repaint accounted for 82.98% including callees, `fx_render_pass_add_blur`
for 76.63%, and `fx_texture_from_buffer` for 28.39%, mostly under the blur
path. Blur node creation accounted for 0.17%. The flat report placed 65.91%
in NVIDIA EGL core. These percentages describe userspace cycles. Kernel
stack profiling was unavailable without OS privilege. The profile and
reports are in `/tmp/sysc-postboot-profile.rqQOqo/` on archPC for this boot.

## Implementation target

Sysc creates a live blur node for each clipped effect rectangle. SceneFX
grows damage to cover the blur's sample region, then repeatedly runs a blur
pass against the partially composed output. That work runs on the same
thread as libinput. Commit `c90ab7f` makes node geometry updates cheaper and
can cut damage from node churn. A visible live blur can still run on each
damaged repaint. [Sysc effect projection](../../internal/wlr/background_effect.go) ·
[SceneFX damage expansion](https://github.com/wlrfx/scenefx/blob/0.5/types/scene/wlr_scene.c#L2865) ·
[SceneFX blur render](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_pass.c#L1101)

Use Niri's default xray behavior for windows and layer surfaces with blur.
Populate one blurred background per output and let every visible effect
region on that output sample it. Keep popups and explicit `xray false`
on live blur. Retain the protocol region, clipping, and popup geometry.
Sysc already parses the xray key and currently issues a warning for it.
[Sysc rule parser](../../internal/config/rules.go#L1171) ·
[Niri effect semantics](https://github.com/niri-wm/niri/blob/8ed0da44d974c32c6877d2f4630c314da0717ecb/docs/wiki/Window-Effects.md#xray)

Start with SceneFX 0.5's existing optimized blur node and
`wlr_scene_blur_set_should_only_blur_bottom_layer`. Put the cache at the
wallpaper/backdrop boundary, below any foreground content. Own the node
with output adoption, placement, mode/scale/transform changes and teardown.
Mark it dirty on creation and on changes to the background it samples,
including map, unmap, movement and damaged wallpaper buffers. Foreground
typing or resize must leave that cache valid. Umbriel uses an optimized
node for each output and invalidates it when its background changes.
[SceneFX cache node](https://github.com/wlrfx/scenefx/blob/0.5/types/scene/wlr_scene.c#L1285) ·
[Umbriel output cache](https://github.com/noctalia-dev/umbriel/blob/3d7052fc3156de832edb845327d0cfe815c2ff50/src/output/output.cpp#L1745) ·
[Hyprland per-monitor blur state](https://github.com/hyprwm/Hyprland/blob/8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd/src/render/SceneResources.cpp#L75)

SceneFX 0.5 skips the live blur kernel and its damage expansion for cached
full-strength blur. It still calls `fx_texture_from_buffer` when drawing
each rectangle. For its 2D DMA-BUF texture path, SceneFX reuses the EGL
image and GL texture name and calls `glEGLImageTargetTexture2DOES` again.
If the native cached path still spends substantial time there, measure
that call directly and move texture reuse into the renderer. The current
profile attributes time to the function and its callees. An individual
driver call's kernel cost remains unmeasured. Also verify that a cache
refresh reads a clean backdrop underneath opaque surfaces. The SceneFX
visibility-culling issue remains a distinct upstream item, `sysc-2303`.
[SceneFX cached sampling](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_pass.c#L1114) ·
[SceneFX EGL binding](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_texture.c#L350)

## Pickup and native acceptance

- Start at `644e18c` on local branch `fix/snapshot-acquire-sync`.
  It contains the node reuse, snapshot fence and output re-enable changes.
  The rebuilt binary is under
  `~/.local/state/sysc-compositor-archpc-20261123/644e18c/`. It passed eight
  targeted tests and config validation. The native slowdown measurements
  above all came from stock `672850b`. I withdrew the next-login override
  after the owner asked for a renderer fix. PID 4769 stayed up.
- On archPC, use the same outputs, foreground workload and effective
  config for the cached candidate and blur-off control. Enable
  `SYSC_BENCH_DIAGNOSTICS` and record frame events, commits, repaint time,
  and presentations. Count cache rebuilds and cached draws. Record
  compositor thread CPU and input dispatch delays during cursor movement,
  Ghostty typing, and drag/resize. With a static backdrop, the rebuild
  count should stay flat during foreground interaction while blur stays
  visible. The cached run should approach the blur-off CPU result and
  remove the input stalls before calling the lag fixed.
- Check wallpaper damage, a move between outputs, rotated or scaled
  output, config reload, capture, and popup live blur. Check the Ghostty
  color flash separately if it persists. Verify physical Acer re-enable
  under `sysc-2416` outside the performance comparison.
- Follow `AGENTS.md`. Keep wlroots work on the compositor thread. Run
  one package with named tests. Any nested desktop run needs consent for
  that run. The source and native data collected here came from archPC.
