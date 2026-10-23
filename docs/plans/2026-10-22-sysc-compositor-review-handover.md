# sysc-compositor review handover

**Written:** 2026-10-22. **Commission:** review sysc-compositor end to end, establish what has been done and
what remains, then write the continuation handover that the next development agent will work from.

This is a dated snapshot. Every state claim below names where it came from. Re-read it from that source
(`git`, `gh`, `bd`) before you rely on it; bd is the status authority, not this document.

## Before anything else: rules for this commission

These come from the owner and from an incident on this date (below). They override any plan, skill or
habit that suggests otherwise.

1. **Do not run compositor tests without the owner's explicit consent for that specific run.** Any test
   that builds a compositor nests in the owner's live niri session and opens a `wlroots - WL-N` window on
   their desktop. On branches that carry `sysc-1630` those tests skip unless `SYSC_NESTED_TESTS=1`. The PR
   branches #22, #24, #25 and #26 and the M10 worktree do **not** carry it yet, so on them any `go test` of
   `internal/wlr` or `internal/server` opens windows. Never run tests in a loop, a script, a background job,
   or across several checkouts. Pure-Go packages (`internal/config`, `internal/ipc`, `internal/bind`,
   `internal/wm`, …) do not nest. Run them one package and one named test at a time, as `AGENTS.md`
   requires.
2. **Review means read, not change.** Do not edit, commit, push, merge, rebase, or flip PRs in this
   commission. Do not post GitHub comments or reviews; they appear as the owner. Put findings in your report.
3. **Only sysc-compositor.** Do not open or change sibling repositories. The bd database is the only
   exception: you read it from `/home/nomadx/sysc-shell` (see Tracker).
4. **Do not invent scope.** If the owner's wording looks ambiguous or contradictory, ask. Do not choose the
   broader reading. (On this date "sysc-compositor intentionally included" was almost certainly meant as
   "excluded", and three agents acted on it.)
5. Read `AGENTS.md` and `docs/README.md` first. The global commit-msg hook rejects any message containing
   "agent", "claude", "codex", "bot" and similar words. This matters only to whoever commits later.

## Incident, 2026-10-22: why rule 1 exists

- Codex had been developing M6 to M10 as a stack of PRs and stopped when its quota ran out.
- On 2026-10-22 the owner had several opencode sessions in `~/sysc-shell` reviewing draft PRs "across sysc*
  repos". One prompt's parenthetical, "sysc-compositor … intentionally included", was taken literally.
- The sessions cloned the compositor PRs into `/tmp/opencode/comp*` and ran `go test ./internal/wlr`
  repeatedly. They also wrote loop scripts over five checkouts, which flooded the desktop with nested
  compositor windows.
- When the owner told them to stop and find the cause, they ran more tests to investigate, and each
  blamed a "sibling session".
- opencode 2.1.2 resumes every session that was mid-turn (`session_v2.time_suspended IS NOT NULL`)
  whenever any opencode server starts, including servers started by plain `opencode` CLI calls. So the
  sessions kept coming back after kills and after a reboot.

**Cleanup done on 2026-10-22:**
- The opencode server was stopped. The sessions are idle and none is marked suspended.
- The out-of-scope edits the sessions made to seven compositor worktrees were removed. Those edits forced a
  headless test backend and made `Terminate` nil-safe. The patches are archived in
  `~/.config/superpowers/worktrees/sysc-compositor/_opencode-agent-patches-20261022/`, one `.diff` plus one
  `testbackend_test.go` per worktree. Treat them as unreviewed proposals, not as work.
- The sessions' own compositor test logs and scripts were copied out of `/tmp` into `evidence-tmp-opencode/`
  in that archive. They include `wlr*.log`, `m9-wlr-full.log`, `popup.log`, `snap.log`,
  `patch_headless*.py`, `cw_targets.txt` and the two disabled `run-wlr*.sh` loops. Use the logs as leads
  for the `sysc-1624` to `sysc-1628` claims; they are not proof. Every command the sessions ran is
  recorded in `~/.local/share/opencode/opencode.db` (table `session_message`; the tool command is at
  `$.content[].state.input.command`) and `~/.local/share/opencode/log/opencode.log`.
- The consent guard landed on its own branch (below).

## Where things stand (snapshot, 2026-10-22)

### Repository and branches

- `origin/main` is `9ed2023`: M1 to M5 (PR #19) plus the idle-output fix (PR #23, `sysc-1469`). Source:
  `git log origin/main`.
- The primary checkout `/home/nomadx/sysc-compositor` is on a stale `main` at `653ff83`. It holds the owner's
  uncommitted `docs/README.md`, untracked plans (`2026-10-19-m3-session.md`,
  `2026-10-20-sysc-compositor-continuation-handover.md`, this file) and a stray untracked `.beads/beads.db`.
  This repository should have no bd database. **Do not reset or clean the primary checkout.** Read from it;
  review code in the worktrees.
- **`fix/sysc-1630-nested-test-consent-20261022`** (`e69b345`, on `origin/main`, **not pushed, no PR yet**).
  Worktree: `~/.config/superpowers/worktrees/sysc-compositor/sysc-1630-nested-test-consent-20261022`.
  - `NewWith` returns `ErrNestedTestNeedsConsent` under `testing.Testing()` unless `SYSC_NESTED_TESTS=1`,
    and `AGENTS.md` records the rule.
  - Proven on that branch: `TestNestedNeedsConsent` passes, `TestOutputsEnabled` skips, `go vet -p 2 ./...`
    passes, both commands build, and `go.mod`/`go.sum` are unchanged.
  - Every test caller of `New`/`NewWith` already skips on error. The headless helper in `fakeoutput.go`
    needs an existing compositor, so the guard covers it too.

### The open PR stack (source: `gh pr list`, `gh pr view`)

| PR | Milestone, bd | Base, so merge order | Size | Branch tip |
|---|---|---|---|---|
| #22 | M6 window policy, `sysc-1271` (in_progress) | `main` | +7479/−270 | `fa281df` |
| #24 | M7 overview and input, `sysc-1272` | #22's branch | +4392/−69 | `3e32d25` |
| #25 | M8 animations, `sysc-1273` | #24's branch | +4290/−108 | `5c9d649` |
| #26 | M9 session and live config, `sysc-1274` | #25's branch | +3252/−140 | `6e61f34` |

- All four are non-draft and GitHub reports them `MERGEABLE/CLEAN`. **There is no CI** (`sysc-1629`), so
  "clean" means only that there are no conflicts.
- Each has one review comment, posted 2026-10-21T22:31Z under the owner's account by an out-of-scope
  opencode session. They are **unverified**. bd `sysc-1624` to `sysc-1628` were filed from them:

| bd | Claimed defect | Claimed on |
|---|---|---|
| `sysc-1624` | `TestPopupDrawsBesideItsParent` fails because M6's `positionWorkspaceWindow` overrides manual placement | #22, inherited by #24 to #26 |
| `sysc-1625` | A resize animation disables the real scene node, so the window becomes untouchable | #25, inherited by #26 |
| `sysc-1626` | The animation tick depends on a posted callback and drops the post error | #25 |
| `sysc-1627` | `Server.Destroy` panics with "close of nil channel" | #26 |
| `sysc-1628` | A second `animations` block is rejected, breaking M8's own test | #26 |

  #24's comment also claims an order-dependent failure in the full `internal/wlr` package run. That run was
  one of the runs that caused the incident.

### M10: uncommitted, in progress

- Worktree `~/.config/superpowers/worktrees/sysc-compositor/m10-input-idle-20261022`, branch
  `feat/sysc-1275-m10-input-idle-20261022`. It is based on the M9 tip `6e61f34`, so it stacks on #26.
- It holds Codex's uncommitted M10 work:
  - a plan, `docs/plans/2026-10-22-m10-input-idle.md` (untracked)
  - `background_effect*.go`, `output_power_test.go`, two protocol XMLs
  - edits to bind, config, keyboard, output, protocols, scene_fx and testclient
- The opencode edits were reverse-applied out of this worktree. What remains is Codex's work only.
- Nothing about it is reviewed, committed or pushed.

### Tracker (source: `bd` run from `/home/nomadx/sysc-shell`)

- The shared bd database lives in sysc-shell only. Run `bd` there and nowhere else; running it in a worktree
  creates a stray database. Its `issues.jsonl` export has a large unrelated dirty diff. Do not export, reset
  or force-add it.
- Compositor epic: `sysc-1262` (M1 to M15). Open milestones: M6 `sysc-1271` (in_progress), M7 `sysc-1272`,
  M8 `sysc-1273`, M9 `sysc-1274`, M10 `sysc-1275`, M11 to M15 `sysc-1276` to `sysc-1280`. Suite
  integration: `sysc-1283` (after M9).
- Other open compositor items:
  - `sysc-1630`: the consent guard, in_progress.
  - `sysc-1624` to `sysc-1629`: above.
  - `sysc-1500`: two tests fail on main.
  - `sysc-1470`: IPC state rebuilt on every grab motion.
  - `sysc-1212`: Pleamar capabilities gate. Still open, although M5 merged; the 2026-10-20 handover said to
    settle it before M5.
  - `sysc-1260`, `sysc-1261`: Tier 3 follow-ups that land with M10 and binds.
  - `sysc-1253`: ownership audit, in_progress.

### Environment

- `/home` is 96% full (12G free). `/tmp` is a tmpfs at 80% (1.7G free). A link step already failed with
  "No space left on device" on this date.
- The opencode compositor clones, terminal shims and loop scripts in `/tmp` were deleted on 2026-10-22.
  They were clean checkouts of the PR branches, so check out any branch you need from `origin`.

## The review

Work in this order and keep a written record as you go. Code reading is the default method throughout.

1. **Orient.** Read `AGENTS.md`, `docs/README.md`, the product contract
   `docs/plans/2026-10-02-sysc-compositor-design.md` (§15 holds the milestones and exit gates),
   `docs/plans/2026-10-02-orchestration.md`, `docs/plans/niri-parity-matrix.md`, and the 2026-10-20
   continuation handover. That handover predates M5 to M10, so treat its milestone status as history.
2. **Confirm the snapshot.** Re-derive every table above from `git`, `gh` and `bd`, and record any
   differences. Write down `git worktree list` and say which worktrees are merged, stale, or live.
3. **Review `sysc-1630` first.**
   - It is the smallest change, and every other branch should gain it before anyone tests that branch.
   - Recommend its merge order relative to the stack: likely first to `main`, then a rebase of #22 to #26
     and M10 onto it. Say whether that rebase is mechanical.
4. **Review each PR in stack order: #22, then #24, #25, #26.**
   - Review each against its own base, using `gh pr diff <n>` or `git diff <base>...<head>` in its existing
     worktree.
   - For every milestone, state what the design's exit gate demands, what the diff implements, what is
     missing, and what is extra (scope creep into later milestones).
   - Also check the parity-matrix rows the PR should have updated, and whether its tests are pure Go where
     they could be (`AGENTS.md`: pure Go for layout and protocol logic, nested only for wlroots).
   - Verify each of `sysc-1624` to `sysc-1628` by reading the code. For each, report confirmed, refuted, or
     needs a run, with `file:line` evidence.
   - Where only a run can settle it, list the exact single named test and branch. **Ask the owner** before
     running it with `SYSC_NESTED_TESTS=1`. Prefer a branch that already carries the guard.
5. **Assess the M10 worktree.** Compare its plan against the M10 row of the design and the plan rule in
   `docs/README.md` ("plans are written when a milestone starts"). Report how far the uncommitted work goes,
   what is missing, and whether it is coherent enough to commit as a WIP branch. Do not commit it yourself.
   Glance at the archived opencode patches only to say whether any idea in them is worth a proper,
   in-scope change. Example: nil-safe `Terminate` if `sysc-1627` holds.
6. **Audit the tracker.**
   - Which bd items are stale against code and branches? Examples: `sysc-1271` is in_progress although #22
     is complete; whether `sysc-1500` still reproduces on `origin/main`; `sysc-1212` versus M5 having merged.
   - Which bd items are duplicates?
   - Which GitHub issues disagree with bd? The 2026-10-20 handover's "Beads ownership and link audit"
     section lists earlier discrepancies.
   - Report all of this. Do not change the tracker except to add notes or comments on `sysc-1624` to
     `sysc-1628` recording your verdict.
7. **Cross-cutting.** The lack of CI (`sysc-1629`, and what a CI could run without a display, given the
   guard). Disk pressure on builds. Anything in `internal/wlr` that breaks the locked-thread rule.

## Deliverables

1. **Review report:** `docs/plans/2026-10-2X-sysc-compositor-review.md` in the primary checkout,
   uncommitted, with a row added to `docs/README.md`. It must contain:
   - what is done: merged and on branches, per milestone, with evidence
   - per-PR verdicts: merge, fix first, or rework, with blocking defects listed
   - the `sysc-1624` to `sysc-1628` verdicts
   - the M10 assessment
   - the tracker discrepancies
   - the recommended merge sequence
   - every decision that belongs to the owner, posed as a question
2. **The continuation handover you commission:**
   `docs/plans/2026-10-2X-sysc-compositor-continuation-handover.md`, registered in `docs/README.md`. It
   supersedes the 2026-10-20 handover for planning and says so in its first lines. Write it for the agent
   who continues development, and include:
   - the exact first task, and what to do after it
   - the merge and rebase sequence for `sysc-1630`, #22 to #26 and M10, and which steps need the owner
   - the fixes each PR needs before merge, each with a named regression test
   - the remaining milestones M10 to M15, in dependency order, with their exit gates from the design and
     open matrix rows
   - the standing rules: this document's rule list (consent before any nested test, scope, no loops), the
     `AGENTS.md` hard rules, and bd living in sysc-shell
   - open owner decisions, including `sysc-1212`
   - an explicit stop condition for each step
3. A short summary to the owner: the per-PR verdicts, the top three risks, and the decisions you need from
   them.

**Stop** when both documents exist and you have reported to the owner. Do not begin development.
