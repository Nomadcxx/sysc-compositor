# Protocol XML

Protocols whose server header wlroots does not install. `go generate ./internal/wlr` turns each file
here into a header under `internal/wlr/protocols/`; the headers are not checked in (design §14).

Files supplied by wlroots are copied unchanged from the wlroots tag the installed library was built
from. Other files record their upstream protocol release. Generated headers match the protocol XML.

| File | Source | sha256 |
|---|---|---|
| `wlr-layer-shell-unstable-v1.xml` | wlroots `0.20.0`, `protocol/wlr-layer-shell-unstable-v1.xml` (version 5) | `87e0b9c837aecd6977f76f3c47d73088b7159871f5d979dc1840f6cadb5e2ed8` |
| `ext-background-effect-v1.xml` | wayland-protocols `1.49`, `staging/ext-background-effect/ext-background-effect-v1.xml` | `e463b7863c97d7be05489b52b15cd0b3a5d8290b51340f62023b47d03b4217e3` |
| `wlr-output-power-management-unstable-v1.xml` | mangowm `0.18.0`, `protocols/wlr-output-power-management-unstable-v1.xml` (wlroots 0.20 reference) | `4a415d02b8e6b1650aea9f352d5381411166cb8221ab9cb92af57193a1c5918d` |
