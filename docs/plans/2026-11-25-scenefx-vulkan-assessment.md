# SceneFX Vulkan assessment (#192 and #204)

Part of `sysc-2417`; the brief is in the
[parity handover](2026-11-25-archpc-nvidia-parity-handover.md). Assessed on
2026-11-25 at SceneFX `vulkan` (#192, `wlrfx:vulkan`) plus #204
(`spoloxs:vulkan-blur` at `8c6f440`), with
[spoloxs/wlroots-vkfx](https://github.com/spoloxs/wlroots-vkfx/tree/vulkan-effects)
at `0691b81`.

**Recommendation: track, do not adopt.** The Vulkan path draws none of the
effects that make sysc look like Niri. It needs a forked wlroots that
upstream has not agreed to and a SceneFX API that has not merged. And the
two NVIDIA costs found so far are already fixed on the GLES path. Revisit
when #179 merges and #192 gains texture effects and optimized blur.

## 1. What works on #192 plus #204

From `render/vulkan/pass.c` at `8c6f440`:

| Feature sysc uses | Vulkan state |
| --- | --- |
| Rounded, clipped, opacity-scaled textures (every window, layer and snapshot) | Missing. `vk_render_pass_add_texture` hands the plain options to `wlr_render_pass_add_texture` (`// TODO: Fallback`), so corner radius and clip region are dropped. Windows would draw square. |
| Rounded rects with clipped regions (borders, focus ring, backgrounds) | Works. `vk_render_pass_add_rect` applies the clipped region and corners. |
| Box shadows | Stub. `vk_render_pass_add_box_shadow` is empty, so shadows vanish. |
| Gradients (rect and rounded rect) | Stubs. |
| Regular blur | Works in #204: dual Kawase plus padding repair, tested by its author on an RTX 5050 (610.43.02), nested, one output, with validation layers. It runs only on the two-pass (blend image) pathway and is skipped elsewhere. |
| **Optimized (cached) blur** | Stub. `vk_render_pass_add_optimized_blur` returns false. Every xray effect here depends on it, so all xray frost would disappear. |
| `read_to_buffer` | Left unimplemented by #204. |

## 2. Adoption cost

- **Forked wlroots.** wlroots-vkfx is wlroots `0.20.0` plus five commits:
  three of ammen99's (export the renderer internals, a validation fix,
  install as a separate library) and two of spoloxs' (split the scene and
  output render passes, allow transfers on the blend image). That comes to
  8 files, +499/−86, against `0.20.0`. It lacks the 30 commits from `0.20.0`
  to the `0.20.2` that Arch ships. Upstream `master` (checked at
  2026-11-21) exports none of these internals. spoloxs asked in #192 whether
  that export has any upstream plan, and nobody in the thread has answered.
- **One wlroots per process.** The fork installs as `libwlroots-vkfx-0.20`
  (pkg-config `wlroots-vkfx-0.20`), so it can sit beside Arch's
  `wlroots0.20`. A Vulkan SceneFX links the fork, though, so sysc would
  have to move all its cgo preambles from `wlroots-0.20` to the fork too.
  Two wlroots builds in one process are not an option.
- **SceneFX API break.** #179 replaces `fx_renderer_create` with
  `scenefx_init*`, and the public headers drop
  `scenefx/render/fx_renderer/fx_renderer.h`. sysc calls
  `fx_renderer_create`, `fx_renderer_create_with_drm_fd`, `fx_get_renderer`,
  `fx_get_render_pass`, `fx_render_pass_add_rounded_rect` and
  `fx_texture_get_attribs`; none of these is in the branch's public headers.
  Measured: sysc main does not compile against it (first error: that
  header, in `animation_shader.go`).
- **Packaging.** It would need AUR packages for wlroots-vkfx and for a
  Vulkan SceneFX. That SceneFX installs `libscenefx-0.5.so`, the same name
  as Arch's `scenefx` and `scenefx-sysc`, so the packages conflict.

## 3. GLES-only parts of sysc

- Renderer creation: `createSceneFXRenderer` (`internal/wlr/scene_fx.go`)
  and `createConfiguredRenderer` (`internal/wlr/debug_devices.go`) create
  only the GLES `fx_renderer`.
- Custom animation shaders (`internal/wlr/animation_shader.go`, 1109 lines):
  GLSL ES compiled at run time, an EGL binding, and `fx_texture_get_attribs`
  for the GL texture. Niri's custom-shader contract is GLSL. Under Vulkan
  this needs a GLSL to SPIR-V path or has to switch off.
- The EGL context pin (`internal/wlr/renderer.go`, `4cdb824`) and
  `glFinish` in `internal/wlr/debug.go` must be skipped under Vulkan.
- Renderer recovery listens on wlroots' generic `lost` event, so it should
  carry over. Not verified.
- The `5399deb` SceneFX patch does not apply. It also is not needed: #204
  keeps its blur targets as renderer-owned `VkImage`s in the scene's command
  buffer, with no EGLImage or DMA-BUF import.

## 4. Does Vulkan help on archPC?

Not measured. Both forks build here: wlroots-vkfx (gles2 and vulkan
renderers) and SceneFX `8c6f440`, installed into
`~/.local/state/sysc-compositor-archpc-20261123/vulkan/prefix`. A sysc
Vulkan build needs a porting branch first (section 2). Even then it would
draw square windows, no shadows and no xray frost, so the nested suite
would compare a different scene from Niri's.

If the owner wants the raw renderer question answered anyway, the cheaper
probe is the fork's own `tinywl`, or the `scene-graph` example in
`vulkan/scenefx-vk/build`, nested with `WLR_RENDERER=vulkan` and then
`gles2` under the same simple-egl load. Each run opens a nested window, so
each needs the owner's OK. Note
[wlroots#4035](https://gitlab.freedesktop.org/wlroots/wlroots/-/issues/4035)
reports the Vulkan renderer as less efficient than GLES2, and #204's own
notes say its render pass split loses subpass merging.

The evidence so far does not point at the GL driver path either. Both
NVIDIA costs found there, the `eglMakeCurrent` release spin and EGLImage
respecification, are fixed on GLES (`4cdb824`, `5399deb`). Native idle is
now 2.8% of a core.

## 5. How alive is it?

- #192: last updated 2026-08-11 and conflicts with `main`. Its checklist
  has only quad effects done. In July the maintainer suggested stubbing the
  remaining effects and adding them one at a time.
- Its prerequisites: #179 (the wlroots GLES2 renderer swap) got changes
  requested on 2026-08-16. Its author kept replying until 2026-10-27, and
  the maintainer holds review until a SwayFX release. #190 has had no
  review since 2026-07-01.
- #204: on 2026-09-05 the maintainer said they are aware of it, but are
  getting the prerequisites in first.
- Issue #7 (add a Vulkan renderer) has been open since 2024. Its rationale
  is that wlroots may soft-deprecate GLES2 once colour management lands.

## Kit

`~/.local/state/sysc-compositor-archpc-20261123/vulkan/`: `wlroots-vkfx/`
(clone and build), `scenefx-vk/` (a worktree of the fork clone on branch
`vk-204`; `vk-192` is #192), `prefix/` (both installed), and `pcshim/` (a
`wlroots-0.20.pc` that points at the fork, for porting experiments). No
sysc branch was kept.
