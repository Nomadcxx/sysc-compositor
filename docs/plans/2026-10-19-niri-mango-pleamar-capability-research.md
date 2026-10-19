# Niri, Mango and Pleamar capability research

**Kind:** report. **Snapshot:** 2026-10-19.

This report maps the remaining Niri compatibility work, compares Niri and Mango by subsystem, records
the current sysc-compositor architecture, and treats Pleamar/pleamar-wm as capability references. It
uses Hyprland as a bounded rendering and performance reference where relevant.

## Decision frame

- **Niri defines the outward shape.** Preserve the Niri window and workspace model, KDL compatibility,
  `niri-ipc` JSON contract, CLI behavior, and the existing sysc-shell consumer contract. This is the
  known-working shape for the desktop.
- **Mango informs wlroots 0.20 mechanics.** Borrow implementation patterns for wlroots APIs, listeners,
  input devices, scene integration, and protocol setup. Its tags, `mmsg`, and layout model are not
  parity requirements.
- **Pleamar and pleamar-wm are capability references.** They show useful outcomes and authoring
  experiences. Their Rust, Luau, Smithay, and wgpu implementation is not a porting target.
- **Hyprland is a secondary systems reference.** At the inspected commit it uses Aquamarine rather
  than wlroots, so it can inform rendering, damage, capture, and performance questions but not direct
  wlroots call patterns.
- **Better than either reference is allowed.** A departure needs a concrete user benefit, a clear
  owner, compatibility boundaries, and an acceptance check. Reference choice is per subsystem.

“Niri + Mango parity” therefore means Niri-compatible behavior and IPC implemented with robust
wlroots practices. It does not mean adopting Mango's user model or copying both projects' feature
lists wholesale.

## Current sysc-compositor shape

The M3 worktree is based on `a9b3ce9` with the nested-output-close fix `bb110cb` and local M3 changes.
M0–M2 foundations are in the baseline. The laptop gate passed on 2026-10-19 in a nested
sysc-compositor session launched from the testbench's host Niri session: sysc-shell connected and drew
its bar and Settings panel, Niri state requests and EventStream returned the expected state, and
`niri validate` accepted the laptop configuration. The two-frame pixel check used host Niri because
direct `grim` capture is unsupported inside the nested compositor. Mango remained the greetd host;
no installed compositor or greetd configuration was changed.

The Go ownership map is currently:

```text
cmd/niri + cmd/sysc-compositor
          │
internal/cli ─ internal/config ─ internal/server ─ internal/ipc / unixsock
                                      │
                                 internal/wlr (cgo → wlroots 0.20)
                                      │
                             Wayland clients and outputs

internal/control: separate local control protocol and injected-input plumbing
```

`internal/wlr` owns the native Wayland/wlroots objects on the locked compositor thread. `internal/server`
copies compositor observations into immutable Go snapshots; IPC goroutines read those snapshots and do
not call wlroots. `internal/config` parses Niri-shaped KDL, and the shared CLI serves both binary names.
The M3 changes add output/layer snapshots and a bounded EventStream implementation.

There is not yet an `internal/wm` policy package or a window-layout engine. The current Niri state has
empty window snapshots and provisional workspace state; mapping, stable window identity, focus policy,
and layout are later work. This is the key architecture boundary to settle before the M5 layout milestone.

## Remaining Niri implementation

The exact request, event, action, configuration, CLI, and D-Bus rows remain in
[`niri-parity-matrix.md`](niri-parity-matrix.md). The milestone grouping is:

| Work | Remaining behavior |
|---|---|
| **M3 gate** | Passed 2026-10-19: nested sysc-compositor and sysc-shell on the laptop testbench; state queries, layers, EventStream, configuration validation, and two visible frames are recorded in the M3 plan. |
| **M4** | Map xdg toplevels into IPC, stable window IDs, focus/close/workspace actions, and window events. |
| **M5–M6** | Niri scrollable columns and dynamic workspaces; core binds; focus/movement; consume/expel; tabs; floating/fullscreen; named workspaces; monitor movement; window/layer rules. |
| **M7–M8** | Overview, gestures, insert hints, spring animations, focus visuals, shadows, radius, clipping, and the Niri-compatible visual behavior listed in the design. |
| **M9** | User-session `--session`, systemd environment and targets, complete CLI/IPC requests, config reload and `LoadConfigFile`. |
| **M10–M14** | Remaining input and shell protocols, lock/idle, screenshots and hotkey UI, portals/PipeWire capture, Xwayland, and accessibility/Orca. |
| **M15** | Output management, VRR, mixed-DPI and recovery cases, remaining actions and requests, then close the full parity matrix. |

The M3 plan records the laptop run and its limits. Local checks and config validation were paired with
the live nested session; the screenshot capture path went through host Niri because nested `grim` is
unsupported.

## Niri and Mango: who is the better reference

| Area | Niri | Mango / wlroots | sysc-compositor choice |
|---|---|---|---|
| Window model, workspaces, focus semantics | Defines the desired scrollable columns, dynamic workspaces, action behavior, and state replies. | Offers different tags and many layouts. Those are distinct product decisions. | **Niri.** Keep its model as the default and its `niri-ipc` replies/events as the shell contract. |
| Config and CLI | KDL schema, include behavior, action vocabulary, CLI and Niri-compatible config are the target. | `config.conf` and `mmsg` are Mango-specific. | **Niri.** Do not make current Niri configs or sysc-shell migrate. Keep any future sysc-only syntax isolated. |
| IPC and event state | Typed request/reply schema, initial EventStream state and event semantics are specified by `niri-ipc`. | No matching public IPC contract to adopt. | **Niri.** The Go state snapshot and stream implementation can improve internals without changing wire behavior. |
| wlroots object lifecycle, globals, scene and input calls | Smithay is a different backend; it is useful for behavior but not the same API. | Runs on the same wlroots 0.20 substrate and shows concrete native calls and setup order. | **Mango plus wlroots 0.20 docs/specs.** Verify each call against the installed wlroots version; treat Mango as a working example, not authority. |
| Input, keyboard groups, pointer grabs, popup hit-testing | Best source for the behavior users expect. | Best direct example for wlroots device/seat operations. | **Both by role.** The existing input comparison report already maps these findings to milestones; extend it only when new evidence appears. |
| Rendering, capture and performance | Best source when matching Niri's visible result or its standard capture behavior. | Useful wlroots scene/protocol examples. | **Protocol spec + wlroots first; Mango for calls; Hyprland and Pleamar for bounded rendering/perf ideas.** Measure before adopting architectural changes. |
| User-authored WM behavior | Niri KDL provides binds, rules, layout knobs and motion settings, but is not a general scene language. | Mango ships fixed C policy/layouts, not the desired user-authored scene model. | **Neither.** Pleamar-wm is the capability reference; implementation requires a new Go-compatible design. |

The existing [input comparison report](2026-10-16-input-pattern-comparison-report.md) already records
the detailed keyboard, focus, pointer-grab, popup, and resize findings. Reuse it rather than creating a
second input backlog.

## What Pleamar adds, and which component owns it

Current Pleamar documentation separates **pleamar**, a shell/runtime toolkit, from **pleamar-wm**, an
optional compositor whose window manager is authored as a scene. Treating the two as one feature set
would assign shell UI to the compositor by mistake.

| Capability shown by the current references | Product owner in sysc | Gap and best reference |
|---|---|---|
| Niri-style scrolling layout plus leader, row, grid, adaptive “marea,” and recursive/fractal layouts | Compositor | Niri columns are the required base. Additional built-in layouts are a possible sysc enhancement, not Mango parity. Decide the supported layout model before M5 hardens it. |
| Live re-layout on config change; drag preview; workspace overview and animated window movement | Compositor policy and renderer | Niri is the compatibility reference for its layout/actions/overview. Pleamar-wm is a source of interaction ideas. Add only concrete behaviors with a stable Go-owned state model. |
| Window thumbnails and live previews | Capture is compositor/protocol work; presentation is sysc-shell | The design already schedules `ext_foreign_toplevel_image_capture_source_v1` with `ext_image_copy_capture_v1` at M12. Preserve the approved privacy rule: suppress `screen-capture` protected windows; keep `screencast`-only rules scoped to screencasts. |
| Dock, bars, shell scenes, glass effects, notification surfaces and shell widgets | sysc-shell | These are client surfaces and rendering features, not compositor-owned UI. The compositor should provide layer-shell, capture, input, and protocol behavior; keep shell visuals in sysc-shell. |
| Independent agent pointer/keyboard, visible activity, and a Stop control | Compositor security/input plus shell presentation | The Go control socket, isolated injected seat, per-window targeting, Stop enforcement, and facts already exist in the current baseline. sysc-shell presence UX remains open (`sysc-1193`); lock/blank behavior remains gated. Mango is useful for wlroots seat mechanics. |
| Remote desktop in a browser | Cross-component session/capture/input work | A separate design is needed for authentication, remote input, capture, bandwidth, and stop/revocation. It is not a Niri or Mango parity item. |
| User-rewritable WM scene with arbitrary rules, zones, decorations and custom animation | Compositor policy plus a rendering/authoring surface | This is the central Pleamar-wm difference. The current sysc design explicitly excluded a scene language; the new owner direction reopens that decision but does not select a mechanism. |

Pleamar's current `README` and `pleamar-wm` README describe eight layout choices including scrolling and
fractal layouts, live window previews, smooth drag/re-layout, a separate agent seat and a browser remote
session. Pleamar itself supplies user-authored shell scenes, a Luau logic runtime, WGSL effects, and
live reload. These are upstream feature claims from the pinned source snapshots, not runtime validation
by this report.

## Better-mousetrap recommendations

The capability request leaves the authoring mechanism open. These are the realistic shapes under the
current Go+cgo constraint:

| Shape | What it provides | Cost and boundary |
|---|---|---|
| **First-party Go capabilities (recommended starting point)** | A selected set of better layouts, drag behavior, or compositor visuals with explicit Go-owned state and narrow config knobs. | Lowest complexity; does not let users invent arbitrary policy. Existing Niri config remains portable. |
| **Separate declarative sysc config** | User-selectable combinations and rules beyond Niri's fixed KDL vocabulary, kept outside `niri/config.kdl`. | Adds a second config surface and parser semantics; still less expressive than a scene language. |
| **Supervised external Go policy process** | Replaceable layout/policy logic without embedding another runtime into the compositor. | Highest lifecycle/API cost: versioning, timeouts, fallback layout, animation handoff, process failure, and security boundaries. This is justified only by a concrete need for user-authored algorithms. |
| **Embedded Luau or another scene language** | Closest match to Pleamar's authoring model and arbitrary logic. | Conflicts with the Go+cgo-only compositor constraint; not an option under the current product rules. |

The recommended starting point is first-party Go behavior after the Niri model is in place. Keep a separate
config or process boundary as a later option only if a named feature cannot be expressed well with that
approach.

1. **Keep the Niri boundary stable.** Existing KDL, niri-ipc, CLI, session variables, and sysc-shell
   behavior remain the default contract. A sysc enhancement should not make an existing Niri config
   fail under Niri or silently change an existing Niri action.
2. **Use Go for compositor policy and cgo for wlroots.** Keep Wayland calls on the compositor thread,
   policy state in Go, and IPC readers on immutable snapshots. Do not port Rust/C++/Luau code or add
   handwritten C files.
3. **Choose features by owner.** Window placement, capture authorization, and input seats belong in
   compositor work. Docks, glass, bars, preview cards, and other drawn shell UI belong in sysc-shell.
4. **Do not build a generic scene engine speculatively.** If the needed outcome is another built-in
   layout or interaction, implement that as a narrowly named Go capability with config only where the
   user needs a choice. If arbitrary user-authored WM policy is required, write a separate design first;
   options include a limited declarative sysc extension or a supervised external layout process with
   deadlines and a built-in fallback. Either costs compatibility/API surface. A Luau-style embedded
   runtime conflicts with the current Go+cgo product constraint.
5. **Set a design checkpoint before M5.** Decide whether “Pleamar-like” means a short list of first-party
   compositor capabilities or user-rewritable layout/policy. M5 introduces the model that later layouts,
   preview geometry, animation, and agent-seat behavior will depend on. Do not defer this decision until
   after those assumptions are embedded in code.
6. **Use the M3 live gate as a baseline.** Its nested laptop run proves the current shell connection,
   layer mapping, state queries, EventStream, and visible rendering path. It does not prove direct
   nested capture, blur, multi-output behavior, or later window-state milestones.

The earlier `sysc-1212` design recommended a permanent scene-language exclusion and said to revisit only
on an explicit owner request. The current request is that trigger: retain Niri's overall shape, but
research a broader capability set inspired by Pleamar. It does **not** authorize changing the language
constraint or importing Pleamar's scene mechanism. Keep `sysc-1212` open through a focused design that
names the first capabilities, their owners, compatibility boundary, and stop condition.

## Sources

| Reference | Pin inspected | Used for |
|---|---|---|
| [Niri](https://github.com/YaLTeR/niri/tree/8ed0da44d974c32c6877d2f4630c314da0717ecb), v26.04 | `8ed0da44d974c32c6877d2f4630c314da0717ecb` | Behavior, KDL, CLI and niri-ipc contract. |
| [Mango](https://github.com/mangowm/mango/tree/a21b10fcbe6156b76ea96178d1941de5cb36137f), v0.18.0 | `a21b10fcbe6156b76ea96178d1941de5cb36137f` | wlroots 0.20 setup, input, seat and scene calls. |
| [Hyprland](https://github.com/hyprwm/Hyprland/tree/8010b4e5c66f97aa82cd5b3332b4ec613779b9a7), main (2026-10-17) | `8010b4e5c66f97aa82cd5b3332b4ec613779b9a7` | Rendering, damage and capture/performance ideas. This snapshot uses Aquamarine and C++, not wlroots. |
| [Pleamar](https://github.com/k4ditano/pleamar/tree/d5a5cb9c2e2b1a608d0ba836e58bbec9107c60d5), main (2026-10-18) | `d5a5cb9c2e2b1a608d0ba836e58bbec9107c60d5` | Shell runtime, scene authoring, renderer and current feature claims. |
| [pleamar-wm](https://github.com/k4ditano/pleamar-wm/tree/687ad9ed9f27b644b6af7eaf41a9163e5d9a52fd) | `687ad9ed9f27b644b6af7eaf41a9163e5d9a52fd` | Current window manager scenes, layouts, previews, agent and remote capabilities. |
| [sysc-compositor design](2026-10-02-sysc-compositor-design.md) and [parity matrix](niri-parity-matrix.md) | Worktree M3 state | Product contract and exact remaining implementation rows. |
| [input comparison](2026-10-16-input-pattern-comparison-report.md) | M0 input audit | Existing Niri/Mango input gap analysis; avoid duplicate work. |
| [WM-as-scene exploration](2026-10-15-tier3-wm-scene-equivalent-design.md) | Owner briefing 2026-10-16 | Previous constraints and options, now reopened by the owner's request. |
