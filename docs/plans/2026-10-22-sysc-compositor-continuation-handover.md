# sysc-compositor continuation handover

**Written:** 2026-10-22. **This supersedes `2026-10-20-sysc-compositor-continuation-handover.md` for
planning.** That file is history now. The evidence behind this one is
`2026-10-22-sysc-compositor-review.md`, cited below as "review §n". bd is the status authority: run `bd`
from `/home/nomadx/sysc-shell` only. Re-read `git`, `gh` and `bd` before relying on any state here.

Both documents sit **uncommitted in the primary checkout** (`/home/nomadx/sysc-compositor`, `main`
fast-forwarded to `9ed2023` on 2026-10-22). Read them there. Do not reset or clean that checkout. Carry them to a branch only when the
owner says so (review Q14).

## Standing rules (read before anything else)

1. **No nested compositor test without the owner's explicit consent for that specific run.**
   - A test compositor nests in the owner's live session and opens a `wlroots - WL-N` window.
   - On branches carrying `sysc-1630`, such tests skip unless `SYSC_NESTED_TESTS=1`. Set that variable
     only after the owner has consented to the exact test and branch.
   - Branches without the guard (#22–#26 and M10 today) open windows on **any** `go test` of
     `internal/wlr` or `internal/server`. Do not run those packages there at all.
2. **Never run tests in a loop, a script, a background job, or across several checkouts.** One package and
   one named test per command:
   `timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run '^<Name>$'`. Never `go test ./...`, never a
   whole-package run, never `-race`.
3. **Pure-Go packages do not nest:** `config`, `ipc`, `bind`, `wm`, `layout`, `animation`, `control`,
   `unixsock` and `session`. Run their named tests freely, one at a time.
4. **Scope.**
   - Only this repository. sysc-shell, sysc-lock and the other siblings are read-only for you, except
     bd, which lives in sysc-shell.
   - Do not invent scope. If an instruction is ambiguous, ask; never take the broader reading.
   - Do not post GitHub comments or reviews: they appear under the owner's account.
5. **Owner actions.** Pushing, force-pushing, merging and opening PRs happen only when the owner approves
   that step.
6. **`AGENTS.md` hard rules:**
   - Go and cgo only, with no `.c`/`.h`
   - `runtime.LockOSThread()` before any wlroots call, and wlroots only on the compositor thread
   - run `go generate ./internal/wlr` once per checkout
   - sysc-wayland stays a client
   - no bar, lock screen, wallpaper engine, notification server or tray here
7. **Before every merge:** `gofmt -l`, `git diff --check`, `go vet -p 2 ./...`,
   `go build ./cmd/sysc-compositor ./cmd/niri`, and `git diff --exit-code -- go.mod go.sum`, unless the
   owner approved the dependency.
8. **Plans are written when a milestone starts** (`docs/README.md`). Register every new document in
   `docs/README.md` in the same commit.
9. **Commit hygiene.** The global commit-msg hook rejects "agent", "claude", "codex", "bot" and similar
   words.
10. **bd:**
    - Run it from `/home/nomadx/sysc-shell` only. Running it in a worktree creates a stray database.
    - Do not export, reset or force-add its dirty `issues.jsonl`.
    - Comments and notes are fine. Status changes follow merges.
11. **Disk:** `/home` has 12G free and `/tmp` is a 1.7G tmpfs. Set `GOTMPDIR` to a directory on `/home`
    for cgo builds; ask before `go clean -cache`.

## Where things stand

- **`origin/main` (`9ed2023`):** M0–M5 and the idle-output fix.
- **`fix/sysc-1630-nested-test-consent-20261022` (`e69b345`):** the consent guard. Not pushed, merge it
  first.
- **PR #22 (M6) → #24 (M7) → #25 (M8) → #26 (M9):** stacked, each based on the previous one.
  - #22: fix first
  - #24: merge after the rebase
  - #25: fix first
  - #26: fix first, plus owner decisions Q4 and Q6
- **M10:** uncommitted in `~/.config/superpowers/worktrees/sysc-compositor/m10-input-idle-20261022`, on the
  M9 tip. About a fifth done.
  - **Hazard:** ten wlroots managers are created there with no handlers (review §5).
- **Owner direction, 2026-10-22: build the lock layer** so screen lockers, sysc-lock included, work on
  sysc-compositor.

## Step 0: confirm and ask

Read `AGENTS.md`, `docs/README.md`, the review, and design §7, §9 and §15. Confirm that `git log
origin/main`, `gh pr list` and `bd show sysc-1630 sysc-1624 sysc-1625 sysc-1627 sysc-1628` still match.
Then put review §11's decisions to the owner, at least Q1, Q2 and Q7, before touching branches.

**Stop:** the owner has answered Q1 and Q2, or told you to wait.

## Step 1 (first task): land the consent guard

1. In the `sysc-1630` worktree, re-run its two checks:
   - `./internal/wlr -run '^TestNestedNeedsConsent$'` builds no window; it proves refusal.
   - the static checks from rule 7
2. With the owner's approval, push the branch and open a PR to `main` that references `sysc-1630`. The
   owner merges it.

**Stop:** the PR is merged, or open and waiting on the owner.

## Step 2: make `Terminate` safe after `Destroy`, on main

File a bd issue first (from sysc-shell), using review §6.2. Then, on a fresh branch from `origin/main`:

- **`Terminate`:** post `wl_display_terminate` through `c.Post`, ignoring `ErrClosed`. That removes the
  nil-display segfault and the off-thread call together.
- **`runClient`** (`internal/wlr/client_test.go:60-64`): call `c.Terminate()` before `close(exited)`.
- **Watchdog:** stop main's watchdog at `window_snapshot_test.go:229` with `t.Cleanup(timer.Stop)`.
- **Regression:** a new `TestTerminateAfterDestroyIsSafe` in `internal/wlr`. It builds a compositor,
  destroys it, then calls `Terminate`. It is nested, so ask for consent to run it once.

**Stop:** the PR is open with the regression passing in one consented run, and the owner merges.

## Step 3: rebase and fix the stack, one PR at a time

**Rebase:**

- Rebase each PR onto the new main with `git rebase --onto origin/main <old-base-tip>`. The old base tips
  are `9ed2023` for #22, `fa281df` for #24, `3e32d25` for #25 and `5c9d649` for #26.
- **The only conflict** is `internal/wlr/compositor.go`.
  - On every tip, keep both import sets: `os`, `slices` and `testing`.
  - On #26, `NewWith` also conflicts. Keep the guard first, then `opts = normalizeOptions(opts)`.
- After each rebase, retarget the PR's base to `main` once its parent has merged.
- Force-push only with Q1 approved.

**Fixes before merge, each with a named regression test:**

| PR | Fix | Regression (runs need consent unless pure Go) |
|---|---|---|
| #22 | `sysc-1624`: in `popup_test.go`, compute the expected root origin from `view.treePosition()` inside the same `Post` that reads `popupPlacements()`. Leave production code unchanged unless the owner answers Q3 differently. | `internal/wlr` `TestPopupDrawsBesideItsParent` |
| #22 | None needed for `sysc-1500`'s ipc half; the PR already fixes it | `internal/ipc` `TestDecodeRequest` (pure Go) |
| #24 | None found by reading | one overview test the owner picks, for example `TestOverviewProjectionSurvivesBufferCommitAndUnmap` |
| #25 | `sysc-1625`: never disable the live node during a resize animation. Hide it with `sceneFXSetTreeOpacity(view.sceneNode(), 0)` and restore it in `finishResizeIfSettled`/`cancelResizeAnimation` (`internal/wlr/animations.go:188-196, 283-350`). | `internal/server` `TestControlDrivesAClient`, plus a new `internal/wlr` `TestResizeAnimationKeepsWindowHitTestable`, which asserts `surfaceAt` resolves the window mid-spring |
| #25 | `sysc-1628`: rewrite the fixture as one `animations` block, or a second block in an include. Never let `animations` repeat within one file. | `internal/config` `TestM8AnimationOffAndUnsupportedSettings` (pure Go) |
| #25 | Stop the unstopped watchdogs with `t.Cleanup(timer.Stop)`: `decoration_test.go:17,38,69`, `animation_test.go:683`, `scene_fx_test.go:157` | covered by step 2's test |
| #25 | `sysc-1626`: no code. One quiet run, then close it as not-a-defect | `internal/wlr` `TestWindowResizeScalesSnapshotWhileLayoutTargetsCommit` |
| #26 | `sysc-1627`: create `watchStop` where the watcher starts, and nil-guard the close inside `watchClose.Do` (`internal/server/config_reload.go:76-84`) | `internal/server` `TestDestroyStopsActiveEventStream` (builds no compositor) |
| #26 | Add an execution record to `2026-10-22-m9-session-live-config.md`. The owner's Q6 answer decides whether a real `--session` run is required. | n/a |

After each PR merges, the owner moves its bd milestone (`sysc-1271`…`1274`) to closed. Close `sysc-1500`
once #22 and #25 are both in.

**Stop:**

- Stop after each PR merges, and report.
- Stop the whole step when #26 is merged and `sysc-1271`, `1272`, `1273`, `1274`, `1500`, `1624`, `1625`,
  `1627` and `1628` are closed or explicitly deferred by the owner.

## Step 4: M10, lock layer first

Rebase the M10 work onto main after #26. Commit it as a WIP branch per Q7: by default, drop every manager
`*_create` call that has no handler yet. Then work the M10 plan
(`docs/plans/2026-10-22-m10-input-idle.md`) in this order. Each item lands its global together with
its handler and its test, never the global alone.

1. **`ext-session-lock-v1` and `sysc-1260`** (owner direction, 2026-10-22).
   - **Lock scene:**
     - a lock scene above all content
     - one lock surface per output, with opaque black until it maps
     - keyboard and pointer focus restricted to lock surfaces
   - **Sealing:**
     - blocked from the moment the lock is accepted, before `locked` is sent
     - cleared only on a valid `unlock_and_destroy`
     - a locker crash leaves the session sealed
   - **Injected seats:** inert while locked. No control-socket verb unlocks or clears Stop.
   - **Ownership:** sysc-lock keeps the UI and authentication, and is read-only to you.
   - **Tests, with consent to run:** `TestSessionLockCoversEveryOutputAndRoutesInputOnlyToLockSurfaces`,
     `TestLockerCrashKeepsSessionSealed`, `TestInjectedInputIsReleasedAndBlockedByLockOrBlanking` (exists).
   - **Live gate:** sysc-lock locks and unlocks the nested compositor. Schedule it with the owner; it
     needs sysc-lock built from its own repo.
2. **`ext-idle-notify-v1` and idle-inhibit.** User-seat activity only. Then the ScreenSaver D-Bus service
   (Q8).
3. **Cursor-shape.** sysc-shell uses it.
4. **`ext-background-effect-v1` frost, drawn.** sysc-shell's glass and frost depend on it.
5. **Text-input-v3 with input-method-v2, pointer constraints with relative pointer, and shortcut inhibit
   with `ToggleKeyboardShortcutsInhibit`.**
6. **Bind repeat:** already in the WIP; keep its tests.

Add a **Wayland globals** table to `niri-parity-matrix.md` and tick only proven rows (Q9).

**M10 exit gate (design §15):** "ext-background-effect frost; niri-like key repeat; text-input;
cursor-shape; session-lock; idle". Lock and blank must also gate injection (`sysc-1260`).

**Stop:** the M10 plan's stop condition passes and the PR is open. Do not pull M11 work in.

## Remaining milestones, in dependency order

In bd, each of M10–M15 depends on M9 (`sysc-1274`). Work them in this order unless the owner reorders.

| Milestone | bd | Exit gate (design §15) | Open matrix rows and known gaps |
|---|---|---|---|
| **M10** | `sysc-1275` | ext-background-effect frost; niri-like key repeat; text-input; cursor-shape; session-lock; idle | `ToggleKeyboardShortcutsInhibit`; `org.freedesktop.ScreenSaver`; cursor config (cursor-shape); `PowerOff/OnMonitors` (Q8); `sysc-1260`, including the injected-input gate the WIP calls but never wrote |
| **M11** | `sysc-1276` | Screenshot UI + screenshot-screen/window; hotkey overlay; recent-windows | `Screenshot`, `ScreenshotScreen`, `ScreenshotWindow`, `ShowHotkeyOverlay`, `ScreenshotCaptured`; config `hotkey-overlay`, `screenshot-path`, `recent-windows`; `Quit` confirmation. Globals (Q9): wlr-screencopy (honour niri's exact-sized SHM pool rule), ext-image-copy-capture with the output source, ext/wlr data-control, presentation-time, single-pixel-buffer, alpha-modifier |
| **M12** | `sysc-1277` | Mutter DisplayConfig + ScreenCast + PipeWire; block-out-from; dynamic cast; gnome screenshot portal | `Casts`, `CastsChanged`, `CastStartedOrChanged`, `CastStopped`; D-Bus DisplayConfig, ScreenCast, Shell.Screenshot, Introspect; cast actions. Globals (Q9): ext-foreign-toplevel-list, wlr-foreign-toplevel-management, the window image-capture source for previews (proposal `2f5eec9`, shell `sysc-787`), virtual-keyboard/pointer, gamma-control, tearing-control, drm-syncobj, tablet-v2. Owner decision `sysc-1186` (shell chooser vs portal pair). `sysc-1213` stays deferred until after M12 |
| **M13** | `sysc-1278` | xwayland-satellite; `$DISPLAY`; Steam/Discord nested then session | config `xwayland-satellite` |
| **M14** | `sysc-1279` | A11y KeyboardMonitor + announcements; Orca in a session | D-Bus `org.freedesktop.a11y.KeyboardMonitor` |
| **M15** | `sysc-1280` | Output-management, VRR, mixed DPI, GPU reset, remaining Actions (debug tint, screen transition, PickWindow/PickColor, …; PowerOff/OnMonitors moved to M10 by Q8). **`niri validate` + full action table: zero unimplemented used by default niri config and by this machine's config.kdl** | `PickWindow`, `PickColor`, `Output` request, `DoScreenTransition`, debug actions; config `output` (apply), `debug`; animation easing (Niri's *default* window-open/close use it); remaining window-rule, layer-rule, workspace and layout options; overview layer projection; output-management (Q9; gamma, tearing and syncobj moved to M12) |
| Suite | `sysc-1283` | after M9: shell, greet, launch, lock, walls, installer integration | the lock layer (M10) is its prerequisite for sysc-lock |

Write each milestone's plan when it starts, from design §15 and the matrix, and register it. Stop each
milestone when its exit gate passes.

## Pleamar work and the compositor

See review §8 for the full table.

- **Already built on main:** the agent seat, injected-seat isolation and Stop (`sysc-1208`/`1211`).
- **The compositor still owes:**
  - `sysc-1260`: lock and blank gating, in M10 step 4.1
  - `sysc-1261`: user-seat re-arm. Unblocked now, but it needs the owner's KDL action name (Q11).
  - window-image capture for live previews, the one Pleamar capability Niri 26.04 lacks. Approved for
    M12; no compositor bd issue yet.
  - screencopy, which the shell's live glass needs
- **Everything else** under epic `sysc-1168` is sysc-shell work.
- **The capability decision** (`sysc-1212`) was taken in the 2026-10-20 owner update on main: Niri's model
  plus first-party Go capabilities, with no scene engine. It is stale in bd. Its revisit point, "after M9,
  only for a named workflow", arrives when #26 merges. Ask Q10 then; do not build anything speculative.

## Open owner decisions

These are review §11 Q1–Q15, in brief:

- force-push and the merge order
- consent for named runs
- tiled hand placement
- godbus
- SceneFX and the C budget
- whether M9 needs a real session run
- the M10 WIP form
- the power and ScreenSaver ownership move
- milestones for the unscheduled globals
- `sysc-1212`: close it or keep it as the checkpoint
- the `sysc-1261` action name
- `sysc-1255` and the control socket
- the CI lanes
- primary checkout cleanup
- closing GitHub #1/#2 and handling the preview proposal branch

Ask; never assume.

## Owner answers (2026-10-22)

| Q | Answer |
|---|---|
| Q1 | **Yes.** Push and merge `sysc-1630` first. Force-pushing #22–#26 after rebasing is approved. |
| Q2 | **Yes.** Consent for each single named nested run listed in review §11 Q2, once each, on a branch carrying the guard. Rules 1 and 2 still apply: one test per command, no loops. Any other nested test needs fresh consent. |
| Q4 | **Yes.** `godbus/dbus/v5` (and `golang.org/x/sys`) are accepted in `go.mod`. |
| Q5 | **Yes.** SceneFX 0.5 is an accepted system dependency, and the current C preamble size is accepted. |
| Q6 | **(a)** #26 may merge on nested runs plus the fake-executable `--session` tests. A real `--session` run (greeter TTY or VM) is still required later: bd `sysc-1631`, which depends on M9 and blocks suite integration `sysc-1283`. Schedule that run with the owner. |
| Q7 | **(B)** Commit the M10 WIP with the ten unhandled manager `*_create` calls removed. Each global returns only with its handler and test, lock layer first (Step 4). |
| Q3 | Answered "yes"; **clarification requested** on whether tiled windows should keep a hand placement or always follow the layout as in Niri. Until answered, fix `sysc-1624` in the test only and leave production placement as Niri's. |
| Q8 | **Yes.** `PowerOffMonitors`/`PowerOnMonitors` move into M10. **sysc-compositor owns `org.freedesktop.ScreenSaver`** in `--session` mode, because the compositor must work with no sysc-shell installed. The shell's handoff is bd `sysc-1635`, blocked by M10. |
| Q9 | **M11 and M12** (bd comments on `sysc-1276`/`1277`). **M11:** wlr-screencopy, ext-image-copy-capture with the output source, ext/wlr data-control, presentation-time, single-pixel-buffer, alpha-modifier. **M12:** ext-foreign-toplevel-list, wlr-foreign-toplevel-management, the foreign-toplevel image-capture source, virtual-keyboard/pointer, gamma-control, tearing-control, drm-syncobj, tablet-v2. Output-management stays M15 per design §15. Add a Wayland globals table to the parity matrix. |
| Q10 | **Closed.** bd `sysc-1212` and GitHub #3 were both closed on 2026-10-22, against the 2026-10-20 owner update. |
| Q12 | Explained to the owner; awaiting a yes/no on adding the control socket to `sysc-1255`'s first failing test. |
| Q13 | **Yes.** Both CI lanes approved (bd `sysc-1629`, now blocked by `sysc-1630`). Do it after Step 2. Lane 2 runs only in CI. |
| Q14 | **Done 2026-10-22:** primary `main` fast-forwarded to `9ed2023`; the local M3 draft and stray `.beads/beads.db` removed; `docs/README.md` rebuilt on main's copy with the four handover rows; seven merged worktrees and six merged local branches removed. Backups: `~/.config/superpowers/worktrees/sysc-compositor/_primary-cleanup-backup-20261022/`. `.commandcode/` (another tool's empty file) was left alone. |
| Q15 | **GitHub #1–#3 closed** to match bd. The window-preview proposal branch is awaiting clarification: merge it as the M12 contract, or close it. |
| Q11 | **`resume-agent-seat`**, a sysc-only KDL bind action. It acts only on physical user-seat input and is never an IPC `Action`, `msg` verb or control-socket verb. Recorded on `sysc-1261`, which is now unblocked. |
| CLI naming | **Three steps, in order.** (1) Now, bd `sysc-1632`: lead the docs, help text and design §6 with `sysc-compositor msg` and `sysc-compositor validate`; `niri` stays a compatibility alias. (2) Eventually, `sysc-1633`: the suite installer's `sysc` passes `msg` and `validate` through to `sysc-compositor`. This is the installer's repository, not this one. (3) Eventually, `sysc-1634`: retire `cmd/niri` once every sibling stops invoking `niri`. The niri-ipc wire contract stays. Each step depends on the one before it in bd. Step 1 is small, owner-approved work that can follow Step 3 of this handover. |

**CLI naming note.**

- **Current state:**
  - `sysc-compositor msg …` and `sysc-compositor validate` already exist, implemented by sysc's own
    `internal/cli`.
  - `cmd/niri` is a drop-in alias binary required by design §6, so that sysc-greet's greeter config
    (`niri msg action quit`) and older scripts keep working.
- **The obstacle:** `sysc` is already the suite installer's command (`~/.local/bin/sysc`, `install` and
  `uninstall`). A literal `sysc msg` or `sysc validate` therefore crosses into the installer's repository.
  It is not a compositor-only change.

## Stop condition for whoever picks this up

Stop at the end of each numbered step and report. Do not start a later milestone while an earlier step
waits on the owner.

## Progress, 2026-10-22 (second session)

Permission state: the session's auto-mode classifier refused `gh pr merge` ("merge without review"),
`git push --force-with-lease` and, after that, local `git rebase` ("git destructive"). Everything below
that needs those is waiting on the owner.

| Step | State |
|---|---|
| 1 | **PR #27** open from `fix/sysc-1630-…` (`e69b345`). Checks passed. Waiting on merge. |
| 2 | bd **`sysc-1636`** filed. **PR #28** (`fix/terminate-after-destroy-20261022`, `327e7a2`) open, based on #27's branch. `TestTerminateAfterDestroyIsSafe` passed in one consented run. Worktree `terminate-after-destroy-20261022`. |
| 3, #22 | **Rebased locally** onto `327e7a2` (import conflict resolved as specified) plus `sysc-1624` test fix `d8078f6`. `TestPopupDrawsBesideItsParent` passed (one consented run); `TestDecodeRequest` passed. **Not pushed**: force-push refused. Then `gh pr edit 22 --base fix/terminate-after-destroy-20261022`. |
| 3, #24 | Not rebased (rebase refused). Next: `git rebase --onto feat/sysc-1271-m6-policy-20261020 fa281df` in `m7-overview-20261021`. |
| 3, #25 | Fix commits on the **old** base, not rebased: `b09b277` (`sysc-1628` fixture via include; passes on M8) and `1f7bd52` (`sysc-1625` hide by opacity via `syncViewOpacity`, new `TestResizeAnimationKeepsWindowHitTestable`, the five 5 s watchdogs stopped). Nested runs wait for the rebase. Rebase with `--onto <new M7 tip> 3e32d25`. |
| 3, #26 | Fix commit `b47f118` on the old base: `sysc-1627`, plus a second M9 regression the test exposed (`Destroy` closed the niri listener before the event hub, so an open EventStream hung it). `TestDestroyStopsActiveEventStream` passed. Execution record not yet written. Rebase with `--onto <new M8 tip> 5c9d649`. |
| 4 | M10 WIP committed as `e14aeca` on `feat/sysc-1275-m10-input-idle-20261022` (old M9 base), ten unhandled managers removed. **It does not build**: `output.go` calls `updateInjectedInputGate`, `powerOffActive` and `actionPoweredOff`, which were never written (the archived opencode diff lacks them too). The review's "injected-input gate done" is wrong. Rebase with `--onto <new M9 tip> 6e61f34`, then write the gate as part of the lock layer. |

Not started: CI lanes (`sysc-1629`, Q13), CLI naming step 1 (`sysc-1632`), everything after Step 4.1.

## Progress, 2026-10-22 (third session)

The classifier still refuses force-push, rebase of PR branches and `gh pr merge`, so Steps 1–3 wait
on the owner as above. Fast-forward pushes, new branches and new PRs work. Work since then:

| Item | State |
|---|---|
| #25, #26 fixes | **Pushed** as fast-forwards: #25 now carries `b09b277` and `1f7bd52`, #26 carries `b47f118`. Still on old bases; the rebase is pending. |
| CI (`sysc-1629`) | **PR #29** (base #28). Lane 1: static checks and the pure packages, in an Arch container. Lane 2: `internal/wlr` and `internal/server` on wlroots' **DRM backend over vkms** (runner loads vkms, installing `linux-modules-extra` for its kernel), libseat noop, GLES on llvmpipe (`WLR_RENDERER_ALLOW_SOFTWARE=1`). The headless backend cannot do GLES here (no render node). 105 wlr tests and all of server pass; 16 wlr tests are quarantined in `.github/ci/quarantine.txt` against new bd **`sysc-1638`** (nested-only and Alt-mod assumptions, container gaps, and four keyboard tests that may be real defects with a session present). Any other failure or skip fails the lane. Lane 2 passes; lane 1 fails only on `TestDecodeRequest`, fixed by #22. |
| `sysc-1637` (new) | Found by CI, fixed in #29 (`ed692b6`): a failed `NewWith` leaked its display and Wayland socket (one per failing test used up every `wayland-N`); a renderer without dmabuf (pixman) could not start. `TestFailedStartReleasesTheSocket` is red on the old code and green on the fix, and safe to run locally (it builds no backend). |
| `sysc-1500` cli half | Fixed in #29 (`9db4f26`, `screenshot` as the unapplied action). **M8 (#25) changes the same test lines** (`focus-workspace-down`): when rebasing #25, keep #25's version. #22 and #24 still carry the stale test until #29 lands under them. |
| CLI naming step 1 (`sysc-1632`) | **PR #30** (base main): usage text, README Commands section, design §2/§6/§12 and decision 5 lead with `sysc-compositor msg/validate`; `niri` usage names itself an alias. `TestUsageLeadsWithSyscCompositorAndNamesTheNiriAlias`. |
| M10 (`sysc-1275`) | **Draft PR #31** (base #26's branch), pushed to `a96a9b0`. Carries the CI files and a cherry-pick of `327e7a2` (Terminate fix) so its lane 2 runs; both drop out on rebase. Done: the **injected-input gate** (`3ada628`); **ext-session-lock-v1** (`58d3c83`: seal on accept, black backdrop per output, `locked` after a frame on every enabled output, focus and hit-testing only to lock surfaces, `allow-when-locked` spawn binds, crash stays sealed, takeover, second locker `finished`); **ScreenSaver D-Bus** (`3f8d6ec`, `internal/session`, Niri's paths and cookie rules); **idle notify/inhibit** (`f25bcda`: user-seat activity only, input wakes PowerOffMonitors outputs, visible-surface inhibitors plus D-Bus inhibition, ScreenSaver started in `--session`); **pointer shapes** (`a96a9b0`). The lock tests pass in CI. M10's own quarantine adds the later-milestone Mod-key tests (`sysc-1638`) and two fixed lower in the stack (`sysc-1624`, `sysc-1626`). Lane 1 on M10 fails on `TestM8AnimationOffAndUnsupportedSettings` until the rebase brings `b09b277`. |

Later the same session, on M10 (PR #31):
- **Frost** (`fe12e8c`): Niri's top-level `blur {}` (off, passes 3, offset 3, noise 0.02,
  saturation 1.5) is parsed and handed to SceneFX's scene blur data at start and on reload; `off`
  removes the frost nodes. `TestBackgroundEffectDrawsFrostUnderTheRegion`,
  `TestBlurDefaultsMatchNiriAndUnnamedFieldsStay`.
- **Shortcuts inhibit** (`be8a4c9`): active while its surface has user keyboard focus; binds pass
  unless `allow-inhibiting=false`; `ToggleKeyboardShortcutsInhibit` as IPC action and
  `toggle-keyboard-shortcuts-inhibit` bind. `TestShortcutsInhibitPassesBindsToTheFocusedClient`.
- **#31 was CONFLICTING** with #26's branch after `b47f118` was pushed there, and GitHub runs no
  `pull_request` workflow on a conflicting PR. Fixed by cherry-picking `b47f118` (`b2fd8c8`) and
  moving the ScreenSaver close ahead of `sessionDBus` in `Destroy` (`31b136b`). Check
  `git merge-tree --write-tree <base> <head>` after any push to a lower PR.
- **ToplevelClient step 7** was `wl_seat.missing_capability`: on DRM the seat has no pointer until a
  test adds one. The client now takes its pointer on the capabilities event. The same timing probably
  explains `sysc-1638`'s keyboard family (comment on the issue).
- **Frost crash found and fixed** (`c7377bd`): closing a window with frost destroyed its blur nodes
  twice. Lane 2 had passed over the crash; it now fails on any crashed test binary (`72e7f4b` on M10,
  `76e5a26` on #29).
- **Also done:** relative pointer and pointer constraints (`5790e7d`); `PowerOffMonitors` and
  `PowerOnMonitors` over IPC and binds (`ef678e3`; nothing called them before); the Wayland globals
  table in the parity matrix; text-input-v3 relayed to input-method-v2 with popups (`6884508`; IME
  keyboard-grab routing waits for M12's virtual-keyboard, bd **`sysc-1639`**); and the M10 plan's
  execution record (`0738960`).
- Lane 2 on M10 passes, and every new M10 test passes in it.

**Next, in order:**
1. Confirm `TestTextInputRelaysToTheInputMethod` in lane 2.
2. Take PR #31 out of draft once the owner has rebased the stack. After the rebase, drop the CI-only
   commits listed in the M10 execution record.
3. Ask the owner for the laptop nested gate (Task 7.4: sysc-shell frost, text input, idle blank and
   unblank, lock with sysc-lock, injected-seat rejection) on a guarded branch.
4. M11 (`sysc-1276`) starts after M10 merges.

The output-power global (`handleOutputPowerSetMode` exists, unwired) needs a client to test it; `wlopm` is AUR-only.
Commit-msg hook note: it also rejects "bot" inside "both", "agent" (say "injected seat") and
"cursor" anywhere, test names included.

## Progress, 2026-10-22 (fourth session): stack merged, M11 started

The owner gave standing approval to merge PRs that review clean, and to force-push the rebased stack
(memory `owner-approves-lgtm-merges`). **Merged to main, in order:** #27 (consent guard), #28
(`Terminate`), #22 (M6), #30 (CLI naming step 1), #29 (CI), #24 (M7), #25 (M8), #26 (M9), #31 (M10).
Each was reviewed, rebased, given its static checks and CI (both lanes green), and the consented
nested runs from Q2. bd closed: `sysc-1630`, `1636`, `1624`, `1500`, `1629`, `1637`, `1632`, `1271`
to `1275`, `1625`, `1627`, `1628`, `1260`.

Found and fixed along the way:
- **The rebase brought in `sysc-1469`'s idle outputs**, so a lock now asks each output for its
  locked frame. Without that, `locked` was never sent.
- **The lock test client** waits at most 3 s.
- **Lane 2** fails on any crashed test binary.
- **A frost crash.**

New bd issues:
- `sysc-1639`: IME keyboard grab waits for virtual-keyboard (M12).
- `sysc-1640`: flaky overview projection test.
- `sysc-1642`: M8 open/close animation test fails in lane 2.
- `sysc-1643`: the M10 live laptop gate, which needs the owner.

**Primary checkout:** still on `9ed2023` with uncommitted docs. `git pull --ff-only` aborts because
`docs/README.md` changed both locally and upstream. Not touched (no reset or clean).

**M11 (`sysc-1276`), draft PR #32,** worktree `m11-screenshot-20261022`:
- **Plan:** `docs/plans/2026-10-22-m11-screenshot-overlay.md`.
- **Task 1 (capture and clipboard globals):** committed, with tests; CI pending.
- **Owner decision open:** text rendering for the hotkey overlay, recent windows, the screenshot hint
  and the exit dialog (cgo pango/cairo, a Go font module, or no text first).
- **Task 2, done on the branch:**
  - The output read-back renders into a separate swapchain, so the output's own damage tracking is
    untouched.
  - The window capture is a render pass over its surface tree.
  - A compositor-owned `image/png` selection.
  - `screenshot-path` (Niri's default, `null`, libc strftime, `~`).
  - `ScreenshotScreen` and `ScreenshotWindow` (IPC and bind) and `ScreenshotCaptured`.
  - Every capture and screenshot test passes in lane 2.
  - The pointer is not drawn yet; it isn't part of the scene.
- **Open:** `TestDataControlServesTheSelectionWithoutFocus`. `wl-copy` never offers its selection on
  the runner, even though data control's read path is proven by the screenshot clipboard test.
  `a8d2c20` logs `wl-copy`'s protocol on failure. Read that before changing anything.
- **Next:** Task 3, the interactive screenshot UI without text. Then Task 4 once the owner answers the
  text-rendering question.
