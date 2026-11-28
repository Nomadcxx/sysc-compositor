# Remaining work handover (2026-11-28)

Track this in `sysc-2417`. Start from main at `3756c30` (`v0.1.0-65-g3756c30`)
or later. Earlier records: the
[archPC session handover](2026-11-25-archpc-session-handover.md), which has
progress sections, and the
[NVIDIA parity handover](2026-11-25-archpc-nvidia-parity-handover.md), which
has the hardware checklist.

On 2026-11-28 the owner reported a few days of exclusive use with
sysc-compositor running flawlessly.

## Progress (2026-11-28, later)

Main is at `10d47ad` (`0.1.0-75-g10d47ad`). The State section below predates
this.

- **Merged after review:** #140 (this handover), #141 (item 1, the
  floating-decoration fix), #142 (item 5's practical fix) and #143 (item 2,
  overview reprojection on each animation tick).
- **archPC:** the kit `main-10d47ad/` is armed for the next sysc sign-in. It
  replaced `profile-fixes/`, which was rolled back, and it has the same
  layout: patched SceneFX with #208 and `native/native.sh`.
- **Laptop:** `90-testbench.conf` now points at
  `~/.local/state/sysc-compositor-testbench/10d47ad/`, which is main. The
  previous drop-in is saved next to it. The live session was still on
  `b36865f`.
- **Still open:**
  - item 1.5, the visual check on the laptop after its next sign-in;
  - item 2's native overview run on archPC (fps and input-delay targets);
  - items 3 and 4, which need the owner at archPC;
  - item 5's root cause. The renderer-only probe was not found on archPC.
  - Items 6 to 8 are unchanged.

## State

- **Main** has everything from the 2026-11-25/26 sessions, and no PRs are open:
  - #135: an input-delay histogram in the stats socket (`/metrics`,
    `input_delay`);
  - #136: a commit publishes window state only when the window mapped or its
    geometry or fullscreen state changed;
  - #137: overview copies update in place when a surface draws;
  - #139: the decorations move with a dragged window. The owner confirmed it
    removed the yellow focus-ring block that trailed a Super+drag.
- **archPC** has been in a Niri session since the comparison run (2026-11-25
  20:38). The kit `~/.local/state/sysc-compositor-archpc-20261123/profile-fixes/`
  (`0.1.0-56-g614d91a`, the same code as main) is armed for its next sysc
  sign-in. Roll back with `sh profile-fixes/rollback.sh`.
- **The laptop (archThink)** had `3756c30+floating.54fd1efabf2e` armed by a
  session on 2026-11-27, while the live session was still on
  `b36865f`. It was not reachable over `ssh laptop` on 2026-11-28, so which
  build it runs now is unverified.
- **Uncommitted work in `~/sysc-compositor-archpc`** (10 files under
  `internal/wlr`, +128/−7) comes from that 2026-11-27 session. It fixes floating windows' shadows and borders drawing under tiled
  windows: it adds `layerFloatingWindows`, `windowAnimationLayer` and
  `syncViewStacking`, plus a regression test in `stacking_test.go`. It
  passed that test on the laptop and is armed there. It is not committed,
  reviewed or on any branch. It sits in the working tree of the branch
  `docs/archpc-handover-20261126`, whose only commit is this handover.

## Benchmarks (archPC, DP-1 3440×1440 at 120 Hz, still wallpaper)

These are `native.sh` runs with the same scene under each compositor: sysc
`profile-fixes/native/results/20261125-202225`, Niri
`.../20261125-203903`.

| Scenario | sysc main thread | Niri main thread | sysc fps | Niri fps |
| --- | --- | --- | --- | --- |
| idle | 11.5% | 23.1% | 119.8 | 119.9 |
| resize | 15.0% | 25.6% | 117.9 | 117.2 |
| scroll | 18.0% | 22.1% | 119.5 | 119.8 |
| workspace | 18.7% | 19.7% | 65.7 | 68.4 |
| open/close | 21.7% | 25.1% | 119.5 | 115.3 |
| overview | 26.3% | 31.7% | **115.1** | **119.6** |
| resize, blur off | 20.7% | 27.8% | 118.5 | 118.5 |

- **Cost:** sysc is cheaper everywhere.
- **Pacing:** the frame rates match except in the overview.
- **Input delay:** sysc's histogram counted 17 pointer events over 20 ms in
  the overview (worst ≤50 ms), and 5 in each resize scenario. Every other
  scenario's worst was ≤8 ms. Niri has no equivalent metric.
- **sysc-shell** used about 30% of a core under Niri and 14% under sysc.
- **The wallpaper was a still image**, so these numbers do not compare with
  the video-wallpaper baseline `main-f3abf75-highp/native/results/20261125-135339`.

## Work that remains, in order

1. **Land the floating-decoration fix.**
   1. Review the uncommitted diff.
   2. Move it to a `fix/` branch from main.
   3. Run `TestAnimationSnapshotsDrawAboveBorders` and the new stacking test.
      Check whether they build a compositor first: a nested test needs the
      owner's OK on archPC; the laptop is pre-approved.
   4. Open a PR.
   5. Check on the laptop that a floating window's shadow and border draw
      above tiled windows and under its own content.
2. **Overview animation.** Every tick of the open/close animation still
   calls `syncOverviewScene`, which destroys and re-creates every copy (see
   `animateOverviewProgress` in `animations.go`). The commit path already
   reuses copies through `overviewPlacement.project`. Make the animation tick
   reproject with the new placements instead of clearing the scene: the
   frames move and zoom, but the set of projections does not.
   - Rebuild fully only when the set of outputs, workspaces or visible windows
     changes.
   - The workspace-switch tick (`animateWorkspaceSwitch`) and the gesture
     update do the same rebuild; fix them the same way.
   - Targets: overview fps from 115 to about 119.6, and the 17 delayed input
     events.
3. **Resize input-delay spikes.** Run
   `PROFILE=1 profile-fixes/native/native.sh 15 DP-1 resize` and find what
   holds the main thread past 20 ms. Suspects: resize transactions,
   `layoutWorkspaceModel` per motion, and full-window damage.
4. **Video-wallpaper comparison.** With the gSlapper video wallpaper on,
   repeat the sysc run and compare it with `20261125-135339`. That measures
   #136 and #137 under the load that motivated them.
5. **CI crash in `fx_renderer_create`.** It hits about half the runs on main
   and every run of some branches.
   - **What we know:**
     - The crash happens when a SceneFX renderer is created in a process
       where one has already been destroyed. The renderer-loss tests are the
       first to destroy one.
     - Freeing every compositor's renderer in `Destroy` made it crash 0.6 s
       into the run. That change is on the local branch
       `fix/test-compositor-renderer-leak`: **do not merge it.**
     - The 2026-11-27 session reproduced both signatures without sysc-compositor,
       in a renderer-only probe built against CI's Mesa and LLVM packages.
       The first signature is the crash inside renderer creation. The second
       is `GL_INVALID_VALUE` on the first draw after a renderer is replaced,
       with EGL pinning on or off.
     - That session stopped before capturing a native stack; getting one is
       still the way to the root cause.
   - **Practical fix:** in `.github/ci/compositor-tests.sh`, run the
     renderer-loss tests (`TestRendererLoss*` and others that replace the
     renderer) in a separate `go test -run` invocation after the main run.
     Then no later test creates a renderer in a process that destroyed one.
   - **Root cause:** report it upstream to SceneFX or Mesa with the probe.
6. **Owner decisions still open:**
   - **SHM recovery backup.** It copies every frame of continuously
     committing surfaces, about 1 GB/s with a software-decoded video
     wallpaper. Stopping that means such a surface shows nothing for one
     frame after a GPU reset.
   - **gSlapper hardware decode.** This needs system package changes, and the
     work belongs in gSlapper's repo.
7. **Smaller open items:**
   - **Ghostty column order** in the nested suite. Capture focus and order
     after each map in sysc and in Niri.
   - **The hardware checklist** in the parity handover (§ around line 202):
     output off and on, mixed scale and rotation, cross-output popups,
     blurred capture, and reload stalls.
   - **sysc-2418 (sysc-shell loop):** the fix is merged as sysc-shell #646;
     recheck the CPU while locked.
   - **sysc-shell's CPU under Niri** (30% against 14% under sysc). Look in
     sysc-shell's repo if it matters.
8. **Release.** The memory rule "no release until NVIDIA fixed" still holds
   until the owner explicitly confirms that archPC is fixed. "Running
   flawlessly" on 2026-11-28 is the signal to ask for that confirmation. Then,
   in order:
   1. Tag the SceneFX fork with `5399deb` plus wlrfx/scenefx#208 (for example
      `0.5-sysc2` from `nvidia-highp-0.5`) and bump `~/sysc/aur/scenefx-sysc`.
      #208's shaders need a GLES 3.2 context.
   2. Switch `~/sysc/aur/sysc-compositor*` from `scenefx` to `scenefx-sysc`.
   3. Tag `v<version>` on main; the owner picks the version, and main is 65
      commits past `v0.1.0`.
   4. Bump `pkgver` and the checksums, build, and push the recipes.

## Tools and kits

- **`native.sh`** (`profile-fixes/native/`) works under sysc and under Niri.
  It finds the compositor from `$NIRI_SOCKET`.
  - Arguments: `native.sh [SECONDS] [OUTPUT] [SCENARIOS...]`. `PROFILE=1`
    adds pprof and perf profiles.
  - Under sysc it also prints frame metrics and, per scenario, an input-delay
    line. The worst value is a bucket, because `max_ms` counts since sign-in.
  - Under Niri, sign in through niri-session so that libinput's lag reports
    reach the journal.
  - The owner must leave input alone while it runs.
- **Kits** live in `~/.local/state/sysc-compositor-archpc-20261123/`. Each has
  `arm.sh` and `rollback.sh`; only one override can be armed at a time.
  - `profile-fixes/`: armed.
  - `main-inputdelay/` and `main-f3abf75-highp/`: older.
  - All of them run the patched SceneFX with #208 through `LD_LIBRARY_PATH`.
  - The local branch `kit/profile-fixes` holds the kit's merge commits; its
    code is all on main.

## Rules and traps

- Nested tests open windows on the owner's desktop. archPC needs the owner's
  OK for each run; the laptop is pre-approved. Native runs need the owner at
  the machine.
- Build with `CGO_CFLAGS_ALLOW=-fno-strict-overflow` and cap repo-wide Go
  commands (`-p 2`, `GOMAXPROCS=4`). The language server reports false cgo
  errors.
- Commit messages carry no AI attribution and none of the commit hook's
  substrings ("bot" in "bottom", "cursor", "agent").
- In Go code of a file that includes plain wlroots headers, do not read
  `wlr_scene_buffer` fields. `image_buffer.go` names that type under
  SceneFX's header, and cgo reports inconsistent definitions. Read the fields
  in a C helper instead.
- The auto-mode classifier refuses `gh pr merge`. The owner approves each
  merge prompt.
