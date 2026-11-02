# Packaging: AUR and cross-distro (tranche 3)

Owner direction 2026-11-02: start packaging sysc-compositor, for the AUR and across distros.
The owner chose these first targets beyond the AUR: Fedora 44 (COPR), Debian 14/sid (.deb), Void,
openSUSE Tumbleweed, Gentoo, and a Nix flake.

This is tranche 3 of three. It needs
[tranche 1](2026-11-02-own-identity-design.md) for its names, paths, default config and
GPL-3.0-or-later license. Its first release does not wait for
[tranche 2](2026-11-02-input-devices-design.md); when that lands, the packages gain the libinput
dependency. Implementation plan: [2026-11-02-packaging.md](2026-11-02-packaging.md).

## Why

- **Only from source today.** sysc-compositor can only be built from a checkout with `make install`.
- **Wrong install paths.** That installs to `/usr/local` and hardcodes `/usr/local/bin` in the
  session and unit files.
- **No real version.** A build from a tarball reports its version as `(devel)`, because the
  version comes from VCS build info.
- **The suite can't depend on it.** sysc-shell and the suite installer cannot depend on a
  package that does not exist.

## What exists to build on

- **The suite's AUR recipes.** They live in `/home/nomadx/sysc/aur/`: the recipes, `check.py` and
  `<name>.install` hooks with the sysc banner. bd `sysc-1705` tracks putting them under version
  control. sysc-compositor's recipes join them, following the same conventions:
  - `GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"`, `GOTOOLCHAIN=local`;
  - files only under `usr/`;
  - a stale `.SRCINFO` fails the check.
- **sysc-greet's pattern** for DEB/RPM: `nfpm` in a release workflow, with per-distro container
  builds, plus a `flake.nix` using `buildGo126Module`.

**Library availability, Repology, 2026-11-02:**

| Distro | wlroots 0.20 | SceneFX 0.5 | Route |
|---|---|---|---|
| Arch | `wlroots0.20` (extra) | `scenefx` (extra) | AUR |
| Fedora 44 | updates | updates | COPR |
| Debian 14 forky / sid | yes | yes | .deb (nfpm) |
| Void | `wlroots0.20` | yes | xbps-src template |
| openSUSE Tumbleweed | yes | **no** | OBS, packaging SceneFX too |
| Gentoo | yes | GURU overlay | ebuild |
| nixpkgs unstable | yes | yes | flake |
| Debian 13, Ubuntu ≤ 26.04 | no | no | not supported |

## Goals

1. **A real release.** Tagged releases (`vX.Y.Z`) build from a tarball without a git checkout or
   network access, and report their real version.
2. **One install layout.** It works for every package format and for `make install`:

   | Path | Contents |
   |---|---|
   | `/usr/bin/sysc-compositor` | the compositor |
   | `/usr/bin/sysc-compositor-session` | the session script |
   | `/usr/share/wayland-sessions/sysc-compositor.desktop` | the login screen entry |
   | `/usr/lib/systemd/user/sysc-compositor.service` | the session unit |
   | `/usr/lib/systemd/user/sysc-compositor-shutdown.target` | the shutdown target |
   | `/usr/share/xdg-desktop-portal/sysc-portals.conf` | portal choices |
   | `/usr/share/sysc-compositor/default-config.kdl` | the default config |
   | `/usr/share/licenses/sysc-compositor/` (Arch) or the distro's license directory | LICENSE and NOTICE |
   | `/usr/share/doc/sysc-compositor/` | README |

3. **AUR packages** `sysc-compositor` and `sysc-compositor-git`, gated by the suite's
   `check.py`.
4. **Recipes and builds for the chosen distros.** Each is proved by installing into a clean
   container or VM and running a smoke check (D6).
5. **sysc-shell's recipe** stops requiring `niri` (tranche 2 D6) in the same change as the AUR
   release.

## Non-goals

- **Distros without wlroots 0.20 and SceneFX 0.5:** Debian 13 and Ubuntu up to 26.04. Bundling a
  private wlroots build is rejected: it would make sysc-compositor ship C the owner does not want
  more of.
- **Static or prebuilt binaries** on GitHub releases. They would not run where the C libraries
  differ.
- **Adding the compositor to the suite installer `sysc`.** That is a separate change in the
  installer repository, which stays Arch-only.
- **Shell completions and a man page.** Later.
- **Becoming a maintainer inside Debian, Fedora, Void or Gentoo themselves.** The first step is
  our own COPR, OBS project, .deb releases and recipes. Submissions upstream come after
  feedback.

## Anti-goals

- **Packages that install files outside `/usr`.** The system config
  `/etc/sysc-compositor/config.kdl` is left for the admin; no package installs it.
- **Network access in distro builds.** Go modules come from a vendor tarball.
- **Conflicting with the `niri` package.** No file is shared: tranche 1 removes the `niri`
  binary and uses `sysc-portals.conf`.

## Decisions

### D1. Version and release artefacts

- **Version string.** `internal/server.version` is set with
  `-ldflags "-X …/internal/server.version=X.Y.Z"`. `Version()` prefers it over build info. The
  Makefile passes `VERSION` (default: `git describe` when available, or `(devel)`).
- **Release workflow.** A tag `vX.Y.Z` runs `.github/workflows/release.yml`, which publishes:
  - the GitHub source tarball, which GitHub provides;
  - `sysc-compositor-X.Y.Z-vendor.tar.xz` (the output of `go mod vendor`) and its SHA256, so
    offline builders (Debian, COPR, OBS, Void, Gentoo) need no network;
  - the Debian 14 .deb, built in a `debian:forky` container with nfpm (D4).
- **Version number.** The first tag is `v0.1.0`. Releases follow semver, with the 0.x line until
  M15 closes (my call).

### D2. Makefile as the single install description

- **Variables.** The Makefile has `PREFIX ?= /usr/local`, `BINDIR`, `DATADIR`, `LIBDIR` and
  `DESTDIR`.
- **Path substitution.** `install` writes `share/` files through
  `sed 's|@BINDIR@|$(BINDIR)|'`, so the session entry's `Exec=` and the unit's `ExecStart=`
  carry the real path. The source files hold `@BINDIR@` instead of `/usr/local/bin`.
- **The protocol step.** `make build` runs `go generate ./internal/wlr` (wayland-scanner) before
  `go build`.
- **Every package format calls `make build install PREFIX=/usr DESTDIR=…`.** Only the AUR also
  sets its own GOFLAGS, through the environment. One description of what gets installed.

### D3. Dependencies

The same list in each distro's names; the plan maps them.

- **Run time:**
  - wlroots 0.20, SceneFX 0.5, libpipewire ≥ 1.4.8, libxkbcommon, wayland, pixman, libdrm;
  - dbus and xkeyboard-config;
  - libinput once tranche 2 lands.
- **Optional:**
  - xwayland-satellite ≥ 0.7, for X11 apps;
  - xdg-desktop-portal-gnome, for screencasts and screenshots;
  - xdg-desktop-portal-gtk, for file pickers;
  - sysc-shell, the desktop shell.
- **Build:** Go ≥ the `go` line in `go.mod`, wayland (for `wayland-scanner`), wayland-protocols,
  pkgconf and gcc.
- **Go version.** `go.mod` says `go 1.26.4`. Each distro's Go must meet it. If one does not, the
  plan lowers the `go` line to the oldest version the code really needs, checked by building with
  that toolchain. It does not override the distro's Go.

### D4. Per-distro routes

- **AUR** (`/home/nomadx/sysc/aur/sysc-compositor{,-git}`):
  - a PKGBUILD following the suite pattern, with the `.install` banner hook;
  - `check.py`'s package count grows from 8 to 10;
  - the release recipe builds from the tag tarball;
  - the git recipe uses `pkgver()` from `git describe`.
- **Debian 14 / sid:** `packaging/deb/nfpm.yaml`, built in the release workflow in a
  `debian:forky` container against Debian's own library packages, then attached to the GitHub
  release. Package names are resolved with `apt-cache` in that container during the plan.
- **Fedora 44:** `packaging/rpm/sysc-compositor.spec`, built by COPR (`nomadcxx/sysc`) from the
  tag and the vendor tarball.
- **openSUSE Tumbleweed:** the same spec, adjusted for openSUSE's package names, in an OBS
  project (`home:nomadcxx:sysc`). That project also packages SceneFX 0.5, which Tumbleweed
  lacks, from SceneFX's own release.
- **Void:** `packaging/void/srcpkgs/sysc-compositor/template`, built with `xbps-src` in a Void
  container.
- **Gentoo:** `packaging/gentoo/gui-wm/sysc-compositor/sysc-compositor-X.Y.Z.ebuild`, using the
  `go-module` eclass with the vendor tarball. It depends on GURU's `gui-libs/scenefx`.
- **Nix:** `flake.nix` at the repository root, on `nixos-unstable` with `buildGo126Module`. It
  provides `packages.default` with `passthru.providedSessions = [ "sysc-compositor" ]`, and a
  small NixOS module that installs the package and registers the session and the portal config.
  The plan checks whether nixpkgs has `buildGo126Module` and `scenefx` 0.5 at that time.

### D5. Accounts and publishing are the owner's

Opening the COPR project, the OBS project and GURU access, pushing to the AUR, and any upstream
submission (Void, Gentoo GURU, nixpkgs) are owner actions. The plan prepares and proves each
recipe locally, then stops at an owner gate in bd.

### D6. Proof for every format

**For each package:**
1. Build it in a clean container (or the session VM for Arch) from the release tarball, offline
   where the format requires.
2. Install it.
3. Run `sysc-compositor --version`. The flag exists already; it must print the D1 version, not
   `(devel)`.
4. Run `sysc-compositor validate -c /usr/share/sysc-compositor/default-config.kdl`.
5. Start `WLR_BACKENDS=headless sysc-compositor` for five seconds, then run
   `sysc-compositor msg version` against it.
6. Check the file list against Goal 2.

**For Arch also:** install it in the session VM and log in through greetd.

## Constraints

- **The suite's AUR recipes stay in `/home/nomadx/sysc/aur/`** and follow `sysc-1705` (tracked
  in git, `check.py`). This tranche adds its two recipes in the same form.
- **No new C:** packaging adds no code beyond `-ldflags` and the version flag.
- **GPL-3.0-or-later** (tranche 1 D5). Package license fields say `GPL-3.0-or-later`. The bundled
  third-party licenses are listed.

## Left to the builder

- The exact package names per distro, resolved in each distro's container.
- Whether the OBS and COPR specs share one file with conditionals or are two files.
- The NixOS module's option names.
- The contents of the `.install` banner hook, following the existing sysc hooks.
