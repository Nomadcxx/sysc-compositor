# Packaging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tagged sysc-compositor releases build offline and report their version. They install one
`/usr` layout, and ship as AUR packages plus recipes for Debian 14/sid, Fedora 44, openSUSE
Tumbleweed, Void, Gentoo and Nix, each proved in a clean container.

**Architecture:**
- **One install description.** The Makefile is the single description of what is installed, with
  `PREFIX` and `@BINDIR@` substitution. Every recipe calls `make build install PREFIX=/usr DESTDIR=…`.
- **Releases.** A tag workflow publishes a vendor tarball and the Debian .deb.
- **Recipes.** These live under `packaging/` in this repository. The AUR recipes live with the
  suite's in `/home/nomadx/sysc/aur/`.
- **Publishing** to COPR, OBS, the AUR and upstream is an owner gate.

**Tech Stack:** GNU make, GitHub Actions, makepkg, nfpm v2.47.0 (sysc-greet's pin), rpmbuild/COPR,
OBS, xbps-src, Gentoo `go-module` eclass, Nix `buildGo126Module`.

**Spec:** [2026-11-02-packaging-design.md](2026-11-02-packaging-design.md). Needs tranche 1 merged
([plan](2026-11-02-own-identity.md)).

## Global Constraints

- License field: `GPL-3.0-or-later`.
- Install layout (design Goal 2):
  - `/usr/bin/sysc-compositor` and `/usr/bin/sysc-compositor-session`;
  - `/usr/share/wayland-sessions/sysc-compositor.desktop`;
  - `/usr/lib/systemd/user/sysc-compositor.service` and `sysc-compositor-shutdown.target`;
  - `/usr/share/xdg-desktop-portal/sysc-portals.conf`;
  - `/usr/share/sysc-compositor/default-config.kdl`;
  - the license directory and the doc directory.
- Nothing goes under `/usr/local` or `/etc` in a package, and nothing collides with the `niri`
  package's files.
- **Run-time dependencies:**
  - wlroots 0.20, SceneFX 0.5, libpipewire ≥ 1.4.8, libxkbcommon, wayland, pixman, libdrm;
  - dbus and xkeyboard-config;
  - libinput once tranche 2 merges.
- **Optional dependencies:** xwayland-satellite ≥ 0.7, xdg-desktop-portal-gnome,
  xdg-desktop-portal-gtk, sysc-shell.
- **Build dependencies:** Go ≥ the `go.mod` line, wayland (`wayland-scanner`), wayland-protocols,
  pkgconf, gcc.
- **Builds are offline** wherever the format requires: Go modules come from
  `sysc-compositor-X.Y.Z-vendor.tar.xz`.
- `CGO_CFLAGS_ALLOW=-fno-strict-overflow` is set by the Makefile.
- Accounts, pushes and upstream submissions are the owner's. Stop at the bd gate.

## Review Focus

1. **Building from the tag tarball** with no `.git`. `--version` prints `0.1.0`, not `(devel)`.
   The test is in Task 1.
2. **`make install PREFIX=/usr`.** The installed `.desktop` `Exec=` and the unit's `ExecStart=`
   say `/usr/bin/…`, and no `@BINDIR@` is left. Task 2.
3. **A distro whose Go is older than `go.mod`'s line.** The build fails with a clear message
   rather than downloading a toolchain (`GOTOOLCHAIN=local`). Task 3 checks each distro's Go.
4. **A system with both `niri` and `sysc-compositor` installed.** No file conflicts, and both
   sessions are listed. Task 4 checks this with pacman.
5. **openSUSE without SceneFX.** The OBS project builds SceneFX first, and the compositor links
   against it. Task 6.

---

### Task 1: Version from ldflags

**Files:**
- Modify: `internal/server/niri.go:28-40` (`Version`), adding `var version string`
- Test: `internal/server/niri_test.go`

- [ ] **Step 1: Write the failing test.** `TestVersionPrefersLinkerVersion` sets `version = "0.1.0"`
  and checks that `Version()` returns `"sysc-compositor 0.1.0 (niri-ipc 26.4.0)"`. With the
  variable empty, it keeps today's build-info behaviour.
- [ ] **Step 2: Run it to see it fail.**
  Run: `timeout 90s env GOMAXPROCS=2 CGO_CFLAGS_ALLOW=-fno-strict-overflow go test -count=1 ./internal/server -run TestVersionPrefersLinkerVersion`.
  Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run it again.** Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: release builds carry their version"`

### Task 2: Makefile install layout

**Files:**
- Modify: `Makefile`, and `share/` files that hold `/usr/local/bin`: the session `.desktop`
  `Exec=` and the unit's `ExecStart=` become `@BINDIR@/…`
- Create: `packaging/check-install.sh`, which asserts the layout of a `DESTDIR`

**Interfaces:**
- Produces:
  - the make variables `PREFIX ?= /usr/local`, `BINDIR = $(PREFIX)/bin`,
    `DATADIR = $(PREFIX)/share`, `LIBDIR = $(PREFIX)/lib`, `DESTDIR`, and
    `VERSION ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//')`;
  - `build` runs `go generate ./internal/wlr`, then
    `go build -ldflags "-X github.com/Nomadcxx/sysc-compositor/internal/server.version=$(VERSION)"`;
  - `install` installs every path in Global Constraints, through `sed 's|@BINDIR@|$(BINDIR)|g'`
    for the `share/` text files. LICENSE and NOTICE go to `$(DATADIR)/licenses/sysc-compositor/`,
    and README to `$(DATADIR)/doc/sysc-compositor/`.

- [ ] **Step 1: Write `packaging/check-install.sh DESTDIR PREFIX`.**
  - It lists the expected files and fails on any missing or extra file.
  - It fails if any installed text file contains `@BINDIR@` or `/usr/local` while PREFIX is not
    `/usr/local`.
  - It checks that `Exec=` equals `PREFIX/bin/sysc-compositor-session`.
- [ ] **Step 2: Run it against today's Makefile to see it fail.**
  Run: `make install PREFIX=/usr DESTDIR=$TMP/root && packaging/check-install.sh $TMP/root /usr`.
  Expected: FAIL (no PREFIX).
- [ ] **Step 3: Implement the Makefile and the `share/` substitution.**
- [ ] **Step 4: Run it again, then check the binary.** Build from `git archive HEAD | tar -x` in a
  temp directory (no `.git`) with `VERSION=0.1.0`, and run
  `$TMP/root/usr/bin/sysc-compositor --version`. Expected: PASS, and it prints `0.1.0`.
- [ ] **Step 5: Commit.** `git commit -m "build: one install layout with PREFIX and DESTDIR"`

### Task 3: Release workflow with the vendor tarball and the Debian .deb

**Files:**
- Create: `.github/workflows/release.yml`, `packaging/deb/nfpm.yaml`
- Modify: `go.mod`'s `go` line, only if Step 1 finds a target distro's Go too old

- [ ] **Step 1: Record each target's Go version and package names.** Run:
  `podman run --rm debian:forky sh -c 'apt-get update -q && apt-cache policy golang-go libwlroots-0.20-dev libscenefx-0.5-dev libpipewire-0.3-dev'`,
  and the matching `dnf info` (fedora:44), `zypper info` (opensuse/tumbleweed) and
  `xbps-query -R` (void) commands.
  - Record the exact package names and Go versions in this plan's execution record.
  - If any Go is below `1.26.4`, find the lowest version the code builds with
    (`go build` with `GOTOOLCHAIN=go1.26.0`, then lower) and set the `go.mod` line to it.
- [ ] **Step 2: Write `packaging/smoke.sh`,** which every format runs inside its installed
  container. It runs:
  1. `sysc-compositor --version` prints the release version.
  2. `sysc-compositor validate -c /usr/share/sysc-compositor/default-config.kdl` exits 0.
  3. In a fresh `XDG_RUNTIME_DIR`, `WLR_BACKENDS=headless WLR_RENDERER=pixman sysc-compositor &`
     runs; after 3 seconds, `NIRI_SOCKET=$(ls $XDG_RUNTIME_DIR/sysc-compositor.*.sock) sysc-compositor msg version`
     exits 0.
  4. It kills the compositor and checks no core dump was left.
- [ ] **Step 3: Write `packaging/deb/nfpm.yaml`.**
  - Its contents come from the `make install` tree at `./root`.
  - `depends` uses Step 1's Debian runtime library names.
  - `recommends` lists xdg-desktop-portal-gnome and xdg-desktop-portal-gtk; `suggests` lists
    xwayland-satellite.
  - The license is GPL-3.0-or-later.
- [ ] **Step 4: Write `release.yml`.** On `push: tags: ["v*"]`:
  1. **The `vendor` job:** runs `go mod vendor` and makes
     `tar -cJf sysc-compositor-$VER-vendor.tar.xz vendor` plus its sha256.
  2. **The `deb` job:** runs in the `debian:forky` container. It installs the build dependencies,
     runs `make build install PREFIX=/usr DESTDIR=root VERSION=$VER`, then
     `nfpm package --packager deb`. It then installs the .deb in a fresh `debian:forky` and
     runs `packaging/smoke.sh`.
  3. **The `release` job:** attaches the vendor tarball, the .deb and `SHA256SUMS` with
     `softprops/action-gh-release@v2`.
- [ ] **Step 5: Prove it.** Run the deb job locally in podman against a test tag built from
  `git archive`. Expected: the .deb installs, and the smoke check passes.
- [ ] **Step 6: Commit.** `git commit -m "ci: release vendor tarball, Debian package and smoke check"`

### Task 4: AUR recipes

**Files** (in `/home/nomadx/sysc/aur/`, following `sysc-1705`):
- Create: `sysc-compositor/{PKGBUILD,.SRCINFO,sysc-compositor.install}` and
  `sysc-compositor-git/{PKGBUILD,.SRCINFO,sysc-compositor-git.install}`
- Modify: `check.py` (`assert len(packages) == 10`), and `sysc-shell/PKGBUILD` (drop `niri` from
  `depends`; add optdepends `sysc-compositor` and `niri`; pkgdesc "Desktop shell for Niri and
  sysc-compositor")

- [ ] **Step 1: Write the PKGBUILDs.**
  - `depends`: `wlroots0.20 scenefx libpipewire libxkbcommon wayland pixman libdrm dbus xkeyboard-config`.
  - `optdepends`: `xwayland-satellite`, `xdg-desktop-portal-gnome`, `xdg-desktop-portal-gtk`
    and `sysc-shell`, each with a reason.
  - `makedepends`: `go wayland-protocols`.
  - `license=('GPL-3.0-or-later')`.
  - `build()` exports the suite GOFLAGS and `GOTOOLCHAIN=local`, then runs
    `make build VERSION=$pkgver`.
  - `package()` runs `make install PREFIX=/usr DESTDIR="$pkgdir"`.
  - The `-git` recipe has `provides=(sysc-compositor) conflicts=(sysc-compositor)`, and its
    `pkgver()` uses `git describe --long --tags | sed 's/^v//;s/-/.r/;s/-/./'`.
  - The `.install` hook follows the existing banner hook shape. That is ≤ 7 lines, linking the
    docs site, so `check.py` passes.
- [ ] **Step 2: Build and check.** Run `makepkg -f && makepkg --printsrcinfo > .SRCINFO` in each
  directory, then `python3 aur/check.py`. Expected: PASS.
- [ ] **Step 3: Check alongside Niri.** In the session VM: `pacman -U` the package with `niri`
  installed, then run `pacman -Qkk sysc-compositor`. Expected: no conflict errors, and both
  `.desktop` sessions in `/usr/share/wayland-sessions`.
- [ ] **Step 4: Commit** in the suite repository, following `sysc-1705`'s tracked-recipes rule.
  The AUR push is an owner gate.

### Task 5: Fedora 44 spec (COPR)

**Files:**
- Create: `packaging/rpm/sysc-compositor.spec`, written for Fedora and Tumbleweed with
  `%if 0%{?suse_version}` blocks for package names.

- [ ] **Step 1: Write the spec.**
  - `Source0` is the tag tarball and `Source1` the vendor tarball.
  - `%prep` unpacks the vendor tarball into the source tree.
  - `%build` runs `make build VERSION=%{version} GOFLAGS=-mod=vendor`.
  - `%install` runs `make install PREFIX=%{_prefix} DESTDIR=%{buildroot}`.
  - `%files` lists Goal 2's paths; `%license LICENSE NOTICE`.
  - `BuildRequires` and `Requires` use Task 3 Step 1's Fedora names.
- [ ] **Step 2: Build in `fedora:44`.** Run `rpmbuild -ba` there, install the RPM in a fresh
  `fedora:44`, and run `packaging/smoke.sh`. Expected: PASS.
- [ ] **Step 3: Commit.** The COPR project `nomadcxx/sysc` is an owner gate.

### Task 6: openSUSE Tumbleweed (OBS), including SceneFX

**Files:**
- Create: `packaging/obs/scenefx/scenefx.spec`, for SceneFX 0.5 from its upstream release tag;
  meson; it builds against Tumbleweed's `wlroots-devel` 0.20.

- [ ] **Step 1: Build SceneFX.** Build `scenefx.spec`, then `sysc-compositor.spec` (Task 5), in
  `opensuse/tumbleweed` with `rpmbuild`, installing the SceneFX RPM in between.
- [ ] **Step 2: Install and check.** Install both in a fresh Tumbleweed and run `packaging/smoke.sh`. Expected: PASS.
- [ ] **Step 3: Commit.** The OBS project `home:nomadcxx:sysc` is an owner gate.

### Task 7: Void, Gentoo and Nix

**Files:**
- Create:
  - `packaging/void/srcpkgs/sysc-compositor/template`;
  - `packaging/gentoo/gui-wm/sysc-compositor/sysc-compositor-0.1.0.ebuild` and `metadata.xml`;
  - `flake.nix` and `flake.lock`, with `nixosModules.default`.

- [ ] **Step 1: Void.**
  - The template uses `build_style=go`, overrides `do_build`/`do_install` with the make targets,
    sets `depends` from Task 3's Void names, and `license="GPL-3.0-or-later"`.
  - Build it with `xbps-src pkg sysc-compositor` in a void-packages checkout inside
    `ghcr.io/void-linux/void-glibc`.
  - Install it, then run `smoke.sh`.
- [ ] **Step 2: Gentoo.**
  - The ebuild inherits `go-module`, has `SRC_URI` with the tag and the vendor tarball,
    `RDEPEND` with `gui-libs/wlroots:0.20` and `gui-libs/scenefx` (GURU), and
    `LICENSE="GPL-3+"`.
  - Run `ebuild … manifest install` in a `gentoo/stage3` container with GURU enabled, then run
    `smoke.sh`.
- [ ] **Step 3: Nix.**
  - `flake.nix` uses `buildGo126Module` with `vendorHash`, `nativeBuildInputs` of `pkg-config` and
    `wayland-scanner`, and `buildInputs` of `wlroots_0_20`, `scenefx`, `pipewire`,
    `libxkbcommon`, `pixman` and `libdrm`.
  - It sets `passthru.providedSessions = ["sysc-compositor"]`.
  - `nixosModules.default` sets `services.displayManager.sessionPackages` and
    `xdg.portal.configPackages`.
  - Run `nix build` and `nix flake check`; then run `smoke.sh` via
    `nix run .#default -- --version` and the others in a `nixos/nix` container.
- [ ] **Step 4: Commit.** `git commit -m "build: Void, Gentoo and Nix recipes"`

### Task 8: First release

- [ ] **Step 1: Record the matrix.** Add a "Packaging" row to `niri-parity-matrix.md`'s session
  section, and an "Install" section to README with each route.
- [ ] **Step 2: Tag `v0.1.0`,** an owner gate: tagging publishes. Watch `release.yml`, and confirm
  the release has the vendor tarball, the .deb and SHA256SUMS.
- [ ] **Step 3: Update the AUR checksums** from the tag tarball. Then the owner gates: AUR push,
  COPR, OBS and GURU. Record each one's result in this plan's execution record.
