# Tier 3 — Session domain: seats, agent input, remote access

**Kind:** design (exploration-grade)
**Date:** 2026-10-15
**Covers:** gh#262 / REC-05 / REC-int-01 (bd sysc-1208) · gh#263 / REC-48 / REC-int-04 (bd sysc-1211) · gh#265 / REC-50 / REC-int-06 (bd sysc-1213)
**Ledger source:** `2026-10-14-pleamar-improvement-ledger.md` lines 117, 567, 587
**Target repo:** `sysc-compositor` — **this document lives here**; transferred from sysc-shell 2026-10-15

---

## D0. Where this document lives, and what is still gated

**Written in `sysc-shell`, transferred here 2026-10-15.** It was written in the wrong repository on
purpose: when it was drafted, `/home/nomadx/sysc-compositor` was a plain directory — `git rev-parse`
returned *"fatal: not a git repository"*, there was no remote and no code — so the requirements had to
live in the shell's plan directory, because that is the only place the shell can still act on them.
The owner has since created `https://github.com/Nomadcxx/sysc-compositor`. **This file is now the
canonical copy.** The sysc-shell file of the same name is a pointer, not a second source; if the two
ever disagree, this one is right.

**The repository and M0 code gates have cleared.** Tier 3 remains owner-gated, and the probe has only
answered D8.6: wlroots supports distinct seats with independent capabilities, and its API leaves
virtual-device attachment to the compositor (D8.6).

| Gate | State on 2026-10-15 |
|---|---|
| The repository exists | **Cleared.** `main`, first commit `4e57b6c`. |
| The product contract exists | Cleared — `2026-10-02-sysc-compositor-design.md`. |
| Code exists | **Met in part.** M0 has a nested compositor with one production seat and cursor. Its pointer move/resize exit gate remains open. |
| A secondary seat can take a virtual device | **Answered by API inspection (D8.6).** wlroots leaves attachment to the compositor and exposes `wlr_keyboard_init` / `wlr_pointer_init`; the routing is code this repository writes and tests. |
| The owner wants computer-use agents | **Yes, 2026-10-15** (D8.2), and a non-user seat is acceptable (D8.4): one agent seat (D8.5). |

Ordering against this repository's own schedule, from `2026-10-02-orchestration.md`: M0 builds one seat
plus a cursor. Everything in D2–D4 changes that base, so it must follow M0. The remaining gates are
the M0 pointer interaction exit gate and a separate implementation commission; the owner decisions
in D8 are answered, and none of this authorizes implementation by itself.

### What is already decided, and must not be re-litigated

The owner's "Go doesn't make good compositors" concern is already answered by a charter on disk. `README.md`: *"Wayland compositor for the sysc desktop. **Go and cgo only** (no `.c` / `.h` here; gSlapper keeps the C). wlroots 0.20."* The design's own package map (`2026-10-02-sysc-compositor-design.md`:410) allocates `internal/ipc/` as *pure Go* for niri-ipc JSON. So:

- **Do not** treat "is Go viable for a compositor" as an open question in any handover. It is a settled charter.
- The open question was narrower and duller: **would the directory become a repository and get built at all.** It has: the repository exists and M0 code is on `main` (D0 table).

`AGENTS.md` hard rules that any transferred design inherits: never `go test ./...`; never `-race`; one package and one named test via `timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run <Name>`; `runtime.LockOSThread()` before any wlroots call and never call wlroots off the compositor thread; listener trampolines in the cgo preamble of a `.go` file; **"sysc-wayland is a client. Do not turn it into the server."**

---

## D1. Verdict per item

| Item | Ledger verdict | Verdict here |
|---|---|---|
| gh#263 / int-04 multi-seat input model | Conditional on remote or agent input | **Requirements and viability, held here since 2026-10-15.** Most **load-bearing** item in the tranche — two others depend on it — and, for that same reason, the one most likely to be dead code standing alone. Load-bearing is not the same as valuable. See D1.1.4. |
| gh#262 / int-01 agent seat + Stop | Conditional on owner pursuing computer-use agents | **Requirements and viability, held here since 2026-10-15.** Do not start before int-04 ships. |
| gh#265 / int-06 browser remote session | Conditional on owner wanting browser remote access | **Requirements and viability, held here since 2026-10-15. Recommend defer.** Highest security surface, weakest case; see D4. |

No item here is recommended against outright. Two are recommended **defer**. None is implementable now. The repository exists; no item may produce a TDD plan until its consumer decision (D8) is answered and M0's exit gate has passed.

### D1.1 Viability note — the four questions this tranche owes an answer

The viability question here is **not** "is Go viable for a compositor". D0 settles that: the charter is Go + cgo + wlroots 0.20 and it is not re-litigated. The questions below are the four narrower ones.

#### D1.1.1 What is proven today

Under the charter, all three items use primitives that exist in wlroots and are reachable from Go: `wl_seat` with explicit capabilities, `virtual-keyboard-unstable-v1` and `virtual-pointer-unstable-v1`, an event loop on a `runtime.LockOSThread`-pinned goroutine, and `SO_PEERCRED` on a Unix socket via the Go standard library (`syscall.GetsockoptUcred`). **Nothing in this tranche needs a capability Go lacks.** That is the whole of the "is Go enough" answer and it is a short one.

**Answered by API inspection (2026-10-15, D8.6):** wlroots 0.20 does not route virtual devices at all. It exposes the requested seat on the device and leaves attachment to the compositor's handler, and `wlr_keyboard_init` / `wlr_pointer_init` let the compositor create in-process devices with no protocol. Seat attachment is therefore code this repository writes, and its proof belongs in gh#263's TDD plan rather than in a pre-decision probe.

#### D1.1.2 What carries real risk

1. **Seat attachment semantics — resolved from a substrate risk into a code risk (D8.6).** wlroots attaches nothing itself, so a virtual device cannot be silently forced onto `seat0` by the library. The risk that remains is this repository's own routing: the handler that picks the seat, and the isolation rules in D2.4, which are hand-written and must be tested as such.
2. **Focus asymmetry surviving a port.** wlroots has no concept of an agent seat. **There is one interesting seat and every other is decoration — corrected 2026-10-15: the seats are real and independently configurable (D8.6), but the *asymmetry between them* is entirely code this repository writes.** Nothing in wlroots enforces that an injected seat must not move the user's data-device focus or set the drawn cursor; both are compositor decisions. So all three rules in D2.4 are code this repository writes and maintains, not configuration it sets. They are exactly what a wlroots port loses, and the ledger is right that they are "easy to lose".
3. **Capture sizing under gh#265.** `design`:298's exact-sized SHM-pool rule for screencopy consumers is a live constraint on a remote frame path that no test exercises until a second machine exists.

#### D1.1.3 pleamar behaviours with no cheap Go analogue

pleamar-wm is Rust on **smithay**. sysc-compositor is Go on **wlroots**. These are not the same substrate, and three of pleamar's mechanisms have no cheap wlroots equivalent:

- **`is_agent_seat` as a first-class concept.** pleamar carries an agent-seat flag through focus, cursor and window tracking (`src/agent.rs`). wlroots has no such notion at all, so reproducing it means maintaining a seat table and threading it by hand through the compositor's own focus and cursor paths. **This is the expensive one.** It is why gh#262 is Scope **L** and not M, and it is the substantive content of the "no cheap analogue" answer.
- **Per-seat cursor suppression** (`src/nest.rs`:3367-3371). In smithay this is an early return inside a cursor-image handler. In wlroots the equivalent is *never reaching* the cursor handler for that seat's events — a **routing** decision rather than a **handler** decision. It moves from a five-line guard to a seat-table lookup on every pointer event. Correct by construction, but a per-event cost that smithay's structure hides.
- **The blanket `stopped-by-user` refusal** (`src/agent.rs`:409-415) is cheap *everywhere* — one flag checked at a dispatch site. **Listed because its cheapness is misleading.** In Go it is only correct if *every* mutating verb funnels through one dispatcher; a verb added later that answers directly bypasses the flag silently. The invariant therefore needs a table test over the verb set (D3.8), not a flag and a review note.

#### D1.1.4 Build / don't-build, with cost classes

Ledger vocabulary is the `Scope:` field (S/M/L) and the `Impact:` field (L/M/H).

| Item | Scope | Impact | Recommendation |
|---|---|---|---|
| gh#263 multi-seat input model | M | M | **Don't build speculatively.** It has no consumer of its own. Build only alongside gh#262 or gh#265, never first. |
| gh#262 agent seat + Stop | L | H | **Don't build yet.** Highest impact in the tranche and the most expensive to get wrong. Gated on gh#263 shipping and on a consumer decision (D8.2). |
| gh#265 browser remote session | L | M | **Defer.** Largest security surface, weakest case, and its prerequisite is not built. |

No item here is recommended *against outright* — that verdict belongs to gh#264 in its own document, where the residue after removing what already exists is empty. **The distinction is not cosmetic:** gh#264 needs no work at all; these three have real, identified residue that is simply not yet wanted. That is a deferral, not a rejection, and it is reversible on one owner answer.

---

## D2. gh#263 — multi-seat input model (REC-int-04, bd sysc-1211)

Addresses GAP-integ-ses-03. Ledger: Scope **M**, Rung new-code, Impact **M**.

### D2.1 The dependency order inverts how the ledger presents it

The ledger lists int-04 as a *prerequisite* of int-01 and int-06, and then treats it as one item among seven. It is actually the load-bearing item: it is the only one of the three with no consumer of its own.

**int-04 is only worth designing together with a consumer.** The ledger says this. The consequence is sharper than the ledger states: if the owner declines both computer-use agents and browser remote access, int-04 is dead code with no consumer, and the correct action is to record a parity exclusion rather than build a seat table. **Do not start the seat table speculatively.** A seat table with one seat is strictly more than M0 already has, and adding the second seat before a consumer exists builds a security boundary nobody asked for.

### D2.2 User-visible behavior

Nothing. This item has no user-visible behavior of its own. It is a capability that becomes visible only through gh#262 (a cursor that is not yours, a glow on your monitor, a Stop button) or gh#265 (a remote browser session with remote input). **That is the honest description and it is why the item is conditional.**

The user-visible consequence of getting it wrong is also concrete: an injected keystroke landing in the user's focused window, moving the user's text selection, and typing into whatever the user was typing. That is the failure mode the isolation rules exist to prevent.

### D2.3 Owners and data flow

Target package is `internal/server` (seat table) and `internal/wm` (focus asymmetry). This is compositor-owned end to end. **sysc-shell owns nothing in this item.**

sysc-shell already binds exactly one `wl_seat` as a client: `internal/platform/wayland/client.go` (`bindSingleton`) and `internal/platform/wayland/registry.go` (`bindVersion`). That single-seat client assumption is *why* the shell is unaffected, and it must not change. A second seat in the compositor is invisible to a client that binds one seat.

Data flow for an injected keystroke:

```
uinput / virtual-keyboard request
  → wlroots input path
  → internal/server seat table: which seat owns this device?
  → seat == agent seat?
      yes → internal/wm focus asymmetry gate (D2.4) → agent seat's own focus only
      no  → normal user-seat focus path
```

The gate lives at the **seat-table lookup and the focus handler**, not at the virtual-keyboard call site. wlroots' virtual keyboard protocol does not carry a seat identity on its own; seat attachment is an explicit decision this design must state, because the ledger's risk note is right that *"virtual-keyboard/virtual-pointer are not per-seat isolation by themselves."*

### D2.4 The isolation rules

These are security-critical and are the easiest thing to lose in a wlroots port. They are stated here as testable invariants:

1. **A keyboard on an injected seat must never move the user's data-device or primary-selection focus.**
2. **An injected seat must never set the drawn cursor.**
3. **An injected seat must not become a second session authority.** sysc-lock's one-graphical-session-per-UID verification (`README.md`:105-108) is the family precedent for this; the agent seat must not acquire the authority sysc-lock holds.

pleamar-wm evidence: `src/nest.rs`:3354-3363 (`focus_changed` early-return for the agent seat), `src/nest.rs`:3367-3371 (`cursor_image` ignored for the agent seat), `src/agent.rs`:1711-1714 (`is_agent_seat` gates).

#### D2.4.1 Why rule 3 is nearly free, and what the substrate does not do for us

The seatd and libinput reality is not background; it decides which of the three rules is structural and which is hand-written.

- **`seat0` is the logind-backed seat.** It holds the DRM master, it is the only seat that can switch VTs, and it is the only seat `seatd` — wlroots' session and seat management daemon — configures from logind. A secondary seat is created by the compositor itself and is **seatd-less**: no logind session, no DRM master, no VT switching, no libinput-logind association.
- **Consequence for rule 3.** A seat with no DRM master *cannot* become a second session authority. sysc-lock's one-graphical-session-per-UID verification is therefore not merely respected by an injected seat — it is **structurally unreachable from one**. The rule holds for free at the substrate level. Keep it in the design anyway, because the way it would be lost is a future commit that grants the agent seat a DRM master or routes its devices through `seatd`.
- **Consequence for rules 1 and 2.** A secondary `wl_seat` is **empty by default** — capabilities are not implied. Attaching `virtual-keyboard-unstable-v1` / `virtual-pointer-unstable-v1` to a chosen seat and setting `WL_SEAT_CAPABILITY_KEYBOARD` / `_POINTER` on it is an explicit act with an explicit failure mode. Good: the routing is visible in the diff. Bad: it is also the place a partial port gets it wrong, which is why D2.7 tests it rather than assuming it (D8.6).
- **libinput.** Physical devices are assigned to a seat by the compositor's own libinput seat assignment. A keyboard physically on `seat0` stays there. **An injected seat must be fed by a virtual device, never by re-routing the user's hardware** — re-routing physical input would deliver the user's own keystrokes to the agent seat and invert every isolation rule in this section.
- **What the substrate does not do.** logind's "active session" notion, `active-by-default`, VT switching and output blanking are all **`seat0`-scoped**. An injected seat has no idle state of its own and does not participate in blanking. That is convenient — agent activity will not keep the screen awake — but it means nothing in wlroots stops an injected seat when the session is locked or blank. That has to be answered explicitly, which is D2.6.1.

### D2.5 Protocols

**Required:** `wl_seat` (already required at design:87); `virtual-keyboard-unstable-v1` and `virtual-pointer-unstable-v1` (already planned at design:307), attached **per seat, explicitly**.

**Optional:** per-seat presence publication (see D2.7). No new protocol.

The virtual-keyboard/virtual-pointer globals already exist in the plan. This item's contribution is that they must be attached to a chosen seat rather than implicitly to the user seat — which is a routing decision, not a new global.

### D2.6 Error and shutdown behavior

An injected input request naming a seat that does not exist fails as an unknown-target error on the control socket and injects nothing. Injected input is never partially applied: the seat lookup is total, so a failed lookup produces no device activity.

On compositor shutdown, any seat with an attached virtual device has it removed. No seat state persists across restarts; a seat is a runtime construct, not configuration.

#### D2.6.1 Lock and idle paths — the interaction the ledger does not name

This is the interaction with the sysc-lock lineage that this design must respect and **must not duplicate**.

- **Blanking and idle are `seat0`-scoped** (D2.4.1). An injected seat has no idle state, so nothing in wlroots suspends it. **Injected seats are inert while the session is locked or blanked** — enforced by this repository, at the injection gate, against the same lock state sysc-lock establishes. It is a rule, not an inherited behavior.
- **Injection must not continue through a lock.** The gh#262 socket is never an unlock path (D3.2), and "the agent may still type while my screen is locked" is the same defect wearing different clothes. Both statements are the same invariant seen from two directions; assert them with the same test.
- **Do not re-specify the lock.** sysc-lock verifies one graphical session per real UID (`README.md`:105-108). That verification is owned there and is not restated here. **The only constraint this design places on it is negative:** an injected seat must not acquire, refresh, satisfy or bypass that verification, and must not appear to `seatd` as a session. Lock work owned elsewhere (bd sysc-910) is not duplicated, revisited or second-guessed by this document.
- **The failure to guard against** is a future commit that routes an injected device through `seatd`, or grants the agent seat a DRM master. Either one converts "structurally unreachable" (D2.4.1) into "enforced by our own code", which is a strictly weaker guarantee and should be treated as a security regression.

### D2.7 Tests and live hardware matrix

Automated, in the compositor repository, under its own test rules (one package, one named test, `GOMAXPROCS=2`, no `-race`, no `./...`):

- seat table lookup: unknown seat → no device activity; agent seat → agent-seat path; user seat → user path.
- focus asymmetry: an injected key event does not move data-device or primary-selection focus. Assert on the focus state before and after, not on absence of a crash.
- cursor: an injected pointer event does not set the drawn cursor.
- a table test over seat/device assignments, because this is pure routing logic and belongs in a table.

Live: **unrun, because nothing is built yet** — not for lack of hardware. A second seat needs no second output and no second physical device: an in-process `wlr_keyboard_init` / `wlr_pointer_init` device in the nested M0 compositor exercises it on this host. **Until that code and test exist no result may be claimed.**

No performance budget is proposed. The orchestration rule is *"record measurements before setting hard budgets"* and there are no measurements.

### D2.8 Non-goals

- No second **user** seat. Multi-seat here means user seat plus non-user seats.
- No per-seat configuration surface in KDL; seats are runtime constructs.
- No shell-side work. The shell's one-seat client assumption is unchanged.
- No gesture, touch or tablet seat.
- No remote or agent consumer in this item — it provides the mechanism for gh#262 and gh#265, it does not introduce either.

### D2.9 Stop / reconsider condition

**Reconsider if** the owner declines both computer-use agents and browser remote access: there is no consumer, and the correct outcome is a recorded parity exclusion, not a seat table.

**Reconsider the seat design itself** only if the D2.7 routing test cannot make an agent-seat device deliver to the agent seat alone. The wlroots API leaves attachment to the compositor (D8.6), so this is now a test of this repository's code, written first in gh#263's TDD plan.

---

## D3. gh#262 — agent seat with compositor-enforced Stop (REC-05, bd sysc-1208)

Addresses GAP-integ-ses-05. Ledger: Scope **L**, Rung new-code, Impact **H**.

### D3.1 Owner and location

**sysc-compositor, not the shell.** sysc-main `AGENTS.md`:9 keeps a lock screen and a compositor out of sysc-shell. A seat is a compositor object. The shell binds one `wl_seat` as a client and consumes published facts; it is not a participant in input routing.

The ledger states plainly: *"no shell-side work is proposed."* That remains true for this item. The shell-side consequences are gh#268 (presence drawing), which is specified separately and gated.

### D3.2 Trust boundary — stated precisely enough to be rejected safely

This is the part of the item that must survive review. Three boundaries, each independently falsifiable.

**1. Process identity.** The control socket lives at `0600` under `$XDG_RUNTIME_DIR`. Every accepted connection verifies `SO_PEERCRED` real-UID against the UID that owns the graphical session. Model: sysc-lock `internal/lockd/client.go`, whose `README.md`:105-108 records the one-graphical-session-per-UID verification.

Be precise about what this is. **A `0600` socket any process running as the user can open is not an authorization boundary. It is a process-identity boundary and a capability boundary at best.** Anything reachable through this socket must grant nothing else — no session token, no unlock, no stored credential. If a design needs the socket to be more than this, it needs a different mechanism with its own review.

**2. The socket must never be an unlock path.** If the agent socket can authenticate a session or unlock the screen, then any process running as the user can act while the screen is locked, and the lock screen's entire value is gone. This is an explicit non-goal (D3.7) and it is the single line in this design most likely to be worth refusing the whole item over.

**3. Seat isolation.** The three rules in D2.4 apply to this seat without exception. A seat that can inject input but is gated from the user's focus and cursor is a bounded capability; a seat that can also move the user's selection is not.

**Residual risk, stated rather than hidden:** a same-user process that can open the socket can inject input into the agent seat and target agent-opened windows. That is an accepted consequence of a same-user capability model, and it is why the agent seat's targets are agent-owned windows rather than arbitrary ones wherever that can be arranged (D3.4).

### D3.3 User-visible behavior

An agent seat exists and is active. The user sees, in gh#268's shell UX: a cursor that is not theirs, with its name on it; a glow on the output the agent works on; an outline on the window it targets; and a pill reading *"An agent is using this monitor"* whose **Stop** is an explicit destructive action. Those are shell draws over compositor facts; the compositor itself draws nothing.

Stop, from the user's side, is a button. What it must mean is D3.5.

### D3.4 Data flow

Compositor-created agent seat for injected input, per-seat targets, control socket, and presence facts. Data flow for a request:

```
agent process
  → 0600 socket, SO_PEERCRED real-UID verified
  → verb set (cua-inject-v1-style), target + action
  → stopped-by-user flag for this seat?  → yes: mutating verbs answer refused
  → target resolution: pid:N / root:N / app-id  (agent-opened windows tracked in internal/wm)
  → seat-targeted virtual device (D2.4 gate)
  → injection
```

**Per-window targets.** The compositor tracks agent-opened windows in `internal/wm` so a request names a window by `pid:N`, `root:N` or `app-id` rather than by pixel coordinates. Coordinate-only targeting means an agent request aimed at a window the user moves lands somewhere else. This is a correctness requirement, not a convenience.

#### D3.4.1 Two seats or one? An unresolved discrepancy, stated rather than papered over

**The handover and the Cua model describe TWO extra seats. The ledger's approach text describes one seat table with per-seat input routing and agent-opened window tracking. This design currently specifies one.** That disagreement is recorded here rather than resolved by assertion, because picking one silently would be the exact failure the handover's "viability notes must compare against real pleamar-wm evidence, not vibes" clause is aimed at.

Two readings are plausible and they are not the same feature:

- **(i) agent seat + agent-dialog seat.** One seat carries the agent's ordinary input; a second exists so that dialogs the agent raises — a permission prompt, a confirmation — are themselves input-isolated, and the agent cannot answer a dialog addressed to the user. **This reading has a security purpose the one-seat model lacks.** If the agent and the user share a seat for dialogs, the agent can type *Allow* into the user's permission prompt, and gh#267's entire card is theatre. It also explains why the handover pairs "two extra seats" with "per-window targets/**dialogs**".
- **(ii) one seat per agent**, with the seat table generalising from one to N. A multi-agent story, with N isolated cursors. Nothing in the ledger's text asks for N, and D2.4's rules are stated per seat rather than per agent.

**Correction 2026-10-15 (ownership audit, T3-2): pleamar implements (ii), not (i).** The gap dossiers record two symmetric agent seats, `cua-agent` and `cua-agent-2`, each with a US keyboard and a pointer, `CURSORS = 2`, and two drawn cursors (`research_notes/pleamar-gap-analysis/03-pleamar-sisters.md`:251-252, `17-domain-integ-session.md`:241-242; the two dossiers disagree on the `agent.rs` line anchors). Reading (i) is therefore a sysc proposal, not pleamar's design. Its consent argument is also weaker than stated: the shell binds one `wl_seat` (D6), so injected input on another seat never reaches a shell-drawn card at all, provided the shell binds the user seat by name (bd `sysc-1254`). **Owner decision 2026-10-15 (D8.5): one agent seat**, written as a table so a second is a row.

**Either way, one seat is a floor, not a ceiling.** The seat table (D2.3) is written as a table, not a singleton, so a second seat is a row and not a refactor.

### D3.5 Stop semantics — where enforcement lives

**Stop is enforced in the compositor, at the point where input is routed.** Not in the shell. Not in the agent client. The shell's Stop button is a *request* over the same socket; it is not the enforcement point, and a shell that fails to render or dies must not un-stop an agent seat.

After Stop, **every mutating verb answers `stopped-by-user`.** Read verbs may continue — otherwise the user cannot verify that Stop worked, and "no state changed" becomes indistinguishable from "nothing happened".

pleamar-wm evidence: `src/agent.rs`:409-415, where mutating verbs answer `stopped-by-user` after Stop.

**Deliberate divergence from pleamar.** pleamar's Stop lapses: mutating verbs refuse "until `done` or a minute" (`17-domain-integ-session.md`:245). Here Stop does not expire. Only the user lifts it, because a Stop that silently ends is a consent failure of the same kind as a faded capture indicator (D4.6.2).

Note the vocabulary boundary (D6): `stopped-by-user` is **not** one of the shell's Tier 1 error codes. It belongs to the compositor's control socket. See D5.

### D3.6 A channel finding the ledger does not name

The ledger proposes publishing `agent.active` and `agent.N.*` on the niri-ipc event stream (niri-ipc pinned 26.4.0). **That would break a commitment the compositor's own design makes.** `2026-10-02-sysc-compositor-design.md`:213 heads its IPC section *"complete niri-ipc 26.4.0"*, and `:410` allocates `internal/ipc/` for that wire format in pure Go. niri-ipc 26.4.0 is a **closed enum**. Sysc-specific fact kinds cannot be added to it without either forking the pin or emitting events a stock niri client will fail to decode — and the whole reason for the parity contract is that an existing `~/.config/niri/config.kdl`, an existing `niri` CLI and existing niri clients keep working.

**Design answer: agent facts ride the session-local control socket this item already requires, not the niri event stream.** One socket, two directions: requests inbound, facts outbound. This adds no divergence from the niri pin, needs no fork, and keeps every agent fact inside the same `0600` peer-verified trust boundary that D3.2 already defines. It also means the shell needs one authenticated channel for the whole feature instead of two.

The shell then reads agent facts from that socket, peer-verified exactly as sysc-lock verifies its registration.

### D3.7 Non-goals

- **The socket is never an unlock path, a session authenticator, or a credential store.** (D3.2)
- No shell-side approval or presence UX in this item — that is gh#267 and gh#268, both gated.
- No agent-opened window ever becomes a second session authority.
- No Stop enforcement in the shell under any circumstance.
- No remote or network reachability of the agent socket. It is session-local; gh#265 is the only item that transports input off-host, and it must not reuse this socket.
- No model runtime, credential store or LLM in the compositor. If an assistant needs a model worker, that is gh#266's owner decision and it is a separate supervised process.

### D3.8 Tests and live hardware matrix

Automated, in the compositor repository, under its rules:

- socket rejects a connection whose `SO_PEERCRED` real-UID is not the session owner (table test over accepted/rejected UID).
- socket file mode is `0600` under `$XDG_RUNTIME_DIR` (assert the mode; do not trust the create call alone).
- Stop: every mutating verb answers `stopped-by-user` after Stop. Table test over the verb set — this is the enforcement invariant and it is exactly the kind of thing a partial port loses.
- Stop: read verbs still answer, so Stop is observable.
- target resolution: `pid:N`, `root:N`, `app-id` each resolve; an unknown target injects nothing.
- an agent seat cannot move user data-device/primary-selection focus and cannot set the drawn cursor (inherited from D2.4, re-asserted per seat).

Live: **unrun, because nothing is built.** A second seat and its device can be created in process in the nested compositor (D2.7); no second output or device is needed. **No live result may be claimed for injection, Stop, or isolation until that code exists.**

### D3.9 Stop / reconsider condition

**Reconsider if** the owner declines computer-use agents: record a deliberate parity exclusion and close gh#262 without building.

**Reject the whole item** if the design ever requires the socket to authenticate a session or unlock the screen. That is not a parameter of this feature; it is the feature's opposite.

---

## D4. gh#265 — browser remote session (REC-int-06, bd sysc-1213)

Addresses GAP-integ-ses-04. Ledger: Scope **L**, Rung new-code, Impact **M**.

### D4.1 Verdict: recommend defer

This is the weakest case of the three and should not be started on the strength of a design document. Reasons, in order:

1. **It is conditional on an owner desire that has not been expressed.** Nobody has asked for browser remote access.
2. **It is the largest security surface in the tranche** — capture plus remote input — and it needs the owner's security model *before* code, not alongside it.
3. **It has a prerequisite (int-04) that is not built**, and building injection before isolation is exactly the wrong order.
4. It is Scope **L** with Impact **M**: the most expensive item in the tranche for the second-least benefit.

The requirements below are recorded so that if the owner does want this later, the constraints are already written down and the failure modes are named.

### D4.2 Owner and location

Session/compositor, **not the shell**. Input injection and capture are session-level security decisions. sysc has only local capture paths today.

sysc-compositor already plans capture: screencopy plus ext-image-copy-capture (`design`:298-299), Mutter ScreenCast/PipeWire at M12 (`design`:327, :335), and the injection globals (`design`:307). This item adds a transport on top of capabilities that are already planned.

### D4.3 User-visible behavior

The user reaches a session from a browser elsewhere: they see their desktop, and they can type and click into it. They are told, on the session, that someone remote is connected. Nothing in the shell changes.

### D4.4 Data flow

```
remote browser
  → TLS-terminating external front (delegated, not ours)
  → session-local transport
  → authenticated session
  → capture: screencopy / ext-image-copy-capture (M12)
  → input: virtual input or uinput, on the USER seat — not an agent seat
```

Two deliberate choices:

- **TLS is delegated to an external front**, as pleamar does. A Go web server with TLS keys is a new secret to store, rotate and protect. pleamar's shape keeps that outside this codebase.
- **Remote input routes on the user seat, not an agent seat.** A remote session is the user, remote — it *is* the user seat, with the user present. This is the opposite of gh#262's non-user seat and the distinction is security-critical: routing remote input through a capability seat would put a network-reachable client behind the isolation rules in D2.4, which is the wrong way round.

pleamar-wm evidence: injection `src/remote.rs`:2019-2035; auth `src/remote.rs`:29-37, :136-206, :247-250 (password + single-use code + hashed sessions); desk indicator `examples/session.plm`:8363-8399.

### D4.5 Protocols

**Required:** the planned capture protocols (`design`:298-299); the injection globals (`design`:307); a session-side transport with an auth model carrying **a second factor**.

**Optional:** a desk indicator, only if the compositor publishes presence facts for it. Do not add a surface for an indicator whose facts do not exist.

### D4.6 Trust boundaries

D3.2's three boundaries apply, with the transport as an added one:

- The transport is the one path in this tranche that leaves the machine. Everything else is same-user, session-local.
- **Auth is the boundary.** Password + single-use code + hashed sessions (pleamar's shape). A second factor is not optional here; a single factor on a path that delivers keystrokes into the user's session is not a control.
- **Capture must honor the compositor's exact-sized SHM-pool rule for screencopy consumers** (`design`:298). A remote viewer is a screencopy consumer and gets no exemption.
- sysc-lock's real-UID/peer verification remains the family precedent for anything session-local.

#### D4.6.1 Threat model, on one page

The transport is the only path in this tranche that leaves the machine. Everything else is same-user and session-local, which is why gh#262's risk is bounded and gh#265's is not.

| Adversary | What they can reach | What stops them | Residual |
|---|---|---|---|
| **Other local users** | the transport's listening port | **Loopback bind by default.** Never `0.0.0.0`. | An external front that terminates TLS and forwards is a separate trust decision the owner must accept explicitly. |
| **Anything reachable over the network** | keystrokes and frames in both directions | Password **plus** a single-use code, sessions stored hashed, TLS terminated by the external front (D4.4) | A single factor on a keystroke path is not a control. The second factor is load-bearing, not decoration. |
| **A stolen or replayed session** | a live authenticated session | Single-use codes expire; sessions are hashed; codes are not reusable | Replay inside an unexpired window is unaddressed and is part of the owner security model that must precede code. |
| **A malicious remote viewer** | sees everything the user sees | **Consent** (D4.6.2) — capture does not start without it | Consent covers *start*, not *content*. The viewer sees whatever is on screen, which is the honest limit. |
| **A same-UID process on this machine** | the gh#262 agent socket, not this transport | Not addressed here — and deliberately. A remote session is the *user, remote*; a same-UID process is a different model with a different socket (D4.7). | Keeping the two transports unmerged is the mitigation. Merging them widens both blast radii. |

**Explicitly out of scope:** TLS termination and certificate handling (external front), codec choice, any web stack, any listener bound beyond loopback, and any path by which sysc-shell acquires a network dependency. If a design needs any of these, it has left this document.

#### D4.6.2 Consent, and what the user sees under capture

**Consent is a user-visible behavior, not an implementation detail.** sysc has no remote capture today; adding one without asking first is the failure this section exists to prevent.

- **Capture does not start without an explicit, per-session grant.** The grant is the user's, made deliberately, revocable, and it covers *this session*, not a standing preference. Mirroring the PipeWire ScreenCast portal model that `design`:327 and `:335` already schedule at M12 — a grant dialog, then a session that can be ended from the session's own surface.
- **The indicator is not dismissible.** While capture is live the user sees an indicator naming the remote session. There is no timeout after which it fades, because a faded indicator is a silent capture. The indicator is drawn session-side; the shell's own local capture paths (screenshot capability, screen-recorder plugin) keep their existing behavior and are **not** governed by this.
- **Ending capture is one action and always available**, including without the remote end asking. Revocation must take effect at the compositor, not at the remote end — a remote-controlled stop is not a stop.
- **Refusal is a normal outcome.** Declining the grant is not an error and does not retry or nag. It uses the existing vocabulary unchanged (D5).
- **Indicator behaviour under capture is a security boundary, not a UX detail.** A remote implementation that draws the indicator only when the remote end cooperates has no indicator at all. The compositor draws it; the transport cannot suppress it.

### D4.7 Non-goals

- **No web stack in sysc-shell.** Streaming and codec choices stay session-side. The shell gains nothing from this item and must not acquire a dependency for it.
- The shell keeps its existing local capture paths (screenshot capability, screen-recorder plugin) unchanged.
- No reuse of the gh#262 agent socket. A network-reachable transport and a same-user capability socket are different trust levels; merging them widens the blast radius of both.
- No agent seat. Remote input is on the user seat (D4.4).
- No sysc-lock, greeter or wallpaper scope; those are excluded by the compositor's own README.

### D4.8 Tests and live hardware matrix

Automated, in the compositor repository: auth state machine including single-use code expiry and session-hash verification; transport rejects before authentication completes; capture pool sizing honored; remote input routed to the user seat and not to an agent seat.

**Live, and unrunnable here:** capture and remote input need a second machine and a network path this host does not have. Unrun. **No claim may be made about remote capture working.**

### D4.9 Stop / reconsider condition

**Recommend defer.** Revisit only when the owner states a want for browser remote access, D2 (int-04) has shipped and been exercised, and an owner security model exists for capture plus remote input.

**Reject** any version of this item that pulls a web stack, a TLS listener or a codec into sysc-shell, or that reuses the gh#262 socket.

---

## D5. One vocabulary, two envelopes

sysc-shell's Tier 1 interface (gh#237–#242) already defines a **closed 13-code error vocabulary** carried through sysc-741's error envelope (`2026-10-15-tier1-interface-design.md`:88):

`unsupported` · `invalid_params` · `unknown_target` · `ambiguous_target` · `stale_target` · `disabled` · `person_only` · `covered` · `busy` · `value_hidden` · `limit_exceeded` · `timeout` · `cancelled`

This tranche must **not** invent a second shell-side vocabulary, and must not add a code to that list for a compositor concern.

**Two envelopes exist and they do not merge:**

| Envelope | Owner | Vocabulary |
|---|---|---|
| Shell IPC (plugin/CLI) | sysc-shell | the 13 Tier 1 codes above |
| Compositor control socket | sysc-compositor | `stopped-by-user` and the socket's own request/failure set |

`stopped-by-user` is a compositor fact and belongs to the second envelope. The shell learns it as a fact and renders it; the shell does not mint it. The only requirement on the shell side is that it displays the refusal without inventing a code, and that D8's *"Refusals explain who must interact without exposing denied values"* (`2026-10-15-tier1-interface-design.md`:128) is honored for agent refusals too.

---

## D6. What the shell does, in total, across gh#262, gh#263, gh#265

**Almost nothing, with one exception found 2026-10-15.**

The shell binds one `wl_seat` as a client (`internal/platform/wayland/client.go` (`bindSingleton`) and `internal/platform/wayland/registry.go` (`bindVersion`)), which makes the compositor's internal seat count invisible to it. **The exception:** `registry.go`'s `addGlobal` keeps the *last* advertised `wl_seat`, so once this repository advertises an agent seat, a shell started while it exists can bind the agent's seat, and removing that seat can look like losing `wl_seat`. The shell must bind the user seat by name (`wl_seat.name`). Tracked as bd `sysc-1254`, gated on gh#262; the guarantee that agent input never reaches shell surfaces depends on it. gh#262's and gh#263's enforcement lives where input is routed. gh#265 keeps its transport, auth and codecs session-side and explicitly bars the shell from acquiring a web stack.

The one shell-side consequence in this tranche is **reading agent presence facts**, which is gh#268, designed and gated separately in `2026-10-15-tier3-agent-experience-design.md`. It depends on D3.6's decision that facts arrive on the session-local socket rather than the niri event stream.

---

## D7. Non-goals across the tranche

- No product code in sysc-shell. This document produces none.
- No repository is created by this design. One was created by the owner; see D0.
- No work in sysc-shell that blocks or depends on Tier 1 or Tier 2 branches.
- No GUI toolkit, scene language or scripting language. The mechanism pleamar uses for WM policy is Luau and is banned outright (gh#264 covers this).
- No new UI primitive in sysc-shell. There is no shell consumer in this tranche.
- No benchmarks, budgets or latency claims. Record measurements first.

---

## D8. Owner decisions this document records rather than makes

1. ~~**Will `sysc-compositor` become a repository and get built?**~~ **Answered 2026-10-15: yes.** `github.com/Nomadcxx/sysc-compositor` exists and M0 code is on `main`; M0's pointer move/resize exit gate is still open.
2. ~~**Does sysc want computer-use agents?**~~ **Owner answered 2026-10-15: yes** (bd `sysc-1208`). gh#262 proceeds after D2 ships; this is a decision, not an implementation commission.
3. **Does sysc want browser remote access?** If no, gh#265 closes as a parity exclusion. This document's own recommendation is *defer*.
4. ~~**Is a second non-user seat acceptable at all?**~~ **Owner answered 2026-10-15: yes** (bd `sysc-1208`, `sysc-1211`). gh#263 now has a consumer.
5. ~~**How many agent seats?**~~ **Owner answered 2026-10-15: one** (bd `sysc-1208`). pleamar runs two symmetric agent seats (D3.4.1, corrected); sysc builds one, written as a table so a second is a row and not a refactor.
6. ~~**Verify before planning:** can wlroots 0.20 attach a virtual device to a chosen seat?~~ **Answered by API inspection 2026-10-15** (D8.6): attachment is the compositor's own handler, so the question moves from feasibility into gh#263's TDD plan.

### D8.6. What the probe actually answered

The test that landed was narrower than the original question. It answers whether a wlroots display
can hold independent seat objects with independent capability state. It does not answer whether a
virtual device can be attached to a chosen seat.

**Proven by test** (`TestSeatTable` in `internal/wlr/seat_probe_test.go`):

- A wlroots 0.20 `wl_display` holds **three simultaneously distinct seats**. `wlr_seat_create` is
  per-display and there is no hidden singleton; the pointers differ and each is independently live.
- `wlr_seat_set_capabilities` is **per seat**. Seats set to `keyboard`, `pointer` and
  `keyboard|pointer` each read their own mask back.
- Clearing every seat but one **does not move the survivor's capabilities**. Seat capability state
  is not shared through any global the probe could reach.

**The seat-object part of D2 is buildable, and D1.1.3's "wlroots has one interesting seat" was wrong
about seat count.** This does not prove the virtual-device path in D2. It was right about pointer
routing, which is a different claim and survives.

**The residual that does survive** — the asymmetry D1.1.3 identified is real, just narrower than
predicted:

- There is **no `wlr_seat_set_pointer`**. A cursor attaches to input devices via
  `wlr_cursor_attach_input_device`, and the compositor feeds the seat by calling
  `wlr_seat_pointer_notify_*` itself. A per-seat cursor is therefore **compositor code, not a wlroots
  call** — the routing decision lives where D2 says it should.
- `wlr_seat_set_keyboard` *is* per seat, so the keyboard half of the isolation rule in D2.4.1 is a
  direct call.
- `wlr_keyboard_create` and `wlr_pointer_create` are **internal in 0.20**. Creating one means
  instantiating a `wlr_virtual_keyboard_manager_v1` / `wlr_virtual_pointer_manager_v1`, and the
  wlroots package **does not ship the protocol XML**. Commit `746afa6` deliberately did not do this:
  virtual-device attachment remains a separate, untested question.

**Virtual devices, answered from the installed headers (wlroots0.20 0.20.2-1, 2026-10-15).** The
protocol XML is not needed for the design in D3.4:

- `wlr/interfaces/wlr_keyboard.h`:20 `wlr_keyboard_init` and `wlr/interfaces/wlr_pointer.h`:18
  `wlr_pointer_init` are public under `WLR_USE_UNSTABLE`, which this package already sets. The
  compositor can create an agent-seat keyboard and pointer in process and feed them from its control
  socket, with no Wayland protocol involved.
- `wlr_virtual_keyboard_manager_v1_create` and `wlr_virtual_pointer_manager_v1_create` include no
  generated protocol header, so they are callable from cgo without XML. `struct wlr_virtual_keyboard_v1`
  carries the client-requested `seat`, and `wlr_virtual_pointer_v1_new_pointer_event` a
  `suggested_seat`. wlroots emits `new_virtual_*` and attaches nothing; the compositor's handler picks
  the seat.
- XML is needed only by an *external* client that injects over Wayland. It is not in
  wayland-protocols 1.49; it would be vendored from wlroots / wlr-protocols (bd `sysc-1242`).

So the probe's `forced_to_seat0` and `unsupported` outcomes have no place in the 0.20 API, and
`attach_to_named_seat` is a property of code this repository will write. **D8.6 is closed as a
feasibility question.** Proof of the routing and of D2.4's isolation rules is the first failing test
in gh#263's TDD plan.

**What this changes in the plan.** Neither D2 nor gh#262's Scope L needs a re-estimate. D8.2 and D8.4
were answered yes and D8.5 answered one seat on 2026-10-15.

---

## D9. Coverage map

| Issue | Ledger rec | Sections | Verdict |
|---|---|---|---|
| gh#262 / sysc-1208 | REC-05 / REC-int-01 | D0, D1.1, D3, D3.4.1, D5, D6 | Requirements + viability; transfer complete 2026-10-15 |
| gh#263 / sysc-1211 | REC-48 / REC-int-04 | D0, D1.1, D2, D2.4.1, D2.6.1, D5, D6 | Requirements + viability; transfer complete 2026-10-15; prerequisite for both others |
| gh#265 / sysc-1213 | REC-50 / REC-int-06 | D0, D1.1, D4, D4.6.1, D4.6.2, D5, D6 | Requirements + viability; **recommend defer**; transfer complete 2026-10-15 |

**Gate 1 has cleared** — the repository exists and these documents were transferred into it on
2026-10-15. **Gates 2 and 3 do not apply to this document**: it contains no assistant surface, and it
defines its own types rather than depending on another implementation's. What remains is not a gate
but an ordering fact: M0 now creates one production seat and cursor, and its pointer move/resize gate
is still open. `TestSeatTable` proves that wlroots can hold multiple independent seats; virtual-device
attachment remains untested.

**M0 code exists, but its pointer move/resize exit gate remains open.** `TestSeatTable` confirms
independent seat objects and capability state, and D8.6 is answered by API inspection. D8.2, D8.4
and D8.5 were answered on 2026-10-15 (yes, yes, one seat). What gates a gh#263 / gh#262 TDD plan is
M0's exit gate and a separate implementation commission.

The remaining item, gh#265, stays a **defer** recommendation. Its capture side is planned for M12, which
has not started; it needs that work, a built gh#263, and the owner's security model.

No bd closure, and no milestone plan — this document still plans no code.
