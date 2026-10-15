# Tier 3: multi-seat input model (gh#2 / sysc-1211) — TDD plan

**Kind:** plan, Tier 3 item and not a milestone.
**Design:** `2026-10-15-tier3-session-domain-design.md` D2 (seat model and isolation rules) and D8 (owner answers).

**Gates met on 2026-10-15:**
- M0's exit gate merged (PR #5, `b8971c5`).
- Owner answers recorded: D8.2 yes to computer-use agents; D8.4 yes to a non-user seat; D8.5 one agent seat.
- The D8.6 feasibility question was answered by reading the wlroots API.

**Not authorised by this plan:** execution. Executing it needs the owner's implementation commission.

**Consumer:** gh#1 / `sysc-1208`, the agent seat with a compositor-enforced Stop. Its control socket, verbs and
Stop come in a later plan, built on the API this plan adds.

## P0. Scope

This plan builds the seat table, one injected seat with in-process devices, and the isolation rules D2.4.1
and D2.4.2, each proven by a test. It also creates the place where rule D2.4.3 would break.

It does **not** build:
- the control socket;
- verbs, targets or Stop (that is `sysc-1208`);
- presence facts;
- a second agent seat;
- any lock or blank handling.

The tree has no session-lock or idle path yet, so D2.6.1 is a recorded ordering constraint, not a task here.
See P5.

**Package deviation from D2.3.** The design names `internal/server` for the seat table and `internal/wm` for
focus handling. In the tree, every wlroots call and every focus path is in `internal/wlr`, and
`internal/wm` does not exist. The seat table goes in `internal/wlr`, next to the code it routes. Splitting
packages now would mean moving M0 code for no consumer. D2.3 gets a one-line note when this plan lands.

## P1. Design decisions fixed here

1. **The user seat stays `seat0` and is created first, in `New`** (`internal/wlr/compositor.go`). Clients that
   bind one seat take the first-advertised one; sysc-shell does this as of `sysc-1254`, PR #270. So the user
   seat is always advertised first. That is a contract, and P2 Task 1 pins it.
2. **The injected seat is not created at startup.** `(*Compositor).AddInjectedSeat(name string)` creates it
   on demand, and `RemoveInjectedSeat` destroys it. Until gh#1's socket asks for a seat, no client sees an
   empty extra `wl_seat`. This follows D2.1: no seat before a consumer.
3. **Injected devices are in-process.** `wlr_keyboard_init` and `wlr_pointer_init` from `wlr/interfaces/`
   create them, and Go code feeds them. No virtual-keyboard or virtual-pointer protocol global is
   advertised (D8.6, `sysc-1242`).
4. **The injected pointer is never attached to `c.cursor`.** `c.cursor` is the drawn cursor and belongs to the
   user seat. The injected seat has its own logical position (`x, y`), used for hit-testing through
   `wlr_scene_node_at` and for `wlr_seat_pointer_notify_*` on that seat only. This is rule 2's structural half.
5. **Each seat has its own keyboard focus.** Focus changes on the injected seat call
   `wlr_seat_keyboard_notify_enter` on that seat only. They never touch `c.focusedView`, never call
   `wlr_xdg_toplevel_set_activated`, and never touch the user seat's selection. Data-device and
   primary-selection focus in wlroots follow each seat's own keyboard focus. Rule 1 therefore holds when no
   code path from the injected seat reaches the user seat, and Task 3 asserts exactly that.
6. **Rule 3 is structural.** The injected seat has no `seatd` or logind session and no DRM master. The only
   test is a negative check: `AddInjectedSeat` refuses the name `seat0`, and no session-backend call takes
   an injected seat.

## P2. Tasks (each one writes its failing test first)

Tests follow this repository's rules: one package, one named test, and
`timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run <Name>`. Never run `./...` or `-race`. Nested
tests use the existing `newNested` helper (`internal/wlr/compositor_test.go`) and skip without a session.

### Task 1 — Seat table with an ordering contract

- **Files:**
  - new `internal/wlr/seat.go`: the `seatEntry` table, `AddInjectedSeat`, `RemoveInjectedSeat` and lookup by
    device;
  - `internal/wlr/compositor.go`: `seat0` becomes the table's first entry;
  - new `internal/wlr/seat_test.go`.
- **Test `TestSeatTableOrder`** (nested): after `New`, the table has one seat, `seat0`. `AddInjectedSeat("agent")`
  returns a seat whose `wl_global` name is greater than `seat0`'s. `AddInjectedSeat("seat0")` is refused.
  A second `AddInjectedSeat` while one exists is refused (D8.5: one seat). `RemoveInjectedSeat` leaves `seat0`
  and the listener-token balance unchanged.
- **Probe:** retire `ProbeSeats` and `TestSeatTable` (`internal/wlr/seat_probe.go`, `seat_probe_test.go`) in
  this task. Their P3.1 removal condition ("gh#263's real seat table lands") is met here. Move any assertion
  still worth keeping into `TestSeatTableOrder`.

### Task 2 — In-process devices on the injected seat

- **Files:**
  - `internal/wlr/seat.go`: an injected keyboard via `wlr_keyboard_init` with a fixed US keymap; an injected
    pointer via `wlr_pointer_init`; capabilities set on that seat only;
  - `internal/wlr/input.go`: `refreshCapabilities` becomes per seat.
- **Test `TestInjectedDevicesStayOnTheirSeat`** (nested):
  - The injected seat advertises keyboard and pointer.
  - `seat0`'s capabilities are unchanged.
  - The injected pointer is not attached to `c.cursor`: `c.cursor`'s position does not move when the injected
    pointer moves. This needs a Go-visible accessor.
  - Destroying the seat destroys both devices and balances listener tokens.

### Task 3 — Isolation rules D2.4.1 and D2.4.2

- **Files:**
  - `internal/wlr/seat.go`: injected-seat routing for key, motion, button and focus;
  - `internal/wlr/input.go`: a `request_set_cursor` handler on each seat that honours only `seat0`;
  - new `internal/wlr/isolation_test.go`.
- **Test `TestInjectedSeatIsolation`** (nested, with one `weston-terminal` client as in
  `TestXDGInitialConfigureAfterInitialCommit`). Each row is checked through state accessors and the client's
  `WAYLAND_DEBUG` log:

| Row | Action | Asserted |
|---|---|---|
| key reaches only its seat | focus the client on the injected seat, inject one key | the client's `wl_keyboard` created from the injected `wl_seat` receives `.key(`; the keyboard from `seat0` receives none |
| user focus unmoved | the user seat focuses the client, then the injected seat focuses and types | `seat0`'s `keyboard_state.focused_surface`, `c.focusedView` and the toplevel's activated state are unchanged |
| selection unmoved | the injected seat focuses and types | `seat0`'s `selection_source` and `primary_selection_source` are unchanged, and no `wl_data_device.enter`/selection event for `seat0`'s device appears in the log |
| drawn cursor unmoved | the client sends `set_cursor` on the injected seat's `wl_pointer` | the `c.cursor` image is unchanged; the same request on `seat0` is honoured |
| unknown seat | inject to a seat name that does not exist | an error is returned and no event appears in the log (D2.6) |

  A row that cannot be observed on a host without weston-terminal skips; it never passes vacuously.

### Task 4 — Teardown and stop condition

- **Files:** `internal/wlr/seat.go` and `internal/wlr/compositor.go` (`Destroy` removes injected seats before
  the display).
- **Test `TestInjectedSeatTeardown`** (nested): create, use and remove the seat 3 times, then `Destroy`.
  `LiveHandles()` returns to its starting value. With the injected seat present, the injected devices are
  destroyed before `wl_display_destroy`.
- **Stop condition (design D2.9).** If the "key reaches only its seat" row cannot be made to pass with
  in-process devices, stop and record why in the design. Do not move the rule into a hook above dispatch
  without a design change.

## P3. Commit boundaries

One commit per task, each with its test. Gate before each commit:
- `gofmt -w . && test -z "$(gofmt -l .)"`;
- `go vet ./...`;
- the task's named test plus the M0 bounded set;
- `git diff --exit-code -- go.mod go.sum`. No new dependency: everything is wlroots 0.20 and cgo already
  linked.

## P4. Live evidence

On the laptop bench, nested in Niri 26.04, run a small test program that calls `AddInjectedSeat`, focuses a
client and types one line. The line must appear in the client while the user's own typing continues in the
user-focused window. Record the run in the plan's evidence section. Super+drag gestures are not evidence:
the parent Niri intercepts them.

## P5. Ordering constraints recorded, not built

- **D2.6.1, lock and blank:** injected seats must be inert while the session is locked or blank. The tree has
  no lock or idle path. When session-lock or idle lands, its plan must add the injection gate and the shared
  test D2.6.1 names. File a bd issue for this when this plan is executed.
- **sysc-shell:** shell clients that bind the first-advertised seat are correct from `sysc-1254` (PR #270).
  The shell runs on Niri today, so nothing in the shell waits on this plan.
- **Consumer:** gh#1's plan (control socket, `SO_PEERCRED`, verbs, Stop) builds on `AddInjectedSeat` and the
  injection functions. Write it after this plan's Task 4 lands.
