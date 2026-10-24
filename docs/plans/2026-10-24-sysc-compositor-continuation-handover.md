# sysc-compositor handover: after the session VM, the real session and lane 2

**Written:** 2026-10-24, late. **This supersedes the 2026-10-24 handover for planning.** The
2026-10-23 continuation handover still holds standing rules 1–11 and the working recipe. bd is the
status authority: run `bd` only from `/home/nomadx/sysc-shell`. Re-read `git`, `gh` and `bd` before
relying on any state here.

## Read first

1. `AGENTS.md`, then `docs/README.md`.
2. `docs/plans/2026-10-23-sysc-compositor-continuation-handover.md`: standing rules 1–11.
3. `docs/plans/2026-10-24-session-vm.md`: the VM, how to drive it, and what it proved.
4. `docs/plans/niri-parity-matrix.md`.

## What landed on 2026-10-24

| PR | What |
|---|---|
| #50 | **The session runs the way Niri's does (owner decision).** systemd refuses a manual start of `graphical-session.target`, so the first real `--session` login failed (`sysc-1631`). Now:<br>• `--session` only publishes the environment.<br>• The compositor sends `READY=1` to `$NOTIFY_SOCKET` in every mode.<br>• `sysc-compositor.service` (`Type=notify`, `BindsTo=graphical-session.target`) and `sysc-compositor-shutdown.target` match Niri's units.<br>• `share/bin/sysc-compositor-session` is `niri-session`'s systemd branch.<br>• The session entry's `Exec` runs that script. `make install` installs the script and both units.<br>Also adds the session VM record. |
| #51 | Records the gate's second run, with sysc-greet as the greeter, under this repo's `niri` alias and its installer's config, unmodified. Also records the `sysc-1626` runs. |
| #52 | **Lane 2 checks what nested runs check (`sysc-1638`).** None of the 26 quarantine entries was a compositor bug.<br>• Tests now press the resolved Mod (`c.modKeycode()`); Mod is Super on DRM.<br>• Tests attach the input devices they rely on before their client starts (`attachFakeKeyboard`).<br>• Tests that drive the host window skip off the nested backend; `.github/ci/skips.txt` lists the allowed skips.<br>• Lane 2 runs under `dbus-run-session` (waybar needs a session bus).<br>The quarantine keeps only `sysc-1640` and `sysc-1642`. |

`origin/main` is `2e9ee30`, with all three merged and both CI lanes green. No PRs are open apart from
the one that adds this handover. In #52's CI run, every formerly quarantined test passed; only
`sysc-1642` failed, and only the listed skips skipped.

### bd changes

- `sysc-1626` (P1) is closed as not a bug. The animation tests passed 30 of 30 runs in the VM's
  lane 2, and the watchdog bug the review found next to it was already fixed on `main`.
- `sysc-1631` has every check passed: the real session through greetd with agreety and with
  sysc-greet, a clean quit, a SIGKILL of the compositor, and a login after the crash. **The owner
  closes it.** Closing it unblocks `sysc-1283` (suite integration).
- `sysc-1638` is closed (#52).
- New: `sysc-1651`. sysc-shell exits 1 when it is stopped after the compositor has gone, so its unit
  ends `failed`. This is in sysc-shell.

## The session VM

It's on the laptop (`ssh laptop`), all in `~/vm/sysc-session`; removing that directory removes it.
It's powered off between uses. It opens no window on the laptop's desktop and leaves the host's
greetd and Niri session alone. Root inside it is fine.

- `./run.sh` boots it with virgl, for sessions. `./run.sh lane2` boots it with no GPU, so vkms is
  the only DRM device, as in CI.
- `./vssh CMD` runs a command inside it. `./qmp.py type TEXT` or `./qmp.py key CHORD` types at its
  console.
- `gvnccapture 127.0.0.1:59 shot.png` takes a screenshot. QMP `screendump` fails on the GL console.
- Its user is `nomadx`, with the throwaway console password `sysc-vm`. greetd currently runs
  sysc-greet: `niri -c /etc/greetd/niri-greeter-config.kdl`.
- **Lane 2 in the VM:**
  - `./vssh './lane2.sh ./internal/wlr'` runs one package, in about 80 s.
  - For CI's exact scoring, run `.github/ci/compositor-tests.sh` with its `pacman` line stripped,
    as root, under the lane-2 environment. The record shows the command.
  - A root run leaves root-owned headers in `internal/wlr/protocols`. Remove them with `sudo`
    before copying the tree in again.
- Inside the VM, whole-package nested runs are allowed. Outside it, rule 2 (one named test per
  command) still holds.
- In auto mode, a detached run inside the VM (`systemd-run --user`) was refused. Run things in the
  foreground.

## Next work, in order

### 1. The M10 gate (`sysc-1643`), now possible without the owner

The bd item asks for a nested laptop session, with the owner typing their password to unlock
sysc-lock. **Run it in the VM's real session instead.** There, the unlock uses the VM's throwaway
password, so no owner step is left. Ask the owner in one line before switching from the laptop to
the VM. The checks:

- sysc-shell frost on the bar and panels;
- a text-input client with an IME;
- idle blank and unblank (`PowerOffMonitors` and `PowerOnMonitors`);
- sysc-lock, built from its own repo inside the VM, locking and unlocking;
- injected-seat rejection while locked.

Record the results in `docs/plans/2026-10-22-m10-input-idle.md`. Never type or store the owner's
real password.

### 2. `sysc-1642` and `sysc-1640`: now reproducible

- `sysc-1642`: `TestWindowOpenAndCloseAnimateReferencedBuffers` fails every time in lane2 mode
  ("window remap did not restart its open animation"). Use the systematic-debugging process, find
  the root cause, and check whether a real session can hit it.
- `sysc-1640`: the overview projection test passed in the last three VM runs. Run it with
  `-count=20` in the VM. If it never fails, take it out of the quarantine and close the issue.

### 3. M12 (`sysc-1277`)

Write its plan when it starts, from design §15 and the matrix:
- ScreenCast over PipeWire, Mutter DisplayConfig and the screenshot portal;
- foreign-toplevel, virtual input, gamma, tearing, syncobj, tablet, and window-image capture.

Decide whether `proposal/window-preview-capture-20261018` becomes its contract. Virtual-keyboard
unblocks the IME keyboard grab (`sysc-1639`). The GNOME portal in the VM logs the missing
`org.gnome.Mutter.ServiceChannel`, which is M12's.

### 4. Smaller items

- **No default config.** With no config, every bind shows "(not bound)". Niri writes a default
  config on first run; this compositor should too (`niri-config`'s default as the reference).
- **Mouse and scroll binds** aren't applied. Applying them also closes the switcher's mouse/scroll
  gap.
- **`sysc-1470` (P2):** IPC state is rebuilt on every pointer motion.
- **Two greeter config sections** are accepted but not applied: `layout.center-focused-column` and
  `layout.border`. The greeter sets both to their defaults.
- **gSlapper:** the greeter's wallpaper and its `place-within-backdrop` layer rule are untested,
  because gSlapper isn't in the VM.

### 5. M13–M15

`sysc-1278` to `sysc-1280`. The VM can now run M13's xwayland-satellite checks and M14's Orca
checks. M15 easing unblocks the screenshot UI's open animation, the switcher's fade-in, and
windows opening and closing the way Niri does.

## Open bd items for the owner

| Item | bd |
|---|---|
| Close the `--session` gate (all checks passed) | `sysc-1631` |
| Close M11 | `sysc-1276` |
| sysc-wayland: absolute `WAYLAND_DISPLAY` (sibling repo) | `sysc-1650` |
| sysc-shell exits 1 on stop (sibling repo) | `sysc-1651` |
| sysc-shell stops owning ScreenSaver | `sysc-1635` |

## Notes

- **Commit messages:** the hook rejects "cursor" even in "cursor theme"; say "pointer theme".
  Check every draft with the grep in rule 9.
- **Laptop leftovers:** `~/sysc-compositor-lane2-keys-20261024`, from #52's nested checks, plus the
  older ones listed in the previous handover. Ask the owner before removing any of them.
- **The session entry:** `make install` puts the binaries and the session script in
  `/usr/local/bin`. The service's `ExecStart` is an absolute path, so a machine installing
  elsewhere has to change both.
- **`/home` is full.** It was at 100% on 2026-10-24, and bd failed with "disk I/O error" until
  `go clean -cache` (owner-approved) freed about 550 MB. Expect this again after builds. Never
  delete anything else without asking. `/tmp` is a separate 8 GB tmpfs, at 79% that night.
