# Frame pacing: source audit for sysc-2056

Baseline: sysc `7d8e05b`, Niri 26.04 at the pin in `docs/README.md`, installed
wlroots 0.20.2 and SceneFX 0.5. This records source evidence, not a game benchmark.
Niri supplies useful policy and compatibility references; wlroots/SceneFX own
sysc's rendering machinery. Do not port Niri's Smithay backend.

## Findings

| Invariant | Existing owner and evidence | Result |
|---|---|---|
| A surface gets callbacks from one output | wlroots 0.20.2 `types/scene/surface.c`, `get_surface_frame_pacing_output` and `handle_scene_buffer_frame_done`; sysc `internal/wlr/output.go`, `sysc_repaint_scene_output` | Reuse the native selection. It picks the fastest unsuspended entered output. Niri's `src/niri.rs::send_frame_callbacks` selects its primary scanout output. These policies need a mixed-refresh benchmark before changing either side. |
| A quiet output stops committing | SceneFX `wlr_scene_output_needs_frame` and `wlr_scene_output_commit`; sysc `TestIdleOutputStopsCommitting` | Already implemented and checked. Do not reintroduce unconditional `wlr_output_schedule_frame`; that caused sysc-1469. |
| No duplicate callback in one refresh cycle | Niri records output and `frame_callback_sequence`; wlroots `types/output/output.c::wlr_output_schedule_frame` coalesces work with `frame_pending`/`idle_frame` | Sysc has no separate sequence guard. A pending native output commit normally supplies throttling. Source differences alone do not prove a busy loop; exercise callback-only commits before adding a guard. |
| No-damage redraw remains paced | Niri `src/backend/tty.rs::{queue_estimated_vblank_timer,on_estimated_vblank_timer}`; wlroots `wlr_output_schedule_frame` and SceneFX's needs-frame handling | Native scheduling is the responsible layer. Measure callback intervals and output commits on no-damage requests before introducing a sysc timer. |
| Hidden applications keep making progress | Niri `src/niri.rs::send_frame_callbacks_on_fallback_timer`, one-second timer with a 995 ms throttle; SceneFX `scene_node_send_frame_done` skips disabled nodes and `wlr_scene_buffer_send_frame_done` skips invisible buffers | Fixed 2026-10-31. `scheduleFrameFallback` (`internal/wlr/output.go`) runs once a second and answers every mapped scene surface that has no unsuspended entered output. Those are the surfaces wlroots' pacing-output check leaves unanswered: hidden workspaces, offscreen columns, fully covered windows and outputs that are off. The scene walk includes disabled trees. Surfaces with a pacing output keep their repaint callbacks, so the 995 ms throttle needs no counterpart. `TestHiddenWorkspaceKeepsFrameCallbacks` failed with 0 callbacks before the fix. It now gets 2–4 in 2.5 s with no output frames drawn. |
| Eligible fullscreen clients can scan out | SceneFX `scene_entry_try_direct_scanout` and its single-render-entry gate; sysc `internal/wlr/appearance.go` removes fullscreen corners/clips and hides shadows/focus rings | Sysc leaves native direct scanout available. An opaque fullscreen surface may occlude the backdrop; overlays, transparency, captures, format/transform mismatch and the backend's state test can prevent scanout. Only a DRM output-sample observation proves engagement on hardware. |
| VRR follows the active application | Niri `src/niri.rs::refresh_on_demand_vrr`; sysc only reports native adaptive-sync state in `OutputSnapshot` | Configuration and activation remain M15 work. Preserve the decision to keep presentation vsynced; tearing hints do not enable tearing. |

Native source references:

- [wlroots 0.20.2 surface handling](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/types/scene/surface.c)
- [wlroots 0.20.2 output scheduling](https://gitlab.freedesktop.org/wlroots/wlroots/-/blob/0.20.2/types/output/output.c)
- [SceneFX 0.5 scene handling](https://github.com/wlrfx/scenefx/blob/37ccd723bef49e6891156ffafce8f549f01446cc/types/scene/wlr_scene.c)

## Measurement still required

Use the same physical output, mode, scale, GPU, game scene and presentation
settings under Niri and sysc. Record frame-time distributions and missed refreshes,
not average FPS alone. Check visible, obscured and hidden-workspace cases;
callback-only commits; fullscreen scanout with overlays closed; and VRR off/on
when M15 supports it. Pair output commits/presentation feedback with client
callbacks so a fast producer cannot masquerade as good presentation pacing.

Include Wine's Wayland driver and Wine through satellite after M13. The session
VM's vkms/llvmpipe runs prove protocol/lifecycle behavior; they cannot qualify
physical scanout, VRR or game quality. No game measurements were taken for this
source audit. Keep sysc-2056 open until the physical benchmark is recorded.
