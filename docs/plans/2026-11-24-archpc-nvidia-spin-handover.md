# archPC NVIDIA spin handover

Start at `4cdb824` on `fix/archpc-backdrop-blur`, in the worktree
`~/sysc-compositor-archpc`. The branch is local and has not been pushed.
`4cdb824-fxtex` is armed for archPC's next sign-in: the `4cdb824-cstacks`
binary with the patched SceneFX from item E loaded through `LD_LIBRARY_PATH`.
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

Still open for item E: the native profile and bench on `4cdb824-fxtex`,
one nested run of `4cdb824-fxtex/nested/nbench.sh 20` to compare with the
nested table above, and where the SceneFX patch lives. This tree takes no
C, so the compositor can only pick it up through a SceneFX build that
carries it.

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
- **Nested tests not yet run** for `e6c2d88` and `4cdb824`:
  `TestRendererLossRestoresLatestSHMCommit` and
  `TestRendererLossRecreatesAndRepaintsExistingBuffers`. Auto mode blocks
  them until the owner OKs that specific run.
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
