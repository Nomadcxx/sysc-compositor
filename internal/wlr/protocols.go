package wlr

// Some wlroots headers include a protocol header that wayland-scanner writes
// while wlroots builds and that wlroots does not install. The header is
// generated here from the protocol XML wlroots 0.20.0 was built with
// (protocols/README.md) into internal/wlr/protocols, which is not checked in.
// Run `go generate ./internal/wlr` once after cloning.

//go:generate mkdir -p protocols
//go:generate wayland-scanner server-header ../../protocols/wlr-layer-shell-unstable-v1.xml protocols/wlr-layer-shell-unstable-v1-protocol.h
//go:generate wayland-scanner server-header ../../protocols/ext-background-effect-v1.xml protocols/ext-background-effect-v1-protocol.h
//go:generate wayland-scanner server-header ../../protocols/wlr-output-power-management-unstable-v1.xml protocols/wlr-output-power-management-unstable-v1-protocol.h
