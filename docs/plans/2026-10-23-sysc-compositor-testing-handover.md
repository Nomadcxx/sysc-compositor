# sysc-compositor handover: next work and where to test it

**Written:** 2026-10-23, after #42. **This supersedes the 2026-10-23 continuation handover for
planning.** Read that one for the full standing rules and working recipe; this file adds what landed
since, the owner's new answers, and every place the next work can be tested. bd is the status
authority: run `bd` from `/home/nomadx/sysc-shell` only. Re-read `git`, `gh` and `bd` before
relying on any state here.

## Read first

1. `AGENTS.md`, then `docs/README.md`.
2. `docs/plans/2026-10-23-sysc-compositor-continuation-handover.md`: standing rules 1–11 and the
   working recipe still apply, except where this file changes them (rule 1 on the laptop).
3. The M11 plan, `docs/plans/2026-10-22-m11-screenshot-overlay.md`, especially the last three
   execution records (recent windows).
4. `docs/plans/niri-parity-matrix.md`.
5. `bd show sysc-1276` and its comments.

## What landed on 2026-10-23

`origin/main` is `0e69c89`. All merged with both CI lanes green:

| PR | What |
|---|---|
| #38 | The 2026-10-23 continuation handover. |
| #39 | The `recent-windows` section, with Niri's defaults and include merging. Its binds are `bind.Binding`s running internal `mru-*` actions, which no config bind can name. Focus stamps take `debounce-ms`; a key sent to the focused window commits its stamp at once. `FocusWindowPrevious` over IPC and as `focus-window-previous`. |
| #40 | `internal/wm/mru.go`: Niri's `WindowMru`, with a randomized port of Niri's proptest. |
| #41 | The switcher, `internal/wlr/switcher.go`: Niri's `WindowMruUi`. Strip layout in `internal/wm/mru_layout.go`; titles and the scope panel in `internal/ui/mru.go`. |
| #42 | The switcher's view springs to the selection with `window-movement`. |

This PR also carries the 2026-10-20 and 2026-10-22 handovers and the 2026-10-22 review, which sat
uncommitted in the primary checkout. They are history now.

## Owner answers (2026-10-23)

- **Q3:** tiled windows always follow the layout; no hand placement is kept. The default rule
  everywhere: follow Niri's or mango's behaviour. Recorded on bd `sysc-1262`.
- **Q12:** yes. `sysc-1255`'s first failing test also covers the compositor's control socket
  (`$SYSC_SOCKET`). Recorded on bd `sysc-1255`.
- **Go cache:** clearing it is approved. `go clean -cache` fixed local links that failed with
  `EDQUOT` on the nearly full `/home`.
- **Merged worktrees:** removed; `proposal/window-preview-capture-20261018` is kept.
- **Testing permissions:** see the next section. They replace standing rule 1 for the laptop only.

## Where to test

There are four places, from cheapest to most real.

### 1. This desktop (`archPC`), locally

- **What it runs:** the owner's live Niri 26.04 session (`niri --session`), on DP-1 and DP-3.
- **What is free:**
  - pure-package tests;
  - tests in `internal/wlr` that build no compositor;
  - `go vet -p 2 ./...` and builds.
  - Use one named test per command (rule 2).
- **What needs consent:** every test that builds a compositor needs the owner's consent for that
  run, because it opens a window on this desktop. That is unchanged. Without `SYSC_NESTED_TESTS=1`
  such a test refuses to start.
- **Tools:** podman, docker, `qemu-system-x86_64` with `/dev/kvm`, and `/dev/dri/renderD128`.
  The vkms module is not loaded.
- **If a link fails with `EDQUOT`:** run `go clean -cache` (approved), then retry.

### 2. The laptop, over SSH: pre-approved for nested runs

- **Access:** `ssh laptop` (192.168.0.64, port 7777, key auth; set up in `~/.ssh/config`).
- **What it has:**
  - Niri 26.04 running as `nomadx` on seat0/tty1;
  - Go 1.27.2, wlroots 0.20.2, SceneFX 0.5.0;
  - `weston-terminal`, `grim`, `wl-paste` and docker. There is no foot and no podman;
  - qemu with `/dev/kvm`, `/dev/dri/renderD128`, and 720 GB free.
- **Owner's permission (2026-10-23):** named nested tests and nested gates may run on the laptop's
  Niri session without asking each time. The rest of the rules still hold:
  - one named test per command;
  - no loops, scripts or background test jobs;
  - never install the build, and never touch greetd, the host lock or the host
    `graphical-session.target`.
- **How:**
  1. Copy a `git archive` of the branch to a fresh directory on the laptop.
  2. Run `go generate ./internal/wlr`.
  3. Run each test as `WAYLAND_DISPLAY=wayland-1 SYSC_NESTED_TESTS=1 timeout 90s env GOMAXPROCS=2
     go test -count=1 ./internal/wlr -run '^Name$'`. Over SSH, `WAYLAND_DISPLAY=wayland-1` points
     the test at the laptop's Niri session (checked 2026-10-23: `/run/user/1000/wayland-1`).
- **Leftovers:** `~/sysc-compositor-vt-session-20261018` on the laptop is an old archive from an
  earlier session. Leave it unless the owner says otherwise.
- **What it can prove now:**
  - **M11 nested gate (Task 5):**
    - grim and `wl-copy`/`wl-paste` against the nested compositor. Run sysc-clipboard from its
      own repo inside it; do not install it.
    - The screenshot UI, the hotkey overlay, the exit dialog and the switcher, driven by real keys.
      Drive them with `wtype` (installed on the laptop; `ydotool` is there too) inside the nested
      session, or ask for someone at the keyboard.
    - Take a grim shot of each and attach it to the M11 execution record.
  - **M10 gate (bd `sysc-1643`):**
    - sysc-shell frost;
    - a text-input client with an IME;
    - idle blank and unblank;
    - sysc-lock, built from its repo, locking and unlocking the nested compositor;
    - injected-seat rejection while locked.
    - Record the results in the M10 plan.
  - **Visual checks** that CI cannot make: the switcher's look against Niri's on the same laptop.
    Run Niri's own Alt-Tab in the host session for comparison.

### 3. A real `--session` run (bd `sysc-1631`): the laptop's spare TTY, with a caveat

- **The owner approved** a run on a spare laptop TTY, with greetd and the host session untouched.
- **The caveat (found 2026-10-23):** session mode runs `systemctl --user import-environment` and
  owns `graphical-session.target` (`internal/session/session.go`). On the laptop that is the same
  systemd user manager as the running Niri session. A same-user run would:
  - overwrite the host session's `WAYLAND_DISPLAY`, `NIRI_SOCKET` and the rest;
  - start sysc-shell's units against the wrong compositor;
  - stop the host's `graphical-session.target` services when it exits.
  So **do not run it as `nomadx` while the Niri session is logged in.**
- **Also:** from SSH alone there is no seat session on a spare VT. Someone has to log in there, or
  autologin has to be set up.
- **Ways to do it, best first:**
  1. **A VM (see 4).** It gives a real greetd login with no risk to either host. Design §16
     already names a session VM.
  2. **The laptop, while nobody is logged into its graphical session.** Check
     `loginctl list-sessions` over SSH: no `seat0` session may be active for `nomadx`. The owner
     logs in on tty2 and runs a script the agent prepared. Ask the owner for that window.
  3. **A second laptop user for tests.** That needs root, so ask the owner.

### 4. A VM on either machine: proposed, not yet set up

- **What exists:** qemu and KVM, on both machines.
- **What to build:** an Arch VM with `virtio-vga-gl` (or `virtio-gpu` with virgl), seatd or logind,
  greetd, sysc-greet, sysc-shell, and a build of this repo. Inside it the agent has root.
- **What that unlocks:**
  - **The `--session` gate (`sysc-1631`) for real:** login through greetd, `graphical-session.target`,
    D-Bus activation, the portals. This unblocks suite integration (`sysc-1283`).
  - **Lane 2 locally:** `modprobe vkms`, then run `.github/ci/compositor-tests.sh` exactly as CI
    does. That runs every nested test with no window on any desktop. It also makes the flaky
    lane-2 tests (`sysc-1640`, `sysc-1642`) and the quarantine (`sysc-1638`) reproducible outside
    GitHub.
  - **The M13 xwayland-satellite** and **M14 Orca** checks later.
- **Before starting:** the VM touches no host session, but it means downloading an image and using
  disk space. Tell the owner in one line before starting, and put it on the laptop unless told
  otherwise (720 GB free there; the desktop has about 11 GB free).

### Opportunity: a headless test mode (no window anywhere)

- **Why it may work:**
  - `NewWith` creates its backend with `wlr_backend_autocreate`
    (`internal/wlr/compositor.go`), which honours `WLR_BACKENDS`.
  - The tests already have `addHeadlessTestOutput` (`internal/wlr/fakeoutput.go`).
  - Both machines have `/dev/dri/renderD128` for SceneFX's GLES renderer.
- **The proposal:** `newNestedWith` runs on `WLR_BACKENDS=headless` plus
  `WLR_RENDER_DRM_DEVICE=/dev/dri/renderD128`, and adds a 1920×1080 headless output. Then the
  `internal/wlr` and `internal/server` tests would run on the desktop with no window opened.
- **Unknown:**
  - which tests assume the nested backend's output name (`WL-1`), or its host keyboard and pointer;
  - whether SceneFX renders on a render node without a DRM master.
- **To prove it:** first run one compositor test this way on the laptop (pre-approved), then show
  the owner a run that maps no window.
- **Ask the owner before relying on it on the desktop.** The consent rule exists because of
  windows, so a mode that opens none is likely acceptable, but it is the owner's call.

### CI (unchanged)

- **Lane 1:** pure packages and static checks, about 1 minute.
- **Lane 2:** all of `internal/wlr` and `internal/server` on vkms, in 3–20 minutes.
- **Reading lane 2:** read `failed or skipped outside the quarantine` in the log. Also confirm that
  the new tests show `--- PASS`; a fast lane once looked suspicious but had run everything.
- **Lane 1 runs with DejaVu only:** pixel-count bounds in `internal/ui` tests must hold for that
  font, not just the desktop's fonts (#41 needed a fix for this).

## Next work, in order

1. **M11 close-out (plan Task 5).**
   - Run the laptop nested gate above.
   - Tick only proven matrix rows; `recent-windows` stays `[~]` until the gaps below close or are
     accepted.
   - Run the static checks, and confirm `go.mod`/`go.sum` are unchanged.
   - Write the final execution record. The owner closes `sysc-1276`.
2. **Recent-windows gaps.** Each is small, and each is listed in the M11 plan:
   - previews moving with `window-movement` when a window closes or the scope changes (Niri's
     `animate_move_from_with_config`, `animate_scope_filter_change`). Springs, so buildable now;
   - clipping previews to the window geometry and its corner radius;
   - mouse and scroll binds reaching the open switcher. This needs mouse/scroll binds applied at
     all, so check the matrix;
   - opened keys for `focus-column-right-or-first`, `focus-column-or-monitor-right`,
     `focus-window-down-or-column-right` and their left forms, once those actions exist;
   - fading new previews in with `window-open`. Niri's default is easing, so this waits for the
     M15 easing work. Do not fake it with a spring.
3. **The M10 laptop gate** (`sysc-1643`), above.
4. **The `--session` gate** (`sysc-1631`), through the VM.
5. **M12** (`sysc-1277`). Write its plan when it starts, from design §15 and the matrix:
   ScreenCast/PipeWire, Mutter DisplayConfig, the screenshot portal, foreign-toplevel, virtual
   input, gamma, tearing, syncobj, tablet, and window-image capture. Decide whether
   `proposal/window-preview-capture-20261018` becomes its contract.
6. **M13–M15:** `sysc-1278`, `sysc-1279` and `sysc-1280`, as in the 2026-10-23 continuation
   handover.

**Smaller open items** (unchanged; details in the continuation handover):

| Item | bd |
|---|---|
| CI quarantine | `sysc-1638` |
| Flaky lane-2 tests | `sysc-1640`, `sysc-1642` |
| IME keyboard grab | `sysc-1639` |
| `resume-agent-seat` bind | `sysc-1261` |
| ScreenSaver handoff | `sysc-1635` |
| Go rewrite report | `sysc-1646` |
| Assistant worker boundary, now including `$SYSC_SOCKET` | `sysc-1255` |

## Notes for whoever picks this up

- **The commit-msg hook** rejects substrings such as "agent", "bot", "both" and "cursor"
  (rule 9). Check every draft first.
- **Naming files:** never end a Go file name in `_windows.go`; Go builds it for Windows only.
  `recent_windows.go` silently vanished from the build this way, so the config file is
  `internal/config/mru.go`.
- **Re-entrancy in the switcher:** `animateValue` calls `set` once with the first value
  synchronously. A `set` that rebuilds the scene must not run while the scene is being built.
  `syncSwitcher` guards this with `switcher.syncing`.
- **Disk:** `/home` on the desktop is at 96%. Keep build and scratch output small, and prefer the
  laptop for large work such as VM images.
