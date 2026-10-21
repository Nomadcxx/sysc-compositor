# M8 Springs and Window Appearance Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Apply the M8 portion of the Niri 26.04 contract for the laptop config: spring animations, focus ring, shadow, geometry corner radius, clipping, and `prefer-no-csd`.

**Architecture:** `internal/config` owns validated KDL values. A pure Go animation value computes spring positions from monotonic time. `internal/wlr` owns animation timing and scene updates on the compositor thread, using SceneFX for shadows and corner clipping. wlroots lifecycle stays with the existing toplevel and surface owners.

**Tech Stack:** Go 1.26, cgo, wlroots 0.20, installed SceneFX 0.5, KDL v1, Niri 26.04 (`8ed0da44d974c32c6877d2f4630c314da0717ecb`), Mango 0.18.0 (`a21b10fcbe6156b76ea96178d1941de5cb36137f`).

---

## Scope and decisions

- Follow product contract §9 and §15. Niri defines KDL and behavior; Mango is only a reference for wlroots 0.20 and SceneFX calls.
- Apply the seven animation kinds set in `/home/nomadx/.config/niri/config.kdl`: `window-open`, `window-close`, `window-resize`, `window-movement`, `overview-open-close`, `workspace-switch`, and `horizontal-view-movement`.
- Decode global `animations off` for the greeter and spring parameters for the configured kinds. Keep unimplemented easing, slowdown, and custom shaders visible as warnings; do not report them as applied.
- Decode the laptop's focus-ring width/colors, shadow softness/spread/offset, window-rule `geometry-corner-radius` and `clip-to-geometry`, and top-level `prefer-no-csd`.
- Use the installed SceneFX 0.5 library only after the nested renderer gate proves compatibility with the current wlroots 0.20 output path. This adds a system library requirement; do not add a Go module.
- Use a single compositor-owned animation tick. Timer callbacks post work through `Compositor.Post`; every wlroots and SceneFX call stays on the locked compositor thread.
- Keep window rules ordered and apply the last matching values, following existing `ResolveWindowRules` behavior. Preserve requested client-side decorations when a client explicitly requests them after the initial server-side preference.
- Do not implement custom shaders, unrelated animation kinds, broad appearance options, or config reload in M8.

## Task 1: Prove SceneFX works with the existing nested renderer path

**Files:**
- Modify `internal/wlr/compositor.go` only as needed to create the SceneFX renderer.
- Add the smallest nested regression in `internal/wlr/`.

1. Add a failing nested check that creates the SceneFX renderer, commits the existing scene output, and renders a SceneFX shadow and rounded buffer node.
2. Run the named `internal/wlr` test and inspect output/frame progress.
3. Link the installed `scenefx-0.5` package through cgo and use `fx_renderer_create` only if the nested scene renders.
4. Stop and revise the effect implementation if SceneFX cannot render through the compositor's current backend/output path.

## Task 2: Add a pure spring value and focused math checks

**Files:**
- Add `internal/animation/spring.go` and `internal/animation/spring_test.go`.

1. Add checks for the initial and target values, critical damping, underdamped motion, velocity, non-finite inputs, and exact completion at the configured epsilon.
2. Implement the Niri 26.04 spring equation with mass 1, damping ratio, stiffness, epsilon, initial velocity, and elapsed monotonic time.
3. Keep time outside the spring value so the compositor can sample it deterministically.
4. Run named `internal/animation` checks.

## Task 3: Decode M8 KDL settings

**Files:**
- Modify `internal/config/config.go`, its tests, and `internal/config/rules.go` as needed.
- Modify `internal/server/server.go` and `internal/wlr/compositor.go` to pass the values.

1. Add failing KDL cases for the seven spring entries, top-level and per-kind `off`, greeter `animations { off }`, focus-ring settings, shadow settings, geometry radius, clipping, and `prefer-no-csd`.
2. Validate finite numeric values and Niri's documented ranges. Accept both one radius and four corner radii in CSS order.
3. Preserve defaults when a section is absent; make repeated window rules resolve in order.
4. Keep unsupported animation shapes and appearance options visible in `Warnings`.
5. Run named `internal/config` and `internal/server` checks, one package per command.

## Task 4: Render configured window appearance

**Files:**
- Modify `internal/wlr/compositor.go`, `internal/wlr/input.go`, and the WLR window/surface lifecycle code.
- Add named nested checks in `internal/wlr/`.

1. Create a compositor-owned focus-ring scene node and SceneFX shadow for mapped toplevel windows; update position, size, focus color, and cleanup with the existing view lifecycle.
2. Apply rule-resolved corner radii to buffers in the window's visual geometry. Clip only when `clip-to-geometry` is true; leave popups and unrelated layer surfaces unchanged.
3. Place shadow and focus-ring nodes relative to the toplevel geometry, including shadow softness, spread, offset, and alpha.
4. Add a nested xdg-decoration test for server-side initial preference and a client request for client-side decoration.
5. Run each named WLR check individually.

## Task 5: Animate the configured compositor transitions

**Files:**
- Modify `internal/wlr/compositor.go`, `internal/wlr/workspaces.go`, `internal/wlr/overview.go`, and `internal/wlr/input.go` as needed.
- Add focused pure and nested tests.

1. Add failure-first checks for overview open/close, workspace switching, horizontal view movement, window open/close, resize, and movement.
2. Retarget an in-flight spring from its sampled value and velocity; synchronize layout changes caused by resize under the `window-resize` spring.
3. Animate overview progress after toggle actions while keeping four-finger updates direct and release velocity intact.
4. Hold a referenced scene buffer for a closing window only until its close animation completes; release it on completion or compositor teardown.
5. Keep layout and IPC snapshots at their committed target values; animate scene presentation on the compositor thread.
6. Run named `internal/wm`, `internal/animation`, and `internal/wlr` checks individually.

## Task 6: Run the M8 gates and record proved coverage

**Files:**
- Update `docs/plans/niri-parity-matrix.md`.
- Update this plan with execution evidence.

1. Run `gofmt` on changed Go files and `git diff --check`.
2. Run named checks in `internal/animation`, `internal/config`, `internal/server`, and `internal/wlr`, one package per command. Never run `go test ./...` or `-race`.
3. Run `go vet -p 2 ./...`; build `./cmd/niri` and `./cmd/sysc-compositor`; confirm `go.mod` and `go.sum` are unchanged.
4. Validate the laptop's actual Niri config without starting its startup commands. Start a nested compositor with a temporary M8 config and prove the seven configured transitions, focus ring, shadow, clipping, and decoration preference with a mapped client.
5. Do not install the binary, alter greetd, replace the live compositor, or modify the user's config. Update the parity matrix only for behavior proved by focused or nested checks.

**M8 stop condition:** The SceneFX renderer passes the nested output gate; the spring math and config checks pass; all seven configured transitions run and settle; appearance and decoration behavior pass nested assertions; the laptop config parses; and the parity matrix records unsupported easing/custom-shader and hardware limits. Stop before M9 session and reload work.

## Execution record — 2026-10-22

- **Task 1:** `TestSceneFXRendererRendersNestedSceneEffects` proved that the SceneFX renderer commits output frames with a shadow and rounded rectangle. `TestWindowEffectsRenderForMappedToplevel` confirmed rounded radii on a mapped client buffer.
- **Task 2:** Named spring checks passed for initial/target values, underdamped motion, velocity, invalid values, and exact settling.
- **Task 3:** Named config checks passed for the seven laptop springs, animation off, appearance settings, one and four corner radii, ordered rules, `prefer-no-csd false`, invalid values, and warnings. The laptop config validated; remaining unapplied sections and bind actions were reported.
- **Task 4:** Nested checks passed for mapped focus ring, shadow, surface corner radii and clipping, fullscreen suppression, and server-side preferences through both xdg-decoration and KDE server-decoration. Both protocols accept a later client-side request. The focus ring uses one rounded SceneFX rectangle with its inner geometry clipped out.
- **Task 5:** Named WM, animation, and nested WLR checks passed for overview open/close, workspace switching, horizontal view movement, window open/close, window movement, resize, and spring retargeting. Layout snapshots retain their committed target values.
- **Task 6:** `gofmt`, `git diff --check`, named checks in `internal/animation`, `internal/config`, `internal/server`, `internal/wm`, and `internal/wlr`, `go vet -p 2 ./...`, and builds of `./cmd/niri` and `./cmd/sysc-compositor` passed. `go.mod` and `go.sum` are unchanged.

The follow-up nested checks found three edge cases. Resize decorations now track the animated snapshot on each spring tick. Rounded corners apply to subsurface corners that align with a window's outer bounds, while interior subsurfaces remain square. A 48×30 child extending past a 100×60 window is clipped to 20×10 at the lower-right edge; `sceneFXCloneView` copies those effective scene-buffer bounds, so close and resize snapshots need no separate clip state.

The host nested renderer has one output. Multi-output, mixed-DPI, VRR, direct-session rendering, and pixel-level visual calibration remain unverified. SceneFX exposes one shadow corner radius; nonuniform window corner rules use their top-left radius for the shadow. Easing, slowdown, custom shaders, and the four recognized animation kinds outside this laptop config remain unapplied and produce warnings. M8 stops before M9 session and reload work.
