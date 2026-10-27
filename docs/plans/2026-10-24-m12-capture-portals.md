# M12: capture, portals and remaining client protocols

> **For implementation:** Use `superpowers:executing-plans` task by task. Each numbered task is a separate reviewable change; keep its regression check with it.

**Goal:** Provide Niri 26.04's portal and screencast contracts, standard live window previews, and the M12 globals assigned by owner decision Q9.

**Architecture:** Keep mapped-window, capture, device and rendering ownership in `internal/wlr`. Extend `internal/session` with the existing godbus dependency for session-only D-Bus services; `internal/server` posts requests to the compositor thread and publishes snapshots and events. Use wlroots and SceneFX for the native operations they already provide.

**Tech stack:** Go + cgo, wlroots 0.20, SceneFX, godbus v5, niri-ipc 26.4.0, system libpipewire (owner-approved below). No standalone C or header files, new implementation language, shell changes, or replacement portal backend.

## Decisions and execution boundaries

- Product scope: design §§7, 8 and 15, the parity matrix, and `sysc-1277`'s Q9 comment. Output management remains M15; output power remains M10.
- The owner approved the standard window-preview direction recorded in the 2026-10-20 continuation handover. The proposal lives on `proposal/window-preview-capture-20261018`, at `docs/plans/2026-10-18-window-preview-capture-proposal.md`.
- On 2026-10-24 the owner approved **fresh IPC IDs after remap**, matching Niri's `MappedId`. An ID belongs to one mapping, never a later mapping of the same xdg role. Keep deliberate resize, floating and windowed-fullscreen state across remap independently of the retired public ID.
- Preserve the decimal-ID join in the proposal. Do not substitute title, app-id, PID or enumeration order for identity. A foreign handle's identifier must agree with the IPC ID from its first announcement.
- Run wlroots on the locked compositor thread. D-Bus and PipeWire callbacks may post work; they must not call wlroots or retain Go pointers in C. Cancel pending requests and release descriptors during shutdown.
- Existing test-consent rules apply. Local compositor tests require consent for that exact run. Use CI lane 2 or the approved session VM for display tests; whole-package runs are allowed only inside that VM. Do not change the host's session, lock, greetd or installed binaries.
- Treat the M10 gaps as follow-up acceptance work, not as evidence that M10 passed: native IME (`sysc-1639`), production locker compatibility (`sysc-1652`, sibling), and idle-policy integration remain open.

## PipeWire decision (approved 2026-10-24)

Standing rule 4 in `2026-10-23-sysc-compositor-continuation-handover.md` says **“No new C libraries.”** The product contract also requires PipeWire streams. The current Go dependencies contain no PipeWire client; the installed system provides `libpipewire-0.3` (1.6.9 on the recovery host). Niri's Rust binding also uses the native library.

The smallest implementation is a cgo binding to the **system libpipewire**, confined to `internal/wlr/pipewire.go`. Keep session state, capture policy, negotiation decisions and teardown in Go. Add `libpipewire` to CI/build requirements, without a Go binding dependency. Keep callbacks and macro wrappers in that Go file's cgo preamble.

The owner approved this scoped exception to rule 4 during session recovery on 2026-10-24: **“Allow libpipewire for M12.”** No further dependency approval is needed for this binding. A purego binding still loads the same C library. Implementing PipeWire's native wire protocol in Go would add a transport and compatibility project. Omitting ScreenCast leaves M12 incomplete.

Tasks 1–5 and 7 do not need a new library. Leave `go.mod` and `go.sum` unchanged throughout unless the owner approves a separate dependency.

## Source map

Use the pinned checkouts; do not build them or move their pins.

| Contract or mechanism | Source |
|---|---|
| Niri 26.04, pin `8ed0da44d974c32c6877d2f4630c314da0717ecb` | `/home/nomadx/references/niri` |
| Mapped identity and foreign-list announcements | `src/window/mapped.rs`, `src/protocols/foreign_toplevel.rs` |
| Capture privacy | `src/render_helpers/mod.rs` (`RenderTarget::should_block_out`), `src/window/mapped.rs`, `niri-config/src/window_rule.rs` |
| D-Bus signatures and errors | `src/dbus/mutter_display_config.rs`, `mutter_service_channel.rs`, `mutter_screen_cast.rs`, `gnome_shell_screenshot.rs`, `gnome_shell_introspect.rs` |
| Stream lifecycle, timing, formats and pointer metadata | `src/screencasting/mod.rs`, `src/screencasting/pw_utils.rs` |
| Cast wire types, actions and events | `niri-ipc/src/lib.rs` (`Cast`, dynamic cast actions and cast events) |
| Mango 0.18.0, pin `a21b10fcbe6156b76ea96178d1941de5cb36137f` | `/home/nomadx/references/mango` |
| wlroots ownership and capture APIs | `src/manage/misc.c`, `src/ext-protocol/foreign-toplevel.c`, `src/main.c`, `src/input/`, `src/ext-protocol/tearing.c` |

Mango supplies API patterns, not its maximize/minimize policy or `shield_when_capture` rule. Installed wlroots headers under `/usr/include/wlroots-0.20/wlr/` define the actual API available here.

## Verification recipe

For each task, write the smallest regression test, demonstrate its failure, implement the owning-layer change, then run the same check again. Proposed test names below become executable checks when their task lands. Run one named test per local command:

```sh
timeout 90s env GOMAXPROCS=2 go test -count=1 ./internal/config -run '^TestBlockOutFromMatchesNiriCaptureTargets$'
```

Substitute that task's package and test name. A skipped compositor test proves no behavior; use the VM or lane 2 for its pass. VM setup and commands live in `2026-10-24-session-vm.md`. Use client checkpoints when observing commits; do not add sleep-based remap or buffer assertions.

Before merging each change: `gofmt -l .`, `git diff --check`, `env GOMAXPROCS=2 go vet -p 2 ./...`, and `env GOMAXPROCS=2 go build -p 2 ./cmd/sysc-compositor ./cmd/niri`. Review locally, keep dependencies unchanged, and require both CI lanes green. Standing merge approval applies. Do not post GitHub comments or invoke external review tools.

## Task 1: mapping identity with retained toplevel state

**Files:** `internal/wlr/input.go`, `windows.go`, `window_snapshot_test.go`, `layout_test.go`; `internal/wm/model.go`, `model_test.go`; affected identity assertions and matrix wording.

1. Add `TestWindowRemapRetiresItsPublicID`: map a controlled client, record its IPC ID, unmap, checkpoint, remap, checkpoint. Require a new nonzero ID, absence of the old ID, and rejection of old-ID focus/close operations. Check the close/open event order in the server's existing event tests.
2. Trace `handleNewToplevel`, map/unmap/destroy, `nextViewID`, and the model's retained maps before changing allocation. The current constructor assigns an xdg-role-lifetime ID; `RemoveWindow` retains state keyed by it.
3. Reserve the initial ID on role creation and allocate a fresh ID on each remap; publish IDs only while mapped. Move retained state to the new ID through the model's owning operation, or retain it on the toplevel owner; choose one owner and clear the old key. Preserve the existing resize, floating placement and windowed-fullscreen behavior. Do not leave retired IDs addressable through the model or control socket.
4. Cover explicit dimensions, floating placement, windowed fullscreen and destruction releasing retained state in the pure-model `TestWindowRemapRetiresItsPublicID` in `internal/wlr/layout_test.go`. Keep the existing model-level size checks. Reset mapping-scoped focus/cast/foreign state while keeping the xdg role alive.
5. Prove the named tests in the right lane, update the existing tests that assert lifetime identity, review, then commit. Update `WindowInfo`'s lifetime comment and the affected parity-matrix entries together.

## Task 2: foreign lists and the identifier prerequisite

**Files:** new `internal/wlr/foreign_toplevel.go`, `foreign_toplevel_test.go`; `compositor.go`, `input.go`, `windows.go`, `workspaces.go`, `client_test.go`, `protocols.go`; protocol XML only if needed by existing generation; extend `internal/wlr/testclient` in Go.

1. Add `TestForeignToplevelIdentifierMatchesIPCFromFirstAnnouncement`. Bind before a window maps, then bind a second client after it maps. Both must receive exactly the same decimal mapping ID; unmap closes the handle and remap creates a new identifier. Include title/app-id updates and client disconnect.
2. Check the constructor boundary first. The installed `wlr_ext_foreign_toplevel_handle_v1_state` accepts title and app-id, but no identifier. The installed constructor generates an identifier and walks bound resources before returning. Replacing `handle.identifier` after construction cannot establish the required first-announcement invariant.
3. Reuse a supported wlroots API if an identifier-capable constructor exists in the actual build. Otherwise implement the two small standard protocol endpoints (ext foreign list and foreign-toplevel capture-source creation) with resource callbacks in Go and trampolines in cgo preambles. Ship the capture-source endpoint with Task 3's privacy and working source handler. Use `wlr_ext_image_capture_source_v1_create_resource` to bridge the source into wlroots' existing copy-capture implementation. Do not mutate wlroots' resource lists, patch the installed library, duplicate the copy-capture engine, or change the approved join contract.
4. Add `zwlr_foreign_toplevel_manager_v1` with mapped-window-owned handles. Keep its parity row partial until M15 supplies Niri column maximize/unmaximize; minimize and rectangle requests follow Niri's no-op behavior. Feed title, app-id, focus, fullscreen and output enter/leave from existing window state. Route supported requests through the current window actions and Niri's semantics. Do not import Mango's minimize/maximize behavior.
5. Add `TestForeignToplevelRequestsUseWindowPolicy` for activation, close and fullscreen, including an unmapped target and session lock. Remove listeners and handles on unmap before destroying their backing scene. Advertise the globals only with working handlers and passing checks.

## Task 3: capture privacy and live window sources

**Files:** `internal/config/rules.go`, `rules_test.go`; `internal/wlr/capture.go`, `screenshot.go`, `windows.go`, `scene_fx.go`; new `window_capture.go`, `window_capture_test.go`; existing capture test client and server config-reload tests.

1. Add `TestBlockOutFromMatchesNiriCaptureTargets`. Parse and merge `block-out-from` using the existing ordered rule resolver. Reject invalid values. `screencast` blocks only screencasts; `screen-capture` blocks screenshots, previews and screencasts. Ordinary display rendering stays visible. Cover rule override/removal on reload and title/app-id rematching.
2. Enforce that policy on all existing capture paths before offering the new one: screenshot screen/window/UI, wlr-screencopy, ext output capture, and later PipeWire. Direct output-scene capture currently bypasses a per-window filter. Render a capture-specific tree when needed; never leak a protected surface through a backdrop, close snapshot, popup, subsurface, overview or switcher clone. Follow Niri's blocked-region rendering for output images.
3. For an allowed foreign handle, lazily create a capture-only client surface tree and wlroots scene-node image source. The source contains that window alone, follows its geometry in buffer pixels, and has a lifecycle owned by that mapping. Refuse a `screen-capture`-protected window source. On policy change, invalidate active window sources before another frame can reach a client.
4. Keep wlroots' frame/session protocol handling. Initial capture produces a full frame; subsequent requests wait for damage, coalesce changes, and send fresh size/format constraints before a resized frame. No periodic preview timer. The client may have one outstanding frame and must destroy it before arming another.
5. Negotiate only SHM formats supported by the renderer and consumer. Follow the proposal's 64 MiB allocation ceiling and overflow/stride/dimension validation. Do not assume that ceiling already protects this server path. Reuse a scene source if it meets the constraints; add a source adapter only where it does not. Leave DMA-BUF previews to a measured need.
6. Add `TestWindowCaptureTracksDamageResizeAndUnmap`: distinct client/backdrop pixels, initial frame, unchanged frame held, damaged frame, subsurface change, resize constraints, unmap, and stale-handle rejection. Add `TestCapturePrivacyChangesBeforeNextFrame` to check window and output routes after a rule reload. Stop sessions and detach listeners before destroying sources, capture trees and foreign handles.

## Task 4: virtual input and the IME keyboard grab

**Files:** new `internal/wlr/virtual_input.go`, `virtual_input_test.go`; `input.go`, `text_input.go`, `text_input_test.go`, `compositor.go`, `client_test.go`; extend the existing Go test client.

1. Add `TestInputMethodGrabReturnsKeysThroughVirtualKeyboard` (`sysc-1639`). Activate text input, install the input-method grab, send a physical key, return an unconsumed key through its virtual keyboard, and require one delivery to the focused client without re-entering the grab. Cover keymap, modifiers, repeat information and grab destruction.
2. Create the wlroots virtual-keyboard and virtual-pointer managers and attach devices through the current seat/device ownership. Validate requested seats and outputs; reject foreign or destroyed objects. Retain device origin so returned IME keys bypass that IME's grab.
3. Apply existing compositor binds, shortcut inhibition, lock/blank and injected-seat Stop rules in their existing order. A virtual protocol must not grant a stopped injected seat control of the user seat or deliver its input to lock surfaces. Keep normal user-seat IME behavior distinct from injected-seat control.
4. Add `TestVirtualInputPreservesSeatAndLockBoundaries`, covering pointer motion/button/axis frames, keyboard disconnect with held keys, unsupported seat/output requests, lock and Stop. Prove handlers, not just registry names, before ticking the globals. Retry the real IME gate in the VM after the protocol check passes.

## Task 5: session-only D-Bus services

**Files:** `internal/session/dbus.go`; new `display_config.go`, `portal.go` and focused `_test.go` files; `internal/server/session.go`, `server.go`, `session_test.go`; existing screenshot/picker and output/window snapshot paths in `internal/wlr`.

1. Use the private D-Bus test helper already in `internal/session/dbus_test.go`. Add `TestPortalServicesUseNiriSignatures` for `org.gnome.Mutter.DisplayConfig`, `org.gnome.Mutter.ServiceChannel`, `org.gnome.Shell.Introspect` and `org.gnome.Shell.Screenshot`, at Niri's exact paths and with its method/property signatures and error replies.
2. Extend `StartSessionDBus` and its shutdown owner. Start these services only for the real session; do not take the host's names during nested runs. Preserve Niri's per-name acquisition/replacement rules and clean up a partially started service on error.
3. Implement DisplayConfig from current immutable output snapshots: state serial, modes, physical/logical monitors, scale/transform, properties and `MonitorsChanged`. Trace `ApplyMonitorsConfig` against Niri; do not promise persistent output configuration or consume M15's output-management work.
4. Implement Introspect from mapped-window snapshots with mapping IDs, title and app-id. Implement Screenshot and PickColor through the compositor's existing UI/rendering paths and Task 3 privacy, including cancellation and encoding/write errors. Keep each concurrent D-Bus request paired with its own reply rather than sharing an uncorrelated result channel.
5. ServiceChannel accepts service-client type 1, creates a Unix socket pair, posts `wl_client_create` to the compositor thread, and returns the other descriptor over D-Bus. Reject other types, close descriptors on failure/cancellation, and preserve the privileged-client classification expected by Niri. Peer credentials from a compositor-created socket pair must not masquerade as the portal's PID.
6. Add `TestServiceChannelOwnsItsDescriptors` and `TestPortalRequestsFailCleanlyOnShutdown`. Prove that nested mode acquires no session service names. Use private buses locally and the VM for full portal integration.

## Task 6: PipeWire ScreenCast and cast IPC

**Prerequisite:** Task 3 privacy and Task 5 session services. The library decision above is approved. This task must deliver real frames before advertising a usable screencast service.

**Files:** new `internal/wlr/pipewire.go`, `screencast.go`, `screencast_test.go`; new `internal/session/screencast.go`, `screencast_test.go`; `internal/server/session.go`, `server.go`, `niri.go`, `eventstream.go`; `internal/ipc/wire.go`, `wire_test.go`; `internal/bind/bind.go`; action decoding/CLI in their current files; CI dependency lists and build documentation if approved.

1. Add `TestScreenCastSessionAndStreamLifecycle` on a private bus. Match Niri's ScreenCast Version, CreateSession, session Start/Stop/Closed, RecordMonitor/RecordWindow, stream Parameters and `PipeWireStreamAdded`. Validate target, cursor mode and remote-desktop-session properties with Niri's wire types. Release objects on stop and caller departure; repeat stop safely.
2. Integrate the system PipeWire loop with the compositor event loop or use a bounded Go handoff that posts rendering to it. Never render from a PipeWire worker. Keep frame buffers alive until the consumer releases them. Negotiate dimensions, formats, strides, timestamps and pointer mode; handle stream pause, resize, removal and daemon failure without a compositor crash.
3. Start with the smallest negotiated format/buffer path that the VM's GNOME portal and consumers accept. Measure before adding zero-copy complexity. If a SHM-first path cannot satisfy a required consumer, add the existing wlroots DMA-BUF capability at this owning boundary and prove format/modifier/fence lifetime. Do not advertise formats the renderer cannot produce.
4. Render window and monitor targets through Task 3 privacy. Support Niri's pointer modes and dynamic target selection. Handle target disappearance and scale/size changes with new negotiation; release the previous target's buffers before a generation can be reused.
5. Add niri-ipc 26.4.0 `Cast` wire types, Casts responses, `CastsChanged`, `CastStartedOrChanged`, `CastStopped`, and SetDynamicCastWindow/Monitor, ClearDynamicCastTarget and StopCast actions. Replace the empty EventStream baseline with the current cast snapshot. Keep casts compositor-owned and expose immutable state to the server.
6. Add `TestCastIPCTracksDynamicTargetAndStop`, plus `TestScreenCastProducesFramesAndSurvivesPipeWireLoss` in the VM: verify a known pixel, dimensions, cursor mode, protected-window pixels, dynamic retargeting, resize, stop and loss of PipeWire. Log and disable ScreenCast when PipeWire cannot initialize, preserving the running desktop and other services. A missing-PipeWire run alone is not milestone proof.

## Task 7: gamma, tearing, explicit synchronization and tablets

Keep these as separate focused changes; they share no new library dependency. Each must have a handler and behavioral proof before its matrix row changes to complete.

| Protocol | Owning files and implementation | Smallest proof |
|---|---|---|
| gamma-control | `internal/wlr/output.go`, `scene_fx.go`, new `gamma_control_test.go`; connect wlroots' manager to SceneFX's gamma-control integration, retaining its failure/restore behavior | `TestGammaControlAppliesAndRestoresOutputRamp`; cover invalid ramp and unsupported output; validate real hardware separately if vkms cannot apply it |
| tearing-control | `internal/wlr/output.go`, `input.go`, new `tearing.go`, `tearing_test.go`; retain per-surface hint, use output async-commit capability and Niri's policy | `TestTearingHintRequiresSupportedPresentation`; cover hint destruction, unsupported backend and failed commit falling back to ordinary presentation |
| linux-drm-syncobj | `internal/wlr/compositor.go`, `scene_fx.go`, new `syncobj_test.go`; create the manager only with a suitable DRM fd and renderer/backend timeline support | `TestExplicitSyncWaitsForAcquireBeforePresentation`; prove acquire/release ordering on a supporting renderer; use a separate capability-negative check on vkms if necessary |
| tablet-v2 | new `internal/wlr/tablet.go`, `tablet_test.go`; use existing input/seat and surface hit-testing paths with wlroots tablet/tool/pad objects | `TestTabletToolFocusButtonsAndRemoval`; cover proximity, axes, tip/button, focus change, pad association and disconnect, plus lock and injected-seat isolation |

Do not fake DRM capabilities to make a registry assertion pass. Record a missing hardware qualification as a limit and keep its tracker gate open. A registry-only test never completes these handlers.

## Task 8: VM gate and milestone close-out

**Files:** this plan's execution record, `docs/plans/niri-parity-matrix.md`, existing session/portal packaging documentation. Track status with bd in `/home/nomadx/sysc-shell` only.

1. Install/document the GNOME + GTK portal pair for the VM's sysc session, with desktop selection and service environment matching Niri. Do not change host portal configuration or implement a portal chooser here.
2. In the VM's real session, verify GNOME portal discovery through ServiceChannel and DisplayConfig; screenshot and color picker; monitor and window screencasts in Firefox/Chromium and OBS; dynamic target switches; pointer modes; and protected-content behavior. Record exact client versions, backend, limitations and named test evidence.
3. Verify the preview contract using a protocol client: join a foreign identifier to Windows IPC, capture distinct pixels, resize, remap, reload privacy and stop. Leave sysc-shell consumer implementation to its own repository (`sysc-787`).
4. Retry the M10 native-IME gate with a real IME and GTK text-input-v3 client. Close `sysc-1639` only after passthrough keys avoid the grab loop. Keep production locker and idle-policy gaps separate; sibling fixes need their own owner scope.
5. Run lane 2 and the standing merge checks for the final changes. Tick only behavior proved by the relevant handler tests. Leave M12 open until capture/portal requirements and the assigned globals have their evidence; owner-only closures remain with the owner.

**Stop condition:** M12's required behavior has passed the VM and CI gates, the matrix names the evidence, and the reviewed changes have merged. Do not start the smaller-item queue or M13–M15 as part of this milestone.

## Execution record: mapped IDs, 2026-10-24

Session recovery confirmed that PRs #54–#56 had merged and that the next work was this milestone. The owner approved the scoped system-libpipewire exception above during recovery.

The first implementation keeps the initial ID reservation at role creation and allocates a fresh ID on remap. `wm.Model.RemapWindow` transfers the retained policy and clears the old keys; the remap also clears its previous focus timestamp. The pure-model `TestWindowRemapRetiresItsPublicID` failed on the previous reused ID, then passed for tiled, floating and windowed-fullscreen mappings, repeated remap, retired actions and retained-state cleanup. The existing open/close animation test now checks fresh IDs and rejection of the previous ID in a running compositor.

Validation used the headless session VM under vkms with a private D-Bus session: `internal/wlr` passed in 55.027 s (166 top-level passes, two existing allowed skips), and `internal/server` passed in 2.439 s. Local named remap and constrained-size checks, vet, formatting, whitespace and builds of both entry points passed. Local code review found no critical or important issues. No host compositor test ran. These checks prove the identity step; portal and capture support still require their own implementation and gates.

## Execution record: foreign lists, 2026-10-24

The ext-list endpoint now announces each decimal IPC ID from its first event. Go owns list and handle membership; the mapping owns the native management handle and listeners. Unmap closes the handles and removes listeners. Retained ext resources become inert and cannot resolve to a later mapping. The wlroots management endpoint publishes metadata, keyboard focus, fullscreen and output state, and routes activation, close and fullscreen through existing window actions. Locked sessions reject those requests.

`TestForeignToplevelIdentifierMatchesIPCFromFirstAnnouncement` failed on missing globals before implementation and passed afterward. It checks early/late binding, one identifier event, metadata changes, explicit handle destruction, stop, observer disconnect, unmap and fresh remap identity. `TestForeignToplevelRequestsUseWindowPolicy` checks the management requests, state, lock rejection and inert handles. Review found that moving an already focused window between outputs could bypass a window notification. The added same-size-output assertion failed on the missed leave event, then passed after foreign synchronization moved into `notifyStateChanged`. The test also covers returning to the original output and disconnect fallback without a window-client commit.

After that correction, the VM's full vkms runs passed: `internal/wlr` in 53.151 s (168 top-level passes, two existing allowed skips), and `internal/server` in 2.435 s. Vet, both entry-point builds, formatting, whitespace and unchanged dependency checks passed. Local review found no remaining critical or important issues. No host compositor test ran.

The ext-list matrix row is complete. Wlr management stays partial for maximize/unmaximize, tracked with M15's missing Niri column sizing policy. The foreign capture-source global remains with Task 3's capture/privacy implementation.


## Execution record: virtual input, 2026-10-27

The virtual keyboard and pointer managers attach validated user-seat devices through the existing input paths. Virtual keyboards retain their Wayland client identity, so only the active IME's own virtual keyboard bypasses its grab. Application delivery uses a stable native seat keyboard and per-source press destinations, preserving held state through late binding, focus changes, overlapping devices and disconnect. Lock, blanking and injected-seat gates release virtual state before blocking it. Explicit inert seat/output resources fail validation instead of becoming unrestricted requests.

Review found that focus changes could replace the active metadata source with the logical seat keyboard. `TestKeyboardFocusPreservesActiveSourceMetadata` failed on stale physical keymaps/repeat and virtual keymap ownership, then passed after source selection retained the real keyboard. All 15 named tests in `virtual_input_test.go` passed individually in the laptop bench's live Wayland session, using Go 1.27.3, wlroots 0.20.2 and SceneFX 0.5.0. No desktop compositor test ran in this continuation.

Local vet, builds of the two entry points, formatting and whitespace checks passed. Spec and code review found no remaining critical or important issue after the source-selection fix. No dependency changed. CI still gates the merge; the real IME/GTK acceptance check remains with Task 8 and `sysc-1639`.


The first Task 4 CI run passed static checks and the full compositor package, then exposed a race in `TestNiriWindowActionsOverSocket`. Its separate connections received `Handled` before the server posted each action, so column consumption could precede column placement. The test now waits for the published placement and grouped state before sending dependent actions. The focused bench test passed after this test-only correction; CI must pass again before merge.
