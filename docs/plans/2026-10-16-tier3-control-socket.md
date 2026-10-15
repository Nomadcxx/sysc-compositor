# Tier 3: control socket, commands and Stop (gh#1 / sysc-1208) — TDD plan

**Kind:** plan, Tier 3 item and not a milestone.
**Design:** `2026-10-15-tier3-session-domain-design.md` D3 (agent seat and Stop) and D5 (the second envelope).
**Builds on:** `2026-10-15-tier3-multi-seat.md`, merged in `d7ced6a`.

**Gates met:**
- Owner answers D8.2, D8.4 and D8.5, 2026-10-15.
- The multi-seat model is merged.
- The Tier 3 lead (bd `sysc-1253`) has standing approval to execute.

## P0. Scope

This plan builds:
- one session-local control socket, `0600` and peer-verified;
- a small JSON-lines command set: read commands, input commands and stop;
- a Stop enforced at the single command dispatcher;
- window targets by id, pid and app-id;
- facts pushed on the same socket for sysc-shell's presence display (`sysc-1193`, D3.6).

It does **not** build:
- a spawn command or tracking of the windows the agent opens: targets are any window (P1.4);
- a second seat;
- lock or blank handling (`sysc-1260`);
- any shell work;
- a Cua wire-protocol port.

## P1. Decisions fixed here

1. **One thread for wlroots.** The socket, framing and dispatcher state are plain Go in a new
   `internal/control` package, using `net.UnixListener` with one goroutine per connection. Every effect that
   touches wlroots is sent to the compositor thread through `(*wlr.Compositor).Post(func())`. `Post`
   writes to an `eventfd` watched by `wl_event_loop_add_fd`, so it needs one fd-source trampoline in
   `internal/wlr`. Replies come back on per-request channels. No wlroots call happens off the compositor
   thread.
2. **Same-user is an identity check, not an authorisation (D3.2).**
   - The socket is `$XDG_RUNTIME_DIR/<display-socket>-control.sock`, mode `0600`, created with
     umask-safe `chmod` and then checked.
   - Every connection's `SO_PEERCRED` uid must equal the compositor's uid, or the connection is closed
     before any byte is read.
   - Nothing reachable over it is a credential, a session or an unlock.
3. **Stop: anyone may set it; nothing on the socket can lift it.**
   - The shell and the agent are the same uid, so the socket cannot tell them apart. Stop is therefore
     always accepted, from any connection.
   - Once stopped, every mutating command answers `stopped-by-user`, while read commands keep answering
     (D3.5). Stop also releases held keys and buttons on the injected seat.
   - Stop holds until the compositor restarts, or until a user-seat-only re-arm action (a KDL bind) is
     added with config in M1. That later action is recorded as follow-up, not built.
   - pleamar's one-minute lapse is rejected for the reason recorded in D3.5.
4. **Targets are any window.** pleamar targets `pid:N`, `root:N` or an app-id for any window. Input
   isolation is already structural (the multi-seat plan), so restricting targets to windows the agent
   opened would remove the point of computer use without adding a boundary. Restricting is a one-line
   policy change in the target resolver if the owner wants it later.
5. **One envelope, its own vocabulary (D5).** Errors are `stopped-by-user`, `unknown-target`, `unknown-verb`,
   `bad-request`, `not-attached` and `busy`. They are not the shell's 13 Tier 1 codes and never mix with
   them.
6. **One driving connection.** The first connection that sends `attach` owns the injected seat (which is
   created on attach and removed on detach or disconnect). A second `attach` answers `busy`. Connections
   that only send `subscribe` and `stop`, such as the shell, need no attach.
7. **Facts are indexed per pointer from the start** (D3.4.1):
   - `agent.active`, `agent.name`, `agent.stopped`;
   - `agent.0.x` / `y` / `down`;
   - `agent.target`, the window id.

   They are pushed to subscribers on change, coalesced to the latest value, and never block the
   compositor thread. A slow subscriber loses intermediate values, never the latest.

## P2. Wire format

Requests and replies are one JSON object per line, with lines capped at 64 KiB:

```json
{"id":1,"verb":"attach","name":"assistant"}
{"id":2,"verb":"windows"}                                  // read
{"id":3,"verb":"point","target":"app-id:foot","x":120,"y":40}  // window-local
{"id":4,"verb":"button","button":"left","pressed":true}
{"id":5,"verb":"key","key":"Return","pressed":true}
{"id":6,"verb":"type","text":"hello"}                     // US-keymap ASCII only
{"id":7,"verb":"stop"}
{"id":8,"verb":"subscribe"}
```

The reply to each is `{"id":N,"ok":true,...}` or `{"id":N,"error":"<code>","message":"..."}`. Facts arrive as
`{"fact":"agent.0.x","value":512}`.

**Read commands:** `windows` (id, app-id, title, pid, geometry) and `state` (all current facts).

**Mutating commands:** `attach`, `detach`, `point`, `button`, `key` and `type`. `stop` is neither: it is always
accepted.

## P3. Tasks (each one writes its failing test first)

Compositor tests follow the repository rules: one package, a named test, `GOMAXPROCS=2`, no `-race`, no
`./...`. The pure-Go `internal/control` package may also run with `-race`, because it links no cgo.

### Task 1 — `Post`: work onto the compositor thread

- **Files:** new `internal/wlr/post.go` (an eventfd, a `wl_event_loop_add_fd` trampoline in the cgo preamble
  and a queue drained on readiness) and `internal/wlr/callbacks.go` (the exported fire function).
- **Test `TestPostRunsOnTheCompositorThread`** (nested):
  - 100 functions posted from 4 goroutines all run, in each goroutine's order, on the thread that called
    `Run`. The check compares a thread id taken through `unix.Gettid` (`golang.org/x/sys` is not a dependency
    yet, so use `syscall.Gettid`).
  - Posting after `Destroy` returns an error and runs nothing.
  - Handles balance.

### Task 2 — Socket, peer check, framing

- **Files:** new `internal/control/socket.go`, `peer.go` and `frame.go`, with tests.
- **Tests:**
  - `TestSocketMode` (mode `0600` and the path rule);
  - `TestPeerDecision`, a table over (peer uid, own uid, cred error) leading to accept or close;
  - `TestFraming` (an oversize line answers `bad-request` and closes; malformed JSON answers `bad-request`
    and keeps the connection; ids are echoed).
  - A real round trip over a socket in `t.TempDir()`.

### Task 3 — Dispatcher and Stop

- **Files:** new `internal/control/dispatch.go`, with an interface `Seat` (`Attach`, `Detach`, `Point`,
  `Button`, `Key`, `Windows`), a fake for tests, and `stop.go`.
- **Test `TestEveryMutatingVerbRefusesAfterStop`.** This is the D3.8 table over the *full* verb table,
  generated from the dispatcher's own registry, so a verb added later without the gate fails the test.
  After `stop`, each mutating verb answers `stopped-by-user` with no call reaching the fake seat; read
  verbs still answer; `stop` is idempotent; no verb lifts it.
- **Test `TestStopReleasesHeldInput`:** a held key and a held button are released on the fake seat when Stop
  arrives.
- **Test `TestSecondAttachIsBusy`**, and detach or disconnect removes the seat.

### Task 4 — Targets and the wlroots seat adapter

- **Files:**
  - new `internal/control/target.go`: parse `id:`, `pid:` and `app-id:`, and resolve against `windows`; an
    unknown target is refused before any effect;
  - `internal/wlr/windows.go`: the window list, with ids stable for a view's lifetime, the client pid from
    `wl_client_get_credentials`, the app-id and the title;
  - `internal/wlr/seat.go`: point at window-local coordinates; text to keycodes for US ASCII, with
    `unsupported` for anything else;
  - `cmd/sysc-compositor/main.go`: start the control server.
- **Test `TestTargetResolution`:** a table, including the ambiguous `app-id` case, where the most recently
  focused window wins and the reply names it.
- **Test `TestControlDrivesAClient`** (nested, weston-terminal):
  - over the real socket: `attach`, `windows`, `point`, `button` and `type "a"`;
  - the protocol log shows key 30 on the injected keyboard only;
  - after `stop`, `type "b"` answers `stopped-by-user` and the log shows no second key.

### Task 5 — Facts

- **Files:** new `internal/control/facts.go` (latest-value coalescing per subscriber).
- **Test `TestFactsCoalesce`:** a slow subscriber gets the latest value and never blocks the publisher.
- **Test `TestFactsFollowInput`:** `point` emits `agent.0.x/y`, `attach` emits `agent.active=true` and its
  name, and `stop` emits `agent.stopped=true`.

## P4. Commit, review, live

- One commit per task.
- Gate on each:
  - `gofmt`;
  - `GOMAXPROCS=2 go vet -p 2 ./...`;
  - each task's named tests plus the M0 and multi-seat sets;
  - `go test -race -count=1 ./internal/control`;
  - `go.mod` changes only if a dependency is justified. None is planned.
- Live run on the laptop: the multi-seat tests plus `TestControlDrivesAClient`, from a throwaway archive.
- PR, merge, close `sysc-1208`.

## P5. Follow-ups recorded, not built

- **A user-only Stop re-arm** (KDL bind) when config lands (M1).
- **sysc-1193:** the shell's presence display subscribes to these facts. That is its plan, in sysc-shell.
- **`sysc-1260`:** inert under lock or blank.
- **A spawn command and windows-the-agent-opened tracking**, if outline or placement needs them.
