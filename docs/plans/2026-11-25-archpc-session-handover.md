# archPC session handover (2026-11-25, evening)

Track this in `sysc-2417`. Continue in the worktree `~/sysc-compositor-archpc`
from main at `f3abf75` or later. The full record is the
[parity handover](2026-11-25-archpc-nvidia-parity-handover.md); docs PR #133
adds its review of the SceneFX PRs. This file says what is running and what
to do next, in order.

## What is running

The owner is signed into the kit
`~/.local/state/sysc-compositor-archpc-20261123/main-f3abf75-highp/`. It holds:

- main `f3abf75` (`0.1.0-50-gf3abf75`), which includes #128 to #132;
- the patched SceneFX `0.5-sysc1` (`5399deb`);
- [wlrfx/scenefx#208](https://github.com/wlrfx/scenefx/pull/208) (GLSL ES
  3.20, so shaders get `highp` on NVIDIA). It is on the fork branch
  `nvidia-highp-0.5` at `79555b7`, and fixes corners quantised beyond
  x≈1024.

Confirm with
`readlink /proc/$(systemctl --user show -p MainPID --value sysc-compositor.service)/exe`.
The plain kit `main-f3abf75/`, without #208, is the fallback:
`sh main-f3abf75-highp/rollback.sh && sh main-f3abf75/arm.sh`. The
owner's config sets `background-color "transparent"`, which #129 needs for
the wallpaper to show.

## Native measurements

`main-f3abf75-highp/native/native.sh [SECONDS] [OUTPUT] [SCENARIOS...]` runs
the parity scenarios in the live session over niri IPC. It works on an
empty workspace of OUTPUT and streams relative pointer motion through the
owner's ydotoold, with no clicks or keys. It records:

- compositor main-thread and process CPU;
- simple-egl fps;
- gSlapper and sysc-shell CPU;
- sysc's frame metrics;
- libinput lag reports.

With `PROFILE=1` it also saves a Go pprof profile and a flat user-space
`perf` profile of the main thread for each scenario. It restores the
config and focus at the end. The owner must leave input alone while it
runs.

Results on DP-1 (3440×1440 at 120 Hz with VRR, video wallpaper on;
`native/results/20261125-135339`):

| Scenario | Main thread | simple-egl fps | Commits/s | Work per commit |
| --- | --- | --- | --- | --- |
| idle | 27.2% | 117 | 190 | 0.79 ms |
| resize | 28.5% | 116 | 183 | 0.83 ms |
| scroll | 33.3% | 114 | 225 | 0.83 ms |
| workspace | 31.9% | 60 | 244 | 0.86 ms |
| open/close | 39.3% | 115 | 221 | 0.74 ms |
| overview | 49.6% | 114 | 248 | 0.64 ms |
| resize, blur off | 31.1% | 119 | 210 | 0.39 ms |

The overview fix (#131) holds natively. No frames were discarded. The
workspace figure is low because simple-egl sits on a hidden workspace half
the time. gSlapper used about 2 cores and sysc-shell about 17%; the
wallpaper causes 47 blur rebuilds a second.

Idle with no bench windows and the video wallpaper on: 14.5% main thread,
183 commits/s. With gSlapper paused: 5.0% main thread and 102 commits/s.
That idle is not truly idle, because the terminal running the session
redraws.

**libinput logs at most 5 lag reports per hour per device.** The ydotool
device hit that cap early in the run, so later zeros mean nothing. Before
the cap: one 28 ms report at idle, and three during scroll (up to 38 ms).

### Profile (`native/results/20261125-143127`, `idle.txt` and `overview.txt`)

Over 90% of main-thread time is in C calls. Where it goes:

- **Overview rebuild per commit:** `syncOverviewScene` takes 19% of a core
  in the overview, a third of the main thread. Every surface commit from a
  window or layer surface (`handleNewToplevel` commit listener,
  `syncOverviewLayer`) destroys and re-projects the whole overview scene.
  pixman region work rises to 23% of samples as a result. Niri renders the
  live windows instead.
- **SHM recovery backup:** `copyBufferPixels` (via
  `watchRendererBuffers`) takes 6% of a core at idle and 9% in the
  overview, a fifth of the main thread. gSlapper decodes in software
  (openh264) and commits full frames over SHM on both outputs, about
  1 GB/s, and the backup copies every frame's damage. NVIDIA's top routine
  (12% at idle) is most likely the SHM texture upload of the same frames.
  Niri and Mango keep no such backup.
- **Per-commit bookkeeping:** `notifyWindowStateChanged` and
  `ResolveWindowRules` run on every toplevel commit, even when nothing
  changed. That costs about 2% of a core.
- **Repaint:** `sysc_debug_repaint` is 11–13% of a core, mostly in
  libnvidia-eglcore. libc's `getpid` shows up as NVIDIA's fork check.

## Next, in order

1. **Input-delay metric.** Add a histogram to the stats socket (`internal/perf`)
   of the delay between each libinput event's timestamp and sysc handling
   it: pointer motion, buttons, keys and axes, with a maximum and a count
   over 20 ms. Add the histogram to `native.sh`'s summary. Then smoothness
   is measured on every event, not capped by libinput's log limit. Restart
   ydotoold between runs only if libinput's own reports are still wanted.
2. **Native Niri run.** Make `native.sh` compositor-agnostic: find the
   compositor pid from `$NIRI_SOCKET` (`niri.wayland-1.PID.sock`,
   `sysc-compositor.wayland-0.PID.sock`), and include the owner's Niri
   config in a Niri copy of `native.kdl`. Leave the metrics out for Niri,
   since it has no stats socket. The owner signs into Niri and runs it
   once, so cost, pacing and lag can be compared like for like.
3. **Fixes from the profile:**
   - **Incremental overview:** on a buffer commit, update the projected
     copies in place (`wlr_scene_buffer_set_buffer` and damage), and
     rebuild only on structural changes: windows, layout, zoom, outputs or
     workspaces. It also serves the workspace switch, which uses the same
     projections.
   - **SHM backup:** stop copying every frame of surfaces that commit
     continuously (video wallpapers, animations). One option is to read
     back a surface's texture once it has stayed unchanged for a while,
     and skip surfaces that keep committing. Reading back after the loss
     is not possible, because the texture is gone by then. A surface that
     keeps committing would then show nothing for one frame after a loss,
     until its next commit. Agree the trade-off with the owner, because it
     changes renderer-loss behaviour.
   - **Commit bookkeeping:** call `notifyWindowStateChanged` and resolve
     rules only when state changed.
   - **gSlapper hardware decode** (work queue item 4, in gSlapper's repo):
     DMA-BUF frames would skip both the SHM upload and the backup. Agree
     system package changes with the owner.
4. **Still open from the parity handover:** floating windows'
   decorations draw under tiled windows (`windowShadowTree` and
   `windowBorderTree` sit below the whole windows layer); Ghostty column
   order in the suite; the CI flake (SIGSEGV in `fx_renderer_create` after
   the renderer-loss tests); the commission checks; `sysc-2418` (sysc-shell
   loop, which persists when unlocked).
5. **Release, once the owner confirms archPC is fixed.** Publish a version
   update of sysc-compositor and a new package on the AUR. Main is 50
   commits past `v0.1.0`; the owner picks the version. In order:
   - Package SceneFX: tag the fork with `5399deb` plus #208 (for example
     `0.5-sysc2` from `nvidia-highp-0.5`) and bump `scenefx-sysc`
     (`~/sysc/aur/scenefx-sysc`). Note #208's shaders need a GLES 3.2
     context.
   - Switch `sysc-compositor` and `sysc-compositor-git`
     (`~/sysc/aur/sysc-compositor*`) from `scenefx` to `scenefx-sysc`.
     That is the dependency switch the parity handover holds back.
   - Tag `v<version>` on main, bump `pkgver` and checksums, build in a
     clean chroot or with `makepkg`, and push the recipes.

   Until the owner confirms, nothing is tagged, bumped or switched.

## Rules and traps

- Native runs and nested runs open windows on the owner's desktop. Get the
  owner's OK for each run. In this session the OK covered the benchmarks
  the owner asked for.
- Build with `CGO_CFLAGS_ALLOW=-fno-strict-overflow`. The language server
  reports false cgo errors.
- Commit messages: no AI attribution and none of the hook's substrings
  (see the commit-hook memory).
- `native.sh` closes `nbench*` and `simple-egl` windows over IPC on exit,
  because a forking Ghostty can outlive its pid.
- `/tmp` is cleared at boot; everything lives under
  `~/.local/state/sysc-compositor-archpc-20261123/`.
