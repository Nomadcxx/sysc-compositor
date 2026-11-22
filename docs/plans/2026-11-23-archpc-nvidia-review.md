# archPC graphics and resize review

## Native run

archPC ran sysc-compositor `0.1.0-13-g672850b`, wlroots 0.20.2, SceneFX 0.5,
and the RTX 4060 with NVIDIA 620.70.06. The owner saw mouse and resize lag and
flicker or a solid color during Ghostty resize. Libinput logged mouse events
up to 84 ms late. The compositor thread used around 90% of a core in several
samples, including 59–62% of a core in kernel time. It spent under 1 ms waiting
for a CPU in each of three two-second samples. The session started before the
driver update at 06:40 AEDT. The original compositor still held its 620.70.06
EGL libraries when newly launched clients began reporting the version mismatch.

Disabling VRR changed the sample from 90% to 94% of one core. With VRR off,
matching the two outputs near 120 Hz changed the sample from 78% to 74%, with
84% after restoring 120/165 Hz. With static PNGs and VRR off, the compositor
used 68% of a core with two outputs and 67% with just the Samsung. These short
samples establish that none of those changes cleared the load. They do not
measure the effect of running at 60 Hz or reproduce the flicker on demand.
The Acer refused the IPC on request. A config reload restored its original
120 Hz mode and VRR, confirmed by the owner. Track re-enable in `sysc-2416`.

## Renderer and presentation

- [Niri 26.04's DRM renderer](https://github.com/niri-wm/niri/blob/v26.04/src/backend/tty.rs#L1929)
  queues finished output work through Smithay. It waits on the rendered
  swapchain buffer only when needed or when the debug override is set.
  [Smithay's fence check](https://github.com/Smithay/smithay/blob/ff5fa7df392cecfba049ffed55cdaa4e98a8e7ef/src/backend/drm/compositor/mod.rs#L1183)
  checks the KMS plane's `IN_FENCE_FD` and excludes NVIDIA versions older than
  560.35.03. That old-driver exception does not apply here.
- SceneFX 0.5 uses native EGL fences with output timelines when the renderer and
  backend support them. wlroots 0.20.2 exports the wait point to KMS. The live
  sysc session advertised `wp_linux_drm_syncobj_manager_v1`, which confirms the
  client protocol is available without proving what each client used on a
  particular frame. [SceneFX output state](https://github.com/wlrfx/scenefx/blob/0.5/types/scene/wlr_scene.c#L3479)
- [Hyprland's renderer](https://github.com/hyprwm/Hyprland/blob/8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd/src/render/GLRenderer.cpp#L153)
  supplies an explicit KMS input fence and retains sampled client buffers until
  GPU work completes. Synchronous completion is a fallback when it cannot
  provide the required fences. This does not justify adding a blanket
  `glFinish` to sysc's output loop.
- [Niri predicts presentation per output](https://github.com/niri-wm/niri/blob/v26.04/src/niri.rs#L4578)
  and samples animations at that time. Sysc uses a shared 16 ms timer for
  animation updates. This can limit resize smoothness above 60 Hz. It cannot
  by itself account for sustained native compositor load outside an animation.
  Sysc also sends frame callbacks after a repaint attempt, including when no
  buffer was submitted. Niri distinguishes a successful submission from a
  no-damage result and uses an estimated vblank in the latter case.

## Resize and remaining proof

[Niri snapshots on a matching client commit](https://github.com/niri-wm/niri/blob/v26.04/src/handlers/xdg_shell.rs#L1467).
It then draws the old and current representations through the resize effect.
Sysc starts the resize animation with the layout request and temporarily
substitutes a scaled scene clone for the live contents. That clone reused the
client buffer without carrying its acquire timeline and point. SceneFX can
then sample it before the producing GPU finishes. The narrow fix retains
the acquire point in the shared resize and close snapshot helper.
`TestSceneSnapshotRetainsBufferAcquirePoint` reproduces the missing fence
without a display or DRM device and checks its reference lifetime.

There is a separate release-side question. A plain clone does not have the
original surface's SceneFX output-sample listener. That listener records
render completion points on the client commit. Verify that a client cannot
reuse the backing buffer while a snapshot can still sample it. The acquire
test does not check that lifetime. Niri's commit-based handoff and Hyprland's
buffer retention are useful references for this check.

Confirm the resize fix on archPC using the same Ghostty resize on the same
output. If the flash remains, compare with Niri and compare resize animations
off versus on. Capture the committed output and a fresh scene render at the
same step to locate the artifact before changing the animation handoff.

The sustained load still needs attribution. On a driver-matched native login,
collect the existing opt-in counters for frame attempts, scene commits,
presentation, and scene time in a short quiet sample and a short resize
sample. Pair them with an authorized kernel CPU profile of the compositor
thread. These measurements separate excess redraws from costly work per frame
and locate any expensive DRM or render ioctl. Unprivileged perf was denied on
archPC and the privileged stack sample did not acquire authentication.
