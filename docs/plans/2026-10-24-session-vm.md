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

Boot with `./run.sh`. greetd runs agreety on tty1 with
`--cmd 'systemd-cat -t sysc-compositor /usr/local/bin/sysc-compositor --session'`. The VM user has a
throwaway console password, typed with `qmp.py type`. The screen is captured with
`gvnccapture 127.0.0.1:59`. QEMU can't `screendump` the `egl-headless` GL console ("no surface"), so
`run.sh` adds a VNC server bound to localhost.

**First login: it failed.** systemd refused `systemctl --user start graphical-session.target`
(`RefuseManualStart=yes`), and the compositor exited back to the greeter. The M9 test used a fake
`systemctl`, so it never saw the refusal. Niri never starts that target itself: `niri.service` has
`BindsTo=graphical-session.target`. The fix is on the same branch as this record:

- `--session` starts `sysc-session.target` (`share/systemd/user/`, installed by `make install`). That
  target binds `graphical-session.target`.
- On exit it still stops `graphical-session.target`. The first fix stopped only the new target, and
  the gate showed `graphical-session.target` staying up: sysc-shell's `Requisite=` keeps it "needed",
  so `StopWhenUnneeded` never fires, and the shell kept restarting. systemd allows a manual stop.

Results with the fix. The shots are `gate-session.png` (the session with sysc-shell's bar) and
`gate-exit.png` (the greeter after quitting), in the assets directory.

| Check | Result |
|---|---|
| `WAYLAND_DISPLAY`, `NIRI_SOCKET`, `SYSC_SOCKET`, `XDG_CURRENT_DESKTOP=niri:sysc`, `XDG_SESSION_TYPE=wayland` in `systemctl --user show-environment` | yes |
| The same five in a D-Bus-activated service's environment (`xdg-desktop-portal`, activated by a Settings call) | yes |
| `sysc-session.target`, `graphical-session-pre.target`, `graphical-session.target` active | yes |
| `sysc-shell.service` started by the target, bar drawn | yes |
| Portals: `xdg-desktop-portal` with the gnome and gtk backends from `niri-portals.conf` | yes. The gnome backend logs a missing `org.gnome.Mutter.ServiceChannel`, which is M12. |
| locale1 and login1 clients in session mode | 2 system-bus connections. `localectl set-x11-keymap de` and then `us` changed `niri msg keyboard-layouts` live. The compositor also owns `org.freedesktop.ScreenSaver` on the user bus. |
| Not in non-session mode | `sysc-compositor -c` nested in the VM session: no system-bus or user-bus connections |
| On `niri msg action quit --skip-confirmation`: targets stop, portal backends stop, variables cleared, greeter back on tty1 | yes |
| A second login after that | yes |

**Found on the way:**
- sysc-shell exits with status 1 when it is stopped after the compositor has gone ("shutdown
  roundtrip … connection reset by peer"). The unit ends `failed`, although the next login still
  starts it. This is a sysc-shell bug, `sysc-1651`; under Niri the order is the same.
- With no config the session's binds are all "(not bound)". Niri writes a default config on first
  run; this compositor doesn't.

**Still open for the owner:**
- `share/wayland-sessions/sysc-compositor.desktop` has `Exec=/usr/local/bin/sysc-compositor` with no
  `--session`. A greeter that launches the entry gets no `graphical-session.target`, no shell and no
  portals. Niri's entry runs `niri-session`. Whether the entry should change is the owner's decision;
  this gate used greetd's command line instead.
- Agreety stood in for sysc-greet. Running sysc-greet with this repo's `niri` alias as the greeter
  compositor is the next run of this gate.

## Housekeeping

- Start the VM with `~/vm/sysc-session/run.sh [lane2]` and stop it with `./vssh sudo systemctl poweroff`.
  Switching modes moves PCI addresses, so the disk is pinned with `bootindex=0`; without that the
  firmware tries network boot first.
- Over SSH, don't wait for QEMU with `pgrep -f` on a pattern that appears in the remote command; use
  `qemu.pid`.
- In auto mode, a detached run inside the VM (`systemd-run --user`) was refused. Lane 2 runs in the
  foreground: `internal/wlr` takes about 80 s.
