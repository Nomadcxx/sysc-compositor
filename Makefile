GO ?= go
DESTDIR ?=

.PHONY: build install

build:
	mkdir -p bin
	$(GO) build -o bin/sysc-compositor ./cmd/sysc-compositor

install: build
	install -D -m 0755 bin/sysc-compositor "$(DESTDIR)/usr/local/bin/sysc-compositor"
	install -D -m 0644 share/wayland-sessions/sysc-compositor.desktop "$(DESTDIR)/usr/share/wayland-sessions/sysc-compositor.desktop"
	install -D -m 0644 share/systemd/user/sysc-session.target "$(DESTDIR)/usr/lib/systemd/user/sysc-session.target"
