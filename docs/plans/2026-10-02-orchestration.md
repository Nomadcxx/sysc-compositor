# sysc-compositor orchestration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Open the *child* plan named in the task (L0 or M0), not this file, when executing code. This file is the map.

**Goal:** Replace niri on this machine with sysc-compositor, without rebuilding the sysc desktop above it, including sysc-launch.

**Architecture:** One compositor process (Go policy + cgo to wlroots 0.20, **no `.c`/`.h` in this repo**) speaking niri KDL, niri-ipc JSON, and a `niri` CLI drop-in. sysc-shell / greet / walls / clipboard stay as they are. sysc-launch gets a small PATH resolver so a session with no `niri` binary still spawns apps. The only handwritten C in the desktop we own is gSlapper.

**Tech Stack:** Go 1.26, cgo in `.go` files, wlroots 0.20, wayland-server, niri-ipc 26.4.0 ABI, systemd user session.

---

## Order

```
L0  sysc-launch PATH          ── independent, start now
M0  nested tinywl             ── this repo's first code
M1  required Wayland globals
M2  greeter: KDL + Spawn/Quit + niri CLI drop-in
M3  EventStream → sysc-shell bars
M4  window ids / Focus / Close
M5–M8  niri window model + visuals
M9  --session + live-reload
M10–M15  frost, shot, cast, xwayland, a11y, parity close-out
S0  bump sysc-shell's sysc-launch  ── after L0 is tagged
G0/W0  greet/walls installer copy  ── optional after M2 (drop-in covers function)
```

Dependencies: L0 does not need a compositor. S0 needs L0 tagged. M2 needs M0–M1 plus `cmd/niri`. A session without a `niri` binary needs L0 **and** M2 (`sysc-compositor msg`). Until L0 lands, `niri` as a symlink of `sysc-compositor` is a workaround, not the product.

Product contract: `docs/plans/2026-10-02-sysc-compositor-design.md`. Remaining IPC/config work: `docs/plans/niri-parity-matrix.md`.

---

### Task 1: L0 — sysc-launch compositor PATH

**Files:** execute `docs/plans/2026-10-02-sysc-launch.md` in `/home/nomadx/sysc-launch`.

**Step 1:** Open that plan. Implement it there. Do not copy launch into this repo.

**Step 2:** Confirm

```
timeout 90s env GOMAXPROCS=2 go test -count=1 . -run 'TestCompositorBin|TestActivateWithoutCompositor|TestActivateSpawnsThroughNiri'
```

passes in sysc-launch.

**Step 3:** Stop. Tag/release and S0 (shell `go.mod` bump) only when asked.

---

### Task 2: M0 — nested tinywl

**Files:** execute `docs/plans/2026-10-02-m0-nested-tinywl.md` in this repo.

**Step 1:** Open that plan. First Go/cgo in `sysc-compositor`.

**Step 2:** Exit gate: nested compositor, foot maps, move/resize, quit, no leaked listeners.

---

### Task 3: Do not pre-write M1–M15 plans

**Files:** none until that milestone starts.

When M0 is done, write `docs/plans/YYYY-MM-DD-m1-required-globals.md` from design §15, then execute it. Same for each later M. Fill `niri-parity-matrix.md` instead of expanding the design.

---

### Task 4: After L0 is tagged — S0 shell bump

**Files:**

- Modify: `/home/nomadx/sysc-shell/go.mod` (`github.com/Nomadcxx/sysc-launch`)
- Test: `/home/nomadx/sysc-shell/internal/shell/runningapps_click_test.go` (injected `Run` still expects argv `[0]=="niri"` — do not change that test)

**Step 1:** Bump the module. Run the shell's named launcher tests only.

**Step 2:** Stop.

---

## What not to schedule

- Rewriting sysc-wayland into a server
- Bar, lock, wallpaper, notify, tray inside this repo
- Mango tags / mmsg
- `.c` / `.h` sources in this repo (cgo preamble in `.go` only; gSlapper keeps its C)
- `go test ./...` or `-race`
