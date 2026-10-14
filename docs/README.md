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
| [plans/2026-10-15-tier3-wm-scene-equivalent-design.md](plans/2026-10-15-tier3-wm-scene-equivalent-design.md) | design | **Transferred from sysc-shell 2026-10-15.** Verdict: recommend against as a build item; the authoring surface is already in the product contract. |
| [plans/2026-10-15-tier3-seat-attachment-probe.md](plans/2026-10-15-tier3-seat-attachment-probe.md) | plan | **Transferred from sysc-shell 2026-10-15.** Viability probe, not a milestone plan. Answers whether wlroots 0.20 attaches a virtual device to a chosen seat. |

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

The bd issues (`sysc-1208`, `sysc-1211`, `sysc-1212`, `sysc-1213`) and GitHub issues gh#262–gh#265 are
tracked in sysc-shell, not here, so one graph holds every blocker across the family. This repository has
no bd database. **bd state is not duplicated into these documents.**
