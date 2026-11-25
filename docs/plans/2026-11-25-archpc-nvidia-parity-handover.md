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

- CI on main has been red since `5dc96e8`.
  `TestPopupsRuleDrawsPopupsWithTheirOwnOpacityAndBlur` broke because
  `viewForSurface` walks a popup up to its toplevel, so a popup's blur took
  the window's geometry and corners. Fixed in PR #128
  (`fix/popup-blur-geometry`). It is green on CI, nested tests included, and
  waits for a merge. The armed `corners/` kit has the bug, but the owner's
  config has no `popups` rule, so the kit stays as it is.
- The `corners/` kit is armed but not yet running: the session predates
  it. First step 1 below still applies after the next sign-in.
- **Blur tint (item 2)** is not a tint difference. `suite.kdl` sets no
  `background-color`, so Niri fills each workspace with its opaque default
  (0.25 grey). That hides the `place-within-backdrop` wallpaper, and Niri's
  xray skips the backdrop wherever an opaque workspace covers it
  (`src/render_helpers/xray.rs`), so its frost is blurred flat grey. sysc
  puts `backgroundTree` below the background layer, so the backdrop
  wallpaper draws over the workspace colour in the normal view, in the gaps
  and in the frost. SceneFX's blur effects already run at brightness and
  contrast 1. The only other difference is the saturation luminance
  weights (SceneFX 0.3086/0.6094/0.0820 against Niri's Rec. 709), which is
  minor.
  Matching Niri means drawing backdrop layers under the workspace colour in
  the normal view. That would turn the owner's desktop black: their sysc
  config sets no `background-color` (sysc's default is opaque black), while
  their Niri config sets `"transparent"`. So the change needs the owner's
  call and a config change with it. Either way, add
  `background-color "transparent"` to `suite.kdl`, as in the owner's Niri
  setup, so the suite compares like with like.
- The suite's scenes differ in another way: in sysc, simple-egl floats at
  column size (about 400×1340, over foot3), while in Niri it floats at its
  own 250×250. The probe renders roughly nine times the pixels in sysc. Fix
  that, or account for it, before reading item 1's overview gap of 124
  against 157 fps.
- **sysc-shell (item 6)** is `sysc-2418` in the sysc-shell beads. The
  writes do not stop at unlock: unlocked, sysc-shell still made 18,703
  write syscalls a second at 17% of a core. That is recorded on the ticket.
- **SceneFX Vulkan:** see the
  [assessment](2026-11-25-scenefx-vulkan-assessment.md). Recommendation:
  track, do not adopt.

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
