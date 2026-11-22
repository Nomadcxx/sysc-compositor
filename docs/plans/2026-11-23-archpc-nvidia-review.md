# archPC NVIDIA rendering review

## Native evidence

archPC reproduced mouse lag, slow Ghostty resize, edge flicker, and a flash of
the background color on sysc-compositor `672850b`, wlroots 0.20.2, SceneFX 0.5,
and an RTX 4060 running NVIDIA 620.70.06. Libinput logged mouse delivery up to
84 ms late. The compositor thread used around 90% of a core in several short
samples, including 59–62% of a core in kernel time. It waited under 1 ms for
CPU scheduling in each of three two-second samples. wlroots calls run on the
same thread that dispatches input, so long render or KMS operations can delay
mouse events. The measurements do not identify which operation took 84 ms.

The NVIDIA package upgrade began after reproduction. The running compositor
kept its original EGL libraries and the kernel kept its loaded 620.70.06
module. Newly launched processes began reporting a mismatch with the 620.73.04
libraries. Capture used the existing compositor process. A fresh cross-compositor
run needs a matched loaded module and userspace.

| Change on archPC | Main-thread CPU, percent of one core |
| --- | --- |
| VRR off | 90% to 94% |
| Outputs matched near 120 Hz with VRR off | 78% to 74%, then 84% after restoring 120/165 Hz |
| Static PNGs and VRR off, Samsung only | 68.3% with two outputs to 67.2% with one |

The load persisted under each condition. The samples do not isolate the cost
of wallpaper decoding or measure 60 Hz. The laptop run also used a different
build and display setup, so it does not isolate the GPU or panel refresh rate.

A 7-second cycle profile attributed 66.68% of userspace cycles to NVIDIA EGL
core. A 10-second DWARF profile attributed 62.48% to the same library, 4.42%
to SceneFX, 2.00% to sysc and 0.85% to wlroots. Recovered stacks included
`sysc_debug_repaint → wlr_scene_output_build_state → scene_entry_render →
fx_render_pass_add_blur` (4.76% of userspace samples) and
`wlr_scene_blur_create` (0.79%). The package upgrade replaced the old NVIDIA shared objects on disk, so
most NVIDIA samples have no recovered caller. Raw
profiles remain on archPC in `/tmp/sysc-archpc-perf.sDRlZ5/`.

A separate 600-sample run at 18 ms intervals measured 27.4% of one core in
userspace and 44.4% in kernel space. The thread was runnable in 428 samples,
in `do_epoll_wait` in 149, in NVIDIA's `os_acquire_rwlock_write` in 17 and
`os_acquire_rwlock_read` in 5, with one unresolved wait. The lock samples
show driver contention but do not account for the kernel CPU time. Kernel
cycle profiling and stack access require privileges unavailable in this
session. Frame counters were not enabled at startup. The existing profile
cannot distinguish excess frames from excess work per frame.

## Damage and frame production

On `672850b`, a foreground surface's ordinary buffer commit calls
`syncBackgroundEffect(effect, true)` even when its effect region, position
and size stay the same. Sysc destroys and recreates all of that effect's
blur nodes. SceneFX damages the old and new scene areas and evaluates live
blur for the affected rectangles. Static wallpapers still exercise this
path when an effect surface redraws. Both node creation and blur rendering
show up in the native profile. How much lag disappears when node churn stops
is open. [Sysc effect commit and redraw](../../internal/wlr/background_effect.go)

- Niri 26.04 defaults window and layer effects to xray when blur is active,
  so a changing foreground surface can reuse a blurred wallpaper or backdrop.
  Popups default to live blur. [Niri effects documentation](https://github.com/niri-wm/niri/blob/8ed0da44d974c32c6877d2f4630c314da0717ecb/docs/wiki/Window-Effects.md#xray)
- Hyprland tracks the cached blur framebuffer and its dirty flag per monitor.
  Its render pass can still request live blur when the background cache does
  not represent what lies beneath the surface. [Hyprland scene resources](https://github.com/hyprwm/Hyprland/blob/8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd/src/render/SceneResources.cpp#L75)
- Umbriel retains a per-output optimized blur node, updates its size and
  position in place, and dirties it when background inputs or output geometry
  change. [Umbriel blur updates](https://github.com/noctalia-dev/umbriel/blob/3d7052fc3156de832edb845327d0cfe815c2ff50/src/output/output.cpp#L1745)

Sysc now retains blur nodes under the same scene parent when the rectangle count
stays the same. It updates geometry and corners in place and maintains the
order beneath the surface. SceneFX leaves unchanged setters and already
adjacent siblings alone. The device-free regression checks repeated updates
and a geometry and corner change with two rectangles. Cached backdrop
selection would change what the effect samples and needs its own damage rules.

Sysc repaints on output frame events, skips rendering when the scene does
not need a frame, and dispatches frame callbacks after repaint attempts.
Umbriel also sends callbacks on no-damage frames to keep FIFO clients moving.
Niri schedules an estimated vblank on no damage and samples animations at
each output's predicted presentation time. Sysc's shared 16 ms animation
timer can undersample motion on a 120/165 Hz output. The timer is a resize
pacing concern. The source does not explain sustained load outside animations
with that timer alone. [Sysc frame handler](../../internal/wlr/output.go) ·
[Sysc animation timer](../../internal/wlr/animations.go) ·
[Umbriel frame-event path](https://github.com/noctalia-dev/umbriel/blob/3d7052fc3156de832edb845327d0cfe815c2ff50/src/output/output.cpp#L2200) ·
[Niri no-damage scheduling](https://github.com/niri-wm/niri/blob/8ed0da44d974c32c6877d2f4630c314da0717ecb/src/backend/tty.rs#L1988) ·
[Niri animation clock](https://github.com/niri-wm/niri/blob/8ed0da44d974c32c6877d2f4630c314da0717ecb/src/niri.rs#L4578)

## Buffer ownership and presentation

Client acquire, client release, and KMS readiness each protect a different
part of a frame. Acquire gates reading the client's buffer. Release gates
client reuse after sampling. A render-completion fence tells KMS when it can
scan out the composed framebuffer. SceneFX 0.5 waits on client points at
texture sampling and supplies a render-completion point when the output has
a timeline. wlroots exports that point as a KMS input fence. The running sysc
advertised DRM syncobj, which confirms protocol availability. Client and
output fence selection remain unmeasured on the affected frames.
[SceneFX texture wait](https://github.com/wlrfx/scenefx/blob/0.5/render/fx_renderer/fx_pass.c#L421) ·
[SceneFX output state](https://github.com/wlrfx/scenefx/blob/0.5/types/scene/wlr_scene.c#L3480) ·
[wlroots atomic fence export](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/backend/drm/atomic.c#L365)

- Niri uses Smithay to queue output work. Smithay checks the KMS plane for
  `IN_FENCE_FD` and disables that path on NVIDIA drivers older than 560.35.03.
  archPC reproduced on 620.70.06. [Niri DRM submission](https://github.com/niri-wm/niri/blob/8ed0da44d974c32c6877d2f4630c314da0717ecb/src/backend/tty.rs#L1929) ·
  [Smithay KMS capability check](https://github.com/Smithay/smithay/blob/ff5fa7df392cecfba049ffed55cdaa4e98a8e7ef/src/backend/drm/compositor/mod.rs#L1183)
- Hyprland exports a native fence to KMS and retains sampled client buffers
  through GPU completion. It waits synchronously when it cannot provide
  the required fence. [Hyprland submission and retention](https://github.com/hyprwm/Hyprland/blob/8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd/src/render/GLRenderer.cpp#L153)
- Umbriel waits asynchronously for the client acquire fence to signal before
  applying the commit. Unmodified wlroots waits for the fence to materialize
  and gives the renderer the dependency. Umbriel also handles unmap and
  superseded release. Its render pass uses a native fence when it has a
  signal timeline. Its composed-output NVIDIA wait covers the fallback where
  the driver ignores implicit fences. These conditions give specific things
  to measure before changing wait behavior. [Umbriel acquire barrier](https://github.com/noctalia-dev/umbriel/blob/3d7052fc3156de832edb845327d0cfe815c2ff50/src/server/syncobj_acquire_barrier.cpp#L48) ·
  [Umbriel EGL submission](https://github.com/noctalia-dev/umbriel/blob/3d7052fc3156de832edb845327d0cfe815c2ff50/umbrielfx/render/fx_renderer/fx_pass.c#L485) ·
  [wlroots syncobj protocol](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/types/wlr_linux_drm_syncobj_v1.c#L235)

Niri associates the resize snapshot with a matching client commit. Sysc
starts on the layout request and temporarily substitutes a scaled scene
clone. The original clone reused the client's buffer without its acquire
point or a listener for GPU sampling. The snapshot now copies the acquire
point, pins the original commit's release state to the locked buffer, and
adds the completion points from SceneFX and custom animation shaders.
The device-free checks cover acquire and release ownership. A resize flicker
could also come from the live-to-snapshot handoff, geometry clipping, or
blur damage. Capture the committed output and a fresh scene render at the
same resize step before assigning the color flash.
[Niri resize commit](https://github.com/niri-wm/niri/blob/8ed0da44d974c32c6877d2f4630c314da0717ecb/src/handlers/xdg_shell.rs#L1516) ·
[Sysc scene clone](../../internal/wlr/scene_fx.go) ·
[wlroots sample listener](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/types/scene/surface.c#L160)

## Acer re-enable

The Acer went dark in the one-output test. An IPC on request failed and a
config reload restored its 120 Hz mode and VRR. The per-output test prepares
a temporary modeset buffer on a shallow copy of the state. Sysc then copies
the caller's bufferless state into a grouped backend test. That backend
does not synthesize a framebuffer and rejects an active connector with no
primary framebuffer. The separate config reload uses the per-output commit,
which can supply a buffer itself. This source path explains the different
outcomes. The hardware run did not capture the backend's exact rejection.

Umbriel prepares a swapchain and builds scene state into each pending output
before the grouped commit, so the transaction carries its framebuffer. Sysc
now uses wlroots' per-output test and commit for one output. For a native
group, sysc negotiates swapchains and supplies cleared framebuffers where
needed, with render-completion points when supported. It adopts the
swapchains after a successful commit and schedules a full scene repaint.
The device-free regression covers repeated re-enable and a resolution
change in a two-output group. The Acer still needs a native re-enable test.
[Sysc grouped output config](../../internal/wlr/output_config.go) ·
[wlroots per-output test](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/types/output/output.c#L718) ·
[wlroots DRM connector validation](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/backend/drm/drm.c#L917) ·
[Umbriel modeset preparation](https://github.com/noctalia-dev/umbriel/blob/3d7052fc3156de832edb845327d0cfe815c2ff50/src/server/server_events.cpp#L3994)

## Native test sequence

1. On a matched stack, capture quiet and input samples using
   `SYSC_BENCH_DIAGNOSTICS` on the baseline. Compare frame
   events, submitted commits, presentations, repaint time and input delay
   with blur on and off, keeping outputs and clients fixed. Use a blur-node
   reuse build as its own comparison. If the load persists, time scene build
   separately from output commit and obtain a kernel profile with privileges.
2. Reproduce Ghostty resize with the same output and application settings on
   Niri, then compare sysc with resize animations enabled and disabled.
   Capture the client commit and GPU completion timing before attributing
   any change to synchronization. Compare committed-frame and fresh-render output.
3. Validate strict off/on, repeated re-enable, and failure recovery on the
   two-output setup before testing mixed refresh and VRR changes again.
   Track lag in `sysc-2417` and re-enable in `sysc-2416`.

The live session stayed up. The next-login override was withdrawn, so
`/usr/local/bin/sysc-compositor --session` is selected.
