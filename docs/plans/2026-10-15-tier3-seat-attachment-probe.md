# Tier 3 — Plan: the per-seat attachment probe (gh#262 / gh#263, gate check)

**Kind:** plan (viability probe, **not** a milestone plan)
**Date:** 2026-10-15 · transferred here from sysc-shell 2026-10-15
**Question:** `2026-10-15-tier3-session-domain-design.md` D8.6 — *"Can wlroots 0.20 attach a virtual device to a chosen seat?"*
**Evidence so far:** commit `746afa6` proves multiple independent seat objects and per-seat capabilities. It does **not** test virtual-device attachment.
**Resolution (2026-10-15):** answered by API inspection, not by this probe — see *Resolution* at the end. wlroots 0.20 attaches no virtual device itself; the compositor's handler does. This plan is **retired**: do not execute P2–P6. The routing proof moves to gh#263's TDD plan.
**Target repo:** `sysc-compositor` — **this plan lives here.**

> **Not scheduled work.** `2026-10-02-orchestration.md` Task 3 forbids pre-writing M1–M15 plans. This is
> a viability probe, not a milestone, so recording it ahead of its milestone does not break that rule —
> but it also is not queued. M0 now exists. The implementation in `746afa6` is narrower than P2: it
> tests seat objects and capabilities, not virtual-device attachment. Do not plan Tier 3 implementation
> before the owner decision.

---

## P0. Why this was the only plan in the tranche at drafting

The other six items fail the plan gate's *"exact files"* requirement, not for want of effort:

| Item | Why no plan is writable |
|---|---|
| gh#262, gh#263 | **At plan creation, the repository had no code.** `internal/server` and `internal/wm` were proposed package paths, not directories; M0 had not been written. |
| gh#265 | Design says **recommend defer**, and an owner security model must precede code. A plan would commit to a threat model that has not been written. |
| gh#264 | Design says **recommend against as a build item** — the residue is a documentation pass over an existing design. Planning work the design rejects is the trap the handover names. |
| gh#266 | The deliverable *is* the decision. A plan presumes the decision has been made. |
| gh#267, gh#268 | No producer exists. The handover's trap list bars "half-built hooks on main". |

**This probe is the exception** because its answer is a fact about wlroots, it is small, it needs no design decision, and every answer it returns is actionable. It is also the cheapest possible way to find out whether gh#262 and gh#263 are buildable at all — and its answer is needed *before* M0's seat work is designed, because it decides whether M0's single seat is the only seat this product will ever have.

---

## P1. Preconditions — re-checked at plan creation on 2026-10-15, corrected after a bad probe

**At that point, everything except the code was met.** M0 has since landed; the test that landed is
narrower than this plan's P2 attachment test.

| Precondition | State on 2026-10-15 |
|---|---|
| `sysc-compositor` is a git repository | **Met.** `main`, first commit `4e57b6c`, remote `Nomadcxx/sysc-compositor`. |
| `internal/server` exists | **Not met.** M0 has not been implemented. `internal/server/` is a package-map line in the design (`:407`), not a directory. |
| wlroots 0.20 shared library | **Met.** `wlroots0.20 0.20.2-1`; `/usr/lib/libwlroots-0.20.so`, `/usr/lib/pkgconfig/wlroots-0.20.pc`. |
| wlroots 0.20 headers | **Met.** `/usr/include/wlroots-0.20/wlr/` with `version.h` and the `backend/ types/ render/ util/ interfaces/` subdirectories. |
| `go.mod` | **Not met.** No module yet — M0 Task 1 creates it. |
| Go toolchain | Met. `go1.27.2`. |
| Wayland libraries | Met. `wayland-server` 1.26.0, `wayland-protocols` 1.49. |
| A seat to attach a device to | Met at design level only — M0 creates the first real one. See P3.2. |

> **Correction, 2026-10-15.** An earlier revision of this table recorded the library and headers as
> *Not met*. That was a bad probe, not a missing dependency: it asked `pkg-config --modversion wlroots`
> (Arch installs the file as **`wlroots-0.20.pc`**, so the unversioned name resolves to nothing) and listed
> `/usr/include/wlr` (Arch installs headers to **`/usr/include/wlroots-0.20/wlr/`**). Both reported absence
> where the dependency was installed and working. Recorded here rather than silently corrected, because a
> precondition table that is never re-checked against a real probe is exactly the failure mode that
> invalidated the source ledger's `file:line` citations.

At drafting, this plan was blocked on M0 code and the binding gate. M0 now exists, but the landed test
is narrower than P2: it creates independent seats and checks capabilities without attaching a virtual
device. The original D8.6 device-routing question remains unanswered.

**Do not install wlroots into the shell's host environment to make this run.** A compositor toolchain is
a compositor dependency; sysc-shell does not need one, and this probe does not belong in the shell.

---

## P2. The failing check, written first

**`TestSeatAttachment` in `internal/server` must fail before any implementation exists.**

It is one table test with three rows, because the answer has three possible shapes and the test must be able to record which one is true rather than merely asserting the hoped-for one:

| Row | Asserts |
|---|---|
| `attach_to_named_seat` | A virtual keyboard attached to `agent` seat delivers its key event to `agent`, and `seat0` receives nothing. |
| `forced_to_seat0` | The device cannot be attached to a named seat; wlroots routes it to `seat0` regardless. Recorded as a **pass** of this row with the flag set — a failure mode discovered is a successful probe. |
| `unsupported` | No per-seat attachment mechanism exists at all. |

A test that cannot express "the answer is no" is not a probe; it is a wish. All three rows are legitimate outcomes.

---

## P3. Exact files

Proposed paths, matching `2026-10-02-sysc-compositor-design.md`:407-411, which allocates `internal/server/` (display, backend, globals, run) and `internal/wm/` (windows, workspaces, outputs, floating, overview) — the two packages `:424` implies carry cgo.

| File | New or changed | Purpose |
|---|---|---|
| `go.mod` | new | Module root. First file in the repository. |
| `go.sum` | new | Generated. |
| `internal/server/seat_probe.go` | new | The probe. `runtime.LockOSThread()` before any wlroots call; all listener trampolines in the cgo preamble. |
| `internal/server/seat_probe_test.go` | new | `TestSeatAttachment` — P2's three rows. |
| `internal/server/probe_result_test.go` | new | Records which row passed into a comment in the package doc, so the answer survives in the tree. |

**Nothing else.** No seat table, no control socket, no `internal/wm` change, no `Stop`, no shell work. The probe creates two seats, attaches one virtual device to the second, injects one key, observes which seat received it, and stops.

### P3.1 Dependency gate — recorded, not assumed

| Field | Value |
|---|---|
| **Behavior replaced** | None. A probe adds no behavior and replaces nothing. |
| **Dependency** | `go.wlroots.dev/wlroots` (Go bindings over wlroots). |
| **Pinned version** | **To be resolved in P4.1, not guessed here.** The pin must be the release whose upstream wlroots is **0.20**, matching the charter. Record the resolved version and the reason in `go.mod` and in the commit. |
| **License** | wlroots is MIT; the Go bindings are MIT. Both are permissively compatible with sysc's shipping posture. **Confirm at pin time, do not assume.** |
| **Package boundary** | Confined to `internal/server`. Nothing outside that package learns the answer. |
| **Removal condition** | The probe is deleted once gh#263's real seat table lands — the test moves into it, the probe does not stay as a second implementation of the same question. |

**Removal condition not met:** `internal/wlr/compositor.go` still has one production `seat` field.
`ProbeSeats` creates temporary seats only for the test; it is not gh#263's seat table. Keep the probe
until that real table lands.

Stdlib-first holds: `SO_PEERCRED` and `syscall.GetsockoptUcred` come from the standard library, and are not part of this probe anyway.

### P3.2 The one thing that must not be faked

**A nested headless backend is not a substitute for the real question.** P4.2 runs the compositor under `WLR_BACKEND=wayland` **nested inside a real Niri session**, because the probe needs a working seat and compositor underneath it to observe routing. A headless backend with no parent compositor cannot exercise attachment the way the real path does.

---

## P4. Minimal implementation steps

1. **Resolve and pin the binding.** Identify the `go.wlroots.dev/wlroots` release over upstream wlroots 0.20. Pin it. Record the version and reason. **If no such release exists, stop here** and report it — that is itself the answer to D8.6, and it is the outcome most likely to invalidate gh#262's Scope L estimate.
2. **Create the module and the cgo preamble.** `go.mod`, `#cgo` directives against the pinned wlroots 0.20, and the listener trampolines. No business logic in the preamble.
3. **Write `TestSeatAttachment` with all three rows.** It fails: the package has no seats and no device.
4. **Lock the OS thread and bring up a nested backend.** `runtime.LockOSThread()` before the first wlroots call. `WLR_BACKEND=wayland` against the live session (`NIRI_SOCKET=$(ls /run/user/1000/niri.wayland-*.sock | head -1)`, `WAYLAND_DISPLAY=wayland-1`, `XDG_RUNTIME_DIR=/run/user/1000`).
5. **Create two seats.** `user` (with keyboard and pointer capabilities) and `agent`. Record whether `agent` requires explicit capabilities to accept input — P4.2 answers this empirically rather than from the documentation.
6. **Create a virtual keyboard and attach it to `agent`.** This is the line under test. If the API offers no seat parameter, record that as `forced_to_seat0` and move on.
7. **Inject one key. Observe which seat received it.** Log, do not assert-and-hope. One row of P2's table becomes the true row.
8. **Write the result into `internal/server`'s package doc** so the answer is in the tree, not only in a commit message.

---

## P5. Commands, and the result each outcome means

Run per the compositor's `AGENTS.md` — **never `go test ./...`, never `-race`:**

```
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/server -run TestSeatAttachment
```

| Outcome | What it means for the tranche |
|---|---|
| `attach_to_named_seat` | **gh#262 and gh#263 are buildable as designed.** The isolation rules in D2.4 stay at the seat-table lookup where the design puts them. Proceed to a plan for gh#263 *only after* the gh#262 consumer decision (D8.2). |
| `forced_to_seat0` | **Buildable, but the design moves.** Rules 1 and 2 relocate from the seat-table lookup to a hook above wlroots' dispatch. That is more code and a per-event cost — the cost D1.1.3 already attributes to per-seat cursor suppression. Re-estimate gh#262's Scope before planning it. |
| `unsupported` | **gh#262 and gh#263 cannot be built on this substrate.** Both become parity exclusions, or the charter's wlroots pin is revisited. Record it either way — this is the outcome that makes the cost of finding out worth this plan. |

These are the planned outcomes of P2's virtual-device test. Commit `746afa6` did not implement that
test, so none of these rows has been reached. **Retired 2026-10-15:** `forced_to_seat0` and
`unsupported` describe behaviour the 0.20 API has no place for (see *Resolution*).

**No performance budget is set here.** The orchestration rule is *"record measurements before setting hard budgets"*, and a probe produces measurements, not budgets.

---

## P6. Commit boundary

**One commit.** The probe is an experiment, not a feature; splitting it would make the answer harder to find. The commit message carries the resolved pin, the version and the reason, and the outcome row.

`gofmt -w .`, `go vet ./...`, and `git diff --exit-code -- go.mod go.sum` before the commit. **The shell's `-race` rule does not apply** — the compositor's `AGENTS.md` forbids it, and that rule outranks the shell's once the file lives in the other repository.

---

## P7. Non-goals

- **No seat table, no control socket, no `Stop`, no `internal/wm` change.** The probe answers one question. The probe *code* stays as evidence — see the closing note — but it is not a seat table and nothing else was built on top of it.
- **No shell work of any kind.** sysc-shell is a client and stays one; the compositor's `AGENTS.md` says so and this plan obeys it.
- **No GH issue transfer.** Already done, and done as a separate action: gh#262 and gh#263 were transferred to this repository on 2026-10-15, as #1 and #2, with their bodies corrected first so they did not arrive asserting this repository did not exist. This plan neither performed nor authorised that.
- **No bd closure.** The seven Tier 3 issues stay open. This plan produces evidence, not a decision.
- **No speculative infrastructure on any branch.** A probe that leaves a seat table and a half-built socket behind is the trap the handover names. It did not: the probe's seats are never stored, so display teardown reclaims them and the probe cannot become owner of compositor state.

---

## Evidence from commit `746afa6`, reviewed 2026-10-15

`TestSeatTable` in `internal/wlr/seat_probe_test.go` creates three distinct `wl_seat` objects,
assigns different capability masks, reads them back, and clears two while checking the first remains
unchanged. It does not create a virtual keyboard, attach a device to a named seat, inject a key, or
observe which seat receives input.

On the laptop testbench, an archive of `b675926` passed the named test with
`timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run TestSeatTable`, against wlroots
0.20.2 with Niri 26.04 live. This repeats the seat-object result on the second machine; it does not
change the coverage of the test.

| Planned outcome | Reached? | Evidence |
|---|---|---|
| `attach_to_named_seat` | **No** | No virtual device was attached or injected. |
| `forced_to_seat0` | **No** | Unexercised; no result can be inferred. |
| `unsupported` | **No** | Unexercised; no result can be inferred. |

The test therefore answers only whether wlroots can hold distinct seats with independent capability
state. It does not establish per-seat virtual-device routing or clear the corresponding D8.6 gate.

## Architecture review of commit `746afa6`

- **cgo boundary: pass.** wlroots calls and types stay in `internal/wlr` Go files. The test result is
  plain Go data, and no `.c` or `.h` source was added. `newNested` locks the OS thread before creating
  the compositor.
- **Dynamic linkage: pass.** On the laptop, a build from a temporary archive of `b675926` completed;
  `ldd` showed `libwlroots-0.20.so` and `libwayland-server.so.0`. `pkg-config` reports wlroots 0.20.2
  and link flags `-lwlroots-0.20 -lwayland-server -lm`.
- **Repository/package boundary: pass.** The separate `github.com/Nomadcxx/sysc-compositor` module
  owns this code under `internal/wlr`; it does not move server or compositor code into sysc-wayland
  or sysc-shell.
- **Evidence correction: required.** `TestSeatTable` passed on the laptop under live Niri 26.04, but
  it tests only independent seats and capability masks. `attach_to_named_seat`, `forced_to_seat0`, and
  `unsupported` were not exercised. In particular, `forced_to_seat0` has no observed result.
- **P3.1 removal condition: not met.** `Compositor` still has one production seat. `ProbeSeats` is a
  temporary test probe, not gh#263's real seat table; retain it until that table lands.

This review covers architecture and evidence only. It does not approve agent-seat or virtual-device
implementation; those remain owner-gated.

**Found while writing it, and not in the plan:**

- **cgo does not work in `_test.go` files.** The first attempt put the cgo in the test and failed to
  build. The probe therefore lives in `internal/wlr/seat_probe.go` and the test asserts on plain Go
  data. The plan's P3 file list assumed otherwise; the split is better than what was planned, because
  the exported result type carries no cgo type at all.
- **`extra` has no default, deliberately.** A probe that picks its own seat count answers a question
  nobody asked. The test supplies it.

**Two things the probe does not decide, recorded so they are not assumed:**

- **Pointer routing.** There is no `wlr_seat_set_pointer` in 0.20. A per-seat cursor is compositor
  code — `wlr_cursor_attach_input_device` plus `wlr_seat_pointer_notify_*` — not a wlroots call.
  Keyboard attachment *is* per seat.
- **Virtual devices.** `wlr_keyboard_create` / `wlr_pointer_create` are internal in 0.20. A virtual
  device needs the v1 protocol XML, which the wlroots package does not ship. That is a separate
  probe and it was deliberately not written here.

The probe is test-only evidence, not a production seat table. Its P3.1 removal condition remains
future until gh#263's real seat table lands. D8.6 records the seat-object result; virtual-device
attachment remains open.

## Resolution — answered by API inspection, 2026-10-15

Source: the Tier 3 ownership audit (`/home/nomadx/sysc-shell/docs/plans/2026-10-15-tier3-ownership-audit-report.md`,
finding T3-1), read against the installed headers of `wlroots0.20 0.20.2-1`.

- `wlr/interfaces/wlr_keyboard.h`:20 `wlr_keyboard_init` and `wlr/interfaces/wlr_pointer.h`:18
  `wlr_pointer_init` are public under `WLR_USE_UNSTABLE`. A compositor creates in-process devices
  without any protocol, which is what the control-socket design (session design D3.4) needs.
- `struct wlr_virtual_keyboard_v1` carries the client-requested `seat`, and
  `wlr_virtual_pointer_v1_new_pointer_event` carries `suggested_seat`. wlroots emits `new_virtual_*`
  and attaches nothing. The compositor's handler chooses the seat.
- Protocol XML is needed only for an external injector client. It is not in wayland-protocols 1.49
  (bd `sysc-1242`).

So `forced_to_seat0` and `unsupported` cannot occur in the library, and `attach_to_named_seat` is a
property of this repository's own handler. **This plan is retired without being executed.** Its one
useful test — a device on seat `agent` delivers to the agent seat and `seat0`'s client sees nothing —
becomes the first failing test of gh#263's TDD plan, written when that milestone is commissioned.
The `ProbeSeats` code from `746afa6` keeps its P3.1 removal condition.
