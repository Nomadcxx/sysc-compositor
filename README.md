# sysc-compositor

Wayland compositor for the sysc desktop. **Go and cgo only** (no `.c` / `.h` here; gSlapper keeps the C). wlroots 0.20. Niri parity: same KDL config, JSON IPC, and CLI so sysc-shell, sysc-greet, sysc-launch, and an existing `~/.config/niri/config.kdl` keep working.

This directory holds the product and the plans. Code starts at M0.

## Commands

```
sysc-compositor                       # run (nested inside an existing session)
sysc-compositor -c CONFIG             # run with a config
sysc-compositor validate [-c CONFIG]  # check a config without running it
sysc-compositor msg [--json] REQUEST  # talk to a running compositor over $NIRI_SOCKET
```

`niri` is a compatibility alias with the same arguments and JSON, so the greeter config and scripts
written for niri keep working. Use `sysc-compositor` in new docs and scripts.

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
