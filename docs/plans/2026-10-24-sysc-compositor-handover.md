# sysc-compositor handover: after the M11 gate, with the session VM next

**Written:** 2026-10-24. **This supersedes the 2026-10-23 testing handover for planning.** That file
still holds the full "where to test" detail. The 2026-10-23 continuation handover still holds standing
rules 1–11 and the working recipe. bd is the status authority: run `bd` from `/home/nomadx/sysc-shell`
only. Re-read `git`, `gh` and `bd` before relying on any state here.

## Read first

1. `AGENTS.md`, then `docs/README.md`.
2. `docs/plans/2026-10-23-sysc-compositor-continuation-handover.md`: standing rules 1–11 and the
   working recipe.
3. `docs/plans/2026-10-23-sysc-compositor-testing-handover.md`: the four places to test, and the
   `--session` caveat.
4. `docs/plans/2026-10-22-m11-screenshot-overlay.md`, the last two execution records: the laptop
   gate, how to drive a nested gate, and the recent-windows gaps.
5. `docs/plans/niri-parity-matrix.md`.

## State on 2026-10-24

- `origin/main` is `56b0410`. No PRs are open.
- The primary checkout is clean on `main`, apart from an untracked `.commandcode/` that is not ours.
- One worktree remains, `proposal/window-preview-capture-20261018`, kept for the M12 decision.

| PR | What |
|---|---|
| #44 | `sysc-1648`: binds matched the shifted keysym, so `Mod+Shift+Slash`, `Mod+Shift+1..9` and the like never ran. They now match Niri's `raw_latin_sym_or_raw_current_sym`. `TestBindKeysIgnoreShiftAndLayoutAsNiri` needs no compositor. |
| #45 | `sysc-1649`: `validate` no longer calls the `binds` section unapplied. |
| #46 | M11 laptop gate record and shots (`docs/plans/assets/2026-10-23-m11-gate/`). The matrix now has ext-image-copy-capture `[x]` and wlr-screencopy `[~]`, because grim 1.5 uses ext-image-copy-capture. |
| #47 | Switcher previews slide with `window-movement` on removal and on scope or filter changes (Niri's `remove_window` and `animate_scope_filter_change`). `Spring.Offset` added. |
| #48 | Switcher previews are clipped to the window geometry and corner radius (Niri's `Thumbnail::render`). |

**M11 (`sysc-1276`) is the owner's to close.** The `[~]` rows left are:
- `Screenshot`: the size binds and the open animation;
- `recent-windows`: the `window-open` fade-in (M15 easing), mouse/scroll binds while open, and opened
  keys for actions not built yet;
- `zwlr_screencopy_manager_v1`: no client drives it;
- presentation-time, single-pixel-buffer and alpha-modifier: no client tests.

## Next work, in order

### 1. The session VM (testing handover item 4): approved, not started

The owner said "Let's start Item 4" on 2026-10-24, which covers downloading an image and using
laptop disk. It runs on the **laptop** (`ssh laptop`). The desktop's `/home` is nearly full, and the
desktop is often at its lock screen.

**Found on the laptop (2026-10-23):**
- qemu-full 11.1.2, `qemu-hw-display-virtio-gpu-gl`, `qemu-ui-egl-headless`, virglrenderer 1.3;
- edk2-ovmf, and xorriso (from libisoburn) for a cloud-init seed;
- `/dev/kvm` (0666) and `/dev/dri/renderD128`;
- 4 cores, 31 GB RAM (about 17 GB free), 719 GB disk free;
- no `cloud-image-utils`, and no existing VM directory.

**Rules for the VM:**
- **No window on the laptop's desktop.** Use `-display egl-headless,rendernode=/dev/dri/renderD128`,
  never gtk or sdl.
- **Look at the screen** with QMP `screendump`, and **type** with QMP `send-key` or
  `input-send-event`, over a unix socket in the VM directory. VNC, if used, binds to localhost only.
- Keep everything under one directory, for example `~/vm/sysc-session/`, so it is easy to remove.
- Root inside the VM is fine. The host's greetd, lock, `graphical-session.target` and the Niri session
  stay untouched.

**Suggested build:**
1. **Base image.** The Arch cloud image
   (`https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2`, about 0.5 GB)
   as a backing file, with an overlay grown to about 40 GB. A NoCloud seed ISO (volume label `cidata`,
   made with `xorriso`) creates a user with the laptop's SSH key and passwordless sudo. Boot with
   q35, KVM, `-cpu host`, 2 vCPUs, 6 GB, OVMF, virtio disk, and `-nic user,hostfwd=tcp:127.0.0.1:2222-:22`.
2. **Packages.** CI's lane-2 list (`.github/ci/compositor-tests.sh`): `go wlroots0.20 scenefx`,
   mesa, seatd, `weston foot grim wl-clipboard ttf-dejavu` and the rest. Add greetd for the session
   gate. Copy the repo in with `git archive`, then run `go generate ./internal/wlr`.
3. **Lane 2 inside the VM.** `modprobe vkms`, then run the tests with lane 2's environment
   (`.github/workflows/ci.yml`, job `compositor`):
   - `SYSC_NESTED_TESTS=1 WLR_BACKENDS=drm LIBSEAT_BACKEND=noop WLR_RENDERER=gles2`;
   - `WLR_RENDERER_ALLOW_SOFTWARE=1 GOMAXPROCS=2` and a private `XDG_RUNTIME_DIR`.

   **Watch out:** the script sets `WLR_DRM_DEVICES` to the *first* `/dev/dri/card*`. In the VM that is
   the virtio GPU, not vkms. Point it at the vkms card, or run without `virtio-vga-gl` for this lane.

   The aim is the whole of `internal/wlr` and `internal/server` with no window anywhere. That makes
   the flaky lane-2 tests (`sysc-1640`, `sysc-1642`) and the quarantine (`sysc-1638`) reproducible.
   Running the whole package is acceptable *only* inside the VM, which has no desktop. Outside it,
   rule 2 (one named test per command) still holds.
4. **The `--session` gate (`sysc-1631`).** Install greetd in the VM; sysc-greet (`/home/nomadx/sysc-greet`,
   Go, niri variant by default) is the real target, and agreety or tuigreet are acceptable as a
   first step. Install this repo's build and its session entry inside the VM only, then log in
   through greetd. Check the list in `bd show sysc-1631`:
   - `WAYLAND_DISPLAY`, `NIRI_SOCKET`, `SYSC_SOCKET`, `XDG_CURRENT_DESKTOP=niri:sysc` and
     `XDG_SESSION_TYPE` reach `systemd --user` and D-Bus activation;
   - `graphical-session.target` starts `sysc-shell.service` (`/home/nomadx/sysc-shell/packaging/systemd/`)
     and the portals;
   - on exit the target stops and the variables are cleared;
   - locale1 and login1 clients start only in session mode.

   **Open question for the owner:** `share/wayland-sessions/sysc-compositor.desktop` has
   `Exec=/usr/local/bin/sysc-compositor` with no `--session`. M3 chose the bare `Exec` on purpose;
   M9 added `--session` later. Niri's entry runs `niri-session`, which runs `niri --session`. Prove
   the gate with `--session` and ask the owner whether the entry should change. Don't change it
   unasked.
5. **Record it.** Write a short plan or record for the VM in `docs/plans/` and register it in
   `docs/README.md`. Results for `sysc-1631` go in bd. A pass unblocks suite integration (`sysc-1283`).
6. **Later uses:** M13 xwayland-satellite and M14 Orca checks.

### 2. The M10 laptop gate (`sysc-1643`): mostly runnable now, one step needs the owner

- Follow "How it was run" in the M11 gate record, and memory `nested-gate-driving`:
  - drive keys with `ydotoold -m` on a private socket, not wtype;
  - check before every key that the host's focused window is the nested compositor;
  - bind the switcher to Ctrl, not Alt, because the host Niri takes Alt+Tab.
- **Without the owner:** sysc-shell frost, a text-input client with an IME, idle blank and unblank,
  and injected-seat rejection while locked.
- **With the owner:** unlocking sysc-lock needs the owner's password typed at the nested lock
  screen. Lock it, show it, and ask for that step. Never type or store the password yourself.
- Record the results in `docs/plans/2026-10-22-m10-input-idle.md`.

### 3. M12 (`sysc-1277`)

Write its plan from design §15 and the matrix when it starts:
- ScreenCast/PipeWire, Mutter DisplayConfig and the screenshot portal;
- foreign-toplevel, virtual input, gamma, tearing, syncobj, tablet, and window-image capture.

Decide whether `proposal/window-preview-capture-20261018` becomes its contract. Virtual-keyboard also
unblocks the IME keyboard grab (`sysc-1639`).

### 4. M13–M15

`sysc-1278`, `sysc-1279` and `sysc-1280`, as in the continuation handover. M15 easing unblocks:
- the `screenshot-ui-open` animation;
- the switcher's `window-open` fade-in;
- Niri's default window open/close.

Mouse and scroll binds are not applied at all; the `binds` row warns about them. Applying them also
closes the switcher's mouse/scroll gap.

## Open bd items

| Item | bd |
|---|---|
| sysc-wayland dials `XDG_RUNTIME_DIR` joined to an absolute `WAYLAND_DISPLAY` (sibling repo; found by the M11 gate) | `sysc-1650` |
| CI quarantine | `sysc-1638` |
| Flaky lane-2 tests | `sysc-1640`, `sysc-1642` |
| IME keyboard grab | `sysc-1639` |
| `resume-agent-seat` bind | `sysc-1261` |
| ScreenSaver handoff | `sysc-1635` |
| Go rewrite report | `sysc-1646` |
| Assistant worker boundary, including `$SYSC_SOCKET` | `sysc-1255` |

## Notes for whoever picks this up

- **The desktop.** The owner may be away from it, with it at its lock screen. Run every nested test on
  the laptop (pre-approved, one named test per command) or in CI. Never set `SYSC_NESTED_TESTS=1` on
  the desktop without consent for that run.
- **`/tmp` is an 8 GB tmpfs with a per-user quota.** When it fills, every command's output is lost
  (EDQUOT). Check `du -sh /tmp/claude-1000/*`. On 2026-10-23 an old session's 3.6 GB `opencode.db`
  copy filled it; the owner approved deleting it. Ask before deleting another session's files.
  `go clean -cache` is approved for EDQUOT on `/home`.
- **Laptop leftovers**, all from builds that are now merged:
  - `~/sysc-compositor-m11-gate-20261023`
  - `~/sysc-compositor-sysc-1648-gate-20261023`
  - `~/sysc-compositor-switcher-moves-20261023`
  - the older `~/sysc-compositor-vt-session-20261018`

  Ask the owner before removing them.
- **First nested run on the laptop** compiles `internal/wlr` from scratch, which takes longer than 90
  seconds. Build first (`go test -c` or `go vet`), then run each test with the standard `timeout 90s`.
- **To prove a test fails first**, change the laptop copy only (for example with `sed`), run it, then
  copy the real file back. Don't juggle `git stash` in the worktree.
- **Test clients worth knowing:**
  - weston-terminal draws a 32px client-side shadow, which exercises geometry clipping;
  - weston-flower sets no app id, which exercises the app-id filter;
  - grim 1.5 captures through ext-image-copy-capture, not wlr-screencopy.
- **Shell traps:**
  - over SSH, never `pkill -f` a pattern that also appears in the remote command;
  - redirect `wl-copy`'s output, or it holds the session open;
  - `grep` does not read `\t` as a tab.
- **The commit-msg hook** still rejects "agent", "bot", "both", "cursor" and the rest (rule 9).
  There is no assistant trailer in commits.
