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
| [plans/2026-10-15-tier3-session-domain-design.md](plans/2026-10-15-tier3-session-domain-design.md) | design | **Transferred from sysc-shell 2026-10-15.** Multi-seat input model, agent seat with compositor-enforced Stop, browser remote access. |
| [plans/2026-10-15-tier3-wm-scene-equivalent-design.md](plans/2026-10-15-tier3-wm-scene-equivalent-design.md) | design | **Transferred from sysc-shell 2026-10-15.** Verdict: recommend against as a build item; the authoring surface is already in the product contract. Ends with an **Owner briefing** (2026-10-16) on what pleamar's scene does, what Niri KDL gives by milestone, and the options. |
| [plans/2026-10-15-tier3-seat-attachment-probe.md](plans/2026-10-15-tier3-seat-attachment-probe.md) | plan | **Transferred from sysc-shell 2026-10-15.** **Retired 2026-10-15 without execution.** The landed test proves three distinct, independently configurable `wl_seat` objects; the virtual-device question was answered by API inspection (wlroots attaches nothing itself, the compositor's handler does), so the routing proof moves to gh#263's TDD plan. |
| [plans/2026-10-15-tier3-multi-seat.md](plans/2026-10-15-tier3-multi-seat.md) | plan | **Tier 3 TDD plan, not a milestone plan.** gh#2 / `sysc-1211`: seat table in `internal/wlr`, one on-demand injected seat with in-process devices, isolation rules D2.4.1–2 as nested tests; retires `ProbeSeats`. Gates met (M0 merged, D8 answered); execution needs the owner's implementation commission. |
| [plans/2026-10-16-tier3-control-socket.md](plans/2026-10-16-tier3-control-socket.md) | plan | **Tier 3 TDD plan.** gh#1 / `sysc-1208`: `0600` peer-verified JSON-lines control socket in a pure-Go `internal/control`, work posted to the compositor thread through an eventfd, Stop that anyone may set and nothing on the socket can lift, targets by id/pid/app-id, per-pointer facts for the shell's presence display. |
| [plans/2026-10-16-input-pattern-comparison-report.md](plans/2026-10-16-input-pattern-comparison-report.md) | report | `sysc-1247`: Niri 26.04 and mangowm 0.18.0 input, seat, focus, move/resize and popup patterns against M0; each marked apply (with its milestone), covered or n/a. |

## Reference sources (pinned 2026-10-16)

Read-only checkouts outside every sysc repo, never built from. Milestone plans cite them by these commits.

| Reference | Path | Pin | Use |
|---|---|---|---|
| Niri 26.04 (Rust, smithay) | `/home/nomadx/references/niri` | tag `v26.04`, `8ed0da44d974c32c6877d2f4630c314da0717ecb` | **Behaviour and contract target**: what each IPC request, config key and layout behaviour must do (`src/layout/`, `src/ipc/`, `src/handlers/`, `src/protocols/`, `niri-config/`). |
| mangowm 0.18.0 (C, wlroots 0.20 + scenefx) | `/home/nomadx/references/mango` | tag `0.18.0`, `a21b10fcbe6156b76ea96178d1941de5cb36137f` | **wlroots 0.20 implementation reference**: same substrate as this repo, so its global creation (`src/main.c`), input and seat code show the wlroots calls directly. Not a source for layouts, tags or `mmsg` (orchestration "What not to schedule"). |

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
