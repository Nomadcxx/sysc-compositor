# Protocol XML

Protocols whose server header wlroots 0.20 does not install. `go generate ./internal/wlr` turns each file
here into a header under `internal/wlr/protocols/`; the headers are not checked in (design §14).

Each file is copied unchanged from the wlroots tag the installed library was built from, so the generated
header matches the structs wlroots compiled against.

| File | Source | sha256 |
|---|---|---|
| `wlr-layer-shell-unstable-v1.xml` | wlroots `0.20.0`, `protocol/wlr-layer-shell-unstable-v1.xml` (version 5) | `87e0b9c837aecd6977f76f3c47d73088b7159871f5d979dc1840f6cadb5e2ed8` |
