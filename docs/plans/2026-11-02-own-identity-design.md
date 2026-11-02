# sysc-compositor's own identity (tranche 1)

Owner direction 2026-11-02: sysc-compositor is its own compositor. It is written in Go, it is a
different implementation from Niri, and it has major differences. It never calls itself niri.
Its one deliberate likeness to Niri is the IPC, so that moving from Niri to sysc-compositor takes
no effort.

This is tranche 1 of three. Tranche 2 is
[input devices and native sysc-shell support](2026-11-02-input-devices-design.md); tranche 3 is
[packaging](2026-11-02-packaging-design.md). Tranche 3 needs this tranche's names, paths and
license. Implementation plan: [2026-11-02-own-identity.md](2026-11-02-own-identity.md).

## Why

The product contract (§1, §6) presented sysc-compositor as "Niri parity". It installs a `niri`
binary, reads `~/.config/niri/config.kdl`, names its socket `niri.*.sock`, sets
`XDG_CURRENT_DESKTOP=niri:sysc`, and tells users "are you running this within niri?". To a user,
it looks like Niri. That was useful while the suite moved over. It stops being acceptable once
sysc-compositor is packaged and installed alongside Niri (tranche 3).

## Goals

1. Nothing a user sees calls sysc-compositor "niri": command names, help, errors, logs, the
   session entry, systemd units, D-Bus metadata, socket file names, config paths, README and
   user docs.
2. Niri IPC compatibility stays exact. The niri-ipc 26.4.0 wire format and the `NIRI_SOCKET`
   variable are kept, so waybar's niri modules, sysc-shell, scripts and Niri's own `niri msg`
   work unchanged.
3. Moving over takes no effort. On first start with no sysc-compositor config, an existing
   Niri config is imported once. With no Niri config either, sysc-compositor writes its own
   shipped default config.
4. Siblings stop invoking `niri`, in the same tranche, so that dropping the alias breaks
   nothing: sysc-shell, sysc-launch and sysc-greet.
5. The license is GPL-3.0-or-later, with a LICENSE file (owner decision 2026-11-02).

## Non-goals

- Changing the IPC wire format, its request names or `NIRI_SOCKET`.
- Renaming code identifiers, test names or contributor-facing comments that cite Niri's source
  as a behaviour reference (`src/ipc/server.rs:81`). Contributors need those citations. Users
  never see them.
- Rewriting historical plans and handovers under `docs/plans/`. They are records.
- Making the config format diverge from Niri's. It stays import-compatible. sysc-specific
  sections may be added later.

## Anti-goals

- **Breaking a working session.** A user who upgrades must keep:
  - their bar, sysc-shell theming, app launching and the greeter;
  - their config: in place, imported, or the default.
- **Deleting or rewriting the user's Niri files.** The import only reads them.
- **Importing again on every start.** That would silently overwrite a user's edits to their
  sysc-compositor config.

## Decisions

### D1. The two surfaces

| Surface | Kept, Niri-compatible | Changed to sysc-compositor |
|---|---|---|
| IPC | niri-ipc 26.4.0 JSON wire, `NIRI_SOCKET`, the `Version` reply shape | — |
| Commands | — | Only `sysc-compositor` (and `sysc-compositor-session`). `cmd/niri` is deleted. |
| Socket file | — | `$XDG_RUNTIME_DIR/sysc-compositor.<display>.<pid>.sock`. `NIRI_SOCKET` points at it. |
| Control socket | `SYSC_SOCKET`, unchanged | — |
| Config | Accepts Niri's KDL format | `~/.config/sysc-compositor/config.kdl` (`$XDG_CONFIG_HOME`), `/etc/sysc-compositor/config.kdl`, `$SYSC_COMPOSITOR_CONFIG` |
| Session env | `NIRI_SOCKET` | `XDG_CURRENT_DESKTOP=sysc` (was `niri:sysc`) |
| Portals | — | `share/xdg-desktop-portal/sysc-portals.conf`: gnome for ScreenCast and Screenshot, gtk for everything else |
| Text | — | Usage, errors, the version mismatch warning, log prefixes, unit `Description=`, session `Name=`/`Comment=`, AT-SPI `ToolkitName`/`Name`, the screencast dynamic-target title and app-id |

**`XDG_CURRENT_DESKTOP` (my call).** It becomes `sysc` alone. xdg-desktop-portal picks its
config from this value. With `niri` in it, Niri's `niri-portals.conf` would apply whenever Niri
is also installed. sysc ships its own portal config instead. Desktop entries with
`OnlyShowIn=niri` will no longer show; that is correct for a different desktop.

**`sysc-compositor msg` and `validate` (my call).** They keep reading `NIRI_SOCKET`. A
`SYSC_COMPOSITOR_SOCKET` alias is not added: one variable for one socket.

### D2. Config location and the one-time import

**Lookup order:**
1. `-c PATH`;
2. `$SYSC_COMPOSITOR_CONFIG`;
3. `$XDG_CONFIG_HOME/sysc-compositor/config.kdl`, or `~/.config/sysc-compositor/config.kdl`;
4. `/etc/sysc-compositor/config.kdl`.

`$NIRI_CONFIG` and the Niri paths are no longer read for loading.

**First start (`sysc-compositor` or `--session`, never `validate` or `msg`).** If nothing was
named with `-c` or the environment variable, and the user config file does not exist:

1. **If `$XDG_CONFIG_HOME/niri/config.kdl` (or `~/.config/niri/config.kdl`) exists, import it:**
   - Copy it to the sysc-compositor config path.
   - Before its first line, add a comment naming the source file and the date.
   - Copy every file it pulls in with `include`, recursively, keeping relative paths. A
     relative `include "sysc-shell.kdl"` then resolves in the new directory.
   - Leave an absolute include unchanged; report it to stderr once as still pointing outside.
   - Skip a missing include, as loading does, and report it.
   - Log `sysc-compositor: imported Niri config from <path>`.
   - Never import again: once the file exists, step 3 of the lookup finds it.
2. **Otherwise, write the shipped default** (D3) to the user config path and log that.
3. **On failure** (a read-only home, or a write error): log the failure and run on the
   built-in defaults, as today. The session still starts.

The import is a copy. Later edits to the Niri config are not followed (my call; the owner said
"auto-import ... if none exists").

**Settings sysc-compositor does not implement** survive the import untouched. `validate` warns
about them as it does today.

**Binds that run `niri`** (`spawn "niri" "msg" …` and `spawn-sh` lines that start with `niri `)
make `validate` and reload print one warning naming `sysc-compositor msg`. They are not
rewritten. They keep working while Niri is installed, because Niri's `niri msg` speaks to
sysc-compositor's compatible socket.

### D3. A shipped default config

- **Source.** `internal/config/default-config.kdl` is sysc-compositor's own default. It is
  embedded in the binary with `go:embed`, which cannot reach outside the package directory, so it
  cannot go missing.
- **Basis.** It is written from Niri's default config structure, which the GPL-3.0-or-later
  license (D5) permits adapting. It is changed for sysc:
  - comments describe sysc-compositor;
  - binds that start terminals and launchers use defaults that exist without sysc-shell
    (`foot` stays as Niri's default does);
  - there are no `spawn-at-startup` lines, because sysc-shell starts from its own systemd user
    unit.
- **Requirements.** `sysc-compositor validate -c internal/config/default-config.kdl` reports no unapplied
  actions beyond the ones the parity matrix already lists as unbuilt. A test pins that.
- **Packages** also install it as `/usr/share/sysc-compositor/default-config.kdl` for reference
  (tranche 3).

### D4. Siblings stop invoking `niri`

All of these land before `cmd/niri` is deleted, and the compositor change releases with them.

**sysc-shell:**
- **Launching.** `spawnDesktopExecLocked` sends a `Spawn` action over `NIRI_SOCKET`
  (`niri.Action`, the existing client) instead of executing `niri msg action spawn`. That works
  under Niri too.
- **Theming.** The Niri theming target writes into the running compositor's config directory:
  - `~/.config/sysc-compositor/` when `SYSC_SOCKET` is set and `NIRI_SOCKET`'s file name
    starts with `sysc-compositor.` (only sysc-compositor sets these);
  - `~/.config/niri/` otherwise.

  The include it manages (`sysc-shell.kdl`) and the managed directive keep their names. The
  plan for this is in sysc-shell, below.

**sysc-launch:** `service.go` spawns through the IPC socket in the same way, instead of
`niri msg action spawn`.

**sysc-greet:**
- It ships `config/sysc-compositor-greeter-config.kdl` and runs
  `sysc-compositor -c /etc/greetd/sysc-compositor-greeter-config.kdl` when the selected greeter
  compositor is sysc-compositor.
- The power-menu and quit commands inside that config use `sysc-compositor msg`.
- Its Niri greeter config is unchanged for Niri users.

**bd.** `sysc-1634` (retire the alias) is carried out by this tranche. `sysc-1633` (the suite
installer's `sysc msg`) is not needed for it and stays separate.

### D5. License

- **LICENSE.** Add the GPL-3.0-or-later text as `LICENSE`. README and the design's §13 say
  GPL-3.0-or-later.
- **Bundled code.**
  - `third_party/godbus` keeps its BSD-2-Clause license file;
  - `protocols/*.xml` keep their MIT notices;
  - a `NOTICE` lists all of these.
- **Siblings.** The sibling repositories are separate programs and keep their own licenses. They
  talk to the compositor over IPC and do not link it.

### D6. Words in user docs

- **README.** It describes sysc-compositor as a Wayland compositor written in Go for the sysc
  desktop. It has a "Moving from Niri" section covering:
  - Niri IPC compatibility;
  - the one-time config import;
  - keeping Niri installed for `niri msg` scripts;
  - the differences users will notice (the parity matrix's unbuilt items).
- **Product contract.** §1, §6 and §12 are amended to these decisions. The section headings that
  say "niri" for the config and IPC are renamed to "Config (Niri-compatible KDL)" and "IPC
  (niri-ipc 26.4.0 compatible)".
- **Parity matrix.** `niri-parity-matrix.md` keeps its name: it is a planning record of
  compatibility work, not user documentation (my call).

## Constraints

- `CLAUDE.md`/`AGENTS.md` rules hold: Go and cgo only, one named test per command, nested tests
  only with consent (laptop pre-approved).
- The siblings each have their own repo rules:
  - sysc-shell: plans in its gitignored `docs/plans/`, bd in `/home/nomadx/sysc-shell`, and its
    commit gates;
  - sysc-launch;
  - sysc-greet.
- The commit-msg hook rejects some ordinary words; see the memory file.

## Left to the builder

- The exact comment wording at the top of an imported config.
- How the include walk is written: reuse the loader's include resolution if it exposes paths,
  rather than parsing twice.
- The default config's exact binds, within D3's rules.
- Whether the session script logs the import to the journal or stderr (both reach the journal
  under systemd).
