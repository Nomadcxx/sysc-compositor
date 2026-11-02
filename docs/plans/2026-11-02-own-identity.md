# sysc-compositor's own identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users never see sysc-compositor call itself niri. Niri IPC compatibility and the move
from Niri keep working. The siblings stop running `niri`.

**Architecture:** The compositor changes stay small:
- the config lookup moves to sysc-compositor paths;
- a first-run step imports the Niri config once, or writes an embedded default;
- user-visible strings, the socket file name, the session desktop name and the portal config
  change;
- `cmd/niri` is deleted.

Three sibling repositories switch from running `niri msg` to the `NIRI_SOCKET` IPC, before the
compositor change is released.

**Tech Stack:** Go 1.26 with cgo (wlroots 0.20), `kdl-go`, systemd user units, xdg-desktop-portal.

**Spec:** [2026-11-02-own-identity-design.md](2026-11-02-own-identity-design.md)

## Global Constraints

- The niri-ipc 26.4.0 wire format, `NIRI_SOCKET`, `SYSC_SOCKET` and the `Version` reply shape do
  not change.
- License: GPL-3.0-or-later.
- Config lookup:
  1. `-c`;
  2. `$SYSC_COMPOSITOR_CONFIG`;
  3. `$XDG_CONFIG_HOME/sysc-compositor/config.kdl` (or `~/.config/sysc-compositor/config.kdl`);
  4. `/etc/sysc-compositor/config.kdl`.
- `XDG_CURRENT_DESKTOP=sysc`.
- Socket file: `$XDG_RUNTIME_DIR/sysc-compositor.<display>.<pid>.sock`.
- Portal config: `sysc-portals.conf`.
- Never run `go test ./...` or `-race`. Run one package with a named test:
  `timeout 90s env GOMAXPROCS=2 CGO_CFLAGS_ALLOW=-fno-strict-overflow go test -count=1 <pkg> -run <Name>`.
- Nested tests run only on the laptop (`~/lt-m15.sh`-style), one per command.
- Commit messages: screen them with the hook regex from memory, and add no Co-Authored-By.
- Comments that cite Niri's source, test names, and historical plan documents stay as they are.

## Review Focus

1. **The user's sysc-compositor directory already holds an include target** (for example a
   `sysc-shell.kdl` written by sysc-shell before first start). The import must not overwrite it.
   It reports the file and keeps the existing one. The test is in Task 3.
2. **A Niri config that sysc-compositor cannot parse** (an unknown section, or a syntax error in an
   include). The import still copies the files byte for byte, because it scans `include` nodes
   without decoding. Loading then reports the error and runs on defaults, as today. The test is in
   Task 3.
3. **`$XDG_CONFIG_HOME` set to a relative path, or `$HOME` unset.** Lookup and import fall back
   exactly as `Locate` does, and import does nothing when no user path can be formed. The test is
   in Task 2.
4. **A second start after the user deleted the Niri config.** Nothing happens: the
   sysc-compositor config exists. The test is in Task 3.
5. **The greeter instance (`-c /etc/greetd/…`).** It never imports or writes a default, because
   the config was named. The test is in Task 3.

---

### Task 1: License files

**Files:**
- Create: `LICENSE` (copy of `/usr/share/licenses/spdx/GPL-3.0-or-later.txt`), `NOTICE`
- Modify: `README.md`, `docs/plans/2026-10-02-sysc-compositor-design.md` §13 ("License BSD-3-Clause")

- [ ] **Step 1: Write `LICENSE` and `NOTICE`.**
  - `NOTICE` names sysc-compositor as GPL-3.0-or-later.
  - It lists `third_party/godbus` (BSD-2-Clause, with its own LICENSE file) and
    `protocols/*.xml` (MIT, with the notices in each file).
- [ ] **Step 2: Set GPL-3.0-or-later everywhere else.** Replace "BSD-3-Clause" in design §13 with
  GPL-3.0-or-later, and add a "License" line to README.
- [ ] **Step 3: Check.** Run: `grep -rn "BSD-3" README.md docs/plans/2026-10-02-sysc-compositor-design.md`.
  Expected: no output.
- [ ] **Step 4: Commit.** `git commit -m "docs: license sysc-compositor GPL-3.0-or-later"`

### Task 2: Config lookup on sysc-compositor paths

**Files:**
- Modify: `internal/config/config.go:1567-1605` (`Path`, `Locate`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `config.Locate(flag string, getenv func(string) string) Path`: same signature, new paths.
  - `config.UserPath(getenv func(string) string) string`: the user config path, or `""` when
    `$XDG_CONFIG_HOME` is not absolute and `$HOME` is empty.
  - `config.NiriUserPath(getenv func(string) string) string`: the same rule for
    `niri/config.kdl`, used by the import.

- [ ] **Step 1: Write the failing test.** `TestLocateUsesSyscCompositorPaths` is table-driven.
  Each case gives an env map and an optional flag; check `Path{Named, Search}`:
  - flag `x.kdl` → `Named: "x.kdl"`;
  - `SYSC_COMPOSITOR_CONFIG=/a.kdl` → `Named: "/a.kdl"`;
  - `NIRI_CONFIG=/n.kdl` with `HOME=/h` → `Search: ["/h/.config/sysc-compositor/config.kdl", "/etc/sysc-compositor/config.kdl"]`;
  - `XDG_CONFIG_HOME=/x` → first search entry `/x/sysc-compositor/config.kdl`;
  - `XDG_CONFIG_HOME=rel` with `HOME=/h` → first entry `/h/.config/sysc-compositor/config.kdl`;
  - no `HOME` and no XDG → `Search: ["/etc/sysc-compositor/config.kdl"]`, and `UserPath` returns
    `""`.

  Also assert that `NiriUserPath` with `HOME=/h` returns `/h/.config/niri/config.kdl`.
- [ ] **Step 2: Run it to see it fail.**
  Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/config -run TestLocateUsesSyscCompositorPaths`.
  Expected: FAIL (Niri paths).
- [ ] **Step 3: Implement.** `Locate` reads `SYSC_COMPOSITOR_CONFIG`, then `UserPath`, then
  `/etc/sysc-compositor/config.kdl`. Rewrite the doc comments without "niri".
- [ ] **Step 4: Run the test again.** Expected: PASS. Then run the existing `TestLocate*` tests in
  the package (`-run TestLocate`), and update any that pinned Niri paths.
- [ ] **Step 5: Commit.** `git commit -m "feat: read config from sysc-compositor paths"`

### Task 3: First-run import of the Niri config, or the default

**Files:**
- Create: `internal/config/firstrun.go`, `internal/config/default-config.kdl`,
  `internal/config/firstrun_test.go`
- Modify: `internal/config/config.go`, which exposes the include resolution used by
  `expandIncludes:353`, and `internal/cli/cli.go`'s `run` (the call before `LoadWithSources`)

**Interfaces:**
- Consumes: `UserPath` and `NiriUserPath` (Task 2), and `resolveIncludePath` (existing).
- Produces:
  - `//go:embed default-config.kdl` as `var DefaultConfigKDL []byte`;
  - `type FirstRun struct { Imported, Wrote string; Copied, Kept, Missing, Outside []string }`:
    - `Imported` is the Niri file imported;
    - `Wrote` is the target written;
    - `Kept` lists include targets that already existed;
    - `Outside` lists absolute includes;
  - `func PrepareUserConfig(p Path, getenv func(string) string, now time.Time) (FirstRun, error)`,
    a no-op returning a zero `FirstRun` when `p.Named != ""` or `UserPath` is empty or exists.

- [ ] **Step 1: Write the failing tests** in `firstrun_test.go`. They use `t.TempDir()` homes and an
  env map:
  - **`TestFirstRunImportsNiriConfigAndIncludes`.** The Niri directory holds `config.kdl`, which
    includes `sysc-shell.kdl` and `sub/a.kdl`; `sub/a.kdl` includes `b.kdl`. Assertions:
    - all four exist under `.config/sysc-compositor/` at the same relative paths;
    - the target `config.kdl`'s first line is
      `// Imported from <niri path> by sysc-compositor on 2026-11-02.`, and the rest equals the
      original byte for byte;
    - the includes are byte-identical;
    - the Niri files are unchanged;
    - `FirstRun.Imported` is the Niri path.
  - **`TestFirstRunImportsOnlyOnce`.** After one import, delete the Niri config and call again.
    `FirstRun` is zero and the target is unchanged.
  - **`TestFirstRunKeepsExistingIncludeTargets`.** A pre-existing
    `.config/sysc-compositor/sysc-shell.kdl` holding `// shell` stays `// shell`, and it appears in
    `Kept`.
  - **`TestFirstRunCopiesUnparseableNiriConfig`.** A Niri config with `bogus-section {}` and an
    include holding `{{ broken` is still copied; the import does not decode.
  - **`TestFirstRunReportsOutsideAndMissingIncludes`.** For `include "/etc/x.kdl"` and
    `include optional=true "gone.kdl"`, `Outside` is `["/etc/x.kdl"]`, `Missing` is
    `["<dir>/gone.kdl"]`, and nothing is copied for them.
  - **`TestFirstRunWritesDefaultWithoutNiriConfig`.** The target equals `DefaultConfigKDL` and
    `Wrote` is set.
  - **`TestFirstRunSkipsNamedConfigs`.** With `Path{Named: "/etc/greetd/g.kdl"}`, and with
    `SYSC_COMPOSITOR_CONFIG` set, nothing is written.
  - **`TestShippedDefaultConfigIsValid`.** `Parse("default-config.kdl", DefaultConfigKDL)` returns
    no error. `Warnings` is empty, and `UnappliedActions` is a subset of the parity matrix's
    unbuilt actions. Copy that list into the test from the matrix's `binds` row: the part 2
    sizing names and the part 4 names. The text has no `spawn-at-startup` and no case-insensitive
    "niri".
- [ ] **Step 2: Run them to see them fail.**
  Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/config -run 'TestFirstRun|TestShippedDefault'`.
  Expected: FAIL (undefined).
- [ ] **Step 3: Write `default-config.kdl`.**
  - Start from `/home/nomadx/references/niri/resources/default-config.kdl`'s structure. The
    license allows adapting it.
  - Rewrite every comment for sysc-compositor.
  - Remove `spawn-at-startup`.
  - Keep `input { touchpad { tap; natural-scroll } }`. It parses now and applies in tranche 2.
- [ ] **Step 4: Implement `PrepareUserConfig`.**
  - Scan only top-level `include` nodes with the kdl-go document parser, the same parser and
    `resolveIncludePath` that `expandIncludes` uses. Do not use `Parse`.
  - Walk includes depth-first with a visited set and `maxIncludeDepth`.
  - Copy with `os.MkdirAll(dir, 0o755)` and `0o644` files.
  - Write the header and config through a temp file in the same directory, then rename it.
- [ ] **Step 5: Wire it into `cli.run`** before `LoadWithSources`, never in `validateConfig` or
  `msg`:
  - on `Imported`, print `sysc-compositor: imported Niri config from <path> to <target>`;
  - print one line per `Kept`, `Missing` and `Outside` entry;
  - on `Wrote`, print `sysc-compositor: wrote the default config to <target>`;
  - on an error, print `sysc-compositor: config: <err>; using the defaults` and continue.
- [ ] **Step 6: Run the tests again** (Step 2's command). Expected: PASS. Then run
  `./internal/cli -run TestRun` if such a test exists (it is pure, so no nested run).
- [ ] **Step 7: Commit.** `git commit -m "feat: import a Niri config once, else write the default"`

### Task 4: Warn about binds that run `niri`

**Files:**
- Modify: `internal/config/config.go` (the spawn decoding near `:1444`, and `Config` gains
  `NiriCommandBinds []string`), `internal/cli/cli.go` (`validateConfig`), and
  `internal/server/config_reload.go`, which logs reload warnings
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test.** `TestBindsRunningNiriAreReported` parses three binds:
  - `Mod+A { spawn "niri" "msg" "action" "quit"; }`;
  - `Mod+B { spawn-sh "niri msg outputs"; }`;
  - `Mod+C { spawn "foot"; }`.

  Expect `NiriCommandBinds == []string{"Mod+A", "Mod+B"}`. Also cover `spawn-at-startup "niri" …`,
  which is reported as `spawn-at-startup`.
- [ ] **Step 2: Run it to see it fail.**
  Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/config -run TestBindsRunningNiriAreReported`.
  Expected: FAIL.
- [ ] **Step 3: Implement.**
  - A spawn whose first argument's base name is `niri`, or a spawn-sh string whose first word
    is `niri`, appends the bind's key. `spawn-at-startup` and `spawn-sh-at-startup` append their
    node name.
  - `validateConfig` prints
    `sysc-compositor: warning: these run `niri`, which is not part of sysc-compositor; use `sysc-compositor msg`: <list>`.
  - Reload logs the same line.
- [ ] **Step 4: Run the test again.** Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: warn about binds that run niri"`

### Task 5: Socket name and CLI text

**Files:**
- Modify:
  - `internal/server/niri.go:19-25`: rename `niriSocketPath` to `ipcSocketPath`, with format
    `sysc-compositor.%s.%d.sock`; the Niri source citation stays in the comment;
  - `internal/server/server.go:87`: the log prefix `ipc socket`;
  - `internal/cli/cli.go`:
    - `:1-3` and `:81-89`: drop the alias line in `usage`;
    - `:314` and `:735`: `NIRI_SOCKET is not set, are you running this within sysc-compositor?`;
    - `:776-785`: `Running sysc-compositor has a different version from the %s CLI:` and
      `Did you forget to restart sysc-compositor after an update?`
- Delete: `cmd/niri/`
- Test: `internal/server/niri_test.go:463` (`TestNiriSocketPath`, which keeps its name),
  `internal/cli/cli_test.go`

- [ ] **Step 1: Update the tests to expect the new strings and path**, then add
  `TestUserVisibleTextNeverSaysNiri` in `internal/cli/cli_test.go`. It runs `cli.Main` with:
  - no arguments, plus a bad flag (usage);
  - `msg version` with `NIRI_SOCKET` unset;
  - `validate` on a temp config.

  It also reads every file under `../../share/`. It asserts that no output or file contains
  "niri" case-insensitively, apart from the exact tokens `NIRI_SOCKET` and `niri-ipc`.
- [ ] **Step 2: Run them to see them fail.**
  Run: `timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/cli -run 'TestUserVisibleTextNeverSaysNiri'`
  and `./internal/server -run TestNiriSocketPath`. Expected: FAIL.
- [ ] **Step 3: Implement the renames and delete `cmd/niri`.** The share files are Task 6's job;
  this test then fails only on `share/` until Task 6.
- [ ] **Step 4: Run the server and CLI string tests.** Expected: PASS, except the `share/` part of
  `TestUserVisibleTextNeverSaysNiri`.
- [ ] **Step 5: Commit.** `git commit -m "feat: sysc-compositor names its socket and speaks for itself"`

### Task 6: Session, portals and D-Bus identity

**Files:**
- Modify:
  - `internal/session/session.go:43`: `XDG_CURRENT_DESKTOP` becomes `sysc`;
  - `internal/session/screencast.go:97`: title `sysc-compositor Dynamic Cast Target`, app-id
    `sysc-compositor`;
  - `internal/a11y/atspi.go:517-522`: `ToolkitName` and `Name` become `sysc-compositor`;
  - `share/wayland-sessions/sysc-compositor.desktop`: add `Comment=A Wayland compositor written in Go`
    and `DesktopNames=sysc`;
  - `share/systemd/user/sysc-compositor.service`: `Description=sysc-compositor Wayland session`,
    with the comments rewritten;
  - `share/systemd/user/sysc-compositor-shutdown.target` and `share/bin/sysc-compositor-session`:
    comments without "niri";
  - `Makefile`: install `share/xdg-desktop-portal/sysc-portals.conf` and
    `internal/config/default-config.kdl` to `/usr/share/sysc-compositor/default-config.kdl`.
- Create: `share/xdg-desktop-portal/sysc-portals.conf`:
  ```ini
  [preferred]
  default=gtk
  org.freedesktop.impl.portal.ScreenCast=gnome
  org.freedesktop.impl.portal.Screenshot=gnome
  org.freedesktop.impl.portal.Access=gtk
  org.freedesktop.impl.portal.Notification=gtk
  org.freedesktop.impl.portal.Secret=gnome-keyring
  ```
- Test: `internal/session/session_test.go`, `internal/session/screencast_test.go`,
  `internal/server/screencast_test.go`, `internal/a11y/atspi_test.go`, `internal/server/spawn_test.go`

- [ ] **Step 1: Update the tests** for `XDG_CURRENT_DESKTOP=sysc`, the cast title and app-id, and
  the AT-SPI names.
- [ ] **Step 2: Run each to see it fail**, one package and named test per command. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run each test again, plus `TestUserVisibleTextNeverSaysNiri`.** Expected: PASS.
- [ ] **Step 5: Check the portal choice.** Compare the conf with
  `/home/nomadx/references/niri/resources/niri-portals.conf`. ScreenCast and Screenshot must point
  at gnome, because sysc implements the Mutter D-Bus APIs (design §8).
- [ ] **Step 6: Commit.** `git commit -m "feat: sysc session identity and portal config"`

### Task 7: User docs

**Files:**
- Modify: `README.md`, `docs/plans/2026-10-02-sysc-compositor-design.md` (§1, §5 heading, §6,
  §12, and the §4 heading), `docs/README.md` (index rows for the three tranche designs and plans)

- [ ] **Step 1: Rewrite the README's opening.** Describe a Wayland compositor written in Go for
  the sysc desktop, with Niri IPC compatibility. Commands list only `sysc-compositor`.
- [ ] **Step 2: Add "Moving from Niri" to the README.** Cover:
  - the import in D2's words;
  - that `niri msg` scripts work while Niri stays installed, or with `sysc-compositor msg`;
  - the portal config;
  - `XDG_CURRENT_DESKTOP=sysc`;
  - a link to the parity matrix for unbuilt features.
- [ ] **Step 3: Amend the design sections** to D1–D6, marking each amendment "owner decision
  2026-11-02".
- [ ] **Step 4: Check.** Run: `grep -n -i niri README.md`. Expected: only the "Moving from Niri"
  section and IPC compatibility lines.
- [ ] **Step 5: Commit.** `git commit -m "docs: describe sysc-compositor as its own compositor"`

### Task 8: Siblings stop running `niri` (each in its own repository and PR)

These land and release **before** the compositor PR merges.

**sysc-shell:** phase A of `/home/nomadx/sysc-shell/docs/plans/2026-11-02-sysc-compositor-native-support.md`:
- launching apps through IPC;
- theming in the compositor's config directory.

**sysc-launch, `service.go:442`:**
- [ ] **Step 1: Write the failing test.** `TestActivateSpawnsOverIPC` injects a fake `NIRI_SOCKET`
  listener (a unix socket in `t.TempDir()`) and activates an entry, with and without a working
  directory. The listener must receive exactly one line:
  `{"Action":{"Spawn":{"command":["sh","-c","cd \"$1\" && shift && exec \"$@\"","sh","/w","app"]}}}`
  (with a directory) or `{"Action":{"Spawn":{"command":["app"]}}}` (without). No process named
  `niri` runs.
- [ ] **Step 2: Implement.** Replace the `niri msg action spawn` argv with a `spawnIPC(ctx, socket, command)`
  that dials `NIRI_SOCKET`, writes the line and reads one reply (`{"Ok":"Handled"}`). An `Err`
  reply or a dial failure is returned as an error. Keep `ServiceConfig.Run` for the non-IPC path.
- [ ] **Step 3: Run the repository's gates, then commit and open the PR.**

**sysc-greet:**
- [ ] **Step 1: Add the config.** Add `config/sysc-compositor-greeter-config.kdl`, a copy of the
  Niri greeter config with `sysc-compositor msg` in its binds.
- [ ] **Step 2: Add the installer backend.** In `cmd/installer/main.go:258`, add the backend entry
  `{name: "sysc-compositor", label: "sysc-compositor", binaries: []string{"sysc-compositor"}}`.
  It writes `command = "sysc-compositor -c /etc/greetd/sysc-compositor-greeter-config.kdl"`.
- [ ] **Step 3: Read outputs over IPC.** `cmd/sysc-greet/secondary.go:115` `niriCommand` reads over
  `NIRI_SOCKET` (JSON requests) instead of running `niri msg --json`. It keeps the same decoding.
  Write a test with a fake socket replying to `{"Outputs":null}`.
- [ ] **Step 4: Run the repository's gates, then commit and open the PR.**

### Task 9: Gate and records

- [ ] **Step 1: Run the touched tests on the laptop**, one per command. Run the nested
  `TestNavigationBindsDriveTheCompositor` once to show the socket rename does not break IPC tests.
- [ ] **Step 2: Run the session VM gate.** Install the branch build, sysc-shell phase A and
  sysc-launch, and remove the old `/usr/local/bin/niri` alias.
  - Move `~/.config/sysc-compositor` away, if present, and log in through greetd.
  - Expect:
    - `~/.config/sysc-compositor/{config.kdl,sysc-shell.kdl,sysc.kdl}` exist, with the header on
      `config.kdl`;
    - the journal shows the import line;
    - `echo $XDG_CURRENT_DESKTOP` is `sysc`;
    - `ls $XDG_RUNTIME_DIR/sysc-compositor.*.sock` exists;
    - waybar's `niri/workspaces` module still shows workspaces;
    - the sysc-shell launcher starts foot;
    - a theme change in sysc-shell rewrites `~/.config/sysc-compositor/sysc-shell.kdl`.
  - Capture screenshots into `docs/plans/assets/2026-11-02-own-identity/`.
- [ ] **Step 3: Write the execution record** in this plan. Close bd `sysc-1634`. Open the PR and
  merge it on green CI under the standing approval, after the sibling PRs.
