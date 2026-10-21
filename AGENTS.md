# sysc-compositor

Niri-parity Wayland compositor for the sysc desktop. Go + cgo, wlroots 0.20.

Read `docs/README.md` before editing. Product contract: `docs/plans/2026-10-02-sysc-compositor-design.md`.

## Hard rules

- Never `go test ./...`. Never `-race`. One package, named test:

  ```
  timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run <Name>
  ```

- A test compositor nests in the owner's live session and opens a window on their desktop. Tests
  that build one skip unless `SYSC_NESTED_TESTS=1`. Set it only with the owner's explicit consent
  for that run, and never in a loop, a script or a background job.

- `runtime.LockOSThread()` before any wlroots call. Never call wlroots off the compositor thread.
- **Go + cgo only.** No `.c` / `.h` in this tree. Listener trampolines go in the cgo preamble of a `.go` file. gSlapper is C and stays in its own repo.
- After cloning, run `go generate ./internal/wlr` once: it writes the protocol headers wlroots does not
  install (from `protocols/*.xml`) into the gitignored `internal/wlr/protocols/`. Never commit them.
- sysc-wayland is a **client**. Do not turn it into the server.
- Do not implement a bar, lock screen, wallpaper engine, notification server, or tray here.

## Sibling repos

| Change | Repo |
|---|---|
| L0 compositor PATH | `/home/nomadx/sysc-launch` |
| S0 module bump | `/home/nomadx/sysc-shell` |

IPC crate pin: niri-ipc **26.4.0**. Behaviour target: niri **26.04**.
