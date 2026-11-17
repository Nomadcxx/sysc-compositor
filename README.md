# sysc-compositor

A Wayland compositor written in Go for the sysc desktop, using wlroots 0.20 and SceneFX.
It supports the niri-ipc 26.4.0 wire format so existing shell clients can connect.

## Install

Every route installs the `sysc-compositor` binary, the `sysc-compositor-session` wrapper,
the `sysc-compositor` login-manager session and its systemd user units. Pick the
sysc-compositor session at your login manager.

**From source.** With the dependencies under [Build](#build):

```
make build
sudo make install PREFIX=/usr    # PREFIX defaults to /usr/local; DESTDIR is honoured
```

**Debian forky.** Each release carries a `.deb`, built and smoke-checked in `debian:forky`,
beside a vendor tarball and `SHA256SUMS`:

```
sudo apt install ./sysc-compositor_<version>_amd64.deb
```

**Nix and NixOS.** `nix build github:Nomadcxx/sysc-compositor` builds the package. On NixOS,
add the flake as an input, import its module and enable it:

```nix
imports = [ inputs.sysc-compositor.nixosModules.default ];
programs.sysc-compositor.enable = true;
```

**Other distributions.** Recipes in this tree build against the release tarball:
[`packaging/rpm`](packaging/rpm) (Fedora 44 and openSUSE Tumbleweed),
[`packaging/obs/scenefx`](packaging/obs/scenefx) (SceneFX 0.5 for Tumbleweed),
[`packaging/void`](packaging/void) and [`packaging/gentoo`](packaging/gentoo). The Arch
recipes, `sysc-compositor` and `sysc-compositor-git`, live in
[Nomadcxx/sysc](https://github.com/Nomadcxx/sysc) under `aur/`. Distribution repositories
are listed here as each one is published.

## Commands

```
sysc-compositor                       # run (nested inside an existing session)
sysc-compositor -c CONFIG             # run with a config
sysc-compositor validate [-c CONFIG]  # check a config without running it
sysc-compositor msg [--json] REQUEST  # talk to a running compositor over $NIRI_SOCKET
```

## Moving from Niri

On the first start without a named config, sysc-compositor copies your Niri config and
relative includes into `~/.config/sysc-compositor/` (or `$XDG_CONFIG_HOME/sysc-compositor/`).
It keeps existing destination files and leaves your Niri files untouched. Later starts use
the copy; they do not import again. Without a Niri config, it writes its shipped default.
Use `-c PATH` or `SYSC_COMPOSITOR_CONFIG` to select a config explicitly.

The `NIRI_SOCKET` variable and IPC remain compatible. Existing `niri msg` scripts work
while Niri is installed; use `sysc-compositor msg` to remove that command dependency.
This package installs no `niri` command. Binds that still run it produce a migration warning.

Sessions publish `XDG_CURRENT_DESKTOP=sysc`. The shipped `sysc-portals.conf` selects the
GNOME backend for ScreenCast and Screenshot and GTK for other desktop portals.
The [parity matrix](docs/plans/niri-parity-matrix.md) lists features still to implement.

## Build

Build with Go 1.26.4 or newer, wlroots 0.20, SceneFX 0.5 and system libpipewire 1.4.8 or newer.
`make build` allows the exact `-fno-strict-overflow` flag supplied by libspa's pkg-config file.
For direct Go build/test/vet commands, set `CGO_CFLAGS_ALLOW=-fno-strict-overflow` too. No Go
PipeWire binding dependency is required.

X11 applications need xwayland-satellite 0.7 or newer and Xwayland installed at
runtime. Satellite starts on demand. The `xwayland-satellite { path "..."; }`
config section overrides its executable; `off` stops publishing DISPLAY to new
applications while existing X11 applications keep running.

## Docs

| Doc | What it is |
|---|---|
| [docs/README.md](docs/README.md) | Map of every design and plan |
| [docs/plans/2026-10-02-sysc-compositor-design.md](docs/plans/2026-10-02-sysc-compositor-design.md) | Product contract |
| [docs/plans/2026-10-02-orchestration.md](docs/plans/2026-10-02-orchestration.md) | Workstreams and order |
| [docs/plans/2026-10-02-sysc-launch.md](docs/plans/2026-10-02-sysc-launch.md) | Sibling: teach sysc-launch this compositor |
| [docs/plans/2026-10-02-m0-nested-tinywl.md](docs/plans/2026-10-02-m0-nested-tinywl.md) | First compositor slice |
| [docs/plans/niri-parity-matrix.md](docs/plans/niri-parity-matrix.md) | Remaining-work checklist |

## What this repo is not

Not a shell, greeter, wallpaper engine, notification daemon, tray, clipboard, or Wayland **client** library. Those already exist. sysc-wayland is client-only and stays that way.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for bundled-code notices.
