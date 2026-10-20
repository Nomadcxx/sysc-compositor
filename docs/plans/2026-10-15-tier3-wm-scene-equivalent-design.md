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
---

## Owner briefing (2026-10-16)

You asked to understand WM-as-scene before deciding on `sysc-1212`. This section explains it. It does
not decide anything; the choice is at the end.

### What pleamar does

In pleamar, the window manager is a scene file, not compiled code.
- `pleamar-wm` is the compositor. It hands the pleamar runtime a factory that starts the Wayland thread
  (`pleamar-wm/src/main.rs:24-29`).
- Everything the user sees as window management lives in one 8,548-line scene, `examples/session.plm`:
  - layout: `screens: each max 4` and `windows win max 16` (`:52-53`), then leader share and carried-window
    boxes as scene arithmetic (`:411-427`);
  - five layouts — leader-left, columns, rows, grid, leader-right (`:387-395`);
  - free-window title bars and buttons (`:6436`);
  - an overview of pool cards (`:5087-5095`);
  - a dock rising from the bottom edge (`:587-605`);
  - live window previews (`:7432-7465`);
  - a drag that shows every window's new box while you carry one (`:429-446`).
- The compositor only obeys. It receives the scene's orders (`ToNest::Configure`, `Send`, `Pointer`,
  `Key`; `src/nest.rs:974`) and refuses nonsense boxes (`:989-1010`).
- Key bindings are events the scene declares (`keys.conf:15-20`), reloaded when the file changes
  (`src/keys.rs:88-102`).
- The scene is replaceable: `~/.config/pleamar/wm/session.plm` overrides the built-in one, and
  `pleamar-wm scene DIR` exports it for editing (`src/main.rs:236`, `:245-282`).

The appeal: someone can invent a new layout, decoration or overview by editing a file, with no rebuild.

These citations come from the frozen trees in the gap dossiers
(`/home/nomadx/sysc-shell/research_notes/pleamar-gap-analysis/17-domain-integ-session.md` §3.1 and
`03-pleamar-sisters.md`). Pleamar is not pinned under `/home/nomadx/references`.

### Why it cannot be ported

The scene's logic is Luau. `AGENTS.md:6` in sysc-shell bans Lua and Luau, and this repository is Go plus
cgo only. A scene language would also break the contract every sysc consumer relies on: Niri's KDL,
JSON IPC and CLI (design §4–§6). This is a constraint, not an effort estimate (D1.1).

### What Niri's surface already gives, and when

| You want to change | Niri gives | Milestone |
|---|---|---|
| What a key or mouse button does | `binds { Mod+T { spawn "kitty"; } }`, any of the 141 actions, mouse and wheel triggers, `repeat=false`, `allow-when-locked` | M5 |
| Layout shape | Scrollable columns; gaps, struts, preset column widths and window heights, centring, default widths | M5–M6 |
| Per-application behaviour | `window-rule { match app-id=… open-floating true }`: size, opacity, rounding, floating, workspace, output, block-out-from; `layer-rule` for shell surfaces | M6 |
| Motion | `animations {}`: off, `slowdown`, and per kind (workspace switch, window open, close, movement, resize, overview, …) a spring (`damping-ratio`, `stiffness`, `epsilon`) or an easing curve | M8 |
| Open, close and resize visuals | `custom-shader` GLSL inside `window-open`, `window-close`, `window-resize` (`niri-config/src/animations.rs:385-437`) | after M8 (design §9: later parity item) |
| Editing without a restart | Live reload on save, including binds and window rules | M9 (design §4, `:176`) |

So binds, rules, timing and even custom open and close visuals are authorable in a file without a
rebuild. What Niri does not give is a **new layout algorithm** or **new compositor-drawn UI** (title
bars, a dock, a custom overview). Those are code.

### What a sysc-native layer beyond Niri would require

There are three shapes. Each keeps Luau out.

1. **More KDL vocabulary.** New layout kinds as data (`layout { kind "grid" }`, master-stack ratios) parsed
   in Go.
   - Cost class: **L**.
   - It is the least expressive: you get the layouts someone wrote in Go, with knobs.
   - It also forks the dialect. `niri validate` rejects unknown nodes, so a config using them stops being
     a Niri config.
2. **An external layout process.** The compositor asks a supervised process for window boxes over a socket.
   - River's layout protocol is the prior art.
   - This matches `AGENTS.md`'s "third-party plugins run as supervised processes", and the process can be
     written in any language.
   - Cost class: **L–XL**. It needs a protocol and versioning, a deadline on every reply with a fallback
     to the built-in layout, and a way to keep animations coherent when boxes come from outside.
   - It covers layout, not drawn UI.
3. **A declarative Go scene layer.** It would describe compositor-drawn UI and layout in data, evaluated
   by Go.
   - Cost class: **XL**. This is a second product: a language, a renderer contract and a test surface.
   - D7 forbids reintroducing a scripting language, and a declarative layer drifts towards one.

All three break parity in some way: a user's `config.kdl` would no longer be portable to Niri, or
sysc-shell would need to know about non-Niri state. All three should wait until M9, when the Niri surface
exists and a real gap can be pointed at rather than predicted.

### The choice

| Option | Meaning | Cost now |
|---|---|---|
| **Exclude** | Close `sysc-1212` as a recorded parity exclusion: the authoring surface is Niri's KDL, by choice. Add that row to `niri-parity-matrix.md`. | None |
| **Keep open** | Leave `sysc-1212` open and revisit after M9 with the Niri surface in hand. (Current state.) | None |
| **Future design** | Commission a design for option 1 or 2 above after M9, stating what it adds beyond the 141 actions and what parity it gives up. | A design document; build cost as above |

The recommendation is unchanged from D8: exclude, or keep open until M9. Nothing in M1–M9 depends on the
answer.

## Owner update (2026-10-20): selected capabilities under the Niri contract

The owner direction recorded on 2026-10-19 keeps Niri's product, KDL, IPC, and CLI shape while allowing
selected Pleamar capabilities implemented with Go and cgo. This update supersedes D8–D10's default
recommendation to exclude the broader capability work. The Luau scene mechanism remains excluded by
the Go+cgo rule. The [2026-10-19 capability report](2026-10-19-niri-mango-pleamar-capability-research.md)
compares the pinned references; this update records the selected scope.

| Capability | Reference evidence | sysc decision and owner | Acceptance and stop condition |
|---|---|---|---|
| Window layout and authoring | Niri defines the scrolling column model and KDL controls. Mango's tags and layouts are a different product. Pleamar-wm offers several layouts through Luau scenes. | Use Niri's model and KDL. Implement policy in first-party Go. Do not add alternate layout modes, a private layout dialect, or an external layout process without a concrete user workflow. The compositor owns layout and input. | M5 must pass its existing pure-Go model and nested bind gates while existing Niri config remains valid. Revisit after M9 only if a named workflow does not fit the Niri model; present any KDL or IPC extension to the owner before implementation. |
| Compositor-drawn UI | Niri owns overview and its compositor overlays. Mango's UI follows Mango behavior. Pleamar-wm draws its own overview, title bars, and dock from its scene. | Implement Niri-owned overview and selector behavior in the compositor at M7 and M11. Keep bars, panels, launchers, and notifications in sysc-shell. Do not add a general UI scene API. | Pass the M7 and M11 behavior gates from the Niri parity plan. Stop at those surfaces; shell UI stays a Wayland client. |
| Agent input and Stop | Niri has no isolated agent seat. Mango provides wlroots seat mechanics, not agent policy. Pleamar-wm separates agent input and exposes Stop. | Keep the existing Go control socket and injected seat. The compositor owns routing and Stop enforcement; sysc-shell owns presence presentation. This uses the local control protocol, not Niri IPC or KDL. | Preserve seat-isolation and post-Stop refusal tests. The local `0600` socket plus `SO_PEERCRED` identifies same-UID peers but does not authenticate them. Any peer may set Stop; no socket verb may lift it. Finish re-arm, lock/blank gating, and shell presence under `sysc-1261`, `sysc-1260`, and `sysc-1193`. Add no second agent seat. |
| Window-image capture for the switcher | Niri defines the selector behavior. Mango supplies wlroots protocol patterns. Pleamar-wm demonstrates live window previews. | Use the already approved `ext_foreign_toplevel_image_capture_source_v1` and `ext_image_copy_capture_v1` path in M12. The compositor owns capture and the M11 selector. No Niri IPC or KDL extension is needed. | Join each source to a stable window ID and geometry; keep at most one frame outstanding per window; refresh on damage; stop on selector close or window removal. Suppress `screen-capture` protected windows and apply `screencast` rules only to screencasts. Do not retain frames after the selector closes. |
| Browser remote access | Niri and Mango provide no browser remote-session contract. Pleamar-wm has a remote-session feature. | Defer under `sysc-1213`. Keep transport out of sysc-shell and do not expose the same-UID control socket to a network. The compositor/session owns future capture, input, and revocation. | No code until M12 capture exists and the owner approves a separate security design with per-session consent, a persistent compositor-drawn indicator, and compositor-enforced revocation. |

The 2026-10-19 research report assigned thumbnail presentation to sysc-shell. The later M11 plan assigns
the switcher selector and overlay to the compositor. This update follows M11 for switcher previews;
sysc-shell keeps its own bars, panels, launcher, and notifications.

### M5 gate

M5 can proceed with the existing Niri layout plan. It implements scrollable columns, dynamic workspaces,
Niri KDL binds, and the input behavior recorded in the Mango comparison. Its layout model remains pure Go;
all scene changes remain on the compositor thread. This selection adds no M5 KDL or Niri IPC surface.

The broader capability gate stops here. Reopen it only for a concrete user workflow that Niri parity does
not cover, or for a separate owner-approved security design. Do not build a generic scene engine to keep
the option open.
