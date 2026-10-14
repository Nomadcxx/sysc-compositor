# M0 nested tinywl

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** A nested `sysc-compositor` that is a Wayland **server** on wlroots 0.20: foot maps, move/resize with the pointer, quit, no leaked listeners.

**Architecture:** One OS thread (`LockOSThread`) runs `wl_display_run`. cgo in `.go` files wraps the wlroots types we call (`internal/wlr`). **No `.c` / `.h` files.** Go owns process lifetime and signals. Nested backend (`WLR_BACKEND=wayland` or wlroots' nested Wayland backend) so this runs inside the current niri session. No KDL, no IPC, no sysc-shell yet.

**Tech Stack:** Go, cgo (`import "C"` only), pkg-config `wlroots-0.20` (this machine: 0.20.2) and `wayland-server` (1.26.0). Link C libraries; do not write C sources. gSlapper’s C is a different repo.

This machine: never `go test ./...`, never `-race`.

---

### Task 1: Module skeleton

**Files:**

- Create: `/home/nomadx/sysc-compositor/go.mod`
- Create: `/home/nomadx/sysc-compositor/.gitignore`

**Step 1: Write go.mod**

```
module github.com/Nomadcxx/sysc-compositor

go 1.26.4
```

**Step 2: .gitignore**

```
/bin/
*.test
*.o
```

**Step 3: Confirm module**

```
cd /home/nomadx/sysc-compositor
go env GOMOD
```

Expected: path ending in `sysc-compositor/go.mod`.

---

### Task 2: cgo wlroots probe that fails until linked

**Files:**

- Create: `/home/nomadx/sysc-compositor/internal/wlr/wlr.go`
- Create: `/home/nomadx/sysc-compositor/internal/wlr/wlr_test.go`

**Step 1: Write the failing test**

```go
package wlr

import "testing"

func TestPkgConfigWlroots(t *testing.T) {
	if ABI != "0.20" {
		t.Fatalf("ABI = %q, want 0.20", ABI)
	}
}
```

**Step 2: Run**

```
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run TestPkgConfigWlroots
```

Expected: FAIL, `ABI` undefined.

**Step 3: Minimal cgo**

`internal/wlr/wlr.go`:

```go
package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
*/
import "C"

const ABI = "0.20"

func init() { _ = C.int(0) }
```

If pkg-config fails at compile, fix the package names (`wlroots-0.20` is installed here). Do not vendor wlroots.

**Step 4: Re-run** — Expected: PASS.

---

### Task 3: Display + nested backend (no clients yet)

**Files:**

- Create: `/home/nomadx/sysc-compositor/cmd/sysc-compositor/main.go`
- Create: `/home/nomadx/sysc-compositor/internal/server/server.go`
- Create: `/home/nomadx/sysc-compositor/internal/wlr/display.go` (cgo preamble in this `.go` file if listeners need a C trampoline; never a sibling `.c`)

**Step 1:** `cmd/sysc-compositor/main.go` locks the thread, creates a server, runs, exits 0 on SIGINT/SIGTERM.

Sketch:

```go
package main

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/Nomadcxx/sysc-compositor/internal/server"
)

func main() {
	runtime.LockOSThread()
	s, err := server.New()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	defer s.Destroy()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		s.Terminate()
	}()
	if err := s.Run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
```

`server.New` (compositor thread only): `wlr_backend_autocreate` (nested when already on Wayland), renderer, allocator, compositor, output layout, xdg-shell, seat, cursor. Add listeners with `cgo.Handle`; store the Handle so `Destroy` / output destroy calls `wl_list_remove` then `handle.Delete()`.

Print `WAYLAND_DISPLAY` (the **nested** socket name) to stderr so a smoke script can spawn foot against it.

**Step 2: Build**

```
cd /home/nomadx/sysc-compositor
go build -o bin/sysc-compositor ./cmd/sysc-compositor
```

Expected: binary exists.

**Step 3: Nested smoke (manual)**

From a terminal already on niri:

```
./bin/sysc-compositor
```

Expected: a nested window appears (wlroots Wayland backend). Ctrl-C exits without abort. `dmesg`/journal not required; if it SIGSEGV, fix listener lifetime before adding clients.

---

### Task 4: xdg-shell map + pointer move/resize + quit

**Files:**

- Modify: `internal/server/` — xdg-shell new_surface / map / unmap / destroy
- Modify: `internal/server/` — seat pointer, cursor, xdg toplevel move/resize
- Bind: Escape or Mod+Shift+E to terminate (tinywl-style; KDL binds are M5)

**Step 1:** Implement the wlroots tinywl loop in Go callbacks. Views are a linked list in Go, not a second scene graph policy. Use `wlr_scene` if it is less code than manual rendering; either is fine for M0.

**Step 2: Smoke**

Nested compositor running, in another terminal:

```
WAYLAND_DISPLAY=<nested> foot
```

Expected: foot maps inside the nested window. Pointer can move and resize the xdg toplevel. Quit key or Ctrl-C destroys all listeners (`cgo.Handle` count returns to zero — if you cannot count Handles, at least ASAN-free: no double-free on quit).

There is no automated DRM test. Optional: a tiny `internal/server/listener_test.go` that documents “Destroy with no outputs must not panic” by constructing a fake-less skip (`t.Skip` if no Wayland display). Do not hang the test on `wl_display_run`.

---

### Task 5: `cmd/niri` stub is not M0

Do not implement `niri msg` here. M2 owns the CLI drop-in. M0 binary name is `sysc-compositor` only.

---

## Exit gate

- [ ] `go build ./cmd/sysc-compositor` works with wlroots 0.20
- [ ] Nested session: foot maps
- [ ] Move / resize with pointer
- [ ] Clean quit, no leaked listeners
- [ ] No KDL, no IPC, no layer-shell yet (M1)
- [ ] No `.c` / `.h` in the tree (`rg --glob '*.c' --glob '*.h' .` empty aside from nothing)

## Machine rules

```
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run TestPkgConfigWlroots
```
