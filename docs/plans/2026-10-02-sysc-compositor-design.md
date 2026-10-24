# sysc-compositor design

The compositor for the sysc desktop. **Go + cgo only** (no `.c` / `.h` in this tree), wlroots 0.20. **Niri parity** is the product contract: same scrollable-tiling model, same KDL config, same JSON IPC, same CLI, same portals and a11y surfaces, so sysc-shell, sysc-greet, sysc-launch, and an existing `~/.config/niri/config.kdl` keep working.

sysc owns the rest of the pipeline (greeter, shell, wallpaper, notify, tray, clipboard, launch, wayland client). This repo is the compositor those pieces sit on.

Pinned niri IPC crate for the wire: **niri-ipc 26.4.0**. Behaviour target: niri **26.04** (the version sysc-shell’s fixtures were captured against), plus later niri additions as they land.

Implementation map: [orchestration](2026-10-02-orchestration.md). Remaining-work ticks: [niri-parity-matrix](niri-parity-matrix.md).

---

## 1. Product

`sysc-compositor` is the session compositor. greetd starts it for the greeter. A `.desktop` / `sysc-compositor-session` starts it for the user.

| Layer | Owner |
|---|---|
| Login | sysc-greet (TUI in kitty; not a Wayland client) |
| Shell | sysc-shell (layer-shell UI; not a compositor) |
| Wallpaper | gSlapper / awww / swaybg, driven by the shell |
| Screensaver | sysc-walls (idle → kitty + sysc-Go) |
| Notifications | sysc-notify (D-Bus; shell draws) |
| Tray | sysc-tray (design-stage; shell draws) |
| Clipboard | sysc-clipboard (ext-data-control client) |
| Launch | sysc-launch (library; Spawn via compositor CLI — see L0) |
| Metrics | sysc-metrics (library) |
| Plugins | sysc-plugins (stdio children of the shell) |
| Wayland **client** lib | sysc-wayland (not a compositor) |
| TUI animations | sysc-Go |
| Compositor | **this repo** |

`XDG_CURRENT_DESKTOP` is **`niri:sysc`** (niri first so `xdg-desktop-portal-gnome` still matches; sysc second so we own identity). Portal config ships `niri-portals.conf` *and* `sysc-portals.conf` pointing at gnome + gtk, same as a niri session.

`$NIRI_SOCKET` is the compositor IPC path (niri’s env name, our process). `$SYSC_SOCKET` is the same inode.

The CLI is `sysc-compositor` (`cmd/sysc-compositor`): `sysc-compositor msg`, `sysc-compositor validate`, `sysc-compositor -c`, `sysc-compositor --session`. A `niri` binary in this repo (`cmd/niri`) is a compatibility alias with the same arguments, and the drop-in for greetd (`niri -c /etc/greetd/niri-greeter-config.kdl`) and sysc-launch (`LookPath("niri")`). The compositor process can be `sysc-compositor` or `niri` (symlink).

**Definition of done (parity):** this machine’s `~/.config/niri/config.kdl` (including `sysc-shell.kdl`) loads, live-reloads, and drives a session that sysc-shell, greeter, Steam/X11 via xwayland-satellite, OBS via gnome portal, and Orca can use without niri installed.

---

## 1.1 What already exists (do not rebuild)

The compositor sits under this. Drawing, history, tray icons, launcher ranking, metrics, plugins, greeter TUI, screensaver TUI, and the Wayland **client** stack are done. `sysc-compositor` does not grow a bar, a notification daemon, or a second Wayland client library.

```
                    compositor  (missing — this repo)
                         │
         WAYLAND + NIRI_SOCKET + graphical-session.target
                         │
     ┌───────────────────┼───────────────────┐
     │                   │                   │
sysc-shell          daemons              other clients
(layer-shell UI)    (no windows)         (kitty, firefox,
     │                   │                gslapper, …)
     │              sysc-notify     D-Bus FDO + presenter.v1.sock
     │              sysc-clipboard  ext-data-control + control.v1.sock
     │              sysc-tray       (design; presenter.v1.sock planned)
     │              sysc-walls      ext-idle-notify → kitty+sysc-Go
     │              sysc-launch     Spawn IPC via compositor CLI (library in shell)
     │              sysc-metrics    /proc library in shell
     │              sysc-plugins    stdio children of the shell
     │
sysc-wayland        client wire + scanner (NOT a compositor)
sysc-Go             TUI animations (greet + walls)
sysc-greet          greetd TUI in kitty (not a Wayland client)
```

| Repo | Role | Talks to compositor | Compositor must provide |
|---|---|---|---|
| **sysc-wayland** tip `v0.3.1`; shell pins **v0.2.2** | Pure-Go **client**: wire, FDs, proxies, scanner. Packages `client`, `cursorshape`, `textinput`, `idle`. No server dispatch, no DRM, no listen. AGENTS.md forbids compositor policy. | Client of `WAYLAND_DISPLAY` | A real Wayland server. Do not extend this module into a compositor. |
| **sysc-shell** | Bars, panels, launcher, OSD, toasts, wallpaper picker. Layer-shell only. Not a lock screen. | `$NIRI_SOCKET` EventStream + FocusWindow / CloseWindow / FocusWorkspace + `"Layers"`; some exec via `niri msg action spawn` | `WAYLAND_DISPLAY` + `NIRI_SOCKET` in the **systemd user env**. `graphical-session.target`. Globals in the next subsection. |
| **sysc-notify** | Owns `org.freedesktop.Notifications`. No windows. | None. Presenter socket to the shell. Shell focuses windows by niri id. | Session bus + EventStream window ids. |
| **sysc-clipboard** | History daemon; client via sysc-wayland. | **`ext_data_control_manager_v1`**, fallback `zwlr_data_control_manager_v1` | Advertise ext-data-control. Unit is `After=graphical-session.target`. |
| **sysc-tray** | Design-stage SNI→shell. No production service yet. Shell already pins `v0.1.0-rc.3`. | Planned presenter socket. No Wayland. | Nothing unique. |
| **sysc-launch** | App scan + rank. In-process in the shell. **Sibling repo** (`/home/nomadx/sysc-launch`). | **`{compositor} msg action spawn --`**. Today `LookPath("niri")` only; production `niriRun` panics never, errors if niri missing. | `sysc-compositor msg` **and** `niri msg` implementing Spawn. **L0** patches launch to resolve `sysc-compositor` then `niri`. Do not rewrite scan/rank. |
| **sysc-metrics** | `/proc` `/sys` library. | None | Nothing. |
| **sysc-plugins** | Stdio plugins. Screen Recorder uses `gpu-screen-recorder`. | Indirect | PipeWire screencast; window ids. |
| **sysc-greet** | greetd TUI in kitty. Not a Wayland client. | Host spawn + quit. Wallpaper is **gSlapper** (layer-shell), not sysc-walls. | Multi-client session (kitty + gSlapper). `niri msg action quit`. |
| **sysc-walls** (`/home/nomadx/Documents/sysc-screen`) | Idle screensaver, **not** wallpaper. `ext-idle-notify-v1` then fullscreen kitty + sysc-Go. Uses `go-wayland`, not sysc-wayland. | Idle protocol; `niri msg version` to detect compositor | `ext-idle-notify-v1`. `WantedBy=graphical-session.target`. |
| **sysc-Go** | TUI animation library for greet and walls. | None | Nothing. |
| Wallpaper engines | gSlapper / awww / swaybg, driven by the shell. | layer-shell Background | layer-shell + Layers IPC. |

### sysc-shell Wayland contract (from code)

**Required or the shell will not start:** `wl_compositor`, `wl_shm` (ARGB8888), `wl_seat`, `zwlr_layer_shell_v1` v4, `wp_fractional_scale_manager_v1`, `wp_viewporter`, ≥1 `wl_output`.

**Optional:** `ext_background_effect_manager_v1` (frost), `zwlr_screencopy_manager_v1` ≤v3 (panel blur fallback; **exact-sized SHM pool** or the connection dies), `zwp_text_input_manager_v3`, `wp_cursor_shape_manager_v1`.

**Namespaces:** bar `sysc-shell:bar`; panels `sysc-shell-panel` / `sysc-shell-shield`; toast/osd/tooltip/tray/plugin/depth-clock as `sysc-shell-*`. Bar is layer Top, keyboard interactivity None.

**Keyboard:** one press per physical key; the shell repeats. Do not send `KeyboardKeyStateRepeated`.

**systemd:** `Requisite=graphical-session.target`. Relies on the compositor putting `WAYLAND_DISPLAY` and `NIRI_SOCKET` into the user manager.

### What this changes in the compositor plan

1. **sysc-wayland is not the compositor kernel.** Server path is cgo + wlroots.
2. **ext-data-control-v1 is session-hard** (clipboard is already deployed).
3. **ext-idle-notify-v1 is session-hard** (sysc-walls).
4. **Do not implement** a lock screen, wallpaper engine, notification server, tray watcher, or bar here.
5. **`niri` CLI drop-in stays** as a compatibility alias of `sysc-compositor` (§6) for greet (`niri msg action quit`), walls (`niri msg version`), scripts, and `niri validate`. **sysc-launch is a sibling change**, not covered by the drop-in alone: its production runner hard-requires `LookPath("niri")`. That is **L0** in the orchestration plan.
6. **Do not rewrite** sysc-launch ranking, XDG scan, or history. Only compositor resolution + docs.

### Sibling workstreams (other repos, tracked here)

| ID | Repo | Change | Gate |
|---|---|---|---|
| **L0** | sysc-launch | Resolve compositor CLI: `sysc-compositor` then `niri`. Rewrite `argv[0]` in the production runner. Tests for prefer-sysc, niri-fallback, neither-is-error. Tag; bump sysc-shell `go.mod`. | Can start now. Needed before a session without a `niri` binary. |
| **G0** | sysc-greet | No code for M2 if `niri` is a symlink/drop-in. Later: installer/docs mention `sysc-compositor`. | After M2 |
| **W0** | sysc-walls | Same: `niri msg version` works via drop-in. Optional later: accept sysc-compositor. | After M2 |
| **S0** | sysc-shell | Bump `github.com/Nomadcxx/sysc-launch` after L0. `runningapps_click_test` injects `Run` and still expects argv `niri` — leave it; injected Run is not the production resolver. | After L0 tag |

---

## 2. Architecture

One OS thread owns Wayland. Go owns policy, KDL, IPC, D-Bus, session.

```
greetd / display manager
        │
        ▼
 sysc-compositor / niri --session
        │  wl_display_run, LockOSThread
        │  cgo → wlroots 0.20
        │
        ├── WAYLAND_DISPLAY
        ├── NIRI_SOCKET / SYSC_SOCKET     niri-ipc JSON
        ├── $DISPLAY                      xwayland-satellite (on demand)
        ├── D-Bus session names           Mutter.DisplayConfig, Mutter.ScreenCast,
        │                                 ScreenSaver, gnome Screenshot/Introspect,
        │                                 a11y KeyboardMonitor, locale1
        └── systemd --user                graphical-session.target + sysc-session.target
```

**Language:** this repo is Go. FFI is cgo in `.go` files (`import "C"`). There are **no** `.c`, `.h`, `.cc`, or `.cpp` sources here. Handwritten C lives in **gSlapper** (wallpaper, other repo) and nowhere else in the sysc session we own. Linking C *libraries* (wlroots, wayland-server, pixman, libinput, libdrm, mesa, PipeWire, scenefx if we use it) is cgo, not a C tree.

**cgo (`internal/wlr`)** wraps wlroots types we call. Listener callbacks → Go via `cgo.Handle`. If wlroots requires a C function pointer, the trampoline is a `static` function in that file’s cgo preamble, not a `.c` file. Prefer `//export` when it is enough. Target wlroots **0.20**. go-wlroots is a pattern, not a dependency.

**Go** owns: KDL config, niri IPC, column/workspace/window policy, binds, rules, animations (CPU-side springs), screenshot UI, D-Bus servers, session.

No second event loop. IPC and D-Bus fds are `wl_event_loop_add_fd` (or a pipe from a D-Bus goroutine onto that loop). Never call wlroots off the compositor thread.

---

## 3. Window model (niri, complete)

Per output: an ordered list of **dynamic workspaces** (GNOME-like; empty ones collapse except the last). Each workspace: a horizontal **strip of columns**. Each column: one or more windows, either stacked (normal) or **tabbed**.

- New tiled window: new column to the right of the focused column (niri default).
- **Consume / expel:** pull a window into the adjacent column or split it out.
- **Floating** layer per workspace (since niri 25.01). Toggle, move, focus-floating vs focus-tiling.
- **Fullscreen** and **windowed (fake) fullscreen**.
- **Overview:** zoomed-out workspaces + windows; four-finger swipe / `toggle-overview`.
- Focus is (output, workspace, column, window) plus a floating-vs-tiling focus mode.
- Workspace id and window id are stable `uint64`.
- `is_active` is per output; `is_focused` is global.
- Named workspaces (`set-workspace-name`).
- Insert hint when dragging; tab indicator; focus ring; optional border; shadow.

Geometry is layout-relative on the strip. The viewport scrolls so the focused column stays in view (`center-focused-column` never/on-overflow/always).

Do not import mango tags or mango’s 14 layouts.

---

## 4. Config: niri KDL

**Parse niri KDL.** No private TOML as the user-facing format. Paths, flags, and live-reload match niri:

- `$XDG_CONFIG_HOME/niri/config.kdl` → `~/.config/niri/config.kdl` → `/etc/niri/config.kdl`
- `-c` / `--config` and `$NIRI_CONFIG` with niri’s empty-string rule
- `include "file.kdl"` positional merge (niri 25.11 rules: binds override, window-rule inserted at include site, some sections do not merge)
- live-reload on save (inotify), including output mode, binds, window-rule
- `niri validate` parses and prints errors; `ConfigLoaded { failed }` event
- `LoadConfigFile` IPC action
- missing user config: write the embedded default (we ship niri’s default as the seed, then can add sysc spawn lines)

Sections that must parse (niri wiki):

| Section | Notes |
|---|---|
| `input {}` | keyboard (xkb, repeat, numlock, `track-layout "window"`), touchpad, mouse, tablet, touch, trackball, focus-follows-mouse, warp-mouse-to-focus |
| `output "NAME" {}` | mode, scale, transform, position, VRR, off, backdrop |
| `layout {}` | gaps, struts, preset-column-widths, default-column-width, center-focused-column, focus-ring, border, shadow, tab-indicator, insert-hint |
| `binds {}` | no implicit defaults; `repeat=false`, `allow-when-locked`, `cooldown-ms`, `hotkey-overlay-title`; keys, mouse wheels, touchpad |
| `switch-events {}` | lid, tablet mode |
| `window-rule {}` | match/exclude (app-id regex, title, is-focused, is-floating, is-urgent, is-window-cast-target, at-startup, …); open-on-output/workspace/floating/maximized/fullscreen/focused; default sizes; opacity; clip-to-geometry; geometry-corner-radius; block-out-from; variable-refresh-rate; draw-border-with-background |
| `layer-rule {}` | namespace match; place-within-backdrop; blur; shadow; geometry-corner-radius |
| `animations {}` | off; per-kind spring (damping-ratio, stiffness, epsilon) or easing; custom shaders later |
| `gestures {}` | dnd-edge-view-scroll, hot-corners, touchpad swipe (3/4 finger) |
| `overview {}` | zoom, backdrop |
| `hotkey-overlay {}` | skip-at-startup |
| `environment {}` | spawn env |
| `cursor {}` | hide-when-typing, hide-after-inactive-ms, xcursor |
| `screenshot-path` | strftime path |
| `prefer-no-csd` | |
| `spawn-at-startup` / `spawn-sh-at-startup` | |
| `workspace "name" {}` | named workspace open-on-output |
| `xwayland-satellite {}` | path override |
| `recent-windows {}` | alt-tab |
| `debug {}` | tint, damage, opaque regions |
| `include` | |

**Acceptance config:** `/home/nomadx/.config/niri/config.kdl` plus `sysc-shell.kdl`. If that file fails to parse or a bind in it is unimplemented, parity is incomplete.

Parser is pure Go (`internal/config`). Use a KDL library if one exists that matches niri’s dialect; otherwise a niri-shaped parser. Golden tests: niri’s default config + this machine’s config + greeter kdl.

---

## 5. IPC: complete niri-ipc 26.4.0

Unix socket, one JSON value per line. EventStream dumps full state first, then deltas.

### Requests (all 16)

`Version`, `Outputs`, `Workspaces`, `Windows`, `Layers`, `KeyboardLayouts`, `FocusedOutput`, `FocusedWindow`, `PickWindow`, `PickColor`, `Action(Action)`, `Output { output, action }`, `EventStream`, `ReturnError`, `OverviewState`, `Casts`.

Reply shape: `{"Ok": ...}` or `{"Err": "..."}`. EventStream: first line `{"Ok":"Handled"}`.

### Events (all 19)

`WorkspacesChanged`, `WorkspaceUrgencyChanged`, `WorkspaceActivated`, `WorkspaceActiveWindowChanged`, `WindowsChanged`, `WindowOpenedOrChanged`, `WindowClosed`, `WindowFocusChanged`, `WindowFocusTimestampChanged`, `WindowUrgencyChanged`, `WindowLayoutsChanged`, `KeyboardLayoutsChanged`, `KeyboardLayoutSwitched`, `OverviewOpenedOrClosed`, `ConfigLoaded`, `ScreenshotCaptured`, `CastsChanged`, `CastStartedOrChanged`, `CastStopped`.

Unknown events are not our problem (we emit; clients ignore). We must emit every event niri emits for the same user action.

### Actions (all 141)

Implement the full enum. Until a given action exists, the socket still **decodes** it and replies `Err` with a stable string (`unimplemented: ToggleDebugTint`). Binds in a loaded config that map to unimplemented actions fail `niri validate` or warn at reload — prefer **validate error** once the parser knows the action name, so a config cannot silently no-op.

Grouped for implementation (not for dropping):

**Session:** Quit, PowerOffMonitors, PowerOnMonitors, Spawn, SpawnSh, DoScreenTransition, LoadConfigFile, ToggleKeyboardShortcutsInhibit, SwitchLayout, ShowHotkeyOverlay.

**Screenshot:** Screenshot (interactive UI), ScreenshotScreen, ScreenshotWindow.

**Window ops:** CloseWindow, FullscreenWindow, ToggleWindowedFullscreen, FocusWindow, FocusWindowInColumn, FocusWindowPrevious, FocusWindow{Down,Up,Top,Bottom,…}, consume/expel, swap, ToggleColumnTabbedDisplay, SetColumnDisplay, Center*, SetWindowWidth/Height, ResetWindowHeight, SwitchPreset*, MaximizeColumn, MaximizeWindowToEdges, SetColumnWidth, ExpandColumnToAvailableWidth, ToggleWindowRuleOpacity, urgency set/toggle/unset.

**Column / strip:** FocusColumn*, MoveColumn*, MoveWindowDown/Up and workspace variants.

**Workspace:** FocusWorkspace{Down,Up,Previous,reference}, MoveWindowToWorkspace*, MoveColumnToWorkspace*, MoveWorkspace*, SetWorkspaceName, UnsetWorkspaceName.

**Monitor:** FocusMonitor*, MoveWindowToMonitor*, MoveColumnToMonitor*, MoveWorkspaceToMonitor*.

**Floating:** ToggleWindowFloating, MoveWindowToFloating, MoveWindowToTiling, FocusFloating, FocusTiling, SwitchFocusBetweenFloatingAndTiling, MoveFloatingWindow.

**Overview:** ToggleOverview, OpenOverview, CloseOverview.

**Cast:** SetDynamicCastWindow, SetDynamicCastMonitor, ClearDynamicCastTarget, StopCast.

**Debug:** ToggleDebugTint, DebugToggleOpaqueRegions, DebugToggleDamage.

`Output` request (not Action): per-connector mode/scale/transform/position/on-off, matching `niri msg output`.

`PickWindow` / `PickColor`: interactive crosshair, return id / colour.

---

## 6. CLI (`cmd/sysc-compositor`, alias `cmd/niri`)

Match niri’s user-facing CLI well enough that scripts and docs keep working:

- no args / `--session`: start compositor (session imports env, starts sysc-session.target and so graphical-session.target)
- `-c` / `--config`
- `sysc-compositor msg [--json] …`  (thin JSON client of `$NIRI_SOCKET`)
- `sysc-compositor msg action …`
- `sysc-compositor msg output …`
- `sysc-compositor msg event-stream`
- `sysc-compositor msg layers|windows|workspaces|outputs|focused-window|keyboard-layouts|version|overview-state|casts`
- `sysc-compositor validate`
- `sysc-compositor msg --print-request` / `raw-request` if present in 26.04
- `-v` / version string (sysc-compositor version + “niri-ipc 26.4.0 compatible”)

**Naming (owner decision 2026-10-22).** `sysc-compositor msg` and `sysc-compositor validate` are the official commands; docs, help text and new scripts use them. `niri` is a compatibility alias with the same flags and JSON, kept so the greeter config and older scripts work unmodified. The niri-ipc wire contract does not change. Later steps: the suite installer's `sysc` passes `msg` and `validate` through (bd `sysc-1633`, installer repo), then `cmd/niri` retires once no sibling invokes `niri` (`sysc-1634`).

sysc-greet’s `niri msg action quit --skip-confirmation` and sysc-launch’s `{compositor} msg action spawn -- …` are the first two CLI tests.

---

## 7. Wayland globals (parity set)

**Hard required (shell + apps):**

- wl_compositor, wl_shm (**ARGB8888** advertised), wl_seat, wl_output, wl_data_device, wl_subcompositor
- xdg-shell, xdg-decoration, xdg-activation
- zwlr_layer_shell_v1 v4
- wp_viewporter, wp_fractional_scale_manager_v1
- linux-dmabuf, presentation-time
- cursor-shape v1
- zwp_text_input_manager_v3 + input-method v2

**Session / capture / lock / daemons (hard for this desktop):**

- `ext_data_control_manager_v1` (sysc-clipboard; wlr-data-control fallback)
- `ext-idle-notify-v1` (sysc-walls)
- zwlr_screencopy_manager_v1 (sysc-shell frost fallback; grim). Honour niri’s exact-sized SHM pool rule or the shell’s capture path protocol-errors
- ext-image-copy-capture / image-capture-source
- ext-background-effect-v1 (shell frost)
- session-lock v1 (external locker; shell does not lock)
- idle-inhibit
- gamma-control
- output-management v1, output-power
- pointer-constraints, relative-pointer, pointer-gestures
- keyboard-shortcuts-inhibit
- virtual-keyboard, virtual-pointer
- tablet-v2, touch
- foreign-toplevel + ext-foreign-toplevel-list
- linux-drm-syncobj, single-pixel-buffer, alpha-modifier, viewporter already listed
- tearing-control
- dma-buf export as niri exposes it

**Keyboard to layer-shell:** one `key` press per physical press; **no compositor auto-repeat** to sysc-shell (the shell repeats). Xdg-shell clients still get niri’s repeat behaviour (compositor or client — match niri for terminals).

**Security:** niri’s security model (privileged protocols, screencopy, layer exclusive). Read niri Security Model wiki and match.

---

## 8. D-Bus (this is niri, not optional)

Session instance only (display manager / `--session`), same as niri nested-off:

| Name | Why |
|---|---|
| `org.gnome.Mutter.DisplayConfig` | gnome portal display settings |
| `org.gnome.Mutter.ScreenCast` | gnome portal screencast (OBS, Firefox, Chromium). PipeWire streams. Window + monitor + **dynamic cast target**. `block-out-from` window-rule |
| `org.gnome.Shell.Screenshot` | gnome screenshot portal |
| `org.gnome.Shell.Introspect` | window list for portal |
| `org.freedesktop.ScreenSaver` | inhibit / lock |
| `org.freedesktop.a11y.KeyboardMonitor` | Orca key grab |
| `org.freedesktop.locale1` client | default xkb from systemd-localed |
| login1 | sleep / lid |

Screencast is PipeWire. Without PipeWire, log and disable ScreenCast like niri does.

Also ship / document `xdg-desktop-portal-gnome` + `xdg-desktop-portal-gtk` as the portal pair, not wlr-only. wlr screencopy remains for grim and sysc-shell fallback.

---

## 9. Visuals and input

- **ext-background-effect-v1** for layer frost (sysc-shell namespaces `sysc-shell:bar`, `sysc-shell-panel`; wallpaper `place-within-backdrop`).
- Window **shadow**, **corner radius**, **clip-to-geometry**, **opacity**, focus-ring / border / tab-indicator / insert-hint from layout + window-rule.
- **Spring animations** for open/close/resize/move, workspace-switch, horizontal-view-movement, overview. Match niri’s spring params from config. Custom GLSL shaders are a later parity item; springs are not optional.
- **Screenshot UI** built into the compositor (niri’s interactive selector), plus one-shot screen/window capture to clipboard + `screenshot-path`.
- **Hotkey overlay** (`show-hotkey-overlay`).
- **Recent-windows / alt-tab** if niri 26.04 config has it.
- **Gestures:** 3-finger (workspace/column), 4-finger overview, hot-corners (off in this machine’s config), dnd edge scroll.
- **Binds:** keyboard, mouse buttons, wheel with cooldown, touchpad; `allow-when-locked` for media/brightness.
- Mixed DPI, per-output scale, VRR on-demand via window-rule.

scenefx is a possible implementation path for shadow/blur/radius on wlroots. Protocol-level frost for the shell is still `ext-background-effect-v1`. Use scenefx if it is less work than a custom GLES path; it is an implementation detail, not a user-facing dependency.

---

## 10. X11

**xwayland-satellite ≥ 0.7**, on demand, same as niri 25.08+:

- create X11 sockets, export `$DISPLAY`
- spawn satellite when an X11 client connects; restart if it dies
- satellite windows are normal Wayland xdg windows to the WM
- `xwayland-satellite { path }` config override

Do not use `wlr_xwayland` as the primary path. Rootful Xwayland / gamescope remain user recipes, not compositor features.

Orca needs XWayland. Steam/Discord/games are the daily-driver gate.

---

## 11. Accessibility

When started as a session (not nested):

- `org.freedesktop.a11y.KeyboardMonitor`
- announce workspace switches, exit dialog, screenshot UI, overview, config errors, hotkey overlay, alt-tab titles (AccessKit or AT-SPI equivalent)
- Super+Alt+S toggle Orca in default binds

Nested / TTY-without-session: do not export a11y names (niri rule, avoid fighting the parent compositor).

---

## 12. Two run modes

```
sysc-compositor --session      # user session: env import, graphical-session.target
niri -c /etc/greetd/niri-greeter-config.kdl   # greeter (alias, so sysc-greet's file runs unmodified)
sysc-compositor validate
sysc-compositor msg …
```

Greeter kdl (already in sysc-greet): empty binds, animations off, spawn wallpaper + kitty, then `niri msg action quit --skip-confirmation`. That file must work unmodified.

Session: import `WAYLAND_DISPLAY`, `NIRI_SOCKET`, `SYSC_SOCKET`, `DISPLAY`, `XDG_CURRENT_DESKTOP=niri:sysc`, `XDG_SESSION_TYPE=wayland`. Start `sysc-session.target` (`share/systemd/user/`), which binds `graphical-session.target`, so sysc-shell.service and portals come up; systemd refuses to start `graphical-session.target` directly. On exit stop `graphical-session.target` itself, which systemd allows; `sysc-session.target` stops with it.

---

## 13. Repository

Module `github.com/Nomadcxx/sysc-compositor`. License BSD-3-Clause.

```
cmd/sysc-compositor/     compositor main (may be the same main as niri)
cmd/niri/                CLI + compositor entry (drop-in)
internal/wlr/            cgo in .go files only (no .c/.h)
internal/server/         display, backend, globals, run
internal/wm/             windows, workspaces, outputs, floating, overview
internal/layout/         column strip, tabs, consume/expel (pure Go)
internal/ipc/            niri-ipc JSON (pure Go)
internal/config/         KDL parse, include, validate, watch (pure Go)
internal/bind/           keymap → Action
internal/anim/           springs
internal/rules/          window-rule / layer-rule
internal/shot/           screenshot UI
internal/cast/           PipeWire + mutter ScreenCast
internal/dbus/           the niri D-Bus set
internal/a11y/           KeyboardMonitor + announcements
internal/x11/            xwayland-satellite integration
internal/session/        systemd / env
internal/proto/          lock, ime, background-effect, …
```

`internal/layout`, `internal/ipc`, `internal/config`, `internal/rules` have **no cgo**. Tests:

```
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/ipc -run TestEventStream
```

Never `go test ./...`, never `-race` on this machine.

---

## 14. cgo rules

- Source is Go. Bindings are cgo. **Do not add `.c` or `.h` files.** gSlapper’s C stays in gSlapper.
- Trampolines for wlroots `wl_listener.notify` live in the cgo comment of a `.go` file (`static void on_foo(...) { ... }`) or as `//export`. Same for any other C callback. If a helper needs more than a few lines of C, you are in the wrong layer — wrap less, call from Go more.
- Do not check in wayland-scanner output. Include the headers wlroots already ships.
  - **Amended 2026-10-16 (M1):** some wlroots headers include a protocol header that wlroots generates
    while building but does not install (`wlr_layer_shell_v1.h` includes
    `wlr-layer-shell-unstable-v1-protocol.h`; the other wlr-protocols types do the same). For those, the
    protocol **XML** is vendored unchanged in `protocols/` from the wlroots tag the library was built
    from, and `go generate ./internal/wlr` runs `wayland-scanner server-header` into the gitignored
    `internal/wlr/protocols/`. Generated headers are still never checked in.
- `runtime.LockOSThread()` before any wlr call
- listener structs kept alive until `wl_list_remove`; `cgo.Handle` in C `data`
- no Go pointers in C structs
- GPU reset on `renderer.lost`
- nested `WLR_BACKEND=wayland` for M0–M5; DRM for greeter and session gates

---

## 15. Milestones

Each gate is a running binary. Nested until greeter/session needs a VT. Parity is not “M9 leftover”; every niri surface has a milestone that owns it.

| | Exit gate |
|---|---|
| **M0** | Nested tinywl: foot maps, move/resize, quit, no leaked listeners |
| **M1** | Required globals: layer-shell v4, fractional-scale, viewporter, ARGB8888, xdg-shell, dmabuf |
| **L0** | **sysc-launch** (other repo): compositor PATH `sysc-compositor` then `niri`. Independent of compositor code. Plan: `docs/plans/2026-10-02-sysc-launch.md`. |
| **M2** | KDL parse of greeter config + Spawn/SpawnSh/Quit IPC + `niri msg` **and** `sysc-compositor msg`. greetd VT: gSlapper + sysc-greet, quit. **sysc-greet works.** |
| **M3** | EventStream + Layers + workspace/window snapshots. **sysc-shell starts and draws bars.** `niri validate` on this machine’s config.kdl (actions may still be unimplemented, parser must accept the file) |
| **M4** | Mapped xdg windows in the stream. FocusWindow / CloseWindow / FocusWorkspace. Float-all is allowed |
| **M5** | Scrollable columns, dynamic per-output workspaces, new window to the right. Binds from KDL: focus/move column, workspace 1–9, close, spawn kitty. **This machine’s core binds work nested** |
| **M6** | Consume/expel, tabbed columns, floating, fullscreen, windowed-fullscreen, named workspaces, monitor moves. Window-rule + layer-rule applied |
| **M7** | Overview + gestures (touchpad 3/4, hot-corners) + insert hint |
| **M8** | Springs for the animation kinds in this machine’s config; focus-ring, shadow, corner-radius, clip-to-geometry, prefer-no-csd |
| **M9** | `--session`, graphical-session.target, `niri` CLI complete (`msg` all requests), live-reload, include merge, LoadConfigFile |
| **M10** | ext-background-effect frost; niri-like key repeat; text-input; cursor-shape; session-lock; idle |
| **M11** | Screenshot UI + screenshot-screen/window; hotkey overlay; recent-windows |
| **M12** | Mutter DisplayConfig + ScreenCast + PipeWire; block-out-from; dynamic cast; gnome screenshot portal |
| **M13** | xwayland-satellite; `$DISPLAY`; Steam/Discord nested then session |
| **M14** | A11y KeyboardMonitor + announcements; Orca in a session |
| **M15** | Output-management, VRR, mixed DPI, GPU reset, remaining Actions (debug tint, screen transition, PickWindow/PickColor, PowerOffMonitors, …). **`niri validate` + full action table: zero unimplemented used by default niri config and by this machine’s config.kdl** |

L0 is independent and can start now. M2/M3 own the sysc pipeline. M5–M8 own the WM. M12–M14 own “can replace niri on this box.” M15 is the parity close-out: tick `niri-parity-matrix.md`; do not keep a second copy here.

---

## 16. Testing

- **KDL:** niri default config, greeter kdl, `/home/nomadx/.config/niri/config.kdl` + include
- **IPC:** sysc-shell niri fixtures; generated tests from niri-ipc 26.4.0 enum (decode every Action)
- **Layout:** column/tab/float/overview geometries as tables
- **Rules:** window-rule match/exclude fixtures
- **Nested smoke:** compositor as Wayland client, spawn foot, EventStream JSON
- **CLI:** `sysc-compositor msg --json windows` (and the `niri` alias) against a nested instance
- **Greeter VT, session VM** before replacing niri as the login session
- **Parity matrix:** `docs/plans/niri-parity-matrix.md` listing every Request, Event, Action, config section, D-Bus name — filled as milestones land. The matrix is the remaining-work list; do not keep a second copy in this design.

---

## 17. Non-goals

- Mango tags, mmsg, mango config.conf
- Rewriting sysc-shell, sysc-notify, sysc-clipboard, sysc-tray, sysc-metrics, sysc-plugins, sysc-greet, sysc-walls, sysc-Go
- Rewriting sysc-launch scan/rank/history (L0 is compositor resolution only)
- Turning sysc-wayland into a compositor (it is client-only; AGENTS.md forbids policy)
- A lock screen, wallpaper engine, or notification server in this repo
- Pure-Go DRM (we link libdrm/wlroots through cgo; we do not reimplement KMS in Go)
- A handwritten C compositor (mango). Reference mango’s init order; do not port `.c` files.
- A compositor framework for other shells
- Pixel-identical GLSL to niri’s custom shaders in the first landing of M8 (springs yes; custom shaders tracked in the parity matrix)

---

## 18. References

- niri-ipc 26.4.0: Request, Event, Action
- [niri IPC](https://niri-wm.github.io/niri/IPC.html), config wiki, Xwayland, Accessibility, Integrating, Screencasting
- `/home/nomadx/.config/niri/config.kdl` — personal acceptance config
- `/home/nomadx/sysc-shell/internal/platform/niri/` — shell ABI
- `/home/nomadx/sysc-shell/internal/platform/wayland/registry.go` — required globals
- `/home/nomadx/sysc-launch/service.go` — production `niriRun` / Spawn argv
- `/home/nomadx/sysc-greet/config/niri-greeter-config.kdl`
- mango `src/main.c` `setup()` — wlroots protocol init order (reference only)
- niri `src/dbus/mod.rs` — D-Bus set to reimplement
