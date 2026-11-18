# Handover: the install-layout gate, 2026-11-19

This records one session's work after v0.1.0. It does not replace the 2026-11-19 continuation
handover (open as PR #125, lands at `docs/plans/2026-11-19-handover.md`): that document still owns
the open beta work and the release state. Read it first. This one covers PR #126 and what is left
around it.

## State at handover

- `origin/main` is `f888f48` (the v0.1.0 release merge). The main checkout is at `8fb0ee8` and is
  two commits behind; it was not moved because another session's untracked
  `docs/plans/2026-11-18-handover.md` and `docs/plans/assets/2026-11-18-handover/` live there.
- PR #126, `ci/check-install-20261119`, commit `c93dca5`, worktree
  `/tmp/opencode/sysc-ci-install`. Open, ready to merge, CI green.
- PR #125 (continuation handover) and PR #122 (2026-11-18 handover) are still open docs PRs.
- The laptop still runs the pre-release build; nothing on it was restarted. See "What remains".

## Why the change

`packaging/check-install.sh` compares an installed tree against the exact layout, rejects an
unsubstituted `@BINDIR@`, rejects `/usr/local` leaking into another prefix, and checks the desktop
`Exec=` and unit `ExecStart=` lines. Until this PR its only caller was the release workflow, through
`packaging/deb/build.sh`.

The first `v0.1.0` tag run therefore stopped inside that script: the pre-#123 check scanned
`share/doc` and `share/licenses` too, so #121's README prose (which mentions `/usr/local`) was read
as a leaked path. #123 fixed the checker by limiting the text scan to installed paths. Nothing
exercised the checker before a tag, so the fix was only visible at the next tag.

The 2026-11-19 handover asked for a small PR that runs the check in CI. That is PR #126.

## What changed in PR #126

`.github/workflows/ci.yml` (lane 1, the `check` job):

- adds `make` to the `pacman -Syu` list — the step cannot run without it, and the first CI run
  proved that (`make: command not found`, run 44319705034);
- inserts, after the plain `go build ./cmd/sysc-compositor`:

```yaml
      # A tag is not the first place the install layout should be checked:
      # the first v0.1.0 tag stopped in check-install.sh, and #123 fixed the
      # check. Run it on pull requests as well.
      - name: install layout
        run: |
          make build
          for prefix in /usr /usr/local; do
            root=$(mktemp -d)
            make install PREFIX="$prefix" DESTDIR="$root" >/dev/null
            packaging/check-install.sh "$root" "$prefix"
            rm -rf "$root"
          done
```

`docs/plans/2026-11-02-packaging.md` gets a "2026-11-19: `check-install.sh` runs in CI" note under
the Task 8 record. No other file changes; no Go code is touched.

## Verification performed

Local:

- `go generate ./internal/wlr`, `make build`, then `make install` plus the checker under `/usr` and
  `/usr/local`: both print `install layout OK under …`.
- The installed tree contains exactly the ten paths the checker expects under `/usr`, no more and no
  fewer.
- Six negative cases, each on a fresh install, all exit 1 with the expected message:

| Case | Message |
|---|---|
| `/usr/local` appended to `sysc-portals.conf` | `… /usr/local under PREFIX=/usr` |
| extra file in `share` | `unexpected: /usr/share/extra-file` |
| `sysc-compositor-session` removed | `missing: /usr/bin/sysc-compositor-session` |
| desktop `Exec=` changed | `session Exec= is not …` |
| literal `@BINDIR@` in `sysc-portals.conf` | `unsubstituted @BINDIR@: …` |
| unit `ExecStart=` changed | `unit ExecStart= is not …` |

CI, in order:

| Run | Commit | Result |
|---|---|---|
| 44319705034 | `f67db79` | failed: `make: command not found` |
| 44320374881 | `c29f53c` | green: check 1m41s, compositor 3m55s; log shows both prefixes |
| 44324219401 | `c93dca5` | green: check 1m41s, compositor 3m51s; log shows both `install layout OK` lines |

`c93dca5` differs from `c29f53c` only in the two wording fixes below.

The same step runs on docs pull requests, since it is part of the `check` job; it passed on an
earlier commit of this document's own PR (run 44324535284) in 1m38s.

## Review of PR #126

The review was done by hand, against the diff and the real scripts. The `review` and `general`
subagents could not run on this machine (`Model unavailable:
synthetic/hf/deepseek-ai/DeepSeek-V4.1-Flash`), so no second opinion exists beyond this section.

Verdict: no blocking or should-fix finding. Two wording problems were found and fixed before the
final push:

1. The CI comment said "v0.1.0's first tag stopped in check-install.sh (#123)", which reads as if
   #123 caused the stop. #123 is the fix. The comment now says the tag stopped there and #123 fixed
   the check.
2. The packaging note was dated 2026-11-18 and implied the README prose was the defect. It is now
   dated 2026-11-19 and says the release workflow was the only caller, the prose tripped it only at
   the tag, and #123 narrowed the scan.

Nits left as they are:

- `rm -rf "$root"` runs only after a passing check. On failure the runner is discarded anyway, so a
  `trap` would add noise for no gain.
- Lane 1 builds twice: `go build ./cmd/sysc-compositor` and `make build`. The second is incremental
  because the Go cache is warm; the plain build is a deliberate independent check.
- The gate cannot prove the checker itself is right — it runs whatever `check-install.sh` says. It
  guards against layout and text regressions, not against a future over-broad check.
- `release.yml` still calls the same script through `packaging/deb/build.sh`, with the deb layout
  rather than the plain install; that is intentional and unchanged.

What the gate would and would not have caught: it would not have caught #121 when it was opened,
because the checker at that time was over-broad, which is exactly why #123 changed it. From now on
a layout change or a leaked path fails on the pull request that makes it; the checker itself still
needs its own review.

## What remains

In the order the 2026-11-19 handover gives; the first two are time-gated, not blocked on code.

1. **Merge #126.** After that the pull-request gate is active on `main`. Nothing needs to be done to
   the release workflow. Optional follow-up, not done here: give `release.yml` and the new step a
   shared `make check-install` target so the two arg lists cannot drift.
2. **`sysc-2293`, hardware confirmation.** The fix is merged (#118, `72c145b`), and the laptop is
   armed with `b36865f` (v0.1.0 plus the SIGUSR1 scene dump) in
   `~/.cache/sysc-compositor-gate-20261118`. The running session is still `43b0593`, pid 1013. Do
   not log the owner out: the RSS sampler reads that pid. After the owner's next login, run
   `sh ~/.cache/sysc-compositor-gate-20261118/verify-2293.sh`; expect the InfoVision make and
   `0x057D`. Check the Bar page in sysc-shell's Displays panel, then close `sysc-2293`.
3. **`sysc-2297`, RSS soak.** Unit `sysc-rss-soak-20261118` started 2026-11-18 11:04 UTC and stops
   at 23:04 UTC; read `~/.cache/sysc-rss-soak-20261118/rss.tsv` after it stops. The trend to 13:58
   was flat (VmRSS about 115 MB at the start, 91.5 MB at five hours); `drm_kb` double-counts shared
   buffers, so use it as a trend only. Flat means record the curve in the issue and close it as not
   reproduced on that build; climbing means a current-main build with `SYSC_BENCH_DIAGNOSTICS` and
   pprof an hour apart.
   Remove `~/.cache/sysc-rss-soak-20261118` when the record is written.
4. **Ghosting (`sysc-2294`)** needs the owner's long-session verdict; no code change is pending.
5. **Packaging.** COPR (`sysc-2211`), OBS (`sysc-2212`) and the GURU submission (part of
   `sysc-2213`) need credentials this machine does not have. Void and nix PRs are the owner's call;
   `sysc-1705` tracks the remaining AUR names.
6. **Tracker.** `sysc-1644` was closed as a duplicate of `sysc-1276`. Notes were added to
   `sysc-2286`, `sysc-2189` and `sysc-2198` explaining why they stay in progress. `BEADS_DB` must be
   `/home/nomadx/sysc-shell/.beads/beads.db` and the commands must run from `/home/nomadx/sysc-shell`;
   `/home/nomadx/sysc/.beads` is stale. The `bd` verb is `bd comments add`, not `bd comment`.
7. **archPC** still needs the NVIDIA userspace/kernel reboot; benchmarking P3 is waiting on the
   owner.

## Traps met in this session

- Lane 1 never installed `make`; `make` in the package list is required, not cosmetic.
- A worktree branch created from `origin/main` inherits it as upstream, so a bare
  `--force-with-lease` guards `main` and reports "stale info". Use
  `git push origin <branch> --force-with-lease=refs/heads/<branch>:<old-sha>`.
- Force-pushing after an amend is normal here, but fetch first and pass the old sha explicitly.
- Author-local dates are one day ahead of UTC in this timezone; the repository names documents by
  author-local date (the session ran on 2026-11-19 +1100 while UTC was still 2026-11-18).
- The commit hook rejects the word "both" and the substrings bot/agent/cursor. Write commit messages
  accordingly.
- Subagent reviews fail with `Model unavailable`; plan on reviewing manually and use the commands in
  "Verification performed" as the audit trail.

## Files that matter

- `.github/workflows/ci.yml` — the new step and the `make` package.
- `docs/plans/2026-11-02-packaging.md` — the Task 8 note.
- `packaging/check-install.sh` — the contract the gate enforces; read it before changing any
  installed path.
- `Makefile` — `install` does not build; `build` runs `go generate` first.
