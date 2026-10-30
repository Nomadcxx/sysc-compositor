# sysc-compositor docs

Map of every design and plan this repository holds. Add a row in the same commit that adds a document.
A document that is not in this map is one the project will lose.

Product contract: `plans/2026-10-02-sysc-compositor-design.md`. Workstreams and order:
`plans/2026-10-02-orchestration.md`.

## Standing rule: plans are written when a milestone starts

`plans/2026-10-02-orchestration.md` Task 3 is explicit — **do not pre-write M1–M15 plans.** When a
milestone starts, write its plan from the design section that covers it, then execute it. This map
records what exists; it is not a queue.

Documents that are **viability assessments or gate checks** rather than milestone plans are marked
below. They may be recorded ahead of their milestone, but they are not scheduled work and must not be
executed until the owner schedules the milestone they belong to.

## What exists

| Document | Kind | What it is |
|---|---|---|
| [plans/2026-10-02-sysc-compositor-design.md](plans/2026-10-02-sysc-compositor-design.md) | design | Product contract. Package map, globals, niri parity surface, milestones M0–M15. |
| [plans/2026-10-02-orchestration.md](plans/2026-10-02-orchestration.md) | plan | Workstreams L0, M0–M15, S0 and their order. The map for execution. |
| [plans/2026-10-02-m0-nested-tinywl.md](plans/2026-10-02-m0-nested-tinywl.md) | plan | M0, the first code in this repository: nested tinywl, one seat plus cursor. |
| [plans/2026-10-02-sysc-launch.md](plans/2026-10-02-sysc-launch.md) | plan | Sibling workstream L0, executed in `/home/nomadx/sysc-launch`, not here. |
| [plans/niri-parity-matrix.md](plans/niri-parity-matrix.md) | checklist | Remaining niri IPC and config parity. Fill this rather than expanding the design. |
| [plans/2026-10-28-release-readiness.md](plans/2026-10-28-release-readiness.md) | assessment | Release gates after capture and virtual input: remaining M12, M13–M15, session integration and hardware acceptance. |
| [third_party/godbus/SYSC.md](../third_party/godbus/SYSC.md) | dependency note | Scoped v5.2.2 descriptor backport, provenance, regression command and removal check. |
| [plans/2026-10-15-tier3-session-domain-design.md](plans/2026-10-15-tier3-session-domain-design.md) | design | **Transferred from sysc-shell 2026-10-15.** Multi-seat input model, agent seat with compositor-enforced Stop, browser remote access. |
| [plans/2026-10-15-tier3-wm-scene-equivalent-design.md](plans/2026-10-15-tier3-wm-scene-equivalent-design.md) | design | **Transferred from sysc-shell 2026-10-15.** Contains the original WM-as-scene assessment and a 2026-10-20 owner update selecting Go-only capabilities while keeping the Niri KDL, IPC, and CLI contract. |
| [plans/2026-10-15-tier3-seat-attachment-probe.md](plans/2026-10-15-tier3-seat-attachment-probe.md) | plan | **Transferred from sysc-shell 2026-10-15.** **Retired 2026-10-15 without execution.** The landed test proves three distinct, independently configurable `wl_seat` objects; the virtual-device question was answered by API inspection (wlroots attaches nothing itself, the compositor's handler does), so the routing proof moves to gh#263's TDD plan. |
| [plans/2026-10-15-tier3-multi-seat.md](plans/2026-10-15-tier3-multi-seat.md) | plan | **Tier 3 TDD plan, not a milestone plan.** gh#2 / `sysc-1211`: seat table in `internal/wlr`, one on-demand injected seat with in-process devices, isolation rules D2.4.1–2 as nested tests; retires `ProbeSeats`. Gates met (M0 merged, D8 answered); execution needs the owner's implementation commission. |
| [plans/2026-10-16-tier3-control-socket.md](plans/2026-10-16-tier3-control-socket.md) | plan | **Tier 3 TDD plan.** gh#1 / `sysc-1208`: `0600` peer-verified JSON-lines control socket in a pure-Go `internal/control`, work posted to the compositor thread through an eventfd, Stop that anyone may set and nothing on the socket can lift, targets by id/pid/app-id, per-pointer facts for the shell's presence display. |
| [plans/2026-10-16-input-pattern-comparison-report.md](plans/2026-10-16-input-pattern-comparison-report.md) | report | `sysc-1247`: Niri 26.04 and mangowm 0.18.0 input, seat, focus, move/resize and popup patterns against M0; each marked apply (with its milestone), covered or n/a. |
| [plans/2026-10-16-m1-required-globals.md](plans/2026-10-16-m1-required-globals.md) | plan | **M1 milestone plan** (`sysc-1266`, with `sysc-1297`): layer-shell (v5, as Niri) with scene layers and usable area, fractional scale, dmabuf feedback, xdg-shell v7 popups parented and unconstrained; each proved with an installed client. |
| [plans/2026-10-17-pointer-focus-correctness.md](plans/2026-10-17-pointer-focus-correctness.md) | plan | **Tier 3 TDD plan**, `sysc-1296`, gates M4: implicit grab while a button is held, `default` image over empty space, popup and subsurface hits resolve to their window, unmap releases focus and grabs, compositor grabs clear pointer focus. |
| [plans/2026-10-17-m2-greeter.md](plans/2026-10-17-m2-greeter.md) | plan | **M2 milestone plan** (`sysc-1267`): KDL v1 config (greeter sections applied, the rest accepted), niri-ipc socket with Version, Spawn, SpawnSh and Quit, `msg` CLI as `sysc-compositor` and `niri`, fullscreen for the greeter; nested gate, then a greetd VT gate scheduled with the owner. |
| [plans/2026-10-19-m3-session.md](plans/2026-10-19-m3-session.md) | plan | **M3 milestone plan** (`sysc-1268`): Niri state queries, bounded EventStream, layer keyboard focus, positional config includes, `niri validate`, and the sysc-shell laptop gate. |
| [plans/2026-10-19-m4-window-ipc.md](plans/2026-10-19-m4-window-ipc.md) | plan | **M4 milestone plan** (`sysc-1269`): mapped xdg toplevel snapshots, Niri window events and actions, stable IDs for the M12 capture join, and the sysc-shell laptop gate. |
| [plans/2026-10-19-niri-mango-pleamar-capability-research.md](plans/2026-10-19-niri-mango-pleamar-capability-research.md) | report | Niri remains the behavior/IPC shape; Mango supplies wlroots patterns; maps current Niri gaps, Pleamar-inspired capabilities, component ownership, and a pre-M5 design decision. |
| [plans/2026-10-20-offscreen-focus-design.md](plans/2026-10-20-offscreen-focus-design.md) | design | `sysc-1436`: use the focused output when a mapped window center falls outside every output. |
| [plans/2026-10-20-offscreen-focus.md](plans/2026-10-20-offscreen-focus.md) | plan | `sysc-1436`: regression-first fix for offscreen `FocusWindow` routing and output snapshots. |
| [plans/2026-10-20-m5-layout.md](plans/2026-10-20-m5-layout.md) | plan | M5 implementation plan (`sysc-1270`): scrolling columns, per-output dynamic workspaces, core KDL binds, and nested laptop gate. |
| [plans/2026-10-20-sysc-compositor-continuation-handover.md](plans/2026-10-20-sysc-compositor-continuation-handover.md) | handover | Superseded (history). M1–M4 state, remaining M5–M15 parity, Tier 3 implementation and open decisions, reference guidance, testbench rules, and next steps. |
| [plans/2026-10-20-m6-window-policy.md](plans/2026-10-20-m6-window-policy.md) | plan | M6 implementation plan (`sysc-1271`): tabs, floating/fullscreen, named workspaces, monitor moves, M6 rules, urgency, and the laptop nested gate. |
| [plans/2026-10-21-m7-overview-design.md](plans/2026-10-21-m7-overview-design.md) | design | Approved M7 design: compositor-owned overview projection, gestures, hot corners, insert hints, and drag edge scrolling. |
| [plans/2026-10-21-m7-overview.md](plans/2026-10-21-m7-overview.md) | plan | M7 implementation plan (`sysc-1272`): overview rendering and IPC, gestures, hot corners, insert hints, DND edge scrolling, and the nested laptop gate. |
| [plans/2026-10-21-m8-animations.md](plans/2026-10-21-m8-animations.md) | plan | M8 implementation plan: the laptop's spring animations, focus ring, shadow, geometry clipping, and decoration preference. |
| [plans/2026-10-22-m9-session-live-config.md](plans/2026-10-22-m9-session-live-config.md) | plan | M9 implementation plan (`sysc-1274`): `--session`, complete `niri msg`, keyboard layouts, D-Bus defaults, include-aware live reload, and `LoadConfigFile`. |
| [plans/2026-10-22-m10-input-idle.md](plans/2026-10-22-m10-input-idle.md) | plan | M10 implementation plan (`sysc-1275`): input protocols, text input, lock/idle, frost, and injected-seat safety. |
| [plans/2026-10-22-m11-screenshot-overlay.md](plans/2026-10-22-m11-screenshot-overlay.md) | plan | M11 implementation plan (`sysc-1276`): capture and clipboard globals (Q9), screenshot actions and UI, hotkey overlay, recent windows; text rendering in pure Go (owner decision 2026-10-23). |
| [plans/2026-10-24-m12-capture-portals.md](plans/2026-10-24-m12-capture-portals.md) | plan | M12 implementation plan (`sysc-1277`): mapped IDs, foreign-toplevel previews and capture privacy, virtual input and IME grabs, GNOME portal services, PipeWire casts, and Q9 globals. Records the approved native PipeWire dependency exception. |
| [plans/2026-10-22-sysc-compositor-review-handover.md](plans/2026-10-22-sysc-compositor-review-handover.md) | handover | Superseded (history). Review commission after the 2026-10-22 test-window incident: test-consent rules, the M6–M9 PR stack and M10 worktree state, unverified review claims `sysc-1624`–`1628`. |
| [plans/2026-10-22-sysc-compositor-review.md](plans/2026-10-22-sysc-compositor-review.md) | report | Review of `sysc-1630`, PRs #22–#26 (M6–M9) and the M10 worktree: verdicts, `sysc-1624`–`1628` findings, the `Terminate`-after-`Destroy` defect, tracker discrepancies, merge sequence and owner answers. |
| [plans/2026-10-22-sysc-compositor-continuation-handover.md](plans/2026-10-22-sysc-compositor-continuation-handover.md) | handover | Superseded (history). Test-consent rules, the consent guard, the `Terminate` fix, stack rebase, M10 with the lock layer first, M11–M15 exit gates, CI, CLI naming, and owner answers. |
| [plans/2026-10-23-sysc-compositor-continuation-handover.md](plans/2026-10-23-sysc-compositor-continuation-handover.md) | handover | Standing rules 1–11 and the working recipe (still in force), M11 state after #34–#37, open work. Superseded for planning by the 2026-10-23 testing handover. |
| [plans/2026-10-23-sysc-compositor-testing-handover.md](plans/2026-10-23-sysc-compositor-testing-handover.md) | handover | Superseded for planning by the 2026-10-24 handover; still the reference for where to test. Recent windows landed (#39–#42); owner answers Q3/Q12; where to test (desktop, laptop over SSH with pre-approved nested runs, the `--session` caveat, a proposed VM, a headless test mode, CI); next work in order. |
| [plans/2026-10-24-sysc-compositor-handover.md](plans/2026-10-24-sysc-compositor-handover.md) | handover | Superseded for planning by the 2026-10-24 continuation handover. State after #44–#48 (M11 gate, shifted binds, switcher slide and clip); next: the session VM on the laptop (setup, lane 2 under vkms, the `--session` gate), the M10 gate, M12–M15; open bd items and working notes. |
| [plans/2026-10-24-session-vm.md](plans/2026-10-24-session-vm.md) | record | The session VM on the laptop (`~/vm/sysc-session`, scripts in `plans/assets/2026-10-24-session-vm/`): lane 2 under vkms with no window anywhere, and the `--session` gate (`sysc-1631`) through greetd, which found that `graphical-session.target` refuses a manual start; the session now runs Niri's way (`sysc-compositor-session`, `sysc-compositor.service`); then the same gate with sysc-greet as the greeter. |
| [plans/2026-10-24-sysc-compositor-continuation-handover.md](plans/2026-10-24-sysc-compositor-continuation-handover.md) | handover | Superseded for planning by the 2026-10-30 post-M12 handover. State after #50–#52 (Niri-style session, sysc-greet gate, lane 2 quarantine down to two); the session VM's use; next: the M10 gate in the VM, `sysc-1642`/`sysc-1640`, M12, small items, M13–M15. |
| [plans/2026-10-30-m12-vm-gate-handover.md](plans/2026-10-30-m12-vm-gate-handover.md) | handover | Superseded (history). M12 Task 8 VM gate evidence, the fixes #68–#70, VM driving lessons and gate assets. |
| [plans/2026-10-30-post-m12-handover.md](plans/2026-10-30-post-m12-handover.md) | handover | **Current planning handover.** M12 closed (#71–#74); lane-2 follow-up, frame-pacing audit, M13 and first-use decision; includes the execution record. |
| [plans/2026-10-30-m13-x11.md](plans/2026-10-30-m13-x11.md) | plan | M13: on-demand satellite sockets and lifecycle, config, DISPLAY publication, and native/X11 application gates. |
| [plans/2026-10-31-frame-pacing-audit.md](plans/2026-10-31-frame-pacing-audit.md) | report | sysc-2056 source audit: native callback scheduling and scanout, hidden-surface fallback gap, and the remaining physical game benchmark. |

## Reference sources (pinned)

Read-only checkouts outside every sysc repo, never built from. Milestone plans cite them by these commits.

| Reference | Path | Pin | Use |
|---|---|---|---|
| Niri 26.04 (Rust, smithay) | `/home/nomadx/references/niri` | tag `v26.04`, `8ed0da44d974c32c6877d2f4630c314da0717ecb` | **Behaviour and contract target**: what each IPC request, config key and layout behaviour must do (`src/layout/`, `src/ipc/`, `src/handlers/`, `src/protocols/`, `niri-config/`). |
| mangowm 0.18.0 (C, wlroots 0.20 + scenefx) | `/home/nomadx/references/mango` | tag `0.18.0`, `a21b10fcbe6156b76ea96178d1941de5cb36137f` | **wlroots 0.20 implementation reference**: same substrate as this repo, so its global creation (`src/main.c`), input and seat code show the wlroots calls directly. Not a source for layouts, tags or `mmsg` (orchestration "What not to schedule"). |
| Hyprland (C++, wlroots) | `/home/nomadx/references/hyprland` | commit `8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd` | **Implementation reference** for watcher/debounce boundaries and cached blur framebuffer invalidation/damage. Niri 26.04 remains the behavior contract; do not import Hyprland config or session semantics. |

Re-pin by recording a new row; do not silently move a checkout.

## Source ledgers

Tier 3 items in this repository are derived from the pleamar improvement and gap ledgers, which stay in
sysc-shell's plan directory because that is where the cross-repo comparison was made:

- `/home/nomadx/sysc-shell/docs/plans/2026-10-14-pleamar-improvement-ledger.md`
- `/home/nomadx/sysc-shell/docs/plans/2026-10-14-pleamar-gap-ledger.md`
- `/home/nomadx/sysc-shell/research_notes/pleamar-gap-analysis/`

**Cite these ledgers for the *reasoning* and this repository's design for the *decision*.** The ledgers'
`file:line` citations have already drifted once — several cited shell files no longer exist — so a
transfer copies the conclusion, not the citation.

## Where Tier 3 state lives

The bd issues stay in sysc-shell, not here, so one graph holds every blocker across the family. This
repository has no bd database.

| Item | bd | GitHub issue here | Was |
|---|---|---|---|
| Agent seat with compositor-enforced Stop | `sysc-1208` | #1 | gh#262 in sysc-shell |
| Multi-seat input model | `sysc-1211` | #2 | gh#263 in sysc-shell |
| WM-as-scene equivalent — *recommend against* | `sysc-1212` | #3 | gh#264 in sysc-shell |
| Browser remote session — *recommend defer* | `sysc-1213` | #4 | gh#265 in sysc-shell |

All four were transferred here on 2026-10-15 with `gh issue transfer`. **Tracked state is not duplicated
into these documents** — bd is authoritative for whether work is done, in flight or gated.
