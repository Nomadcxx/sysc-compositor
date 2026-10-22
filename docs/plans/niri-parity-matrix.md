# niri parity matrix

Fill as milestones land. Do not duplicate this list in the design doc.

Pin: **niri-ipc 26.4.0**. Behaviour: niri **26.04**.

Legend: `[ ]` missing · `[~]` partial · `[x]` done.

## Requests (16)

| Request | Status | Milestone |
|---|---|---|
| Version | [x] | M2 — `sysc-compositor <version> (niri-ipc 26.4.0)` |
| Outputs | [x] | M3 — wlroots output snapshots |
| Workspaces | [x] | M3/M5/M6 — stable per-output IDs, names, active plus trailing empty workspace, dynamic indices |
| Windows | [x] | M4 — mapped toplevels with stable IDs and logical geometry; offscreen centers fall back to the focused output |
| Layers | [x] | M3 — mapped layer-shell surfaces |
| KeyboardLayouts | [x] | M9 — configured XKB names and active index; `SwitchLayout` updates the active layout |
| FocusedOutput | [x] | M3/M6 — active output follows keyboard focus; first connected output is the initial and disconnect fallback; monitor focus warps the pointer to output center |
| FocusedWindow | [x] | M4 — actual keyboard-focused mapped window, otherwise null |
| PickWindow | [ ] | M15 |
| PickColor | [ ] | M15 |
| Action | [~] | M2: all 141 decode; per-action below |
| Output | [ ] | M15 |
| EventStream | [~] | M3/M9 — initial baseline, workspace and keyboard updates, plus config reload results; remaining events land with their owning milestones |
| ReturnError | [x] | M2 |
| OverviewState | [x] | M7 — Niri 26.04 query returns open state |
| Casts | [ ] | M12 |

## Events (19)

| Event | Status | Milestone |
|---|---|---|
| WorkspacesChanged | [x] | M3 — workspace topology changes |
| WorkspaceUrgencyChanged | [x] | M6 — urgency state transitions and clearing on focus |
| WorkspaceActivated | [x] | M3 — focus and activation changes |
| WorkspaceActiveWindowChanged | [x] | M4 — retained layout-focused window per output workspace, even while a layer owns keyboard focus |
| WindowsChanged | [x] | M4 — populated initial EventStream snapshot |
| WindowOpenedOrChanged | [x] | M4 — map and metadata changes |
| WindowClosed | [x] | M4 — unmap/destroy |
| WindowFocusChanged | [x] | M4 — focus changes and clear, following Niri event semantics |
| WindowFocusTimestampChanged | [x] | M4 — monotonic timestamp after Niri's 750 ms debounce |
| WindowUrgencyChanged | [x] | M6 — urgency actions and state transitions |
| WindowLayoutsChanged | [x] | M5 — tile order, position, and size changes |
| KeyboardLayoutsChanged | [x] | M3/M9 — publishes configured layouts and active-index changes |
| KeyboardLayoutSwitched | [x] | M3/M9 — publishes XKB layout changes and `SwitchLayout` actions |
| OverviewOpenedOrClosed | [x] | M7 — initial state and open/close transitions |
| ConfigLoaded | [x] | M3/M9 — initial config result and successful or failed reloads |
| ScreenshotCaptured | [ ] | M11 |
| CastsChanged | [~] | M3 — initial empty baseline; casts in M12 |
| CastStartedOrChanged | [ ] | M12 |
| CastStopped | [ ] | M12 |

## Actions (141)

Until implemented, decode and reply `Err` with `unimplemented: <Name>`. Tick when the action does what niri does.

### Session

| Action | Status | Milestone |
|---|---|---|
| Quit | [~] | M2; no confirmation dialog yet, so it always quits (M11 overlay UI) |
| PowerOffMonitors | [x] | M10 (owner Q8) — keeps output and workspace state; injected input is refused while every output is off, user input wakes them (`TestPowerOffPreservesOutputAndWorkspaceStateAndRestoresOnlyItsOutputs`, `TestIdleNotifiesOnlyUserActivityAndHonoursInhibitors`) |
| PowerOnMonitors | [x] | M10 (owner Q8) — restores only the outputs PowerOffMonitors turned off |
| Spawn | [~] | M2; no XDG_ACTIVATION_TOKEN until xdg-activation |
| SpawnSh | [~] | M2; as Spawn |
| DoScreenTransition | [ ] | M15 |
| LoadConfigFile | [x] | M9 — reloads the active config or switches roots after a successful explicit load |
| ToggleKeyboardShortcutsInhibit | [x] | M10 — IPC action and `toggle-keyboard-shortcuts-inhibit` bind (`TestShortcutsInhibitPassesBindsToTheFocusedClient`) |
| SwitchLayout | [x] | M9 — next, previous, or indexed XKB layout |
| ShowHotkeyOverlay | [ ] | M11 |

### Screenshot

| Action | Status | Milestone |
|---|---|---|
| Screenshot | [ ] | M11 |
| ScreenshotScreen | [ ] | M11 |
| ScreenshotWindow | [ ] | M11 |

### Window focus and workspace

| Action | Status | Milestone |
|---|---|---|
| FocusWindow | [x] | M4 — stable mapped-window ID |
| CloseWindow | [x] | M4 — sends xdg-toplevel close for an explicit ID or the retained Niri layout focus |
| FocusWorkspace | [x] | M4/M5/M6 — stable ID, current-output index, or named workspace |
| FocusWindowDown/Up | [x] | M6 — adjacent tiled window or nearest floating window in the vertical direction |
| MoveWindowDown/Up | [x] | M6 — reorder a tiled window within its column or move a floating window by 50 logical pixels |
| MoveWindowToWorkspaceDown/Up | [x] | M6 — move the active window between adjacent workspaces, with focus following by default |
| MoveColumnToWorkspaceDown/Up | [x] | M6 — move the focused column with its tabs; a focused floating window moves alone |
| MoveWindowToWorkspace / MoveColumnToWorkspace | [x] | M6 — explicit index or name target, preserving stable window/workspace IDs |
| SetWorkspaceName / UnsetWorkspaceName | [x] | M6 — explicit or focused workspace; names are unique case-insensitively |
| MoveFloatingWindow | [x] | M6 — fixed/proportional position changes for an explicit or focused window; scene position and saved position across floating toggles verified |

### M6 columns, floating, fullscreen, urgency, and monitors

| Action | Status | Milestone |
|---|---|---|
| ConsumeWindowIntoColumn / ExpelWindowFromColumn | [x] | M6 — move windows into and out of a focused column |
| ConsumeOrExpelWindowLeft/Right | [x] | M6 — directional consume/expel, including explicit-window IPC targets |
| ToggleColumnTabbedDisplay / SetColumnDisplay | [x] | M6 — normal and tabbed columns; inactive tabs are hidden and excluded from hit testing |
| FocusWindowInColumn | [x] | M6 — focus a tab by 1-based index |
| ToggleWindowFloating / MoveWindowToFloating / MoveWindowToTiling | [x] | M6 — preserve floating placement and restore tiled geometry |
| FocusFloating / FocusTiling / SwitchFocusBetweenFloatingAndTiling | [x] | M6 — switch keyboard focus between the two layouts |
| FullscreenWindow / ToggleWindowedFullscreen | [x] | M6 — client fullscreen covers its output; windowed fullscreen sets XDG fullscreen state while retaining ordinary placement |
| ToggleWindowUrgent / SetWindowUrgent / UnsetWindowUrgent | [x] | M6 — urgency updates flow into window and workspace snapshots and events |
| FocusMonitor / FocusMonitorLeft/Right/Up/Down/Previous/Next | [x] | M6 — output selection and pointer warp; multi-output hardware behavior remains unproven on the one-output testbench |
| MoveWindowToMonitor / MoveWindowToMonitorLeft/Right/Up/Down/Previous/Next | [x] | M6 — explicit output and relative-output moves |
| MoveColumnToMonitor / MoveColumnToMonitorLeft/Right/Up/Down/Previous/Next | [x] | M6 — move a tiled column, or the focused floating window |
| MoveWorkspaceToMonitor / MoveWorkspaceToMonitorLeft/Right/Up/Down/Previous/Next | [x] | M6 — move the selected or focused workspace between outputs |

### Window / column / workspace / monitor / floating / overview / cast / debug

Track the rest against niri-ipc 26.4.0 `Action` as they are implemented in M4–M15. First landing of each group:

- Window focus/close: M4
- Columns + workspaces + core binds: M5
- Consume/expel, tabs, floating, fullscreen, named workspaces, monitor moves: M6
- Overview: M7
- Cast: M12
- Debug tint / opaque / damage: M15

M5 applies `Spawn`, `SpawnSh`, `CloseWindow`, `FocusColumnLeft/Right/First/Last`,
`MoveColumnLeft/Right/ToFirst/ToLast`, and `FocusWorkspace` by stable ID or current-output index.
M6 adds the window, column, floating, fullscreen, urgency, named-workspace, and monitor actions above.

### M7 overview

| Action | Status | Milestone |
|---|---|---|
| ToggleOverview | [x] | M7 — toggles overview state |
| OpenOverview / CloseOverview | [x] | M7 — idempotent state actions |

## Config sections

| Section | Status | Milestone |
|---|---|---|
| input | [~] | M2 keyboard xkb + repeat; M5 nested Mod defaults, core key binds, Mod+drag, and edge resize with tile dimensions retained across unmap/remap for the stable window ID; M6 adds its keyboard action subset; M7 adds overview gestures, hot corners, tiled insert hints, and DND edge scrolling; M9 adds layout switching and session locale defaults; physical input and client gesture delivery remain unverified; remaining input work is M10 |
| output | [~] | M3 reports connected outputs, available/current modes, logical geometry, scale/transform, and VRR capability/state; M15 applies mode/refresh, scale, position, transform, power, and VRR settings |
| layout | [~] | M5 gaps, default column width, scrolling columns, client min/max size bounds, centered window offsets, and retained pointer-resized dimensions; M7 insert hints and tiled drag placement; M8 focus ring, window shadow, rounded outer subsurface corners, geometry clipping, and clipped snapshot bounds. SceneFX shadows use one radius, so nonuniform corner rules use the top-left radius; direct-session visual calibration remains unverified |
| binds | [~] | M5 core key subset plus M6 window, workspace, urgency, and monitor actions; M7 overview actions; unsupported later actions and mouse/scroll triggers warn |
| switch-events | [x] | M9 — lid and tablet switch spawn actions; login1 also updates lid state |
| window-rule | [~] | M6 app-id/title match and exclude, ordered placement/fullscreen overrides, and default size fields; M8 ordered `geometry-corner-radius` and `clip-to-geometry`; remaining Niri rule effects are unimplemented |
| layer-rule | [~] | M6 parses namespace matching and `place-within-backdrop`; M7 does not render qualifying background surfaces within the overview backdrop or project background/bottom surfaces into workspace thumbnails; other Niri layer-rule effects remain unimplemented |
| blur | [x] | M10 — off, passes, offset, noise and saturation with Niri's defaults, applied to SceneFX blur at start and on reload (`TestBlurDefaultsMatchNiriAndUnnamedFieldsStay`) |
| animations | [~] | M8 applies the seven laptop spring kinds and animates overview, workspace, window, movement, and resize transitions; easing, slowdown, custom shaders, and four other recognized animation kinds warn and remain unapplied |
| gestures | [~] | M7 swipe, hot-corner, and DND edge-scroll logic passes synthetic nested tests; physical touchpad input, bound-client forwarding of unsupported gestures, and live client DND remain unverified |
| overview | [~] | M7 IPC, backdrop, window projection, and pointer selection; background/bottom layer projection and visual `place-within-backdrop` behavior remain unimplemented; spring animation is M8 |
| hotkey-overlay | [ ] | M11 |
| environment | [x] | M2 (applied to spawns) |
| cursor | [~] | M5 theme/size and child environment; M10 cursor-shape-v1 from the pointer's client; per-output scale M15 |
| screenshot-path | [ ] | M11 |
| prefer-no-csd | [x] | M8 exposes xdg-decoration and KDE server-decoration with server-side defaults; both accept client-side requests, and undecorated clients receive tiled-edge state |
| spawn-at-startup / spawn-sh-at-startup | [x] | M2 |
| workspace | [~] | M6 named workspace declarations and output placement; remaining Niri workspace options are unimplemented |
| xwayland-satellite | [ ] | M13 |
| recent-windows | [ ] | M11 |
| debug | [ ] | M15 |
| include | [x] | M3 positional merge |

Acceptance file: `/home/nomadx/.config/niri/config.kdl` + `sysc-shell.kdl`. Parser must accept it at M3; unimplemented actions error at `niri validate` once the parser knows the name.

## CLI

| Command | Status | Milestone |
|---|---|---|
| `niri` / `sysc-compositor` no args / `--session` | [x] | M0 binary; M9 publishes session sockets and starts and clears `graphical-session.target` only with `--session` |
| `-c` / `--config` | [x] | M2 (+ `$NIRI_CONFIG`, XDG, `/etc/niri`) |
| `niri msg` / `sysc-compositor msg` | [~] | M2/M4 actions; M9 adds all 16 request forms, output actions, `SwitchLayout`, and `LoadConfigFile`; later action subcommands remain open |
| `niri validate` | [x] | M3 command and parser accept the laptop config; unapplied sections warn |
| `niri msg action spawn` | [x] | M2 (sysc-launch L0) |
| `niri msg action quit --skip-confirmation` | [x] | M2 (sysc-greet) |
| `niri msg action focus-window --id`, `close-window [--id]`, `focus-workspace <index|name>` | [x] | M4/M5; stable IDs, dynamic current-output indices, and mapped windows |

## Wayland globals

Design §7's globals beyond the M1 core, with the milestone each is assigned (owner decision Q9). A
global is ticked only with its handler and a test (Q7). Versions follow the wlroots 0.20 managers.

| Global | Status | Milestone | Proof |
|---|---|---|---|
| ext_session_lock_manager_v1 | [x] | M10 | `TestSessionLockCoversEveryOutputAndRoutesInputOnlyToLockSurfaces`, `TestLockerCrashKeepsSessionSealed` |
| ext_idle_notifier_v1 | [x] | M10 | `TestIdleNotifiesOnlyUserActivityAndHonoursInhibitors` |
| zwp_idle_inhibit_manager_v1 | [x] | M10 | same test (visible-surface inhibitor) |
| wp_cursor_shape_manager_v1 | [x] | M10 | the pointer-shape test in `cursor_shape_test.go` |
| ext_background_effect_manager_v1 | [x] | M10 | `TestBackgroundEffectDrawsFrostUnderTheRegion` (SceneFX blur, Niri's `blur {}`) |
| zwp_keyboard_shortcuts_inhibit_manager_v1 | [x] | M10 | `TestShortcutsInhibitPassesBindsToTheFocusedClient` |
| zwp_relative_pointer_manager_v1 | [x] | M10 | `TestLockedPointerStaysAndReportsRelativeMotion` |
| zwp_pointer_constraints_v1 | [~] | M10 | lock tested; confine implemented, untested |
| zwp_text_input_manager_v3 | [ ] | M10 | |
| zwp_input_method_manager_v2 | [ ] | M10 | |
| zwlr_output_power_manager_v1 | [ ] | M10 | handler written, unwired: no packaged client to test it (`wlopm` is AUR-only) |
| ext_data_control_manager_v1, zwlr_data_control_manager_v1 | [ ] | M11 | |
| zwlr_screencopy_manager_v1 | [ ] | M11 | |
| ext_image_copy_capture_manager_v1 (output source) | [ ] | M11 | |
| wp_presentation | [ ] | M11 | |
| wp_single_pixel_buffer_manager_v1 | [ ] | M11 | |
| wp_alpha_modifier_v1 | [ ] | M11 | |
| ext_foreign_toplevel_list_v1 | [ ] | M12 | |
| zwlr_foreign_toplevel_manager_v1 | [ ] | M12 | |
| foreign-toplevel image-capture source | [ ] | M12 | |
| zwp_virtual_keyboard_manager_v1, zwlr_virtual_pointer_manager_v1 | [ ] | M12 | |
| zwlr_gamma_control_manager_v1 | [ ] | M12 | |
| wp_tearing_control_manager_v1 | [ ] | M12 | |
| wp_linux_drm_syncobj_manager_v1 | [ ] | M12 | |
| zwp_tablet_manager_v2 | [ ] | M12 | |
| zwlr_output_manager_v1 | [ ] | M15 | |

## D-Bus

| Name | Status | Milestone |
|---|---|---|
| org.gnome.Mutter.DisplayConfig | [ ] | M12 |
| org.gnome.Mutter.ScreenCast | [ ] | M12 |
| org.gnome.Shell.Screenshot | [ ] | M12 |
| org.gnome.Shell.Introspect | [ ] | M12 |
| org.freedesktop.ScreenSaver | [x] | M10 — Inhibit/UnInhibit at both of Niri's paths, cookies per caller, dropped when the caller leaves; started in `--session`, feeds idle inhibition (`TestScreenSaverInhibitsPerSenderAndDropsDepartedCallers`) |
| org.freedesktop.a11y.KeyboardMonitor | [ ] | M14 |
| locale1 client | [x] | M9 — XKB defaults and property changes, overridden by explicit KDL settings |
| login1 | [x] | M9 — initial and changed lid state; sleep remains owned by wlroots |

## Deliberate exclusions (recorded, not missing)

Things this product will **not** match niri on, and why. A row here is a decision. A row in the tables
above is a debt.

| Exclusion | Decision | Reason | Item |
|---|---|---|---|
| General user-authored WM policy or scene runtime | Excluded from current scope; revisit through a separate design | No concrete workflow beyond Niri's KDL and the planned Go behavior is identified. Lua/Luau conflicts with the Go+cgo rule; a new KDL dialect or external layout process adds compatibility and lifecycle cost | gh#264 / sysc-1212 |

The [Tier 3 design](2026-10-15-tier3-wm-scene-equivalent-design.md#owner-update-2026-10-20-selected-capabilities-under-the-niri-contract)
records the selected agent-seat and window-capture work. Any KDL or IPC extension needs a concrete user
workflow and owner review.

## Sibling (not this repo)

| Work | Status | Plan |
|---|---|---|
| L0 sysc-launch compositor PATH | [ ] | `2026-10-02-sysc-launch.md` |
| S0 sysc-shell go.mod bump | [ ] | orchestration Task 4 |
| G0 sysc-greet installer copy | [ ] | after M2 |
| W0 sysc-walls version detect | [ ] | after M2 (drop-in may suffice) |
