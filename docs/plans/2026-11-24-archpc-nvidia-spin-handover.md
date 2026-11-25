# archPC NVIDIA spin handover

`fix/archpc-backdrop-blur` is merged into main, on 2026-11-24, after the
named and nested tests listed under remaining work passed. The worktree
`~/sysc-compositor-archpc` still holds the branch.
`corners/` is armed for archPC's next sign-in: main at `5dc96e8` with the
patched SceneFX from item E loaded through `LD_LIBRARY_PATH`.
The spin check, the benchmark and the item E patch below are done. On the
next sign-in, profile and bench it against the numbers here.
[Cache audit](2026-11-23-archpc-cache-audit.md)

## Results on 4cdb824 (2026-11-24)

The spin is gone. Idle, 15 s, `4cdb824-cstacks/profiles/idle.txt` against
`e6c2d88-cstacks/profiles/idle.txt`:

| | `e6c2d88` | `4cdb824` |
| --- | --- | --- |
| Main thread, idle | 25.3% of a core | 10.3% |
| `[vdso]` (`clock_gettime` spin) | 37% | 0 |
| `wlr_egl_restore_context` | 26% | gone |
| NVIDIA `ioctl` leaf | 91 samples | 107 samples (70%) |

About a third of what is left enters the driver from `fx_texture_from_buffer`,
which is item E. Another third enters from `sysc_debug_repaint` with the frames
in between lost, likely the same imports and syncs.

Benchmark, 20 s each with cursor motion, Ghostty typing and a resize:

| Mode | Main thread (user + kernel) | Commits/s | Scene work | libinput |
| --- | --- | --- | --- | --- |
| cached | 17.1% (6.1 + 11.1) | 110 | 3.6 s | one 27 ms debounce |
| live | 78.2% (25.2 + 53.0) | 78 | 16.6 s | 22–59 ms lag, mouse and keys |
| off | 6.0% (5.0 + 1.1) | 120 | 0.7 s | none |

Before the fix, cached blur took 34.5% of a core while idle; under load it now
takes 17.1%. Most of what cached blur still costs over blur off is kernel time
(11.1% against 1.1%), which fits the per-draw texture import of item E. The
mouse's libinput reports hit their hourly rate limit during the live run, so
the off run could only show keyboard lag reports.

### Resize animation

Native, 20 s of Super-right-drag only, cached blur:

| Resize animation | Main thread (user + kernel) | Commits/s | CPU per commit |
| --- | --- | --- | --- |
| on | 23.8% (9.4 + 14.3) | 165 | 1.44 ms |
| off | 20.4% (7.4 + 13.1) | 115 | 1.77 ms |

Nested, `4cdb824-cstacks/nested/nbench.sh 20`: one nested instance with three
translucent foot windows using xray blur over swaybg. Each 20 s phase sends
the same IPC sequence, `set-column-width` ±10% every 0.1 s with focus moving
between columns. This makes the runs repeatable; use it to A/B item E. It
opens one window on the desktop, so get the owner's OK for each run.

| Phase | Main thread (user + kernel) | Commits/s | Scene work per commit |
| --- | --- | --- | --- |
| animation on | 14.3% (7.7 + 6.7) | 100 | 1.01 ms |
| animation off | 11.1% (5.5 + 5.6) | 58 | 1.06 ms |
| blur off, animation on | 10.6% (7.0 + 3.6) | 103 | 0.21 ms |

The resize animation is not the cost. A frame costs the same with or without
it; the animation only adds frames, about 3 points of a core in both the
native and the nested runs. Snapshot cloning is not worth a fix. Blur is the
cost: with three cached consumers, scene work per commit is five times that
with blur off. That is item E.

## Branch so far

| Commit | Fix | Proof |
| --- | --- | --- |
| `92804a3`, `4b436bd` | Cached xray blur per output. Rebuilds only while a consumer is visible, and background-layer bounds pick the outputs to invalidate. | Named tests, renderer probe, native idle run with 0 rebuilds |
| `bb1773a` | Resize and animation snapshots draw above the border tree. The full-tile border fill of a client-decorated window had covered the hidden window's snapshot, which showed as yellow during resize. | `TestAnimationSnapshotsDrawAboveBorders`. The owner confirmed that switching off the resize animation removed the yellow. |
| `e6c2d88` | The SHM recovery backup patches in only commit damage instead of copying the whole frame. It was 12–13% of the idle compositor profile. | `TestRendererBackupCopiesOnlyDamage`, which fails without the surface-damage conversion |
| `4cdb824` | The renderer's EGL context stays bound on the compositor thread, set at startup and again after renderer recovery. | Idle profile with C frames, before the fix only. Offscreen, twelve cached draws dropped from 1.40 to 1.04 ms. |

## Why 4cdb824

On `e6c2d88`, about 90% of the compositor thread's user time was NVIDIA
`libnvidia-eglcore` spinning on `clock_gettime`, and the spin grew when
clients committed less. A profile with C frames placed 26% of samples under
`wlr_egl_restore_context` and 23% under `fx_texture_from_buffer`. Binding is
cheap there: `wlr_egl_make_current` was 1.3%. SceneFX restores the previous
binding after each pass and texture import. With nothing bound, each restore
released the context. The before-profile is in
`~/.local/state/sysc-compositor-archpc-20261123/e6c2d88-cstacks/profiles/idle.txt`.

## First steps (done 2026-11-24)

1. Confirm that `4cdb824-cstacks` is running, by checking `readlink /proc/$(systemctl --user show -p MainPID --value sysc-compositor.service)/exe`.
2. Run `~/.local/state/sysc-compositor-archpc-20261123/4cdb824-cstacks/cprofile.sh idle 15`
   and compare it with the before-profile. The fix holds if
   `wlr_egl_restore_context` drops out and the `[vdso]` and eglcore share
   falls sharply. If the spin moves to another call, that call is next.
   Check the journal for `could not keep the EGL context current`.
3. Benchmark (item 2): run `bench.sh cached|live|off 20` from the same kit,
   and only after the owner says go. Each 20-second window needs cursor
   motion, Ghostty typing and a Super-right-drag resize. Record main-thread
   CPU, commits/s and libinput delays, then run `bench.sh cached 3` to
   restore the owner's config. The idle numbers before the fix were cached
   34.5%, live 86% and off 25.6% of a core, at about 100 commits/s.
4. When the profile is clean, arm the plain `4cdb824` build: run the
   cstacks `rollback.sh`, then `4cdb824/arm.sh`. Never commit the
   cgosymbolizer import.

## Item E: SceneFX effect-buffer import

Every blur pass imports an offscreen effect buffer with
`fx_texture_from_buffer`. The GL texture is already cached on the
`fx_framebuffer`, but each import respecifies it with
`glEGLImageTargetTexture2DOES`, about four NVIDIA ioctls per call. Cached
blur pays this once per consumer, and live blur pays it on every pass.

Mango uses the stock SceneFX 0.5 with the same cached blur API and has no
EGL or NVIDIA handling of its own, so it carries both costs this branch
measured. Upstream SceneFX main has not touched texture import or the EGL
context since 0.5. Umbriel's vendored fork, umbrielfx, already fixes this:
buffers the renderer allocates render into their own texture, and only
renderbuffer-backed views are reimported
([umbrielfx framebuffer](https://github.com/noctalia-dev/umbriel/blob/a909081/umbrielfx/render/fx_renderer/fx_framebuffer.c#L27)).

The patch follows umbrielfx. It is `5399deb` on branch
`keep-offscreen-texture` in `~/.local/state/sysc-compositor-archpc-20261123/scenefx`,
based on the `0.5` tag, and is not pushed anywhere. Buffers from
`fx_framebuffer_get_or_create_custom` are marked renderer-owned; their FBO
attaches the texture with `glFramebufferTexture2D`, and imports skip the
respecification. Client and swapchain buffers keep the renderbuffer path.
The stencil attachment is unchanged. An earlier version kept the
renderbuffer and skipped only the respecification. It gave identical
pixels on 620.73, but it relied on EGLImage sibling coherence, which
umbrielfx avoids.

Proof: `texprobe/` (the cost probe plus a pinned EGL context and a
read-back hash) runs headless on the render node with no window. The
unpatched library is built from the same tag in `scenefx-base/`.

| Per frame, headless 2560×1440 | base 0.5 | patched |
| --- | ---: | ---: |
| cached, 12 consumers, full damage | 1.01–1.13 ms (0.68–0.74 kernel) | 0.10 ms (0.01 kernel) |
| live, 12 consumers, full damage | 9.4 ms (5.8 kernel) | 2.2 ms (0.9 kernel) |
| live, 6 consumers, typing damage | 0.94 ms | 0.30 ms |
| cached and off, 6 consumers, typing damage | 0.02 ms | 0.02 ms |

`texprobe {live|cached} 6 40 check` changes the backdrop every frame and
hashes every frame read back. The hashes match the unpatched library for
live and cached blur; blur off hashes differently, so the hash covers the
blur.

Nested, `4cdb824-fxtex/nested/nbench.sh 20`, same sequence as the nested
table above:

| Phase | stock SceneFX | patched |
| --- | --- | --- |
| animation on | 14.3% CPU, 1.01 ms scene work per commit | 11.2%, 0.23 ms |
| animation off | 11.1%, 1.06 ms | 8.2%, 0.22 ms |
| blur off | 10.6%, 0.21 ms | 10.8%, 0.21 ms |

With the patch, cached blur costs about what blur off does.

Where it lives (owner's choice: fork, package and upstream PR):

- Fork [Nomadcxx/scenefx](https://github.com/Nomadcxx/scenefx). Its `main`
  is upstream main plus the patch (`e5ed765`), which gives the same read-back
  hashes as upstream main and the same speedups. Tag `0.5-sysc1` is 0.5 plus
  the patch (`5399deb`) and keeps 0.5's soname; upstream main versions the
  library.
- Upstream: [wlrfx/scenefx#237](https://github.com/wlrfx/scenefx/pull/237).
- AUR `scenefx-sysc` builds `0.5-sysc1`. It provides `scenefx=0.5` and
  `libscenefx-0.5.so` and conflicts with `scenefx`. It was built, its meson
  tests run, `check.py` passed and it was installed in a clean
  `archlinux:latest` container. The recipe is in `~/sysc/aur/scenefx-sysc`,
  and `check.py` there now knows this one library package.

Native on `4cdb824-fxtex` (signed in 23:16; the process mapped the kit's
patched library), gSlapper paused with SIGSTOP so the backdrop holds still,
as in the earlier runs. Each 20 s window had cursor motion, Ghostty typing
and a resize, and started 7 s after the prompt. The first two cached
attempts ran without the full workload and are discarded.

| | stock SceneFX (`4cdb824`) | patched (`4cdb824-fxtex`) |
| --- | --- | --- |
| idle | 10.3% of a core | 2.8%, 55 commits/s, 0 rebuilds |
| cached | 17.1% at 110 commits/s, 1.56 ms per commit | 12.5% at 164 commits/s, 0.76 ms per commit |
| live | 78.2% at 78 commits/s, lag 22–59 ms | 28.1% at 91 commits/s, no lag reports |
| off | 6.0% at 120 commits/s | 4.0% at 71 commits/s |

libinput still reported lag outside the sampling windows: 30 and 33 ms in
the first 40 s after sign-in, and 23–43 ms around the `load-config-file`
switches between runs. The owner reports that it feels faster and lags
less, but is still short of Niri.

The wallpaper is now an MP4 that gSlapper plays at 30 fps on each output.
That makes the cached blur rebuild on every video frame (48 rebuilds/s), and
idle rises to 11–12% of a core. Each gSlapper process takes about 96% of a
core: GStreamer decodes H.264 in software with openh264. `nvcodec` fails
with "Unable to initialize CUDA library" although `libcuda` and `libnvcuvid`
are installed, and `libgstlibav.so` is blacklisted; an unowned
`libgstlibav.so.bak-avcodec62` sits next to it. Each output decodes the
same file separately.
After that, switch `sysc-compositor` and `sysc-compositor-git` on the AUR
from `scenefx` to `scenefx-sysc` with a pkgrel bump, and drop the package
once a SceneFX release carries #237. Unrelated to this work, `check.py` also
trips on a stale `sysc-terminal` 0.1.0 archive and on `syscgo`'s missing
`.INSTALL`.

## Nested NVIDIA parity suite (2026-11-25)

The owner reports parity on the laptop, which has no NVIDIA GPU, but not on
archPC. `~/.local/state/sysc-compositor-archpc-20261123/corners/nested/suite.sh sysc|niri 15`
runs one nested compositor on the RTX 4060 with `suite.kdl`, which sysc and
Niri read unchanged. The scene has the archPC appearance (gaps, focus ring,
shadows, radius 20 with clip-to-geometry), three translucent foot windows
(SHM) and a Ghostty window (GL, NVIDIA dmabuf) with xray blur, and a floating
`weston-simple-egl` whose fps counts missed frames. The same `niri msg`
actions drive nine 15 s scenarios. The suite screenshots only the nested
output. Each run opens one window on the desktop, so get the owner's OK for
each run.

Parity means three things here: the same screenshots, the same simple-egl
fps, and compositor CPU no higher than Niri's. Results with sysc on
`5dc96e8` plus the patched SceneFX:

| Scenario | sysc main thread | Niri main thread | sysc simple-egl | Niri simple-egl |
| --- | ---: | ---: | ---: | ---: |
| idle | 9.7% | 17.5% | 160 fps | 158 fps |
| resize | 13.6% | 18.7% | 159 | 140 |
| scroll | 11.3% | 16.4% | 161 | 158 |
| workspace switch | 8.7% | 11.9% | 101 | 88 |
| open and close | 13.7% | 17.3% | 159 | 144 |
| overview | 16.2% | 17.8% | **124** | 157 |
| resize, blur off | 12.8% | 17.7% | 159 | 157 |
| video wallpaper, idle | 14.1% | 19.5% | 159 | 159 |
| video wallpaper, resize | 17.5% | 19.4% | 160 | 154 |

simple-egl is hidden for half of the workspace scenario, which explains its
lower fps there. Nested, sysc now costs less than Niri and keeps up in every
scenario except overview, where the probe loses about 20% of its frames.
That is the next nested lead. Nested runs leave out libinput, KMS commits
and sysc-shell, so the native scripts still have to measure those.

The first run found the owner's "corners that are not really rounded": a
window's blur took corner radii only for popups, so a square of frost
showed past every rounded corner of a translucent window. `5dc96e8` follows
Niri's `render_params_for_tile`, and the corner screenshots now match Niri.
Niri's frost is darker than sysc's in the same scene; compare the blur
defaults (noise, saturation, brightness) next.

`corners/` is armed for the next sign-in: `5dc96e8` with the patched SceneFX
and the bench socket.

## Remaining work

- **E. SceneFX texture lifetime (patched 2026-11-24, see below).**
- **Resize animation cost (measured 2026-11-24, not worth a fix).** The snapshot path clones and rescales the
  window on every frame. Measure resize with the animation on and off after
  `4cdb824`.
- **Commission checks.** Test a background popup across DP-1 and DP-3, a
  rotated or scaled output, a power cycle, screenshots and screencasts with
  blur, and an animated wallpaper. A screencast that masks a background
  layer rebuilds the shared cache twice per frame. Niri keeps a separate
  buffer per render target.
- **F. sysc-shell.** The bar makes about 18,700 write syscalls a second to
  its plugin pipes and uses 15% of a core. File this in the sysc-shell repo.
- **Tests before the merge (2026-11-24, owner OK for each nested run).**
  Nested: `TestRendererLossRestoresLatestSHMCommit`,
  `TestRendererLossRecreatesAndRepaintsExistingBuffers` and
  `TestBackgroundEffectDrawsFrostUnderTheRegion` passed. Every other test
  the branch adds passed without nesting, and so did `go vet` and `gofmt`
  on the touched packages. Run them with
  `CGO_CFLAGS_ALLOW=-fno-strict-overflow`. The commission checks above are
  still owed natively.
- **Tickets.** `sysc-2417` (performance), `sysc-2416` (Acer re-enable),
  `sysc-2303` (SceneFX culling).

## Tools and traps

- perf 7.3.5 is unpacked without installing; run it through
  `~/.local/state/sysc-compositor-archpc-20261123/tools/perf`. `perf_event_paranoid=2` allows flat `cpu-clock:u` profiles
  of the running thread, but no call graphs and no `cycles`. Use the cstacks
  build for stacks.
- `csym.py`, kept in each kit, resolves cgosymbolizer's library-only frames
  against `nm -D` and the process maps. Static functions resolve to the
  nearest exported neighbour.
- `/tmp` is cleared at boot. The worktree, staged builds and profiles live
  under `$HOME`.
- `ptrace_scope=1` blocks strace and gdb attach. Measure clients with
  `/proc/PID/io` counters, or pause them with SIGSTOP and SIGCONT. The owner
  allowed that for this work.
- The commit-msg hook rejects substrings such as "bot", "both", "agent" and
  "cursor". Check messages with the grep in the commit-hook memory.
