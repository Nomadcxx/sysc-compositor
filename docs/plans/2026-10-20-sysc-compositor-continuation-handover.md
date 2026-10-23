# sysc-compositor continuation handover

**Updated:** 2026-10-20

This handover covers the remaining compositor program known at this date, including Niri parity and the open Tier 3 work. Read it before choosing the next task. The parity matrix remains the detailed Niri checklist, the design remains the product contract, and Beads remains the status source for tracked work.

## Goal and product direction

Finish sysc-compositor as the normal user session compositor in the **sysc-compositor repository**. The target is Niri's outward behavior and overall shape, informed by Mango's good wlroots patterns, with selected Pleamar capabilities considered through design rather than copied wholesale. Hyprland is relevant for specific rendering, damage, capture, or performance questions.

Niri remains the user facing contract because sysc-shell and the existing desktop already work with Niri. Preserve the Niri KDL vocabulary, niri-ipc JSON, CLI, scrollable column model, session integration, and client compatibility. Use Niri 26.04 and niri-ipc 26.4.0 as the pinned behavior and wire targets. Mango 0.18.0 is the pinned wlroots 0.20 implementation reference for input, seat, scene, listener, and lifecycle patterns. Mango's tags, layout model, and mmsg are not Niri parity and do not transfer automatically.

The user has asked for a better compositor where the evidence supports one. Pleamar is a capability reference for things such as injected input, presence, custom layout and compositor drawn UI, and remote session behavior. It is not a specification to port. Any proposed capability must state its user benefit, compositor or sibling owner, effect on the Niri contract, security model, implementation cost, and a testable stop condition. Keep the compositor in Go and cgo. Do not add handwritten C or header files, Rust, Lua, Luau, or a second shell UI.

The sysc component boundaries still apply:

| Component | Responsibility |
|---|---|
| sysc-compositor | Wayland server, Niri window and workspace policy, Niri IPC and CLI, session protocols |
| sysc-shell | Bars, panels, Settings, launcher, and other layer shell UI |
| sysc-greet | Login selector and greeter TUI |
| Mango | Current greetd host compositor on the laptop |
| sysc-launch, sysc-walls, sysc-notify, sysc-clipboard, sysc-tray | Sibling components using compositor contracts |

The laptop's successful sysc-greet check selected sysc-compositor as a **normal user session** from its Wayland session list. The installed entry is `/usr/share/wayland-sessions/sysc-compositor.desktop`, with `Name=sysc-compositor` and `Exec=/usr/local/bin/sysc-compositor`. sysc-greet passed that Exec to greetd as argv. Mango remains the compositor hosting the greeter TUI. Do not merge these two test targets or change greetd to test the normal session. The desktop entry launches the user binary, but the full M9 `--session` and systemd environment behavior remains outstanding.

## Repository and merged state

The repository is `/home/nomadx/sysc-compositor`, remote `https://github.com/Nomadcxx/sysc-compositor.git`.

- Refreshed `origin/main` is `2888f0b` (2026-10-20), the merge of PR #18.
- PR #17 merged M4 window IPC: mapped window snapshots, stable IDs, focus, close, and workspace focus.
- PR #18 fixed M4 focus and output assignment for a mapped window whose center lies outside the output. Focus routing, keyboard focus eligibility, window snapshots, and workspace selection now use the same focused-output fallback. Its nested wlroots regression and named server fallback check passed. The PR also passed `go vet -p 2 ./...`, both command builds, formatting, and dependency-file checks. GitHub reported no CI checks.
- M1 through M4 are implemented on `origin/main`. M5 through M15 remain in the parity plan and tracker.

The primary checkout is stale and contains local work. Its `main` is `653ff83`, sixteen commits behind `origin/main`. It has a modified `docs/README.md`, an untracked M3 plan at `docs/plans/2026-10-19-m3-session.md`, and this local handover plus its registry row. Preserve all of these. Do not reset, clean, or overwrite the primary checkout. This handover is local and uncommitted, so read it from the primary checkout before creating a worktree. The existing worktree at `/home/nomadx/.config/superpowers/worktrees/sysc-compositor/offscreen-focus-20261020` is the already merged PR #18 branch at `2f9d9e2`; do not use it as the base for new work.

For implementation, fetch the remote and start a fresh branch/worktree from current `origin/main`. Reconcile the local M3 documentation separately. Keep compositor code, plans, and reports in this repository. The shared Beads graph lives in the primary sysc-shell checkout only; that does not make sysc-shell the place for compositor implementation.

## What is implemented and what the live checks prove

### M1 to M4

- **M1:** wlroots globals and layer shell, fractional scale, viewporter, ARGB8888, xdg-shell popups and related required globals.
- **M2:** KDL loading for greeter config, Spawn/SpawnSh/Quit, `niri` and `sysc-compositor` CLI entry points, config selection, and the Mango hosted greetd check.
- **M3:** Niri state queries and EventStream baseline, Layers, workspace/output snapshots, positional KDL includes, and `niri validate` for the laptop config. The user has since reported a nested laptop check with sysc-shell's bar and Settings, state requests, EventStream, and config validation running on sysc-compositor.
- **M4:** mapped xdg windows in snapshots and events, stable window IDs, FocusWindow, CloseWindow, FocusWorkspace, and the offscreen-center fallback from PR #18.

There is an evidence conflict to resolve before citing exact M3 coverage: the local M3 plan's dated execution record says the laptop nested gate was unrun, while later session progress reports that the nested shell and IPC gate passed. The M3 plan was not updated with that later evidence. Verify the live record or repeat the nested check before using that old statement as the current result. The later report did not prove a second output, capture, blur, or post-M4 window behavior.

The matrix at `docs/plans/niri-parity-matrix.md` is authoritative for IPC and config details. On refreshed `origin/main`, it records:

- Outputs, Layers, FocusedOutput, the main workspace events, and M3 include parsing as implemented.
- Workspaces and EventStream as partial: one active empty workspace per output, with dynamic workspaces later; initial event baselines exist, with later events owned by M5 and onward.
- M4 Windows, FocusedWindow, window events, FocusWindow, CloseWindow, and the current-output FocusWorkspace path as implemented. FocusWorkspace is partial because named workspaces arrive in M6.
- Keyboard layout events, Overview state, ConfigLoaded, and Casts as partial initial states.
- All 141 Niri actions decode, but many return `unimplemented: <Name>`.

Do not infer complete parity from M3/M4 completion. The matrix still has missing requests, events, actions, config sections, CLI behavior, and D-Bus names.

## Current implementation shape

The design describes future packages such as `internal/wm`, `internal/layout`, `internal/rules`, `internal/anim`, `internal/shot`, and `internal/cast`. On refreshed `origin/main`, the implementation currently has `cmd/niri`, `cmd/sysc-compositor`, and packages including `internal/cli`, `internal/config`, `internal/control`, `internal/ipc`, `internal/server`, `internal/unixsock`, and `internal/wlr`. There is not yet a separate `internal/wm` or `internal/layout` package. Treat the design package map as intended ownership, not proof that those packages already exist.

`internal/wlr` owns wlroots and its Wayland objects. The compositor runs on one locked OS thread; no wlroots call may run off it. `internal/server` owns policy and posts operations to the compositor thread. IPC, config, CLI, unix socket framing, and control protocol code are Go. Keep this ownership clear as M5 introduces a real window and layout model. In particular, keep pure layout decisions testable without cgo, and keep scene mutation on the compositor thread.

The existing Tier 3 implementation is also in `origin/main`:

- `internal/wlr` and `internal/server` provide the user seat and on-demand injected agent seat. The injected seat has separate keyboard and focus routing and does not take the drawn pointer or user seat clipboard focus.
- `internal/control` provides the local `0600` JSON-lines control socket and Stop behavior. The socket checks `SO_PEERCRED` for the compositor UID. This is same-user identification, not authentication. Any connection can set Stop; no socket command can lift it. Mutations are refused after Stop while reads continue.

## Immediate gate: reconcile sysc-1212 before M5

M5 is the next Niri milestone, but the design gate for Pleamar inspired compositor capabilities needs resolution before M5 implementation. This gate was set by the prior continuation work and is not currently represented as a Beads dependency edge, so do not mistake `bd ready` for a decision that the question is settled.

The older WM-as-scene design and the current parity matrix say the Luau scene mechanism is excluded and recommend against building a scene language. That document predates the user's renewed direction: preserve Niri's overall shape, learn from Mango, and assess Pleamar capabilities while building a better compositor. Keep the hard Go and cgo boundary. Do not treat the old row marked “permanently” as the final answer to the broader sysc-1212 question, and do not silently remove it either. Reconcile the design with the user's current direction, then record which capabilities are wanted and which remain excluded.

The capability report is available at `docs/plans/2026-10-19-niri-mango-pleamar-capability-research.md` on refreshed `origin/main`. It is also tracked in the M3 worktree at `/home/nomadx/.config/superpowers/worktrees/sysc-compositor/m3-session-20261019/`; the stale primary checkout at `653ff83` does not contain it. The Beads description for sysc-1212 points to that worktree's absolute path. The file is now on main, so use the repository path for future references rather than depending on the worktree path. The report's capability recommendations remain useful, but refresh its M3-era repository and milestone status against current main before relying on those details. Relevant source material:

- `docs/plans/2026-10-15-tier3-wm-scene-equivalent-design.md`, including its Pleamar scene briefing and design options.
- `docs/plans/2026-10-15-tier3-session-domain-design.md`, including agent input, multi-seat, and browser remote access.
- `/home/nomadx/sysc-shell/docs/plans/2026-10-14-pleamar-improvement-ledger.md` and `2026-10-14-pleamar-gap-ledger.md`.
- `/home/nomadx/sysc-shell/research_notes/pleamar-gap-analysis/`.
- `docs/plans/2026-10-16-input-pattern-comparison-report.md` for the completed Niri/Mango input comparison and its note that Hyprland was not consulted.

Before M5 code, the next agent should make the capability decision reviewable: identify the specific capability, compare Niri, Mango, and Pleamar behavior, decide its owner and compatibility cost, note security and hardware gates, and give it a measurable stop condition. If the proposed capability changes the public KDL or IPC contract, present that choice to the owner. Then write the M5 implementation plan when M5 starts, as required by `docs/plans/2026-10-02-orchestration.md`. Do not pre-write later milestone plans.

## Remaining Niri parity: M5 through M15

The milestone order and exit gates live in `docs/plans/2026-10-02-sysc-compositor-design.md` §15 and `docs/plans/2026-10-02-orchestration.md`. The table below summarizes them; update the parity matrix and Beads issue as each milestone lands.

| Milestone | Remaining behavior and exit evidence |
|---|---|
| **M5: columns, workspaces, binds** | Build Niri's scrollable horizontal columns and dynamic per-output workspaces. Put a new tiled window to the right of focus. Parse and apply layout and binds. Implement core focus/move column, workspace 1–9, close, and spawn kitty actions. Make this laptop's core binds work in the nested testbench. `WindowLayoutsChanged` and the current partial workspaces/Action/input rows belong here. Apply the Mango lessons below for nested Mod defaults, grab cursors, and configuration ownership. |
| **M6: full window policy** | Consume/expel, tabbed columns, floating, fullscreen and windowed fullscreen, named workspaces, monitor moves, window rules, and layer rules. Finish urgency behavior. Add correct edge-based floating resize and synchronize geometry to client commits. Cover rules, focus policy, and workspace naming with pure Go and nested tests. |
| **M7: overview and gestures** | Overview IPC/state and UI behavior, touchpad 3/4 finger gestures, hot corners, drag-and-drop edge scrolling, and insert hint. Keep the UI in the compositor only where Niri owns it; sysc-shell remains a client. |
| **M8: animation and decoration** | Configurable springs for the animation kinds used by the laptop config; focus ring, shadow, corner radius, clip-to-geometry, and prefer-no-csd. Custom GLSL parity is a later matrix item, not a reason to skip springs. Measure real hardware before setting timing budgets. |
| **M9: session and live config** | Full `--session`, graphical-session target and environment publication, complete `niri` CLI request coverage, live reload, `LoadConfigFile`, `ConfigLoaded` updates, and keyboard layout configuration/actions. M3 positional include parsing is already present; M9 must make reload and merge behavior match Niri. This is where the normal session becomes integrated with user services. |
| **M10: input and idle protocols** | ext-background-effect frost, Niri-like compositor bind repeat, text-input, cursor-shape, session-lock, and idle. Place relative pointer and pointer constraints in this milestone plan because the input comparison identifies them but the original milestone table does not assign them. Lock and idle must also gate injected seats, as required by sysc-1260. |
| **M11: screenshot and switcher UI** | Interactive screenshot selector, screen/window screenshot actions and capture path, hotkey overlay, recent windows/Alt-Tab, and screenshot events. Preserve compositor ownership of selector and overlay behavior. |
| **M12: capture and portals** | Mutter DisplayConfig and ScreenCast, PipeWire streams, dynamic cast targets, block-out-from privacy rules, screenshot portal, Casts IPC/events, and the required D-Bus names. Use the user's approved standard protocol proposal for switcher window images: join `ext_foreign_toplevel_image_capture_source_v1` to the stable window ID and capture frames with `ext_image_copy_capture_v1`; bind dimensions and geometry to the window; allow at most one outstanding frame; update on damage; stop when the switcher closes or the window disappears. Suppress windows protected from `screen-capture`; leave `screencast`-only rules scoped to screencasts. This proposal is approved direction, not implemented code. |
| **M13: Xwayland** | Integrate xwayland-satellite on demand, publish DISPLAY, and qualify Steam/Discord first nested, then in a real session. Keep rootful Xwayland and gamescope as user recipes. |
| **M14: accessibility** | Implement the a11y KeyboardMonitor and announcements, then prove Orca in a user session. |
| **M15: parity close-out** | Output management, VRR, mixed DPI, GPU reset, PickWindow/PickColor, remaining actions and config sections, and full parity matrix review. Zero unimplemented actions used by the default Niri config and the laptop config is the exit gate, along with the IPC and output checks in the design. |

Other matrix gaps to keep visible while executing those milestones:

- Missing requests: KeyboardLayouts (M9), PickWindow and PickColor (M15), Output (M15). OverviewState and Casts are partial until M7/M12.
- Missing events: workspace and window urgency (M6), WindowLayoutsChanged (M5), keyboard layout and config reload completion (M9), overview (M7), screenshot (M11), and cast lifecycle (M12).
- Config gaps: layout/binds (M5); window/layer rules and workspace (M6); gestures/overview (M7); animations and prefer-no-csd (M8); switch-events and reload behavior (M9); cursor (M10); screenshot path, hotkey overlay and recent windows (M11); xwayland-satellite (M13); debug and output management (M15). Input and output parsing are partial; output application is M15.
- D-Bus gaps: ScreenSaver (M10); GNOME DisplayConfig, ScreenCast, Screenshot, and Introspect (M12); a11y KeyboardMonitor (M14); locale1 and login1 integration (M9).
- Actions are tracked against the pinned 141-action enum. Do not mark an action complete because it decodes; it must perform Niri behavior. Spawn and Quit remain partial for activation tokens and confirmation behavior.

## Tier 3 and connected work

The status below is the Beads status read from `/home/nomadx/sysc-shell` on 2026-10-20, cross checked against refreshed compositor code. GitHub issues #1–#4 in sysc-compositor all still show OPEN. GitHub issue state therefore disagrees with Beads and code for some transferred items; do not use GitHub state alone to infer missing implementation.

The repository map records these four as transferred from sysc-shell GitHub issues #262–#265 on 2026-10-15. Before transferring any more issues, inspect the source issue and its transfer history, check whether sysc-compositor already has a matching issue, and confirm the work is still valid and owned by the compositor. Do not create duplicates or move shell UI and assistant worker work into this repository. The current #1/#2 records need an issue-state audit because their implementation is present while GitHub still lists them open. Keep #3 open for the capability decision; #4 remains deferred behind M12 and a security design.

| Beads item | Current state and remaining work |
|---|---|
| **sysc-1208 / GitHub #1: agent seat and Stop** | Beads is closed; control socket and injected input implementation are on origin/main, with recorded laptop and desktop tests. GitHub #1 still shows OPEN. Do not reimplement. The plan's follow-ups remain: user-seat-only re-arm through KDL (sysc-1261), lock/blank inertness (sysc-1260), and shell presence UI (sysc-1193). |
| **sysc-1211 / GitHub #2: multi-seat isolation** | Beads is closed; one injected agent seat, seat ordering, input routing isolation, and teardown are implemented and tested. GitHub #2 still shows OPEN. Do not reimplement. The plan chose one agent seat with facts indexed per pointer; it did not choose two agent seats. |
| **sysc-1212 / GitHub #3: Pleamar capabilities beyond Niri** | Open. This is the design gate described above. The repo matrix's old permanent scene exclusion and the user's newer broader request need explicit reconciliation. |
| **sysc-1213 / GitHub #4: browser remote session** | Open and recommended to defer. Its design depends on M12 capture and a separate owner approved security model. Do not reuse the same-user agent socket for network access, and do not add a web stack or TLS listener to sysc-shell. |
| **sysc-1260: injected seats inert while locked or blank** | Open. Implement when M10 adds lock/idle paths. Test the injection gate and preserve ordinary user input. |
| **sysc-1261: user-only re-arm after Stop** | Open. Depends on M5 KDL binds. Only a user-seat action may clear Stop; no control socket verb may clear it. |
| **sysc-1193: agent-seat presence UI** | Open cross-repo sysc-shell work: monitor glow, self-drawn cursors, and a Stop UI. The compositor supplies facts; shell owns presentation. |
| **sysc-1253: Tier 3 ownership and assumption audit** | In progress in the shared graph. sysc-1254, the shell binding the user seat rather than the last advertised seat, is closed. sysc-1255, isolating the assistant worker from AF_UNIX, the runtime directory, compositor environment, and `/dev/uinput`, remains open in the assistant/plugin boundary, not as compositor UI. |
| **sysc-1296: pointer focus correctness** | Closed and implemented before M4: implicit grab, default cursor over empty space, popup/subsurface root lookup, unmap cleanup, and compositor grab focus clearing. |
| **sysc-1299: frame done after scene commit** | Closed. |
| **sysc-1436: M4 offscreen FocusWindow regression** | Still open in Beads although PR #18 merged its fix to origin/main. Do not repeat the code work. The issue export is already dirty with unrelated changes; leave tracker reconciliation to a clean Beads operation. |
| **sysc-1262 / sysc-1270–1280: parity epic and M5–M15** | Epic and remaining milestone issues are open. M5 is sysc-1270, followed by the order shown above. |
| **sysc-1283: sysc suite integration** | Open and depends on M9. It covers shell, greet, launch, lock, walls, and installer integration. The session selector entry exists, but full session service integration is not finished. |

Other connected work includes sysc-1317, shell keyboard layout selection, which depends on M9. Shell controls remain in sysc-shell. Keep its Beads issue in the shared graph; do not implement shell UI in this repository.

## Beads ownership and link audit

The Beads database intentionally lives in `/home/nomadx/sysc-shell`; the issue record schema has no repository field. The graph is shared across repos by design, so use issue titles, labels, parent epics, and document paths to identify code ownership.

- **Compositor milestone scope:** sysc-1262 names `/home/nomadx/sysc-compositor` and owns M1–M15. Its M1–M15 children use paths relative to that repository's docs tree. The M5–M15 dependency chain is intact.
- **Tier 3 transfer:** sysc-1208, sysc-1211, sysc-1212, and sysc-1213 map to sysc-compositor GitHub #1–#4 in this repository's README. Their Beads label is `pleamar`, reflecting the source work, rather than `compositor tier3`; confirm whether adding the compositor label would improve filtering when the issue export can be reconciled.
- **Correct sibling ownership:** sysc-1264 is L0 work in sysc-launch; sysc-1265 is S0 in sysc-shell; sysc-1193 is shell presence UI; sysc-1253 and sysc-1255 are cross-repo ownership and assistant-worker work in sysc-shell; sysc-1283 is the suite integration epic. These belong in the shared graph but their implementation does not move into sysc-compositor.
- **Stale path:** sysc-1212's description points at the M3 worktree path for the capability report. The report is tracked on origin/main at the repository-relative path above. Normalize the Beads reference to that stable path when tracker edits are safe.
- **Potentially stale status:** sysc-1311 remains in progress although commit `bb110cb` adds `TestNestedSessionEndsWhenLastOutputCloses` and the fix is on origin/main. Recheck its acceptance gate before closing it.
- **Dependency/notes conflict:** sysc-1316 is closed, but its notes say to keep it open until integrated and say M6 is a compatibility gate rather than a blocker; the graph still lists M6 as a dependency. Sysc-1317's notes likewise say M9 is related work, not a blocker, while its graph lists M9 as a dependency. Reconcile these edges and statuses against the current shell branches before changing them.
- **Merged but open:** sysc-1436 remains open in Beads although PR #18 merged its fix. Do not reimplement it; reconcile its status with the issue export only after the existing dirty export is handled.

`bd validate` found no orphan dependency references, but reported 65 duplicate groups and 27 pollution candidates across the shared tracker. Treat those as tracker wide cleanup signals, not evidence that compositor work belongs in another repository.

## Mango, Hyprland, and Pleamar references

The input comparison at `docs/plans/2026-10-16-input-pattern-comparison-report.md` compared Niri 26.04 and Mango 0.18.0 against the M0 code. It already applied the most useful Mango substrate lessons: one keymap group for the user seat, a separate injected seat, pointer implicit grab, default cursor restoration, popup and subsurface root lookup, focus cleanup on unmap, and frame completion behavior. These are implemented and should not be reopened without a reproducer.

The same report leaves these later items for milestone plans:

- **M5:** choose nested Mod defaults like Niri, show move/resize grab cursors, configure cursor theme and size, and preserve per device input handling.
- **M5/M6:** resize from the edge nearest the pointer; double click reset behavior belongs with M6 layout actions.
- **M6:** synchronize floating resize to client configure commits instead of moving the scene tree ahead of the client.
- **M10:** compositor bind repeat, relative pointer, pointer constraints, and cursor-shape.
- **M15:** load cursor themes at each output scale for mixed DPI.

The report explicitly did not consult Hyprland because the first input questions had direct answers in Niri and Mango on the matching wlroots version. The user's current direction names Hyprland as relevant. Use it when a concrete rendering or performance question would benefit from it, record the exact version/commit, and keep it a bounded reference rather than a competing behavior contract.

Pleamar's scene mechanism is Luau and cannot be ported under the Go+cgo constraint. The broader user request is to assess useful capabilities, not necessarily a scene language. Separate declarative layout choices, compositor drawn UI, input automation, capture, and remote access into individual decisions. Preserve the Niri public contract unless the owner approves a documented extension.

## Laptop testbench and engineering rules

The user has explicitly approved the laptop testbench for nested compositor checks. Keep tests nested in the currently running Niri session. At runtime, discover NIRI_SOCKET, WAYLAND_DISPLAY, and XDG_RUNTIME_DIR; do not reuse stale socket names or assume the compositor environment reaches the agent shell. The host has no runtime virtual output, so two output, mixed DPI, and VRR results need another fixture. Record hardware behavior as unrun when there is no second output.

Do not install experimental builds, edit greetd configuration, restart greetd, or reboot for nested tests. The greeter host remains Mango. The normal-user session entry is a separate integration that has already been selected and launched.

Repository rules from AGENTS.md:

- Go and cgo only. No `.c` or `.h` files. Keep wlroots calls on the locked compositor OS thread.
- Never run `go test ./...` or `-race`. Run one named package test at a time:

    timeout 90s env GOMAXPROCS=2 go test -count=1 <package> -run '<TestName>'

- For a fresh checkout, run `go generate ./internal/wlr` once to create ignored protocol headers.
- After code changes, run formatting checks, `go vet -p 2 ./...`, builds for `cmd/sysc-compositor` and `cmd/niri`, and confirm `go.mod`/`go.sum` did not change unless the task needs a dependency.
- Add a focused runnable regression for non-trivial behavior. Use pure Go tests for layout and protocol logic; reserve nested tests for wlroots and live integration.
- Read `docs/README.md`; register each new design or plan. Write M5's implementation plan when M5 starts. Do not pre-write M6–M15 plans.

The shared Beads database is `/home/nomadx/sysc-shell/.beads/beads.db`; run tracker commands from that primary checkout. Its tracked `issues.jsonl` already has a large dirty diff from other work. Do not export, reset, or force-add it as part of compositor work. A closed issue does not prove the fix is on main; check origin/main, as with sysc-1436.

## Suggested continuation sequence

1. Read `docs/README.md`, this handover, the design, orchestration, parity matrix, and the M3/M4 records. Confirm the latest origin/main and working tree before starting a clean worktree.
2. Reconcile sysc-1212 using the extant Tier 3 documents and Pleamar ledgers. Record which requested capabilities fit Niri parity, which require a deliberate extension, who owns each, and the acceptance evidence. Resolve the contradiction between the old scene exclusion and the user's current direction.
3. Recheck Tier 3 code and issue state, including the transfer history for GitHub #1–#4 and any still-local sysc-shell source issues. Normalize sysc-1212's capability-report reference and audit sysc-1311, sysc-1316, sysc-1317, and sysc-1436 status/dependency inconsistencies. Keep merged agent seat, multi-seat, pointer focus, and offscreen-focus work closed in the implementation sense; report the GitHub/Beads discrepancy without duplicating code. Transfer only still-valid compositor-owned issues that lack a destination issue.
4. Once the M5 gate is clear and M5 is scheduled, write its plan from design §15 and the current matrix. Keep implementation scoped to M5, use Niri for behavior and Mango for wlroots patterns, and add the first failing focused checks before code.
5. Run the nested laptop gate. Update the parity matrix and record results, including anything that needs a second output. Then continue through M6–M15 in dependency order.
6. Complete the normal session integration after M9, then M12 portal/capture, M13 Xwayland, M14 accessibility, and M15 parity close-out. Keep sysc-shell and sysc-greet changes in their repositories and use sysc-1283 to coordinate suite integration.

Stop each milestone when its exit gate passes. Do not pull later work into the current milestone just because the parity list is long.
