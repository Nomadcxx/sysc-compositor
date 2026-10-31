# M14: accessibility KeyboardMonitor and announcements

Commission: the owner's go-ahead to continue the plan, 2026-10-31. Tracker: `sysc-1279` in
`/home/nomadx/sysc-shell`. Contract: design §11 and §15; parity matrix's
`org.freedesktop.a11y.KeyboardMonitor` row. Use `superpowers:executing-plans` to execute these
tasks.

## Design and reference comparison

Niri 26.04 is the only reference with this feature; Mango 0.18.0 and Hyprland at the pinned commit
have no KeyboardMonitor, AT-SPI or AccessKit code. Niri's behavior is therefore the contract:

- `src/dbus/freedesktop_a11y.rs` serves `org.freedesktop.a11y.KeyboardMonitor` at
  `/org/freedesktop/a11y/Manager` under the name `org.freedesktop.a11y.Manager`. It requests the
  name with allow-replacement, replace-existing and do-not-queue, as KWin and Mutter do (both
  cited in the file). Per caller unique name it keeps `watched`, `grabbed`, a modifier keysym set
  and `(keysym, mods)` keystrokes. It forgets a caller when NameOwnerChanged reports it gone.
- `KeyEvent(released b, state u, keysym u, unichar u, keycode q)` goes only to callers that watch
  or grab the key, addressed to that caller.
- A key is grabbed by a full-keyboard grab, by being a grabbed modifier, by being pressed while a
  grabbed modifier is held, or by matching a grabbed keystroke exactly. A grabbed press is
  suppressed with its release, and a second press of an already suppressed key is blocked.
- A grabbed modifier pressed twice within the repeat delay passes through, so Insert or Caps Lock
  keep their normal meaning when tapped twice. The first press of a grabbed modifier must not
  change XKB state (`KbMonBlock::ModifierFirstPress`).
- Other grabbed keys update XKB state but reach no client and no bind. Modifier releases are still
  forwarded to the focused client (the Orca+Shift+A menu workaround in `src/input/mod.rs`).
- `src/dbus/mod.rs` starts the monitor only in a session instance (or under a debug flag). Niri's
  wiki: nested and plain TTY instances export no accessibility interfaces.
- `src/a11y.rs` uses AccessKit's Unix adapter. Its tree is a `Window` root (toolkit `niri`) with
  a polite live `Label` and four groups: "Screenshot UI", the exit dialog, "Overview" and "Recent
  windows". AT-SPI focus follows the compositor's keyboard focus among them. Announcements cover
  workspace switches ("Workspace N name"), the config error text, the hotkey overlay text and the
  recent-windows scope. The recent-windows node flips between `Group` and `Button`, labelled with
  the selected window's title. A trailing space is appended to repeat an identical text.
- AccessKit 0.21 / accesskit_unix 0.17 emits a live node's name change as
  `org.a11y.atspi.Event.Object.Announcement` with `detail1` = politeness and `any_data` = the
  text, plus `PropertyChange accessible-name`. The adapter registers with the AT-SPI registry
  only while AT-SPI is enabled.

wlroots fits the first-press rule natively. In 0.20.2 `wlr_keyboard_notify_key`
(`types/wlr_keyboard.c`) emits the key signal before it updates XKB state and only then checks
`event->update_state`. The compositor's key listener can therefore read pre-press state, as
Niri's `a11y_process_key` does, and clear `update_state` for a first modifier press.

Use the existing godbus (`third_party/godbus`) for both buses. Add no Go or native library;
AccessKit is Rust and is not linked. The AT-SPI side is a small hand-written application object,
not a general toolkit bridge. sysc-shell's own AT-SPI export (`sysc-1706`, `sysc-1301`) stays in
that repository.

No default bind changes. Niri's in-code defaults have no binds; `Super+Alt+S` toggling Orca lives
only in `default-config.kdl`, which sysc does not write (owner decision, 2026-10-31). A user config
carrying `Super+Alt+S allow-when-locked=true hotkey-overlay-title=null { spawn-sh "pkill orca ||
exec orca"; }` must parse and act; add that to the bind regression if it is not already covered.

## Task 1: the KeyboardMonitor model

Files: new `internal/a11y/keyboard_monitor.go`, `keyboard_monitor_test.go`. Pure Go, no D-Bus and
no wlroots.

Add one named table regression, `TestKeyboardMonitorGrabsLikeNiri`, before the code. It should
cover watch-only delivery, a full grab, modifier grabs with a held modifier, exact keystroke
masks, a release following a suppressed press, a duplicate press from a second keyboard, a double
press within the repeat delay passing (and its release passing), a press after the delay
blocking again, caller removal rebuilding the grabbed modifiers, and the three decisions `Pass`,
`ModifierFirstPress` and `Block`. Then implement `Monitor` with a mutex: `Grab/Ungrab/Watch/
Unwatch/SetKeyGrabs(caller, ...)`, `Forget(caller)` and `ProcessKey(...) (Decision,
[]Delivery)`. `ProcessKey` returns the callers to signal; it emits nothing itself.

## Task 2: the D-Bus service

Files: new `internal/session/keyboard_monitor.go`, `keyboard_monitor_test.go`;
`internal/server/session.go`.

Follow `screensaver.go`: subscribe to NameOwnerChanged before exporting, export the interface and
introspection at `/org/freedesktop/a11y/Manager`, then request the name with Niri's flags. Name
loss or a closed connection forgets every caller, so no grab outlives the service. Deliveries go
out from one sender goroutine through a bounded queue; the compositor thread never writes to the
bus. If the queue fills, drop deliveries and log once, without blocking input. Add
`TestKeyboardMonitorServesCallersOnPrivateBus` using the package's private-bus helper. It covers
method calls from two callers, unicast KeyEvent arguments with signature `buuuq`, caller departure clearing grabs, and refusal while another owner holds the
name without replacement. Start it from `StartSessionDBus` only.

## Task 3: the compositor key path

Files: `internal/wlr/keyboard.go`, new `internal/wlr/a11y_keys_test.go`; server wiring.

At the top of `handleKeyboardKeyFrom`, for the user seat's keyboards only (never virtual or
injected seats), read the keysym, effective modifier mask and UTF-32 from the pre-update XKB
state. Pass them, the event time and the configured repeat delay to the monitor hook.
- On `ModifierFirstPress`, clear `event.update_state`, mark the key suppressed and return.
- On `Block`, mark it suppressed and return, but let a modifier's release reach the client, as
  Niri does.
- Binds, the exit dialog, the screenshot UI and VT switching never see a blocked key.

Regressions: `TestKeyboardMonitorBlocksGrabbedKeysBeforeBinds` (a grabbed bind key reaches neither
bind nor client; its release does not leak), `TestKeyboardMonitorFirstModifierPressKeepsXKBState`
(a grabbed Caps Lock's first press leaves the locked mask and LED unchanged; a double tap toggles
it) and `TestKeyboardMonitorReleasesGrabbedShiftToClient`. With no monitor attached, nested runs
behave exactly as now.

## Task 4: AT-SPI announcements

Files: new `internal/a11y/atspi.go`, `atspi_test.go`; server wiring for session instances.

Connect to the accessibility bus from `org.a11y.Bus.GetAddress` on the session bus. Watch
`org.a11y.Status` `IsEnabled`/`ScreenReaderEnabled` and register only while enabled.
- **Objects:** export a root application object (`org.a11y.atspi.Application` and `Accessible`,
  role application, toolkit name `niri`, the version) and a window child holding a live label
  and the four group nodes at stable object paths. Implement the `Accessible` methods Orca reads:
  name, description, role, role name, state set, attributes (`live: polite` on the label),
  parent, child count and index, children, application and interfaces.
- **Registration:** embed through `org.a11y.atspi.Socket.Embed` on the registry, and answer
  `GetApplication` and the registry's children queries.
- **Events:** emit `Object.Announcement` and `Object.PropertyChange accessible-name` as AccessKit
  does, with the trailing-space repeat rule. Emit `StateChanged focused` and window activate or
  deactivate when focus moves among the nodes.

Regression: `TestAnnouncementsReachAnATSPIListener` runs a private session bus plus a private bus
standing in for the a11y bus and registry. It checks embedding, the tree a listener walks, and
the announcement signal body for a workspace switch and a repeated identical text. A disabled
status registers nothing.

## Task 5: announcement and focus sources

Files: `internal/wlr/{workspaces,exit_dialog,screenshot_ui,overview,switcher,hotkey_overlay}.go`
as needed, the config-error path in `internal/server`; tests next to each.

- Announce workspace activation on the focused output as "Workspace N" plus a name if it has
  one, the config error text, and the hotkey overlay's text as one announcement.
- Move AT-SPI focus to the screenshot UI, exit dialog, overview and recent-windows nodes as
  keyboard focus enters them. In recent windows, announce the selected title, the scope change
  text and "Recent windows empty, <scope>".
- Reuse the existing UI text owners (`internal/ui`) for the strings. Add `A11yText` beside them
  only where Niri's text differs from the drawn text.
- Each source has one named regression. Session-less compositors keep a nil announcer.

## Task 6: real Orca gate and finish

In the session VM (virgl `./run.sh`), log in through greetd and install `orca` and
`speech-dispatcher` there only. Check:
- **Discovery:** `busctl --user status org.freedesktop.a11y.Manager` names sysc.
- **Keys:** Orca's key commands work and `Orca+Space` opens its preferences. Caps Lock or
  Insert as the Orca key leaves the lock state alone on a single press and toggles on a double
  tap. Typing into foot still works.
- **Speech:** Orca's debug log (`orca --debug-file`) records the spoken workspace switch, exit
  dialog, screenshot UI, overview, recent-windows titles, config error and hotkey overlay.
  Audio is not required; the debug log is the evidence.

Run lane 2 and the standing merge checks. Update the matrix's KeyboardMonitor row and add an
AT-SPI announcements row naming the tests and the VM evidence. Record execution here. The owner
closes M14 after the Orca gate passes.

**Stop condition:** Orca drives the VM session with its key grabs and speaks every announcement
above. CI is green, the matrix names the evidence and the changes have merged. Do not start M15.
