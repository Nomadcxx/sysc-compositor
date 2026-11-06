# Laptop check through sysc-greet

Your existing sysc-greet already lists sysc-compositor. Its old entry launches
/usr/local/bin/sysc-compositor directly; the prepared update uses the tested
sysc-compositor-session wrapper and current input build. Niri stays available.
The installed binary was dated October 18 when inspected.

On the laptop, before rebooting, run as nomadx:

```sh
bash ~/.cache/sysc-compositor-gate-20261102/install-for-next-login.sh
```

The script verifies the staged checksums, requests sudo, backs up the seven
installation targets, and updates the existing session entry. It does not restart
your current desktop. After reboot, select sysc-compositor in sysc-greet.

From a terminal in the new session, check:

```sh
printf '%s\n' "$XDG_CURRENT_DESKTOP" "$NIRI_SOCKET"
sysc-compositor msg version
```

Expect desktop `sysc` and a socket basename starting `sysc-compositor.`. First
startup imports your Niri config/includes into ~/.config/sysc-compositor if the
new config is absent. It preserves your original ~/.config/niri files.

For the touchpad check, use a complete touchpad section in the sysc config (or
its last included input file), preserving any settings you already want:

```kdl
input {
    touchpad {
        tap
        natural-scroll
        accel-speed 0.3
    }
}
```

Open foot and try a tap click and two-finger natural scrolling. A three-finger
upward swipe should switch to the next workspace down. Then remove `tap`, save,
and confirm tap clicks stop without restarting. Restore your preferred settings
afterward. Report what worked and any unexpected direction or delay.

This checks the compositor input implementation. The new shell Mouse & touchpad
page remains in draft PR #612 and needs its own supported scripts/deploy rollout
and live checks under Niri and sysc-compositor; this installer does not change the
shell binary.

To revert the compositor update, log into Niri and run:

```sh
bash ~/.cache/sysc-compositor-gate-20261102/before-input-gate-install/restore.sh
```

The generated sysc config remains for inspection. Niri continues using its own
config. The AUR dependency gate also remains separate from this local check.
