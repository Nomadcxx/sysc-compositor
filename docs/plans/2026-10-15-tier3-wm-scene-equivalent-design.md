# Tier 3 — WM-as-scene equivalent: the authoring surface (gh#264)

**Kind:** design (exploration-grade)
**Date:** 2026-10-15
**Covers:** gh#264 / REC-49 / REC-int-05 (bd sysc-1212)
**Ledger source:** `2026-10-14-pleamar-improvement-ledger.md` line 577
**Target repo:** `sysc-compositor` — **this document lives here**; transferred from sysc-shell 2026-10-15

---

## D0. Where this document lives

**Transferred from sysc-shell 2026-10-15**, on the same transfer recorded in
`2026-10-15-tier3-session-domain-design.md`:D0. That document holds the charter that must not be
re-litigated — Go + cgo + wlroots 0.20, with the C confined to gSlapper — and the state table.

This item is the least affected of the four compositor items: it has **no seat dependency, no security
surface and no consumer outside the compositor**. It is included because gh#264 must map to a design
section or an explicit verdict, and the explicit verdict here is *recommend against*.

**Being in the right repository does not change the verdict.** D1 does not depend on the repository not
existing — it depends on the scene mechanism being blocked by constraint and the residue already being
specified in `2026-10-02-sysc-compositor-design.md`. Both facts were true before the transfer and are
true after it.

---

## D1. Verdict: recommend against, on evidence

**gh#264 should not be built as a separate work item.** Not because the idea is wrong, but because the thing it asks for is **already specified in the compositor's own design**, and the residue after removing what already exists is a decision the owner has not asked to make.

This is the cheapest of the seven items to resolve and the one with the clearest answer.

### D1.1 The pleamar mechanism cannot be ported

The ledger is right and this is not a cost judgement:

- pleamar's WM policy is a **scene language** written in Luau (`marea-plm/scene/`, `examples/session.plm`). sysc-main `AGENTS.md`:6 bans Lua and Luau. This is a hard product constraint, not a preference.
- sysc-compositor's contract is **niri KDL config, JSON IPC and CLI** (`README.md`:3; `2026-10-02-sysc-compositor-design.md`:169-171) so that sysc-shell, sysc-greet, sysc-launch and an existing `~/.config/niri/config.kdl` keep working.

A Luau scene layer would break both at once. **Porting pleamar's mechanism is blocked, permanently, by constraint — not by effort.**

### D1.2 What remains is already in the plan

The ledger's compliant equivalent — *"the authoring surface itself"* — is precisely what `2026-10-02-sysc-compositor-design.md` already specifies:

| Ledger asks for | Already specified at |
|---|---|
| KDL binds + the action vocabulary | `design`:229 — *"Actions (all 141)"*, wire pinned to niri-ipc 26.4.0 |
| Per-kind motion parameters | `design`:192 — `animations {}` block: off, or per-kind spring *(damping-ratio, stiffness, epsilon)* or easing; `design`:345 |
| Live-reload on save | `design`:176 (inotify, including output mode, binds, window-rules) |
| No-recompile authoring | niri's own validate path, `design`:177 |

And the schedule already places them:

- `design`:462 — **M8**: *"Springs for the animation kinds in this machine's config"*
- `design`:463 — **M9**: live-reload

So REC-int-05's approach, read carefully, is **a request to make explicit something the compositor design already commits to and already schedules.** The honest amount of new work is a documentation pass over a design that exists, plus one row in the niri parity matrix.

**That is the finding.** The rec as written is satisfied by the existing plan.

---

## D2. User-visible behavior

None new. The user changes WM policy and motion by editing `config.kdl` and saving:

- binds and actions are the niri vocabulary, all 141 of them;
- motion is per-kind in `animations {}` — damping-ratio, stiffness, epsilon, or an easing curve;
- a save live-reloads, including output mode, binds and window-rules, without a recompile or a restart.

This is a **surfacing and documentation** outcome, not a capability outcome.

## D3. Owners and data flow

Compositor-owned: KDL parsing in `internal/config`; policy evaluation in `internal/wm`; animation parameters at the CPU-side spring layer (`design`:142 assigns Go the policy and animation work).

**sysc-shell owns nothing and needs nothing in this item.** It is a layer-shell client. It reads the same config file and renders whatever the compositor does; it has no opinion about springs, binds or actions. Verified against the shell's own one-seat, layer-shell client shape (`internal/platform/wayland/client.go` `bindSingleton`; `internal/platform/wayland/registry.go` `bindVersion`).

#### D3.1 What "Niri parity" concretely buys each consumer

The handover asks this precisely, and the honest answer is that parity is not an abstraction here — it is four named things that keep working, or do not.

| Consumer | What parity means concretely | Consequence of getting it wrong |
|---|---|---|
| **An existing `~/.config/niri/config.kdl`** | The user's current file keeps working unchanged. Same schema, same binds block, same `animations {}`. | A migration. Every user edits their config by hand immediately. This is the promise the README makes in its first paragraph and the reason the whole KDL authoring surface is niri's and not a new dialect. |
| **sysc-shell** | It stays a layer-shell client with no new protocol, no new global and no new config key of its own for WM policy or motion. Its one-seat client assumption is untouched. | A protocol fork, and the compositor's parity contract dies with it. |
| **sysc-launch** | Same CLI surface and same action vocabulary — the 141 actions are the vocabulary launch drives. | Two action vocabularies to keep in step, and a second place for an action to go missing. |
| **sysc-greet** | Reads the same config; greeter behavior is not a separate policy dialect. | The greeter and the session drift apart, which is the failure sysc-lock's one-session-per-UID rule exists to prevent. |

**The composite is the parity matrix.** `2026-10-02-sysc-compositor-design.md` already names a niri-parity-matrix document, and D1.2's table above is four rows of it. The one addition this item would make to that matrix is a row stating that the authoring surface is niri's vocabulary and **that the scene language is a deliberate, recorded exclusion** — not an oversight and not a gap awaiting work.

#### D3.2 Viability note

- **Proven today:** KDL parsing, the niri action enum and the config validate path are all wire-and-schema work in pure Go with a pinned enum (niri-ipc 26.4.0, `design`:213, `:410`). Motion parameters are ordinary CPU-side spring parameters; `design`:142 assigns Go this work outright. **Nothing here is at risk from the language choice.**
- **Real risk:** only the schedule. M8 (springs) and M9 (live-reload) sit late in the milestone list (`design`:462-463), so the authoring surface is complete before it is *ergonomic*. That is a sequencing fact, not a design risk, and it is already visible in the existing plan.
- **No cheap Go analogue — none needed.** This is the one item in the tranche where the smithay comparison comes out clean. pleamar's mechanism is a Luau scene language; sysc's is KDL plus a pinned enum, which wlroots does not constrain at all. There is nothing to port, so there is nothing that lacks a Go equivalent. The gap is the one the constraint creates, and it is permanent by design (D1.1).
- **Build / don't-build:** `Scope: L`, `Impact: M` in the ledger, but **the residual scope after D1.2's removal is a documentation pass plus one parity-matrix row.** Against that residue the recommendation is **do not build** — not because the work is hard, but because it is already done.

## D4. Protocols

**Required:** none new. niri KDL config (`design`:169-171) and niri-ipc 26.4.0 JSON (`design`:213), both already pinned and already implemented as pure Go (`design`:410).

**Optional:** inotify live-reload (`design`:176) at M9. Optional in the sense that the plan schedules it at M9 and the rest of the authoring surface works without it.

No new protocol, no new socket, no new fact.

## D5. Error and shutdown behavior

Config errors follow niri's own validate path (`design`:177): a bad config is reported, the previous known-good config stays active, and the compositor does not start half-configured. Live-reload that fails leaves the running state untouched.

Motion parameters outside a sane range fall back to the resolved default rather than dropping the animation — a spring with nonsensical stiffness should animate with a default, not vanish.

**This is inherited behavior, specified here so it is not left to M8's implementer to guess.**

## D6. Tests and live hardware matrix

Automated, in the compositor repository under its rules (`timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run <Name>`, no `-race`, no `./...`):

- KDL round-trip over the action table — table test, decode every action kind. `design`:478 already commits to this: *"generated tests from niri-ipc 26.4.0 enum (decode every Action)"*.
- `animations {}` parsing: valid springs, valid easings, out-of-range values falling back.
- a config error leaves the previous config active (no partial apply).

Live, and **partly unrunnable here**: single-output spring timing on DP-1 scale 1 is measurable on this machine. **Two-output motion and fractional-scale behaviour are unrunnable** — Niri here has no runtime virtual output, so every two-output check is unrunnable on this host until a second hardware fixture exists. Do not report a two-output motion result.

**No motion budget is proposed.** *"Record measurements before setting hard budgets."* There are no measurements.

## D7. Non-goals

- **No scene language.** No Lua, no Luau, no embedded scripting, in sysc-shell or sysc-compositor. This is the load-bearing non-goal.
- **No compiled animation DSL.** The authoring surface is KDL. A Go-native authored policy layer, if it ever exists, lives in `internal/config` and `internal/wm` and **must not reintroduce a scripting language.**
- No shell-side motion work. Tier 1's motion tranche (`2026-10-15-tier1-motion-design.md`) is a separate, already-designed concern for shell surfaces; this item does not touch it.
- No work that blocks Tier 1 or Tier 2 branches.
- No new UI primitive.
- No port of pleamar's scene mechanism, in whole or in part.

## D8. Owner decisions this document records rather than makes

1. **Does the owner want more than niri's action and motion vocabulary?** If **no** — which is the default and this document's recommendation — gh#264 closes as a **deliberate parity exclusion**, recorded as: *pleamar's scene mechanism is not ported; the authoring surface is niri KDL, and this is a chosen constraint, not an oversight.*
2. **If yes**, that is **a new design**, not a continuation of this one. It would be a declarative Go policy layer over the existing KDL config, and it must state what it adds beyond the 141 actions and what it costs in parity. Do not write it speculatively.
3. **Should the existing authoring surface be made more explicit anyway?** Cheap, non-blocking, and arguably worth doing when the compositor repository is created — but it is a documentation task on an existing design, not a build item, and it does not need a Tier 3 work item to justify it.

## D9. Stop / reconsider condition

**Recommend against** building gh#264 as a work item. It is satisfied by `2026-10-02-sysc-compositor-design.md` §4 (`:169-192`), §5 (`:213`, `:229`) and M8/M9 (`:462-463`), and the niri parity matrix.

**Reconsider** only on an explicit owner request for a vocabulary richer than niri's, which is a separate design under gate 2 of this tranche and must clear the dependency gate (behavior replaced, license, pinned version, package boundary, removal condition) before any code.

---

## D10. Coverage map

| Issue | Ledger rec | Sections | Verdict |
|---|---|---|---|
| gh#264 / sysc-1212 | REC-49 / REC-int-05 | D0, D1, D2, D3.1, D3.2, D7, D8, D9 | **Recommend against as a build item**; satisfied by the existing compositor design; default is a recorded parity exclusion |

No bd closure, no GH transfer, no TDD plan.