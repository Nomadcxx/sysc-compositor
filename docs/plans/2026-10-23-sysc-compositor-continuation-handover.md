# sysc-compositor continuation handover

**Written:** 2026-10-23. **This supersedes the 2026-10-22 continuation handover for planning.** That
handover and the 2026-10-22 review sit uncommitted in the primary checkout (see "Owner decisions
pending"). This file stands on its own. bd is the status authority: run `bd` from
`/home/nomadx/sysc-shell` only. Re-read `git`, `gh` and `bd` before relying on any state here.

## Read first

1. `AGENTS.md`, then `docs/README.md` (the map of every plan).
2. The product contract, `docs/plans/2026-10-02-sysc-compositor-design.md`, §15 for milestones.
3. The current milestone plan, `docs/plans/2026-10-22-m11-screenshot-overlay.md`, including all of
   its execution records.
4. `docs/plans/niri-parity-matrix.md` for what is done (`[x]`), partial (`[~]`) or missing.
5. `bd show sysc-1276` (M11) and its comments, from `/home/nomadx/sysc-shell`.

## Standing rules

These come from the owner and from the hooks on this machine. Breaking them has hurt before.

1. **No nested compositor test without the owner's explicit consent for that one run.**
   - A test that builds a compositor (anything in `internal/wlr` or `internal/server` that calls
     `newNested`/`NewWith`) nests in the owner's live session and opens a window on their desktop.
   - Without `SYSC_NESTED_TESTS=1` those tests refuse to start, so a plain run is safe. Never set
     that variable locally unless the owner consented to that exact test.
   - **Prove nested behaviour in CI lane 2 instead.** That is how every nested test in
     M11 was proved.
2. **One package and one named test per command; never in a loop, script or background job:**
   `timeout 90s env GOMAXPROCS=2 go test -count=1 <pkg> -run '^<Name>$'`.
   - Never `go test ./...`, never `-race`.
   - A hook blocks uncapped `./...` commands. Use `go vet -p 2 ./...` with `GOMAXPROCS=2`.
3. **Pure-Go packages are free to test:** `config`, `ipc`, `bind`, `wm`, `layout`, `animation`,
   `control`, `unixsock`, `session`, `text`, `ui`. A test in `internal/wlr` that builds no
   compositor is also safe locally: `TestKeyNameIsXKBCanonical`, `TestQuitBindRunsWhileLocked`.
4. **Go and cgo only.**
   - No `.c`/`.h` files.
   - **No new C libraries.** The owner rejected pango/cairo outright ("This is a Go project and you
     want to add more C?"). Reach for pure Go first.
   - wlroots and SceneFX are tolerated because nothing replaces them yet. Whether Go can is bd
     `sysc-1646`.
5. **wlroots discipline:** `runtime.LockOSThread()` before any wlroots call, and wlroots only on the
   compositor thread. Work from other goroutines goes through `c.Post`.
6. **Run `go generate ./internal/wlr` once per new worktree.** Set `GOTMPDIR=$HOME/.cache/gotmp`,
   because `/tmp` is a small tmpfs.
7. **Scope.**
   - Only this repository; siblings are read-only, except bd in sysc-shell.
   - Never post GitHub comments or reviews: they appear under the owner's account.
   - Do not invent scope. Ask when an instruction is ambiguous.
8. **Merging.** The owner gave standing approval (2026-10-22, reaffirmed 2026-10-23) to merge PRs
   that review clean with both CI lanes green, "as we go". Merge bottom of a stack first, with merge
   commits, as the history does.
9. **The commit-msg hook** rejects these as case-insensitive *substrings*: claude, anthropic,
   copilot, cursor, codex, gemini, bot, agent, llm and others.
   - Hit so far: "both", "bottom", "agent seat", "cursor-shape", test names containing "Cursor".
   - No Co-Authored-By trailer for an assistant.
   - Check a draft first:
     `grep -noiE "(claude|anthropic|chatgpt|openai|copilot|cursor|cody|tabnine|codex|gemini|bard|gpt-[0-9]|llm|ai assistant|bot|agent)" msg.txt`.
10. **Before every merge:**
    - `gofmt -l .` and `git diff --check`;
    - `go vet -p 2 ./...` and `go build -p 2 ./cmd/sysc-compositor ./cmd/niri`;
    - `go.mod`/`go.sum` unchanged unless the owner approved the dependency.
11. **Plans are written when a milestone starts.** Register every new document in `docs/README.md`
    in the same commit. Status lives in bd, not in documents.

## Where things stand

- **`origin/main` is `298db25`.** M0–M10 are merged, and M11 is mostly done. No PRs are open.
- **Merged on 2026-10-23 (M11 Task 4):**

| PR | What |
|---|---|
| #34 | Go text rendering and the exit dialog. `internal/text` (go-text/typesetting for font discovery, per-rune fallback and HarfBuzz-port shaping; `x/image` rasterizes; a strict Pango-markup subset parser). `internal/ui` draws panels as images. `internal/wlr/image_buffer.go` is a read-only `wlr_buffer` over a C copy of Go pixels. `Quit` asks first through Niri's exit dialog (IPC `skip_confirmation`; the `quit` bind with `skip-confirmation`, allowed while locked); `exit-confirmation-open-close` animation. |
| #35 | The hotkey overlay. `hotkey-overlay` config; every bind's `hotkey-overlay-title` kept in `config.OverlayBinds`, also for binds whose action is not applied yet; Niri's `collect_actions`/`format_bind`/`key_name` in `internal/ui/hotkey_overlay.go`; shown at startup unless `skip-at-startup`; it takes no input. |
| #36 | The pointer in screenshots (`internal/wlr/pointer_image.go`: the theme image by name or the client cursor surface, via a shared `renderSurfaceTree`). `show_pointer` now works for `Screenshot`, `ScreenshotScreen` and `ScreenshotWindow`. The screenshot UI gets Niri's help panel and capture button, P, Ctrl+C, and the 32×32 click. |
| #37 | Screenshot selection moves by Niri's `allowed_during_screenshot` binds (50px moves, monitor moves). |

- **Lane 2 found a real bug in #36:** the panel was placed before `c.shotUI` was set. It's fixed in
  `07fdaa2`. Trust lane 2.

## Next: recent windows (the rest of M11 Task 4)

This is the last large M11 piece. Matrix rows: `recent-windows [ ]`; `FocusWindowPrevious` decodes
but is unimplemented. Write it into the M11 plan as a Task 4 section first, then work it as several
PRs. The Niri sources are `src/ui/mru.rs` (about 1,900 lines), `niri-config/src/recent_windows.rs`,
and `src/input/mod.rs` (search `window_mru_ui`, `MruAdvance`, `FocusWindowPrevious`).

1. **Focus timestamps and `FocusWindowPrevious`.** Pure Go, in `wm` or the view layer.
   - Each window keeps a focus timestamp.
   - A focus change stamps it only after `debounce-ms` (default 750). The first stamp, or a
     debounce of 0, stamps at once (`niri.rs` around line 1279).
   - Opening the switcher commits any pending stamp (`mru_apply_keyboard_commit`).
   - `FocusWindowPrevious` focuses the window with the newest timestamp that is not the current
     window (`input/mod.rs` around line 860).
2. **Config:** the `recent-windows` section.
   - `on`/`off`, `debounce-ms`, `open-delay-ms` (default 150).
   - `highlight`: `active-color` 0.6 grey, `urgent-color` (1, .6, .6), `padding` 30,
     `corner-radius` 0.
   - `previews`: `max-height` 480, `max-scale` 0.5.
   - Its own `binds` block of `next-window`/`previous-window` with `scope=` and `filter=`. The
     defaults are Alt and Mod with Tab (filter all) and with grave (filter app-id); Shift reverses.
   - `animations.recent-windows-close`: a spring, 1.0/800/0.001.
3. **The MRU model (pure Go, unit-tested):**
   - every window across workspaces, newest first;
   - scopes All, Output and Workspace; the app-id filter;
   - forward, backward, first and last; the current id always in the filtered list (Niri's
     `verify_invariants`, with tests in `src/ui/mru/tests.rs` to port).
4. **The UI:**
   - opens on the active output after `open-delay-ms`;
   - previews use `sceneFXCloneView`, as the overview already does (`internal/wlr/overview.go`);
   - titles go through `internal/text`, with Niri's scope panel markup (`mru.rs` around line 1728);
   - the highlight, and the 0.8 black backdrop;
   - the close animation, and pointer selection.
5. **Input:**
   - the switcher's own binds while open: Niri's `make_preset_opened_binds` (Escape cancels,
     Return/Space confirm, a/o/w set the scope, s cycles it, Home/End, arrows);
   - releasing every modifier confirms (`input/mod.rs` around line 484);
   - `MruConfirm` focuses the selection, `MruCancel` restores.
6. **Hotkey overlay follow-up.** The overlay should list the MRU binds once they exist; check
   Niri's `collect_actions`.

**Text is solved.** Use `text.NewLayout` and `text.ParseMarkup`, draw into an `*image.RGBA` in
`internal/ui`, then show it with `sceneImage`/`sceneImagePlace` (and `sceneImageIgnoreInput` when it
must not take the pointer).

**Stop:** the recent-windows rows are proved, `FocusWindowPrevious` is ticked, and the PRs are merged.
Then close out M11 (Task 5 of the plan) and ask the owner to schedule the laptop gate.

## Other open work

| Item | Where | Note |
|---|---|---|
| M11 laptop nested gate | M11 plan Task 5 | grim, wl-copy/wl-paste against sysc-clipboard, screenshot UI, overlays. Needs the owner. |
| M10 laptop gate | bd `sysc-1643` | Needs the owner. |
| Real `--session` run | bd `sysc-1631` | Blocks suite integration `sysc-1283`. Needs the owner. |
| Eased animations | M15 row "animation easing" | `screenshot-ui-open` (200ms ease-out-quad) and Niri's default window open/close are easing. The engine is springs only. Do not fake with a spring. |
| Screenshot size binds | screenshot matrix row | Need `set-window-width/height` and `set-column-width`, which are not built yet. |
| Go rewrite investigation | bd `sysc-1646` | Report only: SceneFX first, then wlroots; what still needs cgo (EGL/GLES, Vulkan, libinput). |
| CI quarantine | bd `sysc-1638` | The quarantined lane-2 tests fail on DRM for nested-only assumptions, Alt-as-Mod (Mod is Super there), container gaps, and keyboard timing. New nested tests should bind with Ctrl or a resolved modifier, not `Mod`. |
| Flaky lane-2 tests | bd `sysc-1640`, `sysc-1642` | Overview projection; M8 open/close animation. |
| IME keyboard grab | bd `sysc-1639` | Waits for virtual-keyboard (M12). |
| `resume-agent-seat` bind | bd `sysc-1261` | Unblocked. A KDL-only action on physical user-seat input, never IPC or the control socket. |
| ScreenSaver handoff | bd `sysc-1635` | sysc-shell side. |

**Milestones after M11** (each plan is written when it starts, from design §15 and the matrix):

| Milestone | bd | Gist |
|---|---|---|
| M12 | `sysc-1277` | ScreenCast/PipeWire, Mutter DisplayConfig, screenshot portal; foreign-toplevel, virtual input, gamma, tearing, syncobj, tablet globals; window-image capture for previews |
| M13 | `sysc-1278` | xwayland-satellite |
| M14 | `sysc-1279` | a11y KeyboardMonitor; Orca |
| M15 | `sysc-1280` | output management, VRR, the remaining actions, easing; zero unimplemented actions used by the default and this machine's configs |

## Working recipe

- **Worktrees** go under `~/.config/superpowers/worktrees/sysc-compositor/<name>-<date>`, on a
  branch `feat/sysc-<bd>-<topic>-<date>` from `origin/main`. Then run `go generate ./internal/wlr`.
- **Stacked PRs work well.**
  - Open each on the branch below it.
  - After the lower PR merges, run `gh pr edit <n> --base main` **before** merging the next one.
    Otherwise it merges into the dead branch.
  - Fix a lower PR on its own branch, then `git merge` that branch into the ones above. No rebase
    or force-push needed.
- **CI.**
  - Lane 1 (static checks and pure packages) takes about 1 minute.
  - Lane 2 (`internal/wlr` and `internal/server` on DRM/vkms) takes about 15–20 minutes, most of it
    the "software DRM device" step.
  - If that step runs far longer than usual, `gh run cancel` it, wait, then `gh run rerun`.
  - Lane 2 fails on any failure or skip outside `.github/ci/quarantine.txt`. Read the line
    `failed or skipped outside the quarantine:` in `gh run view <id> --log-failed`.
- **Visual checks without a desktop.** A throwaway `zz_preview_test.go` in `internal/ui` or
  `internal/text` can write a PNG to the scratchpad. Look at it, then delete the test.
- **bd.** Record progress as comments on the milestone issue (`bd comments add sysc-1276 "..."`).
  The owner closes milestones after merge.

## Owner decisions pending

1. **The primary checkout** (`/home/nomadx/sysc-compositor`) is still on `9ed2023`, with
   uncommitted 2026-10-22 docs: the review, the review handover, and the 10-20 and 10-22
   continuation handovers, plus a modified `docs/README.md`.
   - `git pull --ff-only` aborts on the README.
   - Do not reset or clean it. Ask whether to carry those docs to a PR.
2. **Eleven merged worktrees** remain under `~/.config/superpowers/worktrees/sysc-compositor/`
   (ci-lanes, cli-naming, m6–m10, m11-screenshot, selection-fix, sysc-1630, terminate-after-destroy).
   Ask before removing them. Keep `proposal/window-preview-capture-20261018`: whether to merge it as
   the M12 contract or close it is still open (2026-10-22 Q15).
3. **Q3 clarification:** should tiled windows keep a hand placement, or always follow the layout as
   in Niri? Until answered, production follows Niri.
4. **Q12:** add the control socket to `sysc-1255`'s first failing test? Awaiting a yes or no.
