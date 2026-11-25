# archPC NVIDIA parity handover

Track this in `sysc-2417`. Start on main at `617a254` or later, in the
worktree `~/sysc-compositor-archpc`. The detailed record of everything so
far is the [spin handover](2026-11-24-archpc-nvidia-spin-handover.md).
Read that first; this file says what is left and how to pick it up.

## Goal

The owner sees parity with Niri on their laptop, which has no NVIDIA GPU.
On archPC (RTX 4060, NVIDIA 620.73.04, DP-1 3440×1440@120 with VRR and DP-3
2560×1440@165) sysc still feels slower and lags at times. Parity means
NVIDIA parity: for the same scene and input, sysc matches Niri on

- **looks:** the same screenshots, including corners, shadows, focus ring
  and blur, also during animations;
- **smoothness:** no missed frames for a GL client, and no libinput "event
  processing lagging behind" reports;
- **cost:** compositor main-thread CPU no higher than Niri's.

Release nothing (no tags, no AUR bumps, no dependency switch) until the
owner confirms that archPC is fixed.

## Where things stand

On main:

| Commit | What it fixed |
| --- | --- |
| `92804a3`, `4b436bd` | Cached xray blur per output, rebuilt only while a consumer shows |
| `bb1773a` | Resize and animation snapshots draw above window borders |
| `e6c2d88` | The SHM recovery backup copies only damage |
| `4cdb824` | The EGL context stays bound, which ended the NVIDIA `eglMakeCurrent` spin |
| `5dc96e8` | Window blur takes the window's rounded geometry. It fixed the owner's "corners that are not really rounded". |

In SceneFX (no C in this tree, so the fix lives outside it): effect buffers
render into their sampled texture, so blur passes stop respecifying an
EGLImage on every draw. The fork [Nomadcxx/scenefx](https://github.com/Nomadcxx/scenefx)
carries it on `main` (upstream main plus `e5ed765`) and as tag `0.5-sysc1`
(0.5 plus `5399deb`, the same soname as Arch's `scenefx`). Upstream:
[wlrfx/scenefx#237](https://github.com/wlrfx/scenefx/pull/237). AUR package:
`scenefx-sysc` (recipe in `~/sysc/aur/scenefx-sysc`). Nothing depends on it
yet; switching `sysc-compositor` to it waits for the owner's go.

Measured so far (detail in the spin handover):

- Native, gSlapper paused: idle went from 25.3% (before `4cdb824`) to 10.3%,
  then to 2.8% of a core with the SceneFX patch. Cached blur under load
  costs 0.76 ms per commit, against 1.56 ms before the patch. Live blur went
  from 78% to 28% of a core and lost its input-lag reports.
- Nested parity suite: sysc costs less than Niri in all nine scenarios and
  keeps up with it in all of them except overview (simple-egl at 124 fps
  against 157).
- libinput still reports 23–43 ms bursts natively: at sign-in, and around
  config reloads.
- The wallpaper is a 30 fps MP4. gSlapper decodes it in software, once per
  output, at about 96% of a core each, and every video frame rebuilds the
  cached blur (48 rebuilds/s).

## Progress on 2026-11-25

- **CI** on main was red from `5dc96e8`: a popup's blur took its window's
  geometry and corners, because `viewForSurface` walks a popup up to its
  toplevel. PR #128 fixed it and is merged (`ee63678`).
- `corners/` is rolled back. `main-f3abf75-highp/` (all of the above plus
  SceneFX #208) is armed for the next sign-in.
- **Blur tint (item 2)** was not a tint difference. With no
  `background-color` in `suite.kdl`, Niri fills each workspace with its
  opaque default. That hides the `place-within-backdrop` wallpaper, so
  Niri's xray frost blurred flat grey. sysc drew backdrop layers over the
  workspace colour. PR #129 draws them under it, in their own tree, with no
  pointer input, as Niri's `render_inner` does. The owner agreed, and their
  sysc config now sets `background-color "transparent"` (backup:
  `config.kdl.before-transparent-bg` in the kit root), as their Niri config
  does. The suite config sets it too. SceneFX's blur effects already run at
  brightness and contrast 1, so the frost now matches in the screenshots.
- **Open-floating size.** A window that a rule opens floating took its
  tiled placement, so the suite's simple-egl probe opened at about
  400×1340 in sysc and 250×250 in Niri. PR #130 floats such a window at its
  own size, centred, as Niri does.
- **Overview pacing (item 1).** `parity2/nested/probe.sh` holds the
  overview open with `WAYLAND_DEBUG` on simple-egl. sysc gave it **1 fps**:
  the overview's copies cover the originals, the scene gives those no
  output, and they fell back to the once-a-second timer. Every frame-paced
  window in sysc's overview froze. The suite's 124 fps was an average over
  open, close and animation. PR #131 answers the frame callbacks of every
  source tree projected onto an output after it repaints; its test fails
  without the fix (4 callbacks in 5 s) and passes with it. That covers the
  workspace switch too, whose 90 fps against Niri's 102 is likely the same
  cause. Still to do: one probe or suite run with `parity3/` (main plus
  #129–#131) to confirm, which needs the owner's OK.
- Suite with `parity2/` (main plus #129 and #130, fixed `suite.kdl`),
  main-thread CPU and simple-egl fps:

  | Scenario | sysc | Niri |
  | --- | --- | --- |
  | idle | 9.4%, 160 | 19.1%, 154 |
  | resize | 12.3%, 159 | 20.5%, 133 |
  | scroll | 10.2%, 160 | 18.7%, 158 |
  | workspace | 8.2%, 90 | 12.8%, 102 |
  | openclose | 13.7%, 157 | 19.6%, 146 |
  | overview | 14.5%, 120 | 20.5%, 158 |
  | resize, blur off | 12.5%, 159 | 20.6%, 155 |
  | video idle | 13.9%, 159 | 22.9%, 155 |
  | video resize | 17.1%, 155 | 23.9%, 155 |

- **Looks still differ** in two ways in `idle-end.png`:
  - Floating decorations draw under tiled windows. `windowShadowTree`
    and `windowBorderTree` hold every window's shadow, border and
    background below the whole windows layer. A floating window's shadow
    and its focus-ring background (Niri's `draw-border-with-background`
    for a client-decorated window such as simple-egl) are therefore hidden
    under the tiled window behind it. Niri draws each tile's decorations
    with the tile. Fixing this means stacking each floating window's
    decorations with it.
  - Niri shows Ghostty in the third column where sysc shows the third
    foot, from the same open order. Not yet explained.
- **CI flake on main:** `internal/wlr` sometimes dies with SIGSEGV at
  PC 0 inside `fx_renderer_create`. It happens when
  `TestFractionalScaleFollowsOutput` builds its compositor, right after the
  renderer-loss tests. Other runs log `EGLImageTargetRenderbufferStorageOES`
  `GL_INVALID_VALUE`, "Failed to create FBO" and "could not keep the EGL
  context current" there instead. Seen on main's run for `ee63678` (also on
  re-run) and once on #131, which passed on re-run. It is the same vkms EGL
  instability as the `fb9caea` flake. `wlr_egl_destroy` unbinds the context
  before destroying it, so the `4cdb824` pin is not leaking a binding.
  Still open.
- **sysc-shell (item 6)** is `sysc-2418` in the sysc-shell beads. The
  writes do not stop at unlock: unlocked, sysc-shell still made 18,703
  write syscalls a second at 17% of a core. That is recorded on the ticket.
- **SceneFX Vulkan:** see the
  [assessment](2026-11-25-scenefx-vulkan-assessment.md). Recommendation:
  track, do not adopt.

## SceneFX open PRs (reviewed 2026-11-25)

Of the 21 open PRs on [wlrfx/scenefx](https://github.com/wlrfx/scenefx/pulls):

- **#208 (adopt for archPC):** GLSL ES 3.20 shaders, so NVIDIA gets `highp`.
  A probe on the RTX 4060 (620.73.04) shows `#version 100` shaders do not
  get `GL_FRAGMENT_PRECISION_HIGH`, and its `mediump` is real fp16 (10-bit
  mantissa). `corner_alpha` works in absolute `gl_FragCoord`, so any corner
  past x≈1024 loses its antialiasing steps, and past 2048 it is quantised to
  2 px. A headless 3440×1440 render of the same radius-20 rect at x=101 and
  x=3201 gives 12 and 21 distinct corner shades with SceneFX 0.5. With #208
  it gives 12 and 12, the same corner. Mesa runs `mediump` at full
  precision, which is why the laptop looks right. #208 applies cleanly on
  `0.5-sysc1`: fork branch `nvidia-highp-0.5` (`79555b7`). texprobe cost
  is unchanged (cached 14.84 against 14.86 ms/frame). `blur1`/`blur2`
  stay `mediump` under #208.
  It supersedes #177, which forces `highp` behind a build option.
- **#234 (minor):** box shadows square their alpha, but only on a
  transparent target. In sysc that means captures, not the outputs.
- **#178, #218 and #216:** shader rewrites. #178 has open rendering bugs,
  and #218 depends on #216 (rounding power, which Niri lacks). Track only.
- **#179, #190, #192 and #204:** the renderer overhaul and Vulkan; see the
  [Vulkan assessment](2026-11-25-scenefx-vulkan-assessment.md).
- The rest (#102, #112, #125, #133, #135, #153, #155, #160, #188, #212,
  #213) are stale, cosmetic or not used by sysc.

## First steps

1. The `corners/` kit is armed for the next sign-in: main at `5dc96e8` with
   the patched SceneFX through `LD_LIBRARY_PATH`, plus the bench socket.
   Confirm it is running:
   `readlink /proc/$(systemctl --user show -p MainPID --value sysc-compositor.service)/exe`
   should be under `corners/`, and `grep libscenefx /proc/<pid>/maps`
   should show the kit's library. Ask the owner whether terminal corners
   now look rounded natively.
2. Check CI on main for `5dc96e8` and `617a254`; the network dropped before
   it could be read. `TestBackgroundEffectDrawsFrostUnderTheRegion` covers
   the corner change. A one-off `TestSceneFXRendererRendersNestedSceneEffects`
   failure on `fb9caea` (EGLImage import on vkms) did not recur. If it comes
   back, suspect buffers leaking between in-process test compositors.

## Work queue

1. **Overview frame pacing (nested).** simple-egl loses about 20% of its
   frames in sysc's overview and none in Niri's. Run
   `corners/nested/suite.sh` (each run needs the owner's OK), profile the
   overview scenario, and compare it with Niri's overview rendering in
   `~/references/niri` (v26.04).
2. **Blur tint.** Niri's frost is darker than sysc's in the same scene; see
   the `idle-end.png` screenshots under `corners/nested/results/`. Compare
   the blur defaults (passes, offset, noise, saturation, brightness,
   contrast) and how opacity combines with the frost.
3. **Native scripts for sysc and Niri** (not Mango, which sysc-shell does
   not support yet). Use the owner's real session: sysc-shell, gSlapper,
   both outputs. Drive identical input with ydotool, which injects through
   uinput and so passes through libinput: pointer motion, typing into a test
   terminal, and Super+right-drag resizes. The owner is in `input`. Record
   compositor main-thread CPU, libinput lag lines, gSlapper and sysc-shell
   CPU, and for sysc the bench socket metrics. The owner signs into each
   session and starts the script, then leaves input alone for the run. See
   the nested-gate-driving memory for the ydotool traps (private
   `YDOTOOL_SOCKET`, keyboard codes, no wtype).
4. **gSlapper decode** (gSlapper's own C repo, not this tree). GStreamer's
   `nvcodec` reports "Unable to initialize CUDA library" although `libcuda`
   and `libnvcuvid` are installed. `libgstlibav.so` is blacklisted, with an
   unowned `libgstlibav.so.bak-avcodec62` next to it. Each output decodes
   the same file separately. Agree system-package changes with the owner.
5. **Commission checks**, natively: a background popup across DP-1 and
   DP-3, a rotated or scaled output, a power cycle, screenshots and
   screencasts with blur, and config reload stalls.
6. **sysc-shell** writes about 18,700 times a second to its plugin pipes and
   commits about 55 times a second at idle. File that in the sysc-shell
   repo.

## Investigate SceneFX Vulkan: #192 and #204

Assess whether these PRs could help archPC, and our SceneFX fork.

- [#192 Vulkan](https://github.com/wlrfx/scenefx/pull/192) by ErikReider
  (open, last updated 2026-08-11, +3815/−4607 across 71 files). It adds a
  Vulkan renderer to SceneFX and depends on #179 and #190. Its checklist
  covers quad effects only; texture effects, blur, optimized blur, box
  shadow, gradients, stencilling and the saved-blur blit are unchecked. It
  needs the [wlroots-vkfx](https://gitlab.freedesktop.org/ammen99/wlroots/-/tree/vulkan-effects)
  fork, because upstream wlroots does not expose some Vulkan functions.
- [#204 vulkan: implement blur](https://github.com/wlrfx/scenefx/pull/204)
  by spoloxs (open, last updated 2026-09-05, +1372/−16 across 13 files).
  It builds on #192 and implements regular blur. It needs wlroots functions
  that do not exist upstream (`wlr_vk_render_pass_suspend`, `_resume`,
  `_get_blend_image_view`, `_get_blend_image`), from
  [spoloxs/wlroots-vkfx](https://github.com/spoloxs/wlroots-vkfx/tree/vulkan-effects).

Why it might matter: every NVIDIA cost found so far sat in the EGL and GLES
driver path, first the `eglMakeCurrent` release spin and then EGLImage
respecification. A Vulkan renderer avoids libnvidia-eglcore entirely.

Questions to answer, with evidence:

1. What works today on #192 plus #204? List the features sysc uses and mark
   each one: rounded texture effects, clipped regions, box shadows, regular
   blur, and **optimized (cached) blur**, which every xray effect here
   depends on.
2. What would adoption cost? sysc links Arch's `wlroots0.20`. Both PRs need
   a wlroots fork, which would mean packaging a forked wlroots alongside
   `scenefx-sysc`. Check how far the wlroots changes are from upstream.
3. Which parts of sysc are GLES-only and would break?
   - `createConfiguredRenderer` (`internal/wlr/debug_devices.go`) creates
     only the GLES `fx_renderer`.
   - Custom animation shaders are GLSL (`internal/wlr/animation_shader.go`),
     like Niri's custom-shader contract.
   - The EGL context pin (`4cdb824`, `internal/wlr/renderer.go`).
   - Renderer recovery.
   - The `5399deb` SceneFX patch.
4. Does Vulkan actually help on this machine? If the branches build, stage
   a kit outside the tree (forked wlroots and SceneFX through
   `LD_LIBRARY_PATH`, plus a sysc build with a Vulkan renderer path on a
   local branch) and run the nested parity suite against the GLES build and
   Niri. Use blur off if blur is incomplete, and say so in the results.
5. How alive are they? Check review activity, the maintainers' stated plans
   in the PR threads and issue #7, and how they relate to upstream wlroots'
   own Vulkan work.

Deliver a written assessment with a recommendation (adopt, track, or
ignore) and add it to this file or a report next to it. Keep any
experimental work on a branch of the fork, not its `main`. Adopting it
needs the owner's OK, because it changes the renderer and the packaging.

## Kits and tools

Everything lives under `~/.local/state/sysc-compositor-archpc-20261123/`;
`/tmp` is cleared at boot.

| Path | Purpose |
| --- | --- |
| `corners/` | Armed build. `arm.sh`, `rollback.sh` and `bench.sh MODE SECONDS` (modes: `cached`, `live`, `off`, `cached-noresize`). |
| `corners/nested/suite.sh sysc\|niri 15` | Nested parity suite. Results and screenshots go to `nested/results/<time>-<compositor>/`. |
| `main-f3abf75/`, `main-f3abf75-highp/` | Main `f3abf75` with the patched SceneFX, and the same with #208 on top. `-highp` was **armed** on 2026-11-25. Switch with `sh main-f3abf75-highp/rollback.sh && sh main-f3abf75/arm.sh`. Each has `bench.sh`. |
| `parity2/`, `parity3/` | Unarmed builds with the patched SceneFX: main plus #129 and #130, and plus #131 too. `nested/suite.sh` with the transparent-background `suite.kdl`, and `nested/probe.sh sysc\|niri SECONDS` (overview held closed, open, closed; simple-egl under `WAYLAND_DEBUG`). |
| `vulkan/` | wlroots-vkfx and SceneFX #204 builds; see the Vulkan assessment. |
| `4cdb824-fxtex/` | Previous kit, with `cprofile.sh NAME SECONDS` (C-stack profiles through the `cstacks` binary) and `nested/compare.sh`. |
| `texprobe/` | Headless SceneFX probe on the render node (no window). `texprobe {live\|cached\|off} CONSUMERS FRAMES [check]`; `check` hashes read-back frames. |
| `scenefx/`, `scenefx-base/`, `scenefx-main-base/` | Fork clone (`keep-offscreen-texture`, `keep-offscreen-texture-main`) and unpatched builds for A/B. |
| `umbriel/` | Umbriel source; umbrielfx is the SceneFX fork the item E fix follows. |
| `tools/perf` | perf 7.3.5, unpacked without installing; flat `cpu-clock:u` only (`perf_event_paranoid=2`). |

## Rules and traps

- Every nested run on this desktop needs the owner's OK for that run. Run
  one package with named tests, and set `SYSC_NESTED_TESTS=1` only with that
  OK. Build and test with `CGO_CFLAGS_ALLOW=-fno-strict-overflow`; the
  language server lacks it and reports false cgo errors.
- The owner allows pausing or killing processes and re-arming builds for
  this work (SIGSTOP/SIGCONT on gSlapper is fine for benches). Prefer
  reversible steps and say what was done.
- Under zsh, `kill -STOP $pids` with several PIDs in one variable fails;
  loop over them.
- bench.sh samples 2 s after loading a config. Give the owner a lead-in
  (`sleep 5` before the call) and check that commits per second rise above
  the idle rate of about 55, or the workload did not run.
- Commit messages: no AI attribution, and none of the hook's substrings
  ("bot" catches "both" and "bottom", plus "agent" and "cursor"). Check
  with the grep in the commit-hook memory.
- `ptrace_scope=1` blocks strace and gdb attach. Use `/proc/PID` counters or
  the cstacks profiler.
