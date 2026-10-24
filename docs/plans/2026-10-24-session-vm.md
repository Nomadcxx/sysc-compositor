# Session VM on the laptop: lane 2 under vkms and the `--session` gate

**Recorded:** 2026-10-24. Item 1 of the 2026-10-24 handover, approved by the owner the same day.
Status lives in bd: `sysc-1631` (session gate), `sysc-1638`, `sysc-1640` and `sysc-1642` (lane 2).

## What it is

An Arch VM on the laptop (`ssh laptop`), all of it in `~/vm/sysc-session/`. Removing that directory
removes the VM. It opens no window on the laptop's desktop, and it doesn't touch the host's greetd,
lock screen or Niri session. Root inside the VM is fine.

The scripts are in `assets/2026-10-24-session-vm/`:

| File | What |
|---|---|
| `run.sh` | Boots the VM headless. `./run.sh` gives it `virtio-vga-gl` (virgl) for the session gate. `./run.sh lane2` gives it no GPU, so vkms is the only DRM device, as on the CI runner. |
| `vssh` | SSH into the VM: `127.0.0.1:2222`, with the VM's own key `id_ed25519`. |
| `qmp.py` | `type TEXT` and `key CHORD` on the VM keyboard over `qmp.sock`. `shot FILE` works only in lane2 mode (see below). |
| `lane2.sh` | Lane 2 inside the VM (lives at `~/lane2.sh` there): `.github/ci/compositor-tests.sh`'s environment, minus pacman. |
| `user-data`, `meta-data` | The NoCloud seed (`seed.iso`, volume label `cidata`, made with `xorriso`). |
| `greetd-config.toml` | The VM's `/etc/greetd/config.toml`. |

### How it was built

1. **Base image:** the Arch cloud image, checked against its `.SHA256`, used as the backing file for a
   40 GB qcow2 overlay. OVMF, with a private copy of `OVMF_VARS.4m.fd`. q35, KVM, `-cpu host`,
   2 vCPUs, 6 GB.
2. **Seed:** cloud-init creates `nomadx` (uid 1000, passwordless sudo, the VM's own SSH key) and
   grows the root partition.
3. **Packages:** lane 2's list, plus `greetd greetd-agreety socat` and
   `xdg-desktop-portal xdg-desktop-portal-gtk xdg-desktop-portal-gnome`. Then reboot onto the new
   kernel.
4. **The repos:** `git archive` from the desktop, then `go generate ./internal/wlr`. sysc-shell is
   built from its committed `HEAD` into `~/.local/bin/sysc-shell`, and its
   `packaging/systemd/sysc-shell.service` is enabled. `niri-portals.conf` is copied from the laptop
   (the VM has no `niri` package).
5. **vkms:** a udev rule gives the video group the DRM card nodes. `lane2.sh` loads vkms itself;
   it doesn't load at boot, so it can't add a phantom output to the session.

## Lane 2 in the VM

Boot with `./run.sh lane2`, then run `./vssh './lane2.sh ./internal/wlr'` and the same for
`./internal/server`. Inside the VM, running a whole package is allowed: there is no desktop to open a
window on. Outside the VM, rule 2 still holds.

Results on `10e104b`, 2026-10-24, scored like CI against `.github/ci/quarantine.txt`:

- `internal/wlr`: 142 passed in 78 s, and nothing failed or skipped outside the quarantine.
  - `sysc-1642` (`TestWindowOpenAndCloseAnimateReferencedBuffers`) failed: "window remap did not
    restart its open animation".
  - Two quarantined tests passed: `TestOverviewProjectionSurvivesBufferCommitAndUnmap` (`sysc-1640`)
    and `TestExclusiveZoneShrinksUsableArea`.
  - The other 26 quarantined failures match `sysc-1638`'s descriptions.
- `internal/server`: passed.

**Use `lane2` mode, not the GPU mode, for lane 2.** With the virtio GPU present,
`TestDmabufClientMaps` crashes the test binary with SIGSEGV inside llvmpipe's JIT code. Under virgl,
`weston-simple-dmabuf-egl` allocates a real dmabuf on the VM's render node, which the CI runner
doesn't have. The handover worried that CI's script picks the first `/dev/dri/card*`. In the VM, vkms
(`faux_driver`) happened to be `card0`, but `lane2.sh` finds it by driver anyway.

## The `--session` gate (`sysc-1631`)

Boot with `./run.sh`. greetd runs agreety on tty1 (`greetd-config.toml`). The VM user has a
throwaway console password, typed with `qmp.py type`. The screen is captured with
`gvnccapture 127.0.0.1:59`. QEMU can't `screendump` the `egl-headless` GL console ("no surface"), so
`run.sh` adds a VNC server bound to localhost.

### First run: `--session` could not start the session

agreety ran `sysc-compositor --session` directly. systemd refused the compositor's
`systemctl --user start graphical-session.target` (`RefuseManualStart=yes`), and the compositor
exited back to the greeter. The M9 test used a fake `systemctl`, so it never saw the refusal.

Niri never starts that target itself. `niri.service` has `BindsTo=graphical-session.target`, and
the session entry runs `niri-session`, which starts the service, waits for it, then stops the
session through `niri-shutdown.target` and unsets the variables. A first fix bound the target from
a `sysc-session.target`. It worked, but it left the entry's bare `Exec` with no session at all, so
the owner chose Niri's shape (2026-10-24). The branch now does this:

- `--session` publishes the environment, and the compositor sends `READY=1` to `$NOTIFY_SOCKET` in
  every mode, as Niri does. Neither touches a target.
- `share/systemd/user/sysc-compositor.service` is `Type=notify` with `BindsTo=` and
  `Before=graphical-session.target`, as `niri.service` is. `sysc-compositor-shutdown.target` is
  `niri-shutdown.target`.
- `share/bin/sysc-compositor-session` is `niri-session`'s systemd branch. The session entry's `Exec`
  is now `/usr/local/bin/sysc-compositor-session`. `make install` installs all of these.

### Results through the session entry's command

greetd ran `agreety --cmd /usr/local/bin/sysc-compositor-session`, the entry's exact `Exec`. The
shots are `gate-session.png` (the session with sysc-shell's bar) and `gate-exit.png` (the greeter
after quitting), in the assets directory.

| Check | Result |
|---|---|
| `WAYLAND_DISPLAY`, `NIRI_SOCKET`, `SYSC_SOCKET`, `XDG_CURRENT_DESKTOP=niri:sysc`, `XDG_SESSION_TYPE=wayland` in `systemctl --user show-environment` | yes |
| The same five in a D-Bus-activated service's environment (`xdg-desktop-portal`, activated by a Settings call) | yes |
| `sysc-compositor.service` (`Type=notify`), `graphical-session-pre.target`, `graphical-session.target`, `xdg-desktop-autostart.target` active; the target became active at the service's READY | yes |
| `sysc-shell.service` started by the target, bar drawn | yes |
| Portals: `xdg-desktop-portal` with the gnome and gtk backends from `niri-portals.conf` | yes. The gnome backend logs a missing `org.gnome.Mutter.ServiceChannel`, which is M12. |
| locale1 and login1 clients in session mode | 2 system-bus connections. `localectl set-x11-keymap de` and then `us` changed `niri msg keyboard-layouts` live. The compositor also owns `org.freedesktop.ScreenSaver` on the user bus. |
| Not in non-session mode | `sysc-compositor -c` nested in the VM session: no system-bus or user-bus connections |
| On `niri msg action quit --skip-confirmation`: the service and targets stop, the shell and portal backends stop, the variables are unset, greeter back on tty1 | yes |
| On `kill -9` of the compositor: the script starts the shutdown target, everything above stops, the variables are unset, greeter back | yes; the service is left `failed` |
| A login after the crash (the script's `reset-failed`) | yes |

The locale1, D-Bus and non-session rows were checked with the first fix. This branch doesn't change
that code.

**Found on the way:**
- sysc-shell exits with status 1 when it is stopped after the compositor has gone ("shutdown
  roundtrip … connection reset by peer"). This is a sysc-shell bug, `sysc-1651`; under Niri the order
  is the same. `xdg-desktop-portal-gnome` exits 1 the same way, which isn't ours.
- With no config the session's binds are all "(not bound)". Niri writes a default config on first
  run; this compositor doesn't.

### sysc-greet as the greeter

Run the same day on `f5b761f`, with sysc-greet at `a390e38`. The VM was set up by hand the way
sysc-greet's installer sets up its niri variant, without pulling niri from the AUR:
- kitty installed;
- the binary, `ascii_configs`, `fonts`, `wallpapers` and `themes` in `/usr/share/sysc-greet`;
- `/etc/greetd/kitty.conf`;
- the greeter user's home at `/var/lib/greeter`, in the video, render and input groups;
- the installer's niri greeter config, copied word for word to `/etc/greetd/niri-greeter-config.kdl`.

greetd runs `niri -c /etc/greetd/niri-greeter-config.kdl`, so this repo's `niri` alias is the
greeter compositor.

| Check | Result |
|---|---|
| `niri validate` accepts the installer's config as written | yes. It warns that `layout.center-focused-column` and `layout.border` are accepted but not applied; both are set to their defaults there. |
| sysc-greet runs fullscreen in kitty and lists the session entry (`sysc-greet.png`) | yes: "sysc-compositor (Wayland)" |
| Logging in runs the entry's `Exec`, `sysc-compositor-session` | yes. The service, `graphical-session.target` and sysc-shell came up with all five variables. |
| The greeter's compositor quits after login (`niri msg action quit --skip-confirmation` in its config) | yes |
| The greeter's compositor, a plain `-c` run, publishes nothing | the greeter user's systemd had no session variables and no `graphical-session.target` |
| Quitting the session returns to sysc-greet | yes. It remembered the user and asked for the password only. |

Not covered: gSlapper isn't in the VM, so the greeter's wallpaper daemon and its
`place-within-backdrop` layer rule weren't exercised.

With this run every check in `sysc-1631` has passed.

### `sysc-1626`, checked here

Run on `f5b761f` in lane2 mode. These three tests each passed 10 out of 10 runs with `-count=10`,
and none stalled:
- `TestWindowTransitionOpacityClampsUnderdampedOvershoot`
- `TestWindowResizeScalesSnapshotWhileLayoutTargetsCommit`
- `TestWindowResizeSnapshotPreservesGeometryClip`

The watchdog bug that the 2026-10-22 review found next to it is already fixed on `main`. `sysc-1626`
is closed as not a bug.

## Housekeeping

- Start the VM with `~/vm/sysc-session/run.sh [lane2]` and stop it with `./vssh sudo systemctl poweroff`.
  Switching modes moves PCI addresses, so the disk is pinned with `bootindex=0`; without that the
  firmware tries network boot first.
- Over SSH, don't wait for QEMU with `pgrep -f` on a pattern that appears in the remote command; use
  `qemu.pid`.
- In auto mode, a detached run inside the VM (`systemd-run --user`) was refused. Lane 2 runs in the
  foreground: `internal/wlr` takes about 80 s.
