# M9 Session and Live Config Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the M9 Niri 26.04 session startup, `niri msg` request surface, keyboard layout integration, and transactional live config reload.

**Architecture:** Keep config and IPC in their existing Go packages. Return include paths from config loading. Parse reload candidates off the compositor thread, then apply a valid candidate on the locked wlroots thread. Gate environment publication and target lifecycle on `--session`; call systemd utilities with argument arrays. Use godbus for the session-only `locale1` and `login1` clients.

**Tech Stack:** Go, cgo, wlroots 0.20, KDL v1 (`kdl-go`), niri-ipc 26.4.0, Linux inotify via the standard library syscall package, godbus/dbus v5.

## Contract and scope

Issue `sysc-1274` follows design §4, §5, §6, §8, §12, and §15 and the M9 rows in `niri-parity-matrix.md`. Niri 26.04 at `8ed0da44d974c32c6877d2f4630c314da0717ecb` defines behavior. Mango 0.18.0 at `a21b10fcbe6156b76ea96178d1941de5cb36137f` and Hyprland at `8b7aefe32e3459a6e3a92cbd6dd6c638d1b8dbdd` provide implementation references.

M9 covers `--session`, graphical-session environment import and cleanup, all `niri msg` requests, keyboard-layout queries and switching, `ConfigLoaded`, config reload and include watching, `LoadConfigFile`, `switch-events`, `locale1`, and `login1` lid state. For M10–M15 behavior, the CLI sends the request and the server returns `unimplemented: <Name>` until that milestone implements it.

Keep the startup config active after a failed reload. Parse each candidate before application. On failure, emit `ConfigLoaded { failed: true }` and leave the running config untouched. Update include watches from parse attempts so editing or creating an optional include can repair the config. Do not rerun `spawn-at-startup` on reload. Run all wlroots access and config application on the locked compositor thread.

Import session variables after both sockets exist and before startup commands or `graphical-session.target` services run. Cleanup must not wait synchronously for the target to stop while the compositor may belong to it. Nested qualification must not start, stop, or install the host's graphical-session target.

## Tasks

### Task 1: Complete the `niri msg` request surface

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/ipc/wire.go`
- Modify: `internal/server/niri.go`
- Test: `internal/cli/cli_test.go`
- Test: `internal/ipc/wire_test.go`
- Test: `internal/server/niri_test.go`

1. Add CLI forms for all 16 request variants: `version`, `outputs`, `workspaces`, `windows`, `layers`, `keyboard-layouts`, `focused-output`, `focused-window`, `pick-window`, `pick-color`, `action`, `output`, `event-stream`, `request-error`, `overview-state`, and `casts`.
2. Keep request encoding and decoding in `internal/ipc`. Add typed fields only for request payloads the CLI needs to send (`Output` and action arguments); unit queries stay represented by their name.
3. Accept and format every response kind returned by Niri 26.04. `--json` prints the response payload without the outer `Ok`; human output may use the existing indented JSON formatter where no current Niri-specific formatter exists. Preserve the version mismatch and `ReturnError` exit behavior.
4. Parse `action` and `output` forms with Niri's spelling and validation. Keep actions implemented in later milestones as decodable IPC, but return their existing stable unimplemented errors.
5. Check whether Niri 26.04 exposes `--print-request` or `raw-request`; implement only the forms present in the pinned source.
6. Run the named CLI, IPC, and server tests. Do not use `go test ./...` or `-race`.

### Task 2: Expose include dependencies from config loading

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

1. Add a load result containing the parsed `*Config` and the absolute root/include paths encountered. Keep `Load(path)` as the compatibility wrapper used by existing callers.
2. Collect include paths before attempting reads, including missing optional includes and paths whose parse fails. Preserve the existing recursion limit, cycle detection, optional-file behavior, and relative-to-includer resolution.
3. Match Niri 26.04's positional merge rules in `niri-config/src/lib.rs`: mergeable sections combine in order, binds use Niri's first-seen override rules, and `window-rule` remains at its include position. Do not replace the parser or add an include framework.
4. Decode `switch-events` (`lid-open`, `lid-close`, `tablet-mode-on`, and `tablet-mode-off`) as Niri's spawn actions and merge each configured action positionally.
5. Run named config tests for include ordering, optional includes, recursion, switch-event merge, and the laptop acceptance config. Never change the source acceptance files.

### Task 3: Add a config watcher and transactional reload

**Files:**
- Create: `internal/config/watch_linux.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/niri.go`
- Modify: `internal/server/eventstream.go`
- Modify: `internal/wlr/compositor.go`
- Modify: `internal/wlr/keyboard.go`
- Modify: the smallest existing `internal/wlr` files that own reapplying rules, layout options, binds, gestures, animations, and cursor settings
- Test: `internal/config/config_test.go`
- Test: `internal/server/eventstream_test.go`
- Test: focused existing `internal/wlr` tests for reconfiguration

1. Use inotify to watch the root config and every include. Watch parent directories so atomic replacement and creation of missing optional includes are observed. Debounce write bursts and close all descriptors with the server.
2. Parse a changed config away from the compositor loop. Post only its result to the compositor loop. On success, apply supported M2–M8 settings together, update the future-spawn environment, replace the active config and watch set, then emit successful `ConfigLoaded`. Do not repeat startup spawns.
3. On parse or read failure, keep the current compositor settings, update watches from the failed attempt where available, and publish failed `ConfigLoaded`.
4. Route `LoadConfigFile` through the same reload path; an optional path changes the active root only when that file loads successfully.
5. Add one coherent compositor reconfiguration entry point if the existing setters do not cover all M2–M8 settings and switch-event actions. It must run on the compositor thread and reuse existing apply/reflow paths.
6. Run focused named tests for watcher events, failed reload preservation, successful reload application, action reload, and `ConfigLoaded` event delivery. Keep tests nested; do not interact with the host session manager.

### Task 4: Implement keyboard layout query, switching, and D-Bus defaults

**Files:**
- Modify: `internal/ipc/wire.go`
- Modify: `internal/server/niri.go`
- Modify: `internal/server/server.go`
- Modify: `internal/wlr/keyboard.go`
- Create: `internal/session/dbus.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `internal/ipc/wire_test.go`
- Test: `internal/server/niri_test.go`
- Test: focused existing `internal/wlr/keyboard_test.go`
- Test: a small isolated `internal/session` check using a private D-Bus connection or injected property source

1. Serve `KeyboardLayouts` from the existing XKB snapshot and publish layout name/index changes through `KeyboardLayoutsChanged` and `KeyboardLayoutSwitched`.
2. Implement `SwitchLayout` on the compositor thread, using XKB state to select the next or previous configured layout and updating client-visible keyboard state.
3. In session mode, subscribe to `org.freedesktop.locale1` XKB properties and use them only when the KDL keyboard section has no explicit XKB settings. Apply initial and changed properties on the compositor thread.
4. Add a wlroots switch-toggle listener for lid and tablet switches. Route it through Niri's `switch-events` spawn actions; update lid state from both hardware events and `login1`, since Niri uses both sources to avoid missed lid changes.
5. In session mode, subscribe to `org.freedesktop.login1` lid state as Niri does. Sleep suspension remains owned by wlroots' native session events. Treat absent services or properties as a logged, nonfatal session integration failure.
6. Add godbus v5 at the version already used by sysc-shell (`v5.2.2`); do not add a second D-Bus implementation.
7. Run named IPC, server, keyboard, switch-event, and session tests.

### Task 5: Add explicit `--session` activation lifecycle

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/server/server.go`
- Create or modify: `internal/session/session.go`
- Test: `internal/cli/cli_test.go`
- Test: a small isolated `internal/session` command-argument and lifecycle check

1. Parse `--session` alongside `-c`/`--config`; preserve ordinary no-argument nested startup without session side effects.
2. After server socket creation and before `Server.Run`, set and publish `WAYLAND_DISPLAY`, `NIRI_SOCKET`, `SYSC_SOCKET`, `DISPLAY` when present, `XDG_CURRENT_DESKTOP=niri:sysc`, and `XDG_SESSION_TYPE=wayland` to D-Bus activation and the systemd user manager. Use `exec.Command` with explicit arguments; do not construct shell commands.
3. Start `graphical-session.target` only for `--session`. On session shutdown, asynchronously stop the target and clear the published variables so teardown cannot wait on the compositor itself.
4. Ensure locale1/login1 clients are started only for session mode and are closed on shutdown.
5. Run named CLI/session tests with fake command executables. Never start or stop the host target.

### Task 6: Close M9 docs and qualification

**Files:**
- Modify: `docs/plans/niri-parity-matrix.md`
- Modify: `docs/README.md`
- Modify: this plan if implementation reveals a contract correction

1. Tick only M9 parity rows proven by the implementation; leave future-milestone rows open.
2. Run `gofmt`, `git diff --check`, `go vet -p 2 ./...`, and build `cmd/niri` and `cmd/sysc-compositor`.
3. Run the named package checks required by `AGENTS.md`, never `go test ./...` or `-race`. Validate the laptop and greeter configs without installing the compositor, changing greetd, or starting/stopping host graphical-session targets.
4. Confirm `go.mod`/`go.sum` contain only the D-Bus dependency required for the two M9 system-bus clients. Create a stacked M9 PR against M8; do not merge it.

## Execution record (2026-10-22)

PR #26, rebased onto M8 (#25) after the consent guard (#27), `Terminate` (#28), M6 (#22), CLI naming
(#30) and CI (#29) landed on main.

| Commit | What |
|---|---|
| `c01d20a` | session and live config: locale1 and login1 clients, `--session`, config reload |
| `c373d35` | review fixes: `Destroy` is safe before the config watcher starts (`sysc-1627`); the event hub closes before the niri listener, so an open EventStream no longer hangs `Destroy` |

**Rebase resolutions:**
- `--session` joins #30's usage text: `sysc-compositor [--session] [-c CONFIG] …`. The usage test
  was updated to match.
- `normalizeOptions` replaces the inline option defaults, and #29's deferred cleanup in `NewWith` is
  kept.

**Proof:**
- On the developer machine: `TestDestroyStopsActiveEventStream` (builds no compositor),
  `TestM8AnimationOffAndUnsupportedSettings`, `TestUsageLeadsWithSyscCompositorAndNamesTheNiriAlias`,
  `TestFailedStartReleasesTheSocket`, and the session D-Bus tests on a private bus. Static checks
  passed.
- Nested coverage comes from CI lane 2.
- After the rebase, `sysc-compositor validate` accepts both the laptop's `~/.config/niri/config.kdl`
  and sysc-greet's `config/niri-greeter-config.kdl`. It warns about sections and bind actions that
  are still unapplied, all of them later milestones.
