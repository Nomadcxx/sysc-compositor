GO ?= go
DESTDIR ?=

.PHONY: build install

build:
	mkdir -p bin
	$(GO) build -o bin/sysc-compositor ./cmd/sysc-compositor

install: build
	install -D -m 0755 bin/sysc-compositor "$(DESTDIR)/usr/local/bin/sysc-compositor"
	install -D -m 0644 share/wayland-sessions/sysc-compositor.desktop "$(DESTDIR)/usr/share/wayland-sessions/sysc-compositor.desktop"
	install -D -m 0755 share/bin/sysc-compositor-session "$(DESTDIR)/usr/local/bin/sysc-compositor-session"
	install -D -m 0644 share/systemd/user/sysc-compositor.service "$(DESTDIR)/usr/lib/systemd/user/sysc-compositor.service"
	install -D -m 0644 share/systemd/user/sysc-compositor-shutdown.target "$(DESTDIR)/usr/lib/systemd/user/sysc-compositor-shutdown.target"
