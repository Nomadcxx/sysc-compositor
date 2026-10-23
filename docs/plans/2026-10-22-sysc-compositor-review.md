# sysc-compositor review, 2026-10-22

**Kind:** report. **Commission:** `2026-10-22-sysc-compositor-review-handover.md`. **Method:** code reading. The
only tests run were three pure-Go named tests (listed under Evidence). No compositor was built in a test,
and no window opened. One M8 binary was built to `$TMPDIR` to inspect its link order, then deleted.

This is a dated snapshot. bd (run from `/home/nomadx/sysc-shell`) is the status authority. Re-read `git`,
`gh` and `bd` before acting on any state claim here.

## 1. Snapshot check

The commission's tables match `git`, `gh` and `bd` on 2026-10-22, with these additions:

- `origin/main` is `9ed2023`. The primary checkout `main` is `653ff83`, 26 commits behind.
- The primary checkout's untracked `docs/plans/2026-10-19-m3-session.md` is an **older draft** of the file
  `origin/main` now tracks (6 lines added, 18 removed against main). Its `docs/README.md` also diverges from
  main's: main has newer rows for the WM-as-scene update, M3, M4, the capability report, offscreen focus
  and M5. A fast-forward of the primary `main` will refuse until the untracked M3 draft is moved aside.
  `.beads/beads.db` (stray) and `.commandcode/` are untracked too.
- PR heads, bases, sizes and `MERGEABLE/CLEAN` match the commission. `gh pr checks` has no checks.
- bd statuses match. `sysc-1624`–`1630` have no labels and no dependency edges, so `bd blocked` does not
  show them blocking `sysc-1271`/`1273`/`1274`. M10–M15 each depend on M9 (`bd dep tree sysc-1280`).
- bd change made by this review: one verdict comment on each of `sysc-1624`–`1628`, nothing else.
  `issues.jsonl` was not touched (only `beads.db` changed).

### Worktrees (`git worktree list`)

| Worktree | Tip | State |
|---|---|---|
| primary `/home/nomadx/sysc-compositor` | `653ff83` | stale `main` with the owner's local docs. Keep. |
| `sysc-1630-nested-test-consent-20261022` | `e69b345` | **live**, not pushed, no PR |
| `m6-policy-20261020` … `m9-session-20261022` | `fa281df`, `3e32d25`, `5c9d649`, `6e61f34` | **live**, PRs #22, #24, #25, #26, clean trees |
| `m10-input-idle-20261022` | `6e61f34` + uncommitted | **live**, uncommitted M10 work |
| `fix/sysc-1469-idle-output-20261021` | `03a87b4` | merged (#23), removable |
| `m0-pointer-routing` | `adfeef1` | merged (#5), upstream branch gone, removable |
| `m3-session-20261019`, `m4-windows-20261019`, `m5-layout-20261020`, `offscreen-focus-20261020` | | merged, removable |
| `vt-session-20261018` | `bb69d4d` | 3 commits not ancestors of main, but `git cherry` shows all three patch-equivalent on main (#16, `bb110cb`). Removable |
| `proposal/window-preview-capture-20261018` | `2f5eec9` | **unmerged** one-doc proposal, no PR. See §8. |
| `_opencode-agent-patches-20261022` | | archive, not a worktree |

## 2. What is done

| Milestone | Where | Evidence |
|---|---|---|
| M0–M4 | `origin/main` | merged PRs #5–#18; bd `sysc-1266`–`1269` closed |
| M5 | `origin/main` | PR #19 merged; bd `sysc-1270` closed |
| idle-output fix | `origin/main` | PR #23; bd `sysc-1469` closed |
| consent guard | branch only | `e69b345`, §3 |
| M6 | PR #22 | §4.1 |
| M7 | PR #24 | §4.2 |
| M8 | PR #25 | §4.3 |
| M9 | PR #26 | §4.4 |
| M10 | uncommitted | §5 |

Through M9 the compositor creates 16 wlroots globals: compositor, subcompositor, seat, data-device,
xdg-shell, xdg-output, xdg-activation, xdg-decoration, KDE server-decoration, layer-shell,
fractional-scale, viewporter, pointer-gestures, linux-dmabuf, plus a cursor and a keyboard group. **Not
created anywhere through M9:** presentation-time, screencopy, ext/wlr data-control, foreign-toplevel and
ext-foreign-toplevel-list, image-copy-capture, virtual-keyboard/pointer, tablet-v2, gamma-control,
output-management, single-pixel-buffer, alpha-modifier, tearing-control, drm-syncobj. Design §7 lists them
all as required. §15 assigns only some of them to a milestone, and the parity matrix has no globals table,
so most are tracked nowhere (§8, owner question Q9).

## 3. `sysc-1630` (consent guard), `e69b345`

**Verdict: merge first.**

- `NewWith` returns `ErrNestedTestNeedsConsent` when `testing.Testing()` and `SYSC_NESTED_TESTS != "1"`
  (`internal/wlr/compositor.go:220-222`). This covers `wlr_backend_autocreate` (`compositor.go:265`).
  The only other backend, `wlr_headless_backend_create` (`fakeoutput.go:25`), needs a compositor first.
- Every test caller of `New`/`NewWith`/`server.New` skips on error: the wlr helpers `newNestedWith`,
  `post_test.go:19`, `seat_test.go:90`, `keyboard_test.go:91` and `compositor_test.go:40,107`, plus
  `server/control_test.go:59` and `window_action_test.go:32`. The same holds on M6–M10. The M9 `--session`
  CLI tests only parse arguments.
- Importing `testing` in production code is the intended use of `testing.Testing()` (Go ≥1.21; the module
  says 1.26.4).
- **The rebase is mechanical.** `git merge-tree` against each stack tip conflicts in
  `internal/wlr/compositor.go` only. On all four tips the conflict is the import block: keep `os`,
  `slices` and `testing`. On M9 it is also the first statement of `NewWith`: keep the guard, then
  `opts = normalizeOptions(opts)`.

## 4. The PR stack

### 4.1 #22, M6 window policy: **fix first** (one test)

- **Exit gate (§15):** consume/expel, tabbed columns, floating, fullscreen, windowed-fullscreen, named
  workspaces, monitor moves, window-rule and layer-rule applied.
- **Implemented:** all of it, in pure-Go `internal/layout`, `internal/wm` and `internal/config/rules.go`,
  with nested WLR integration. Its plan carries a detailed execution record, including a nested
  sysc-shell run.
- **Partial, recorded honestly in the matrix:**
  - window-rule applies placement, fullscreen and default sizes only. A rule with any unsupported match
    key is disabled entirely, with a warning (`rules.go:87-90`).
  - layer-rule only classifies `place-within-backdrop`.
  - Monitor moves are proven on a synthetic second output only.
- **Extra:** xdg-activation for urgency (`xdg.go` `handleActivationRequest`). This fits M6's urgency events.
- **Matrix:** updated for every M6 row.
- **Tests:** pure Go wherever the logic is pure (layout, wm, config, bind, ipc). The nested tests cover
  scene and xdg behaviour.
- **Fixes `sysc-1500`'s ipc half.** `TestDecodeRequest` passes on `fa281df` and fails on main; both were
  run.
- **Blocking:** `sysc-1624`, below.

### 4.2 #24, M7 overview and input: **merge after the rebase**, with one consented run

- **Exit gate:** overview, gestures (touchpad 3/4, hot corners), insert hint.
- **Implemented:**
  - overview state, IPC, projection and pointer selection
  - gesture, hot-corner and DND edge-scroll logic
  - insert hints, with a pure-Go `wm/overview.go` and a renderer viability gate first
- **Missing, and recorded in the matrix:**
  - background and bottom layer surfaces are not projected into overview thumbnails
  - `place-within-backdrop` is not drawn
  - physical touchpad, bound-client gesture forwarding and live DND are unverified
- **No defect found by reading.**
- **The "order-dependent full-package failure"** claimed in the PR comment is **unverified and not
  reproducible under the rules.** It came from a whole-package run, which `AGENTS.md` forbids, made
  during the five-checkout loop. The likeliest cause is the watchdog defect in §6.2, which crashes or
  terminates whichever test is running when a stale timer fires.
- **Watch:** `internal/wlr/testclient` hand-writes `wl_interface` tables in a cgo preamble. That is
  allowed (no `.c`/`.h`, not scanner output), but see §7.3.

### 4.3 #25, M8 animations and appearance: **fix first**

- **Exit gate:** springs for the animation kinds in this machine's config, plus focus-ring, shadow,
  corner-radius, clip-to-geometry and prefer-no-csd.
- **Implemented:**
  - pure-Go springs (`internal/animation`), applied for the laptop's seven animation kinds
  - focus ring, shadow, corner radii and clipping through SceneFX 0.5
  - xdg-decoration and KDE server-decoration
- **Missing, recorded:** easing, slowdown and custom shaders warn. Niri's *default* window-open and
  window-close use easing, so they do not animate by default (M15 matters).
- **New system dependency:** `scenefx-0.5`.
  - Nine `internal/wlr` files include wlroots' `wlr_scene.h`; `scene_fx.go` includes SceneFX's.
  - SceneFX appends fields to `wlr_scene`, `wlr_scene_buffer` and `wlr_scene_rect`, so the layouts are
    prefix-compatible.
  - Both libraries export `wlr_scene_*`. The built M8 binary lists `libscenefx-0.5.so` before
    `libwlroots-0.20.so` in `DT_NEEDED`, so every scene call binds to SceneFX consistently.
  - This is safe today, but it depends on cgo flag order (risk, Q5).
- **Blocking:**
  - `sysc-1625` (confirmed by reading)
  - `sysc-1628`'s fixture, which lives in this PR
  - the unstopped test watchdogs this PR adds (§6.2)

### 4.4 #26, M9 session and live config: **fix first**, plus an owner decision

- **Exit gate:** `--session`, graphical-session.target, `niri` CLI complete (`msg` all requests),
  live-reload, include merge, LoadConfigFile.
- **Implemented:**
  - all 16 request forms in `msg`
  - inotify reload, parsed off-thread and applied through `Post`
  - `ConfigLoaded`, `LoadConfigFile` and keyboard layouts
  - `switch-events`, plus locale1 and login1 through godbus, with callbacks posted to the compositor
    thread
  - `--session` environment publication and the target lifecycle
  - include merge: later sections set only the fields they name, matching Niri's `m_merge`
  - Niri's own duplicate-section rule (`niri-config/src/lib.rs:157-176`), copied exactly
    (`config.go:339-345`)
- **Missing:**
  - later `msg action` subcommands
  - **any real `--session` run.** It is proven only with fake executables, the plan has no execution
    record, and §15 says "each gate is a running binary" (Q6)
- **Extra:** `go.mod` gains `github.com/godbus/dbus/v5 v5.2.2` and `golang.org/x/sys` (Q4).
- **Blocking:** `sysc-1627`.
- **Fixes `sysc-1500`'s cli half:** M8 had already changed the expectation (`cli_test.go:438,458`). The fix
  is confirmed by reading, not run.

## 5. M10 worktree (uncommitted)

- **Plan:** `2026-10-22-m10-input-idle.md` was written when M10 started, which `docs/README.md` allows. It
  covers the §15 M10 row plus the 2026-10-20 handover's additions (relative pointer, pointer constraints,
  shortcut inhibit, `sysc-1260`). It moves `PowerOffMonitors`/`PowerOnMonitors` from M15 to M10 and
  adds the `org.freedesktop.ScreenSaver` service. Both moves are stated with reasons (Q8). The stop
  condition is clear. It is not yet registered in `docs/README.md` on a pushed branch.
- **Done, about a fifth of the plan:**
  - bind repeat on a Wayland timer, with config `repeat=` and tests
  - `PowerOffMonitors`/`PowerOnMonitors`, **partly.** Correction, 2026-10-22: `output.go:367-409` calls
    `updateInjectedInputGate` and uses `powerOffActive` and `actionPoweredOff`, none of which exist, so
    `go vet ./internal/wlr` fails on the M10 tip `e14aeca`. The injected-input gate is not written; the
    first review read the call sites as the implementation. It belongs to the lock layer (Step 4).
  - the `ext-background-effect-v1` protocol objects with a region committed per surface. No blur is drawn.
  - two vendored protocol XMLs
- **Not done:** lock surfaces, idle activity and inhibit, text-input relay, cursor-shape, pointer
  constraints, relative motion, shortcut inhibit, SceneFX blur, ScreenSaver D-Bus, matrix and execution
  record.
- **Hazard.** `compositor.go` creates ten managers: cursor-shape, idle-inhibit, idle-notifier,
  input-method, shortcuts-inhibit, output-power, pointer-constraints, relative-pointer, session-lock and
  text-input. **None has a listener.** Advertised without behaviour:
  - a session-lock client (sysc-lock, swaylock) gets a global that never locks or answers
  - `ext-idle-notify` clients never see user activity, so sysc-walls and the shell's idle path would
    blank or lock on a timer while the user types
  - cursor-shape requests from sysc-shell are ignored
- **Verdict:** coherent enough to commit as a **WIP branch only.** Never merge or run it in a session in
  this state. Either drop the unhandled `*_create` calls from the WIP commit or land each manager only
  with its handler and test.
- **Owner direction, 2026-10-22:** build the lock layer so screen lockers, sysc-lock included, work on
  sysc-compositor. That is `ext-session-lock-v1` with `sysc-1260`. The continuation handover makes it the
  first M10 deliverable. The lock UI and authentication stay in sysc-lock (`AGENTS.md`: no lock screen
  here).
- **Archived opencode patches:**
  - The per-worktree `testbackend_test.go` is broken: `useHeadlessBackend` calls itself.
  - The diff's idea of a headless backend with `wlr_headless_add_output` is the right basis for a CI
    lane (§7.1).
  - The nil-safe `Terminate` points at a real defect, but the patch does not fix it (§6.2).

## 6. `sysc-1624`–`1628` verdicts

| bd | Verdict | Evidence | Fix, and its regression test |
|---|---|---|---|
| `sysc-1624` | **Confirmed; it is the test's contract**, not popup logic | Main already positions windows from the layout. M6 adds a reposition on every geometry-changing commit (`internal/wlr/input.go:765-772` at `fa281df`, through `positionWorkspaceWindow`, `workspaces.go:305-318`). weston-terminal's configured commit snaps the hand placement from `placeView` (`popup_test.go:33`) back to the tile before the popup is sampled. The popup still hangs under its parent with an output constraint. | On #22: compute the expected root origin from `view.treePosition()` inside the same `Post` that reads `popupPlacements()`. Test: `TestPopupDrawsBesideItsParent`. |
| `sysc-1625` | **Confirmed by reading** | `animateViewSize` disables the live node (`animations.go:306` at `5c9d649`), and `setViewSceneEnabled` keeps it disabled while a snapshot exists (`:188-196`). `wlr_scene_node_at` skips disabled nodes. The clone holds plain buffers, so `sysc_scene_surface_at` (`input.go:48-56`) returns nil. Every new window triggers it (`workspaces.go:495-496`). A disabled node also gets no frame callbacks. | On #25: keep the node enabled and hide it with `sceneFXSetTreeOpacity(…,0)`; restore it in `finishResizeIfSettled` and `cancelResizeAnimation`. Tests: `TestControlDrivesAClient` (server), plus a new `TestResizeAnimationKeepsWindowHitTestable` (wlr). |
| `sysc-1626` | **Refuted as stated** | `Post` fails only after `stopPost` and never drops work while open (`post.go:55-71`). Ticks sample `ValueAt(now-started)` (`animations.go:41-58`), so lateness cannot starve a spring. The 0.39 s "did not complete" matches a nested session ending because its host window closed. The run happened during the flood, while the owner was closing windows. | Close after one quiet consented run of `TestWindowResizeScalesSnapshotWhileLayoutTargetsCommit`. The real defect nearby is §6.2. |
| `sysc-1627` | **Confirmed by reading** | `stopConfigWatcher` closes `watchStop` unguarded (`server/config_reload.go:76-84`). Only `New` makes it (`server.go:67`). `Destroy` calls it first (`:408-409`). `TestDestroyStopsActiveEventStream` builds `&Server{…}` (`eventstream_test.go:329`). | On #26: create the channel when the watcher starts and nil-guard the close inside the `Once`. Test: `TestDestroyStopsActiveEventStream` (builds no compositor). |
| `sysc-1628` | **Confirmed failing (run); the fault is M8's fixture** | `TestM8AnimationOffAndUnsupportedSettings` fails on `6e61f34` with "duplicate node animations". M9 matches Niri 26.04, which rejects a repeated `animations` in one file (`niri-config/src/lib.rs:157-176`). | On #25: one `animations` block per file, or move the second block into an include (sections merge across includes). Do **not** add `animations` to `multipleTopLevelNodes`. Test: the same one. |

### 6.2 New defect: `Terminate` after `Destroy` (not in bd yet)

- `Compositor.Terminate` is `wl_display_terminate(c.display)` (`compositor.go:424` on main). After
  `Destroy`, `c.display` is nil (`:473`).
- Timers that outlive their compositor call it:
  - `runClient` closes `exited` before calling `c.Terminate()` (`client_test.go:60-64`), so the test can
    return and `Destroy` can run while `Terminate` is still in flight.
  - Watchdogs that are never stopped: `time.AfterFunc(…, c.Terminate)` at main's
    `window_snapshot_test.go:229`, and M8's `decoration_test.go:17,38,69`, `animation_test.go:683` and
    `scene_fx_test.go:157`.
- **Evidence:** `evidence-tmp-opencode/m9-wlr-full.log` shows `SIGSEGV` in `_Cfunc_wl_display_terminate(0x0)`
  from `time.goFunc`.
- **Fix, on main:** make `Terminate` post `wl_display_terminate` through `Post` and ignore `ErrClosed`.
  That also keeps the libwayland call on the compositor thread. Also swap the two lines in `runClient`,
  and stop each watchdog with `t.Cleanup(timer.Stop)`.
- **Test:** `TestTerminateAfterDestroyIsSafe`. It is nested, so it needs consent.
- A nil check alone narrows the race but does not close it.

## 7. Cross-cutting

### 7.1 CI (`sysc-1629`)

- **Lane 1, needs no display:** `gofmt`, `go vet -p 2 ./...`, both builds, `go.mod`/`go.sum`
  diff, and named tests of the pure packages: `config`, `ipc`, `bind`, `wm`, `layout`, `animation`,
  `control`, `unixsock`, `session`. It needs an Arch image with wlroots 0.20, SceneFX 0.5 and
  wayland-protocols for the cgo build.
- **Lane 2 (owner decision):** `internal/wlr` and `internal/server` with `SYSC_NESTED_TESTS=1`,
  `WLR_BACKENDS=headless`, `WLR_RENDERER=pixman` and an output added through `wlr_headless_add_output`.
  It is safe there because CI has no desktop. Tests that spawn weston-terminal or foot need those
  installed.
- The guard keeps lane 2 off developer desktops by default.

### 7.2 Disk

- `/home` is at 96% with 12G free.
- `/tmp` is a tmpfs at 80% with 1.7G free, and cgo test links land there because `GOTMPDIR` is unset.
- The Go build cache is 3.6G. Worktrees are under 8M each.
- Set `GOTMPDIR` to a directory on `/home` for cgo builds, and run `go clean -cache` when space is short
  (owner).

### 7.3 Locked-thread rule and cgo size

- **Locked-thread rule:** no violation found on M9 or M10.
  - Every production `time.AfterFunc` in `internal/wlr` (animation tick, DND edge tick, focus debounce,
    close snapshot) only calls `Post`.
  - Server actions run inside `s.c.Post`. Config reload applies through `Post`, and D-Bus callbacks post
    through `postSessionUpdate`.
  - `quit: s.c.Terminate` runs off-thread. libwayland allows that, but §6.2 moves it onto the thread
    anyway.
- **cgo size:** C in `internal/wlr` cgo preambles grows from 885 lines (main) to 892 (M6), 1577 (M7),
  2397 (M8), 2411 (M9) and 2827 (M10). Design §14 says "if a helper needs more than a few lines of C, you
  are in the wrong layer". Most of the growth is SceneFX helpers and the hand-written test-client
  protocol tables (Q5).

## 8. Pleamar: tracked state and compositor impact

- **Sources:** the compositor's `origin/main` docs, bd, and the two sysc-shell ledgers. The ledgers were
  read only, at the owner's request.
- **bd:** 73 items carry the Pleamar tag (11 closed, 15 in progress, 47 open) under epic `sysc-1168`. Six
  carry the `compositor` label. The rest are sysc-shell work (springs, config diagnostics, glass, plugin
  trust, describe/act, installer) with no compositor dependency, except the rows below.

**Decided on main but stale in bd: `sysc-1212`.** The *Owner update (2026-10-20)* in
`2026-10-15-tier3-wm-scene-equivalent-design.md` on `origin/main` decides five things:

- **Layout:** stays Niri's model, with no alternate modes or private dialect.
- **Compositor-drawn UI:** only Niri's (overview M7, selector M11).
- **Agent seat:** stays as built.
- **Window-image capture:** at M12 through the standard protocols.
- **Browser remote:** deferred.

Its own revisit trigger is "after M9 only if a named workflow does not fit the Niri model". bd
`sysc-1212` is still open, and its close condition ("a design that names the first feature set, owners,
boundary, acceptance, stop") is met by that update. The 2026-10-20 handover's "settle `sysc-1212` before
M5" was overtaken the same day, and M5 merged under it. **Once #26 merges, the revisit point has
arrived.** That is Q10.

**Things Pleamar does that Niri cannot, and their compositor owner:**

| Capability | Niri 26.04 | sysc-compositor state | Tracked by |
|---|---|---|---|
| Agent seat, compositor-enforced Stop, injected-seat isolation | none | **done on main** | `sysc-1208`/`1211` closed; GitHub #1/#2 still OPEN |
| Agent inert under lock or blank | none | not started. M10 lock layer (owner direction 2026-10-22) | `sysc-1260` |
| User-seat-only re-arm after Stop | none | **unblocked now**: M5 binds landed (bd text still says "with M1 config"). Needs a new KDL action name, a sysc extension (Q11) | `sysc-1261` |
| Live per-window pictures (switcher, dock, preview) | none: no `ext-image-copy-capture` or foreign-toplevel image source in `src/` | none. Approved for M12. The proposal is on unmerged branch `proposal/window-preview-capture-20261018` (`2f5eec9`) | shell `sysc-787` only; **no compositor bd item** |
| Live glass lens (recapture behind the panel) | Niri has wlr-screencopy | **no screencopy in sysc-compositor**, and no milestone owns it | shell `sysc-797`, `sysc-1197`, `sysc-1244`; nothing compositor-side |
| Shell-composed screen-share chooser | portal pair | design keeps the GNOME portal pair at M12 | `sysc-1186` (owner decision, changes M12) |
| Browser remote session | none | deferred behind M12 and a security design | `sysc-1213`, GitHub #4 |
| Desktop widgets on the bottom layer | n/a | layers render, but M7's overview omits background and bottom layers | shell `sysc-1184`; matrix overview row |
| WM-as-scene, shell-drawn title bars, shell-composed workspace wave | none | excluded by the 2026-10-20 update; workspace springs are M8 | `sysc-1212`; ledger GAP-uiux-surf-04/05 |
| Agent-presence UI | n/a | compositor facts exist | shell `sysc-1193` |

**Cross-repo security note.** `sysc-1255` (assistant worker boundary) denies the worker `NIRI_SOCKET`, the
shell IPC socket and the runtime directory. It does not name the **compositor control socket**, which can
inject input on the agent seat. Under sysc-compositor that socket is the higher-value target. Recommend the
owner add it to `sysc-1255`'s first failing test (Q12).

**Tracker hygiene:**

- `sysc-1522` duplicates `sysc-1190` (both open).
- `sysc-1506` duplicates `sysc-1196` (both closed).

## 9. Tracker discrepancies

- **`sysc-1271` (M6):** in_progress, but #22 is complete and open. Same pattern as M7–M9: open in bd and
  complete on a branch. Close each when its PR merges, not before.
- **`sysc-1500`:** still reproduces on main for the ipc half (run) and the cli half (read). #22 fixes the
  ipc half and #25 the cli half, so close it when both merge.
- **`sysc-1212`:** stale, §8.
- **`sysc-1261`:** description stale ("with M1 config"); the blocker (binds) is gone.
- **`sysc-1624`–`1630`:** unlabelled, no edges. Suggested edges:
  - `sysc-1624` blocks `1271`
  - `1625` and `1628` block `1273`
  - `1627` blocks `1274`
  - `1630` blocks all four
  - `1626` should close as not-a-defect after its run
- **§6.2 defect:** needs a bd issue.
- **GitHub against bd:**
  - #1 and #2 are OPEN, but `sysc-1208`/`1211` are closed with the code on main. That discrepancy
    persists from the 2026-10-20 audit.
  - #3 and #4 match bd (open).
  - #20/`sysc-1469` (closed) and #21/`sysc-1470` (open) match.
- **Earlier audit items resolved since 2026-10-20:** `sysc-1311` and `sysc-1436` are now closed.
- **No compositor duplicates** in `bd duplicates`.

## 10. Recommended merge sequence

1. Push `fix/sysc-1630-…`, open a PR, merge it to main. *(owner: push and merge)*
2. A small main PR for §6.2 (`Terminate` through `Post`, the `runClient` order, main's watchdog), with its
   regression test. *(owner: merge; consent for one nested run)*
3. Rebase #22 onto main, keeping both sides of the import conflict, and fix `sysc-1624`.
   Force-push, run `TestPopupDrawsBesideItsParent` once with consent, merge. *(owner: force-push, consent,
   merge)*
4. `git rebase --onto origin/main fa281df` for #24, retarget it to main, run one overview named test with
   consent, merge.
5. Rebase #25 the same way. Fix `sysc-1625`, the `sysc-1628` fixture and M8's watchdogs. Run the two
   regressions, merge.
6. Rebase #26 and fix `sysc-1627`. Owner decides Q4 and Q6. Merge.
7. Rebase M10 onto main, commit it as WIP without the unhandled managers, and continue M10 with the lock
   layer first.

## 11. Owner decisions

- **Q1.** May the `sysc-1630` branch be pushed and merged before the stack, and may the next developer
  force-push #22–#26 after rebasing? The alternative is merging main into each branch.
- **Q2.** Consent for these single named nested runs once the guard is on the branch:
  - `TestPopupDrawsBesideItsParent` (#22)
  - an overview test of your choice (#24)
  - `TestControlDrivesAClient` and `TestWindowResizeScalesSnapshotWhileLayoutTargetsCommit` (#25)
  - `TestDestroyStopsActiveEventStream` (#26; it builds no compositor but lives in `internal/server`)
  - `TestTerminateAfterDestroyIsSafe` (main)
- **Q3.** `sysc-1624`: should a tiled window ever keep a hand placement? Niri never does, so the review
  assumes "no" and fixes the test.
- **Q4.** Accept `godbus/dbus/v5` (and `x/sys`) in `go.mod` for M9's locale1/login1 and M10's
  ScreenSaver?
- **Q5.** Accept `scenefx-0.5` as a system dependency, and C in preambles growing past 2,800 lines? Or
  should the next work cap it, for example by generating the test-client protocol tables or using
  SceneFX headers everywhere?
- **Q6.** Is M9's gate met by nested runs and fake executables, or must `--session` run in a real session
  or VM before #26 merges (§15 "running binary", §16 "session VM")?
- **Q7.** M10 WIP: commit it with the unhandled managers removed, or exactly as it is on a never-merge
  branch?
- **Q8.** Approve moving `PowerOffMonitors`/`PowerOnMonitors` into M10. Should the compositor or
  sysc-shell own `org.freedesktop.ScreenSaver` (both try today)?
- **Q9.** Assign the unscheduled §7 globals to milestones. Proposed:
  - data-control, virtual-keyboard/pointer, presentation-time, single-pixel-buffer and alpha-modifier
    with M10
  - screencopy, image-copy-capture, foreign-toplevel, the list and the image source with M12
  - gamma, output-management, tearing and syncobj with M15
  - tablet-v2 where?
  Also add a globals table to the parity matrix.
- **Q10.** `sysc-1212`: close it against the 2026-10-20 owner update, or keep it open as the post-M9
  checkpoint? Is there a named workflow Niri's model does not cover?
- **Q11.** `sysc-1261`: what should the user-seat re-arm bind action be called? It extends Niri's KDL.
- **Q12.** Add the compositor control socket to `sysc-1255`'s worker denial list?
- **Q13.** CI lanes 1 and 2 (§7.1): approve them, and on which runner image?
- **Q14.** Primary checkout cleanup:
  - replace the local M3 draft with main's copy
  - reconcile `docs/README.md`
  - delete the stray `.beads/beads.db`
  - remove the merged worktrees listed in §1
- **Q15.** Close GitHub #1 and #2 to match bd? Merge or close the window-preview proposal branch?

**Answers so far (2026-10-22):**

- Q1, Q2, Q4 and Q5: yes.
- Q6: (a). The real session run is filed as `sysc-1631`.
- Q7: (B).
- Q10: `sysc-1212` closed in bd.
- Q11: `resume-agent-seat`.
- CLI naming: step 1 now, then steps 2 and 3 (`sysc-1632`, `sysc-1633`, `sysc-1634`).
- Q8: yes; the compositor owns ScreenSaver (`sysc-1635` tracks the shell's handoff).
- Q9: the globals go to M11 and M12.
- Q13: yes.
- Q14: done.
- Q15: GitHub #1–#3 closed.
- Q3, Q12 and the proposal branch: awaiting clarification.

Details are in the continuation handover.

The continuation handover records the details.

## Evidence: tests run (pure Go, one package and one named test each)

- `./internal/ipc -run '^TestDecodeRequest$'` on `e69b345` (main plus guard): FAIL, the same
  output as `sysc-1500`.
- The same test on `fa281df` (#22): ok.
- `./internal/config -run '^TestM8AnimationOffAndUnsupportedSettings$'` on `6e61f34` (#26): FAIL, "duplicate
  node `animations`".
