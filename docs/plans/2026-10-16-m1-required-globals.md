# M1: required globals (TDD plan)

**Kind:** plan (milestone).
**Design:** `2026-10-02-sysc-compositor-design.md` §7 and §15 M1, and the shell contract at `:87`.
**Inputs:** `2026-10-16-input-pattern-comparison-report.md` X1 and C1.
**bd:** `sysc-1266` (M1) and `sysc-1297` (popups, which gates M1).
**Baseline:** `origin/main` `7055566`.

## Exit gate

Design §15: *layer-shell v4, fractional-scale, viewporter, ARGB8888, xdg-shell, dmabuf.*

Each global must be advertised **and** behave for a real client:
- a layer surface is configured, placed by its anchors, and moves the usable area by its exclusive zone;
- a surface gets its output's fractional scale;
- a dmabuf client receives feedback and maps;
- an xdg popup draws beside its parent, inside the output.

The set must satisfy sysc-shell's `requiredSingletons` and `wl_shm` ARGB8888
(`sysc-shell/internal/platform/wayland/registry.go`, `origin/main` `ad56fbcd`). The shell starting and
drawing is M3, not this gate.

## Decisions

| Item | Choice | Why |
|---|---|---|
| `zwlr_layer_shell_v1` | version 5 | The host Niri advertises 5 (`wayland-info`, 2026-10-16); design §7's v4 is the minimum, and the shell binds at most 4. v5 adds `set_exclusive_edge`, which wlroots' scene configure honours. |
| `xdg_wm_base` | version 7, up from 5 | The host Niri advertises 7, and wlroots 0.20's maximum is 7. |
| `wl_compositor` | version 6, up from 5 | v6 is `preferred_buffer_scale`; the shell binds up to 6. |
| `wp_fractional_scale_manager_v1` | version 1, and wlroots' scene sends the scale | `types/scene/surface.c:135` (0.20.0) calls `wlr_fractional_scale_v1_notify_scale` on output enter. |
| `zwp_linux_dmabuf_v1` | version 5 with renderer formats, plus `wlr_scene_set_linux_dmabuf_v1` for feedback | Failure is an error from `New`, because the global is required. |
| `wl_shm` | version 2, up from 1 | Niri advertises 2 (`release`). `wlr_shm_create_with_renderer` always offers ARGB8888 and XRGB8888; the test pins it. |
| Scene order | trees for background, bottom, windows, top and overlay, under the scene root | The layer protocol's stacking. Toplevels move from the root into `windows`. |
| Layer arrangement | per output, `wlr_scene_layer_surface_v1_configure` with exclusive surfaces first, then the rest against the remaining usable area | The same as Mango and tinywl-derived compositors; the usable area is kept per output for M4/M5 window placement. |
| Layer surface with no output | the output under the pointer, else the first; closed if there are none | The protocol lets the compositor choose. |
| Layer keyboard interactivity | **not in M1** | The protocol says "should". The shell's panels need it at M3, so it is noted on `sysc-1268`. |
| Popups | tree under the parent's tree on the initial commit, then `wlr_xdg_popup_unconstrain_from_box` against the output's box, in parent-surface coordinates | Report X1; Mango `manage/layer.c:418-448`. Popups of layer surfaces use the same path. |
| Popup grabs | not in M1 | A popup without its grab still draws and dismisses on the client's own terms. Grabs land with pointer correctness (`sysc-1296`). |

| `zxdg_output_manager_v1` | version 3, added during execution | waybar refuses to start without it ("Failed to acquire required resources"), grim needs it, and the host Niri advertises v3. One wlroots call over the output layout. |
| Protocol headers | `protocols/wlr-layer-shell-unstable-v1.xml` from the wlroots 0.20.0 tag; `go generate ./internal/wlr` writes the header into the gitignored `internal/wlr/protocols/` | `wlr_layer_shell_v1.h` includes a header wlroots does not install. Design §14 is amended; generated output is still never checked in. |

**Test clients**, all installed here: `wayland-info` (globals), `swaybg` (a background layer anchored to
every edge), `waybar` with a temporary config (a top layer with an exclusive zone), `foot` (fractional
scale), `weston-simple-dmabuf-feedback` (dmabuf), and `weston-terminal`, whose right-click menu is an
`xdg_popup`.

A Go test client would need xdg and layer bindings that the pinned sysc-wayland does not ship, so it is not
used.

## Task 1: test harness and the globals list

**RED:** `TestRequiredGlobalsAdvertised` runs `wayland-info` against the nested socket and requires:
- `wl_compositor` ≥ 6, `wl_subcompositor`, `wl_shm` with format 0 (ARGB8888), `wl_seat`, `wl_output`;
- `xdg_wm_base` = 7, `zwlr_layer_shell_v1` = 5, `wl_shm` = 2;
- `wp_fractional_scale_manager_v1`, `wp_viewporter`, `zwp_linux_dmabuf_v1` ≥ 4, `wl_data_device_manager`,
  `zxdg_output_manager_v1` ≥ 3.

**GREEN:**
- a shared `runClient(t, c, argv, env, stop func(line string) bool)` helper in `client_test.go`;
- the version bumps;
- `wlr_layer_shell_v1_create(display, 5)`, `wlr_fractional_scale_manager_v1_create(display, 1)`,
  `wlr_linux_dmabuf_v1_create_with_renderer(display, 5, renderer)` and `wlr_scene_set_linux_dmabuf_v1`.

**Commit:** `feat(wlr): advertise the M1 globals`

## Task 2: scene layers and popups (`sysc-1297`)

**RED:** `TestPopupDrawsBesideItsParent`:
1. weston-terminal maps.
2. The test focuses it and sends a right button press and release through the user seat at the window's
   centre (`wlr_seat_pointer_notify_*`, the path a real click takes).
3. On the popup's first commit, assert that its scene tree's parent is the toplevel's tree, not the root,
   and that its layout box lies inside the output.

**GREEN:**
- background, bottom, windows, top and overlay trees;
- toplevels into `windows`;
- `new_popup` creates the tree under `popup->parent`'s tree on the initial commit and unconstrains it;
- `handleNewSurface` stops creating trees for popups.

**Commit:** `fix(wlr): draw xdg popups under their parent and inside the output`

## Task 3: layer-shell surfaces and the usable area

**RED:**
- `TestLayerSurfaceFillsItsOutput`: swaybg on a nested output gets
  `zwlr_layer_surface_v1.configure(serial, W, H)` with the output's size, and its tree maps under the
  background tree.
- `TestExclusiveZoneShrinksUsableArea`: waybar with `{"layer":"top","position":"top","height":30}` maps
  under the top tree. The output's usable area becomes `(0, 30, W, H-30)`, and closing waybar restores it.

**GREEN:**
- `new_surface` for layer surfaces, with output assignment;
- `wlr_scene_layer_surface_v1_create` under the layer's tree;
- commit, map and unmap re-arrange the output;
- `arrangeLayers(output)` keeps `usable` per output;
- an output being destroyed closes its layer surfaces.

**Commit:** `feat(wlr): place layer-shell surfaces and keep each output's usable area`

## Task 4: fractional scale and dmabuf behave

**RED:**
- `TestFractionalScaleFollowsOutput`: foot on an output at scale 1 receives `preferred_scale(120)`; with the
  output's scale set to 1.5 through a test helper, `preferred_scale(180)`.
- `TestDmabufClientMaps`: `weston-simple-dmabuf-feedback` receives `zwp_linux_dmabuf_feedback_v1.done` and
  maps a surface.

**GREEN:**
- the test helper `setOutputScale` (an output state commit);
- the xcursor manager loads at each output's scale (report C1), so the pointer is not blurred at 1.5.

**Commit:** `test(wlr): prove fractional scale and dmabuf with real clients`

## Gates

Per `AGENTS.md`, one package and named tests only:

```
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/wlr -run '<Name>'
```

No `-race` (cgo), no `./...`. `gofmt -l .` is empty, and `GOMAXPROCS=2 go vet -p 2 ./...` passes.

**Live gate** on the laptop (`ssh -p 7777 nomadx@192.168.0.64`):
1. Run the named tests from a `git archive` copy under Niri.
2. Run `cmd/sysc-compositor` nested, with swaybg, waybar and foot inside it, and take a grim shot of the
   nested window.
3. Run `wayland-info` against the nested socket.

Record the results in this plan's execution record.

## Stop condition

Stop at the exit gate. Not in M1:
- keyboard interactivity for layer surfaces (M3);
- popup grabs and pointer focus (`sysc-1296`, before M4);
- window placement inside the usable area (M4/M5);
- any IPC (M2).
