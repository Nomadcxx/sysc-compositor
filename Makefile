GO ?= go
DESTDIR ?=
PREFIX ?= /usr/local
BINDIR = $(PREFIX)/bin
DATADIR = $(PREFIX)/share
LIBDIR = $(PREFIX)/lib
# A release tarball has no .git: its recipe passes VERSION. VERSION replaces
# the VCS stamp, which a tarball unpacked under another repository (an AUR
# clone) would take from the wrong tree or fail on.
VERSION ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//')
# libspa's pkg-config metadata supplies this flag; cgo needs an exact allowance.
export CGO_CFLAGS_ALLOW ?= -fno-strict-overflow

.PHONY: build install

build:
	$(GO) generate ./internal/wlr
	mkdir -p bin
	$(GO) build -buildvcs=false -ldflags "-X github.com/Nomadcxx/sysc-compositor/internal/server.version=$(VERSION)" -o bin/sysc-compositor ./cmd/sysc-compositor

# Text files under share/ name the binaries as @BINDIR@.
SUBST = sed 's|@BINDIR@|$(BINDIR)|g'

install: build
	install -D -m 0755 bin/sysc-compositor "$(DESTDIR)$(BINDIR)/sysc-compositor"
	install -d "$(DESTDIR)$(DATADIR)/wayland-sessions" "$(DESTDIR)$(LIBDIR)/systemd/user"
	$(SUBST) share/bin/sysc-compositor-session >"$(DESTDIR)$(BINDIR)/sysc-compositor-session"
	chmod 0755 "$(DESTDIR)$(BINDIR)/sysc-compositor-session"
	$(SUBST) share/wayland-sessions/sysc-compositor.desktop >"$(DESTDIR)$(DATADIR)/wayland-sessions/sysc-compositor.desktop"
	$(SUBST) share/systemd/user/sysc-compositor.service >"$(DESTDIR)$(LIBDIR)/systemd/user/sysc-compositor.service"
	chmod 0644 "$(DESTDIR)$(DATADIR)/wayland-sessions/sysc-compositor.desktop" "$(DESTDIR)$(LIBDIR)/systemd/user/sysc-compositor.service"
	install -D -m 0644 share/systemd/user/sysc-compositor-shutdown.target "$(DESTDIR)$(LIBDIR)/systemd/user/sysc-compositor-shutdown.target"
	install -D -m 0644 share/xdg-desktop-portal/sysc-portals.conf "$(DESTDIR)$(DATADIR)/xdg-desktop-portal/sysc-portals.conf"
	install -D -m 0644 internal/config/default-config.kdl "$(DESTDIR)$(DATADIR)/sysc-compositor/default-config.kdl"
	install -D -m 0644 LICENSE "$(DESTDIR)$(DATADIR)/licenses/sysc-compositor/LICENSE"
	install -D -m 0644 NOTICE "$(DESTDIR)$(DATADIR)/licenses/sysc-compositor/NOTICE"
	install -D -m 0644 README.md "$(DESTDIR)$(DATADIR)/doc/sysc-compositor/README.md"
