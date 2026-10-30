# Handover: after M12 (2026-10-30)

Read this first. The previous handover is
[2026-10-30-m12-vm-gate-handover.md](2026-10-30-m12-vm-gate-handover.md), whose remaining work is
done. Standing rules from the 2026-10-23 continuation handover still hold. Status lives in bd
(`/home/nomadx/sysc-shell`); the gate assessment is
[2026-10-28-release-readiness.md](2026-10-28-release-readiness.md).

## Where things stand

Main is `7d8e05b`. Merged on 2026-10-30, all after both CI lanes passed:

| PR | Change |
|---|---|
| #72 | a session started without a config now loads one created later: the user-then-system search runs on every load and both paths are watched, as Niri's `ConfigPath::Regular` |
| #71 | the M12 VM gate handover |
| #73 | the M12 plan's "Execution record: VM gate, 2026-10-29/30", matrix (`org.gnome.Mutter.ScreenCast` `[x]`) and release assessment |
| #74 | `sysc-2078` test fixes: the cast test waits for the closed window to unmap; the wofi layer test waits on wofi's focused layer, allows 8 s / 10 s and reports wofi's exit and stderr |

M12 is closed (`sysc-1277`, owner decision). Hardware qualification moved to `sysc-2081`: gamma
on a physical panel, explicit sync on NVIDIA, a physical tablet and pad.

## Open items

1. **`sysc-2078`, one unexplained kill.** After the fixes, two full lane-2 `./internal/wlr` runs
   passed (226 tests, four listed skips). A third was SIGKILLed during
   `TestLayerKeyboardFocusKeepsWindowActionsAndSnapshot` with no test output. `/proc/vmstat`
   `oom_kill` stayed 0; systemd-oomd is inactive; the user slice has no memory limit; the
   journal shows nothing. Every lane-2 session in the 6 GB VM peaks at 5.1–5.3 GB with
   300–500 MB swapped. Next: run full lane 2 a few times (one command each) on the merged #74
   tree, and if the kill recurs, catch the sender (e.g. `auditctl -a always,exit -F arch=b64
   -S kill,tgkill,tkill -F a1=9` in the VM) before changing any test.
2. **`sysc-2056`:** game frame pacing against Niri 26.04.
3. **M13, X11 applications (`sysc-1278`):** xwayland-satellite lifecycle, DISPLAY publication,
   native/X11 focus, Steam/Discord. Write the milestone plan first, from design §15 and the matrix.
   Compare Niri, Mango and Hyprland before recommending any policy.
4. **First-use:** Niri writes its default config to `~/.config/niri/config.kdl` at first start;
   sysc does not (release assessment, first-use gate). It is an owner decision whether to match.

## Process warnings from this session

- **Another agent worked in this session's worktree.** While `sysc-2078` was in progress,
  something committed the uncommitted edits in `.recovery/test-races`, added its own changes, and
  opened and merged #74 under the owner's account. Codex, an opencode server and a second
  `claude --resume` were running. Before starting, check `git status`, `git worktree list` and
  open PRs, and ask the owner which agent owns which work. A duplicate PR (#75) was closed.
- `gh pr merge` was refused by the auto-mode classifier even with the standing merge approval;
  it worked once auto mode was off.
- The global commit-msg hook rejects a `Co-Authored-By: Claude` trailer; omit it.
- `/tmp` on the desktop is a near-full tmpfs and gcc ignores `GOTMPDIR`: build with
  `GOTMPDIR=$PWD/.gotmp TMPDIR=$PWD/.gotmp`, and remove the directory before committing.
- Never `pgrep -f`/`pkill -f` over `vssh` (the pattern matches your own shell).

## Session VM

`~/vm/sysc-session` on the laptop is powered off. `/usr/local/bin/sysc-compositor` is #72's build
(the earlier one is `.bak-20261029`); there is no `~/.config/niri/config.kdl` (the block-out rule
is kept at `~/blockout.kdl`). `~/sysc-compositor` in the VM is a pre-#74 tree: replace it with
`git archive` of main before lane 2. `./run.sh` boots virgl for session gates and
`./run.sh lane2` boots vkms only for lane 2. Gate scripts, including `castrun.sh` for portal cast
frames, are in [assets/2026-10-29-m12-vm-gate](assets/2026-10-29-m12-vm-gate/).

## Execution record, 2026-10-30/31

The owner assigned this session all handover work. Worktree:
`~/.config/superpowers/worktrees/sysc-compositor/post-m12-20261030`, branch
`work/post-m12-20261030`. The primary checkout remains untouched.

Three foreground `./internal/wlr` lane-2 passes on merged #74 (`7d8e05b`)
each passed 226 tests with the same four listed skips: 75.652 s, 75.980 s and
75.925 s. The unexplained SIGKILL did not recur. During these runs the VM
audited b64 `kill`, `tkill`, `pidfd_send_signal` with signal argument 9 and
`tgkill` with its third argument 9. Audit lost zero events, recorded no matching
signals, and `/proc/vmstat`'s `oom_kill` stayed zero. This does not identify the
earlier sender or prove the kill cannot recur. The tracker already had
`sysc-2078` closed when this session started; no further test change was made.
Logs are under `~/.cache/sysc-post-m12/lane2-wlr-{1,2,3}.log` on the desktop.
The original VM checkout was preserved as
`~/sysc-compositor.pre-post-m12-20261030` before installing the #74 archive.

The [frame-pacing source audit](2026-10-31-frame-pacing-audit.md) records the
native mechanisms and remaining benchmark. M13's plan is
[2026-10-30-m13-x11.md](2026-10-30-m13-x11.md).

**Owner decisions:** keep in-memory defaults; do not write a first-use config.
Borrow Niri's approach where it fits; this project is not a Go rewrite of Niri.
Keep wlroots/SceneFX and satellite responsible for their existing machinery.
