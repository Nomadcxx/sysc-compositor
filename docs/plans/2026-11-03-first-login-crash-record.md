# First-login compositor crash — record

**Date:** 2026-11-03
**Machine:** archThink (the laptop). The desktop, archPC, never ran this build.
**Bead:** `sysc-2226` (P1, bug)
**Pull request:** [#90 — drop the pointer image commit listener when a named shape replaces it](https://github.com/Nomadcxx/sysc-compositor/pull/90)
**Branch:** `fix/cursor-shape-listener-20261103`
**Commit:** `5dff6d0 fix(wlr): drop the pointer image commit listener with a named shape`

## Symptom

The compositor aborted and took the whole desktop session with it. Two aborts in
one boot, on the same code path, about 23 minutes apart:

| Time (archThink clock, UTC) | PID | Note |
|---|---|---|
| 2026-11-03 00:50:37 | 1054 | 19 s into the first login of the boot |
| 2026-11-03 01:14:00 | 2864 | second login; the session wrapper restarted it 9 s later |

The crash looked random because it needs a client that asks for a named pointer
shape and then destroys its own pointer surface. Whether a given client does
that, and when, decides whether the session survives.

## Evidence

`journalctl --user -b -u sysc-compositor.service` on archThink, boot 0:

```
Nov 03 00:50:37 archThink sysc-compositor[1054]: sysc-compositor: types/wlr_compositor.c:733: surface_handle_resource_destroy: Assertion `wl_list_empty(&surface->events.commit.listener_list)' failed.
Nov 03 00:50:37 archThink sysc-compositor[1054]: SIGABRT: abort
Nov 03 00:50:37 archThink sysc-compositor[1054]: signal arrived during cgo execution
Nov 03 00:50:37 archThink sysc-compositor[1054]: github.com/Nomadcxx/sysc-compositor/internal/wlr._Cfunc_wl_display_run(0xd3af880)
Nov 03 00:50:37 archThink sysc-compositor[1054]:         /home/nomadx/.cache/sysc-compositor-gate-20261102/internal/wlr/compositor.go:814
Nov 03 00:50:37 archThink sysc-compositor[1054]: github.com/Nomadcxx/sysc-compositor/internal/server.(*Server).Run(0x1646bed30000)
Nov 03 00:50:37 archThink sysc-compositor[1054]:         /home/nomadx/.cache/sysc-compositor-gate-20261102/internal/server/server.go:493
Nov 03 00:50:37 archThink sysc-compositor[1054]:         /home/nomadx/.cache/sysc-compositor-gate-20261102/internal/cli/cli.go:191
```

The second abort (PID 2864, 01:14:00) is byte-for-byte the same assertion and the
same stack. The assert fires inside wlroots, on the compositor thread, while
`wl_display_run` is the only thing running.

Two other failures in the same window are separate, already-resolved problems and
must not be confused with this one:

- Boot −1 (00:41:42 → 00:49:04) never started the compositor at all. The
  `sysc-lock-session.service` unit still carried `BindsTo=niri.service`, so
  starting sysc-compositor pulled Niri onto the same seat and libseat refused:
  `Could not take control of session: Device or resource busy`. That was fixed
  on the laptop by dropping the two stale dependency lines.
- `/var/tmp/sysc-first-login-crash-20261103.log`, cited by the bead description,
  does not exist on archThink. The journal is the evidence.

### No other crash signature

Across every boot in archThink's journal there are exactly two occurrences of
this assertion, the two aborts above, and no other sysc-compositor abort,
segfault or panic. The only other failure-looking entries in the unit's history
are the two seat failures listed above (the stale `BindsTo=niri.service` unit on
2026-11-02 13:25:47 and 2026-11-03 00:41:56). An xwayland-satellite panic at
2026-10-05 04:44:48 is not a compositor crash: niri logged "quitting after
confirming exit dialog" in the same second.

## Root cause

`internal/wlr/input.go:1559` `handleRequestSetCursor` gives the focused client
two listeners on the surface it draws the pointer with:

- a commit listener, `c.cursorSurfaceCommit` (`input.go:1573`), and
- a destroy listener, `c.cursorSurfaceDestroy` (`input.go:1591`).

The destroy listener is the one that tears the commit listener down again.

`internal/wlr/cursor_shape.go:37` `handleRequestSetShape` handles
`wp_cursor_shape_manager_v1.set_shape`, the request a client makes when it wants
a named theme pointer instead of its own image. It replaced the client's surface
but dropped **only** the destroy listener and left the commit listener attached to
the old surface, then cleared `c.cursorSurface` and `c.cursorClientSet`.

The handler that would have removed the commit listener had itself just been
removed. When the client later destroyed that surface, wlroots checked the
surface's commit listener list, found the listener still on it, and aborted:

```
types/wlr_compositor.c:733: surface_handle_resource_destroy:
Assertion `wl_list_empty(&surface->events.commit.listener_list)' failed.
```

### Audit of every other commit listener

Every other `events.commit` listener in `internal/wlr` pairs the commit listener
with a destroy listener that drops the pair together:

| Site | Commit listener | Released by |
|---|---|---|
| `xdg.go` popup | popup commit | popup destroy |
| `layer.go` | layer commit | layer destroy |
| `overview.go` | `dropOverviewSurfaceListeners` | overview surface destroy |
| `background_effect.go` | background commit | background destroy |
| `window_capture.go` | capture commit | capture destroy |
| `input.go` toplevel | `initialCommit` + view listeners | `handleToplevelDestroy` → `releaseView` (`input.go:1738`) |
| `input.go` pointer surface | `cursorSurfaceCommit` | `cursorSurfaceDestroy` |

The named-shape path was the only leak.

## The fix

`internal/wlr/cursor_shape.go`, in `handleRequestSetShape`, directly after the
`sysc_cursor_shape_request_valid` guard and before the existing destroy drop:

```go
c.drop(c.cursorSurfaceCommit)
c.cursorSurfaceCommit = nil
```

## The regression test

`internal/wlr/cursor_shape_test.go`, `TestCursorShapeDropsPreviousSurfaceListeners`:

- a focused client that asks for the `text` shape leaves `cursorSurfaceCommit`
  and `cursorSurfaceDestroy` nil, `cursorClientSet` false, `cursorShape == "text"`,
  and exactly one unrelated listener on the compositor;
- an unfocused client changes nothing.

`internal/wlr/fakepointer.go` gained `requestCursorShapeForTest`, which builds a
calloc'd `wlr_seat` and `wlr_seat_client`, sets
`seat->pointer_state.focused_client` when the case needs focus, synthesises a
`wlr_cursor_shape_manager_v1_request_set_shape_event` and calls
`handleRequestSetShape`. It needs no backend, no Wayland connection and opens no
window, so it runs without `SYSC_NESTED_TESTS`.

## Verification

```
CGO_CFLAGS_ALLOW=-fno-strict-overflow GOTMPDIR=... TMPDIR=... \
  timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr \
  -run TestCursorShapeDropsPreviousSurfaceListeners -v
```

- with the fix: `--- PASS`
- with the fix reverted and the test kept: `--- FAIL` — "named cursor shape
  retained the previous cursor surface listeners"

The same patch was applied to the source that actually ships on the laptop
(`feat/sysc-input-20261102`, the input-gate bundle) and the test passed there
too. `gofmt`, `git diff --check` and `go vet -p 2 ./...` are clean. CI lane 1
(`check`) is green on #90; lane 2 (`compositor`) runs the new test under vkms.

## Laptop rollout state

The binary installed on archThink came from the input-gate bundle
(`~/.cache/sysc-compositor-gate-20261102`, a plain copy of the input branch,
sha256 `134be07c413771c0379d00f1e7a74a50c7591937f8d584399c9a416d5c9bbd0b`),
so it carries the native pointer settings. A plain `main` build would have
regressed handedness, scroll factor and tap-to-click.

The fixed build was produced from that same bundle source plus the two-line fix:

| Artifact | Value |
|---|---|
| Fixed binary | `~/.cache/sysc-compositor-gate-20261102/bin/sysc-compositor` |
| sha256 | `f6049c27289940922b797d96b76277f72fce1a7142d0b0e727c3b9f77528e200` |
| Previous build kept at | `~/.cache/sysc-compositor-gate-20261102/bin/sysc-compositor.pre-fix` |
| Installer | `~/.cache/sysc-compositor-gate-20261102/install-fixed-compositor.sh` |

The installer needs the owner's `sudo` password, so it was staged, not run. It
writes the new file under a temporary name and renames it over
`/usr/local/bin/sysc-compositor`, which keeps the running compositor alive: the
live session keeps the old binary and the fix takes effect at the next login.
The installer keeps the previous binary in `before-crashfix-install/` with a
`restore.sh` next to it.

## What is still open

- Merge #90 to `main` once lane 2 is green, then merge `main` into
  `feat/sysc-input-20261102` so the draft input PR carries the fix.
- Run the installer on archThink and log in again to confirm the session survives.
- Close `sysc-2226` after that gate passes.
- `sysc-2227` is a separate gap: the compositor skips touch devices and never
  advertises `WL_SEAT_CAPABILITY_TOUCH`, so the touchscreen does not work at all.
- The physical touchpad gate and the packaging gate still hold PR #89 and the
  shell PR #612 in draft.
