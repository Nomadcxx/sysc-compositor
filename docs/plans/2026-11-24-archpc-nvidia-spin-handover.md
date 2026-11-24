# archPC NVIDIA spin handover

Start at `4cdb824` on `fix/archpc-backdrop-blur`, in the worktree
`~/sysc-compositor-archpc`. The branch is local and has not been pushed.
The plain `4cdb824` build is armed for archPC's next sign-in. The spin check
and the benchmark below are done; continue with the remaining work.
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

## Remaining work

- **E. SceneFX texture lifetime.** Each cached blur draw still imports the
  optimized buffer with about 4 ioctls, roughly 0.7 ms per frame for twelve
  draws offscreen. Keep the `wlr_texture` in `fx_offscreen_buffers` until
  the buffer is reallocated. This is an upstream SceneFX patch, since this
  tree takes no C.
- **Resize animation cost.** The snapshot path clones and rescales the
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
