# niri parity matrix

Fill as milestones land. Do not duplicate this list in the design doc.

Pin: **niri-ipc 26.4.0**. Behaviour: niri **26.04**.

Legend: `[ ]` missing · `[~]` partial · `[x]` done.

## Requests (16)

| Request | Status | Milestone |
|---|---|---|
| Version | [x] | M2 — `sysc-compositor <version> (niri-ipc 26.4.0)` |
| Outputs | [x] | M3 — wlroots output snapshots |
| Workspaces | [~] | M3 — one empty active workspace per output; dynamic layout lands later |
| Windows | [x] | M4 — mapped toplevels with stable IDs and logical geometry; offscreen centers fall back to the focused output |
| Layers | [x] | M3 — mapped layer-shell surfaces |
| KeyboardLayouts | [ ] | M9 |
| FocusedOutput | [x] | M3 — active output follows keyboard focus; first connected output is the initial and disconnect fallback |
| FocusedWindow | [x] | M4 — actual keyboard-focused mapped window, otherwise null |
| PickWindow | [ ] | M15 |
| PickColor | [ ] | M15 |
| Action | [~] | M2: all 141 decode; per-action below |
| Output | [ ] | M15 |
| EventStream | [~] | M3 — initial baseline, workspace and keyboard updates; remaining events land with their owning milestones |
| ReturnError | [x] | M2 |
| OverviewState | [ ] | M7 |
| Casts | [ ] | M12 |

## Events (19)

| Event | Status | Milestone |
|---|---|---|
| WorkspacesChanged | [x] | M3 — workspace topology changes |
| WorkspaceUrgencyChanged | [ ] | M6 |
| WorkspaceActivated | [x] | M3 — focus and activation changes |
| WorkspaceActiveWindowChanged | [x] | M4 — retained layout-focused window per output workspace, even while a layer owns keyboard focus |
| WindowsChanged | [x] | M4 — populated initial EventStream snapshot |
| WindowOpenedOrChanged | [x] | M4 — map and metadata changes |
| WindowClosed | [x] | M4 — unmap/destroy |
| WindowFocusChanged | [x] | M4 — focus changes and clear, following Niri event semantics |
| WindowFocusTimestampChanged | [x] | M4 — monotonic timestamp after Niri's 750 ms debounce |
| WindowUrgencyChanged | [ ] | M6 |
| WindowLayoutsChanged | [ ] | M5 |
| KeyboardLayoutsChanged | [~] | M3 — current XKB layout baseline; config support in M9 |
| KeyboardLayoutSwitched | [~] | M3 — XKB layout changes; switch action in M9 |
| OverviewOpenedOrClosed | [~] | M3 — initial closed state; overview in M7 |
| ConfigLoaded | [~] | M3 — initial config-load result; reload in M9 |
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
| PowerOffMonitors | [ ] | M15 |
| PowerOnMonitors | [ ] | M15 |
| Spawn | [~] | M2; no XDG_ACTIVATION_TOKEN until xdg-activation |
| SpawnSh | [~] | M2; as Spawn |
| DoScreenTransition | [ ] | M15 |
| LoadConfigFile | [ ] | M9 |
| ToggleKeyboardShortcutsInhibit | [ ] | M10 |
| SwitchLayout | [ ] | M9 |
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
| FocusWorkspace | [~] | M4 — current output workspace by ID/index; named workspaces arrive with M6 |

### Window / column / workspace / monitor / floating / overview / cast / debug

Track the rest against niri-ipc 26.4.0 `Action` as they are implemented in M4–M15. First landing of each group:

- Window focus/close: M4
- Columns + workspaces + core binds: M5
- Consume/expel, tabs, floating, fullscreen, named workspaces, monitor moves: M6
- Overview: M7
- Cast: M12
- Debug tint / opaque / damage: M15

## Config sections

| Section | Status | Milestone |
|---|---|---|
| input | [~] | M2 keyboard xkb + repeat; rest M5 (parse M3) |
| output | [~] | M3 accepted, M15 apply full |
| layout | [ ] | M5 |
| binds | [ ] | M5 |
| switch-events | [ ] | M9 |
| window-rule | [ ] | M6 |
| layer-rule | [ ] | M6 |
| animations | [ ] | M8 |
| gestures | [ ] | M7 |
| overview | [ ] | M7 |
| hotkey-overlay | [ ] | M11 |
| environment | [x] | M2 (applied to spawns) |
| cursor | [ ] | M10 |
| screenshot-path | [ ] | M11 |
| prefer-no-csd | [ ] | M8 |
| spawn-at-startup / spawn-sh-at-startup | [x] | M2 |
| workspace | [ ] | M6 |
| xwayland-satellite | [ ] | M13 |
| recent-windows | [ ] | M11 |
| debug | [ ] | M15 |
| include | [x] | M3 positional merge |

Acceptance file: `/home/nomadx/.config/niri/config.kdl` + `sysc-shell.kdl`. Parser must accept it at M3; unimplemented actions error at `niri validate` once the parser knows the name.

## CLI

| Command | Status | Milestone |
|---|---|---|
| `niri` / `sysc-compositor` no args / `--session` | [ ] | M0 binary; `--session` M9 |
| `-c` / `--config` | [x] | M2 (+ `$NIRI_CONFIG`, XDG, `/etc/niri`) |
| `niri msg` / `sysc-compositor msg` | [~] | M2: `version`, `request-error`, `action quit/spawn/spawn-sh`, `--json`; M4 adds `focus-window`, `close-window`, and `focus-workspace`; niri 26.04's own `niri msg` also works against our socket |
| `niri validate` | [x] | M3 command and parser accept the laptop config; unapplied sections warn |
| `niri msg action spawn` | [x] | M2 (sysc-launch L0) |
| `niri msg action quit --skip-confirmation` | [x] | M2 (sysc-greet) |
| `niri msg action focus-window --id`, `close-window [--id]`, `focus-workspace <index|name>` | [x] | M4; laptop-tested against mapped windows and the current output workspace |

## D-Bus

| Name | Status | Milestone |
|---|---|---|
| org.gnome.Mutter.DisplayConfig | [ ] | M12 |
| org.gnome.Mutter.ScreenCast | [ ] | M12 |
| org.gnome.Shell.Screenshot | [ ] | M12 |
| org.gnome.Shell.Introspect | [ ] | M12 |
| org.freedesktop.ScreenSaver | [ ] | M10 |
| org.freedesktop.a11y.KeyboardMonitor | [ ] | M14 |
| locale1 client | [ ] | M9 |
| login1 | [ ] | M9 |

## Deliberate exclusions (recorded, not missing)

Things this product will **not** match niri on, and why. A row here is a decision. A row in the tables
above is a debt.

| Exclusion | Decision | Reason | Item |
|---|---|---|---|
| A scene language for window motion and WM policy | Not built, permanently | The mechanism pleamar uses is Luau, and scripting languages are banned in this product. KDL binds, the 141-action table and per-kind springs in `animations{}` are the authoring surface, and they are already specified | gh#264 / sysc-1212 |

**A future agent reading this should not "fix" it.** The exclusion was reached deliberately and
re-checked after the repository was created: being in the right repository did not change it. If the
constraint on scripting languages ever lifts, that is a new design, not a resumption of this one.

## Sibling (not this repo)

| Work | Status | Plan |
|---|---|---|
| L0 sysc-launch compositor PATH | [ ] | `2026-10-02-sysc-launch.md` |
| S0 sysc-shell go.mod bump | [ ] | orchestration Task 4 |
| G0 sysc-greet installer copy | [ ] | after M2 |
| W0 sysc-walls version detect | [ ] | after M2 (drop-in may suffice) |
