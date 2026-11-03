# Compositor performance research: sysc-compositor against Niri, Hyprland and Mango

**Kind:** plan · **Date:** 2026-11-03 · **Status:** written, not commissioned

> This is a cross-cutting workstream plan, not a milestone plan. The standing rule in `docs/README.md`
> says plans are written when a milestone starts; this work belongs to no single milestone and gates on
> nothing. Execution needs the owner's commission, and nothing in it may be executed before then.
> It does not pre-write any M1–M15 plan.

## 1. What this research decides

**Find and fix sysc-compositor's own performance gaps.** The peers are the yardstick. The deliverable
is a ranked, evidence-backed list of remediations for this compositor — not a comparative performance
claim about the four of them.

That distinction governs everything below:

- A peer delta is **not** a defect by itself. It becomes a defect only when sysc loses a metric that
  peers win **and** the loss is attributed to a mechanism inside sysc.
- The final artefact is a fix list: symptom, evidence, root cause, proposed fix, risk, expected
  impact, and the test that verifies it. It feeds the `sysc-1133` performance ledger.
- Claims about the peers themselves ("Mango is faster than Hyprland") are out of scope. Where a peer
  is faster for an interesting reason, that reason is copied into a sysc fix.

The research answers one question: **where does sysc-compositor spend time it should not, and why?**

## 2. Scope

### In scope — four dimensions

| Dimension | The question it answers |
|---|---|
| Frame pacing and input latency | Does a frame land on time, and how long does an input take to reach the screen? |
| CPU efficiency and the idle floor | What does the compositor cost when it does nothing, and what does a frame cost? |
| Memory and Go GC behaviour | Does the allocator or the collector cost latency, or grow without bound? |
| IPC and control-plane latency | How long does a command take to be acknowledged, and to be visible? |

### Out of scope

- **Startup and login time.** Not selected for this round.
- **Stability under churn.** Not selected for this round.
- **The game and GL loop.** Not selected, so **`sysc-2056` stays open**. The harness must be able to
  add a GL or Vulkan client as a new scenario without changing its design; that is a P6 add-on, not a
  rewrite.
- **Rendering quality, colour management, HDR.** Not performance.
- **Any M1–M15 milestone scope.**

### The three dimensions already tracked

- `sysc-2056` — game frame pacing against Niri. In progress, and blocked on a physical benchmark.
  This plan builds the benchmark; P6 runs the game case.
- `sysc-1470` — repeated IPC snapshot rebuilding during motion. Open. This plan measures it
  (P4) and either explains it or closes it.
- `sysc-1133` — the suite-wide performance and responsiveness fix ledger. The ranked fix list
  from P5 lands here.

## 3. The comparison is three comparisons, not one

The four compositors do not share a substrate, and pretending otherwise would make every number
uninterpretable.

| Compositor | Language | Scene substrate | Renderer | Role in this research |
|---|---|---|---|---|
| **sysc-compositor** | Go + cgo | SceneFX 0.5 | wlroots 0.20.2 | **The subject.** |
| **mangowm 0.18.0** | C | SceneFX 0.5 | wlroots 0.20.2 | **The substrate control.** |
| **Hyprland 0.57.0** | C++ | wlroots scene | aquamarine 0.15.1 | Peer. Renderer-fork confound. |
| **Niri 26.04** | Rust | its own | Smithay / GLES | **The behaviour contract and the user-visible bar.** |

Three rules follow, and they decide how each number may be used:

1. **sysc ↔ Mango is the only attribution-clean pair.** Both run the same renderer, the same scene
   library and the same wlroots. A delta between them is compositor logic and nothing else. This is
   the comparison that can produce a fix.
2. **sysc ↔ Niri is a bar, not a cause.** Niri has its own renderer in a different language. If
   Niri wins a metric, sysc has not thereby found a defect; it has found a target. Niri's source
   is read to learn *how* it achieves the result, never to claim a renderer-level equivalence.
3. **sysc ↔ Hyprland is the weakest peer.** Hyprland shares wlroots but forks the renderer through
   aquamarine, so a delta mixes compositor logic with renderer differences. Hyprland is worth
   measuring because it is what users compare against, and worth *not* over-reading.

## 4. The bench

One machine, one mode, one environment, recorded with every run. The frame-pacing audit already
demanded "the same physical output, mode, scale, GPU and game scene" for every compositor; that
requirement is met here or the run is void.

### Hardware (observed 2026-11-03)

| Item | Value |
|---|---|
| CPU | AMD Ryzen 7 5700X, 8 cores / 16 threads, up to 4.67 GHz |
| Memory | 32 GB |
| GPU | NVIDIA GeForce RTX 4060, driver 615.78.08 |
| Display | Acer X34 P, 3440x1440, connected over DP-1 |
| Mode used | 3440x1440 @ **120.000 Hz**, scale 1.0 |
| VRR | The monitor supports it and Niri reports it enabled |
| CPU governor | `powersave` under `amd-pstate-epp` — **must be pinned for every run** |

### Rules that make the run comparable

- **Native, not nested.** Every measurement run takes over tty1 with a real compositor driving the
  display. A nested run adds a compositor layer and destroys the latency and scanout numbers it
  would measure. The owner has already declined a standing local nested lane; this bench is a
  separate consented activity, not a lane. Results from any nested run are labelled *logic only,
  not latency*.
- **Identical environment for the three wlroots compositors.** The GPU is driven by the NVIDIA
  proprietary driver, so the wlroots backends need the same settings for all three: `GBM_BACKEND=nvidia`,
  `WLR_NO_HARDWARE_CURSORS=1`, and the same output name. A compositor that needed an extra variable
  to start is a finding, but it may not be given a different environment than its peers.
- **One mode, fixed.** 3440x1440 @ 120 Hz, scale 1.0, one connected output. Every other mode is
  out of scope. The logical layout position is recorded as an annotation because it differs between
  compositors for reasons that have nothing to do with performance.
- **Pinned CPU governor.** `powersave` under `amd-pstate-epp` is the machine default and would make
  every CPU-bound comparison noisy and biased towards whichever compositor happened to wake the
  CPU first. Pinned to `performance` for the run; recorded.
- **A noise floor before any comparison.** The same compositor is run twice under identical
  conditions. The spread between the two runs is the minimum detectable difference. A delta inside
  the noise floor is reported as **no difference**, never as a win.
- **Repetition.** Three runs per cell. Medians with the spread shown. A single run is not a result.

### VRR is recorded, not compared

The monitor supports VRR and Niri enables it. wlroots 0.20 exposes adaptive sync as an output state
flag (`wlr_output_state_set_adaptive_sync_enabled`) and reports `adaptive_sync_supported` and
`adaptive_sync_status`; whether the NVIDIA proprietary driver negotiates it is not established here.
The plan therefore **does not assert the asymmetry in advance**. Each compositor reports its
adaptive-sync state at startup, and that report is recorded with every run as an annotation. If only
one compositor can do it, VRR becomes a sysc capability gap — already M15 work — and is excluded
from the benchmark axes. Every comparable axis runs at a fixed 120 Hz regardless.

### Peer availability on this machine

| Peer | State | Cost to run |
|---|---|---|
| Niri 26.04 | Installed at the exact reference pin (`8ed0da4`) | none |
| Hyprland 0.57.0 | Installed (`0aa6745f`); the reference checkout is at `8b7aefe`, so the installed build is not the pinned commit and the difference is recorded | none |
| mangowm 0.18.0 | **Not installed.** Built from a scratch copy of the reference checkout | build |

The reference checkouts are read-only and are never built from. The Mango build copies
`/home/nomadx/references/mango` to a scratch directory first. Every dependency it needs is already
on the machine: meson, ninja, gcc, `wlroots-0.20` 0.20.2, `scenefx-0.5` 0.5.0, `wayland-server`
1.26.0, `xkbcommon` 1.13.2, `libinput` 1.32.0, `libpcre2-8` 10.49, `pixman-1` 0.46.4, `libcjson`
1.7.19, `pangocairo` 1.58.2, `wayland-protocols` 1.49, `wayland-scanner` 1.26.0.

`perf` is **not installed**. `kernel.perf_event_paranoid` is 2, which permits user-space sampling
once the tool is present. Installing it is a P0 task; without it the C-side attribution layer is
lost and the plan degrades to Go profiles and `/proc` counters only.

## 5. Measurement architecture

Three layers, because no single tool sees the whole frame.

| Layer | Where it lives | What it sees | Runs against |
|---|---|---|---|
| **L1 external** | the `sysc-bench` repo | user-visible outcomes: presentation feedback, frame-callback cadence, control round-trips | all four compositors |
| **L2 in-process** | sysc-compositor | *why*: Go CPU and heap profiles, execution traces, GC statistics, frame and commit counters, snapshot-rebuild counters | sysc only |
| **L3 system** | the machine | C-side CPU cost (`perf`), process RSS and wakeups, GPU busy, DRM fdinfo | all four compositors |

### Why both profilers are required

**pprof cannot see C frames.** sysc-compositor is Go with cgo, and the C half of every frame — the
wlroots commit, the SceneFX render, the allocator — is invisible to Go's profiler. A Go CPU profile
of a busy compositor can be nearly empty while the process burns a core. `perf` samples the same
process with C symbols intact and works identically on all four peers, so it closes the gap and
keeps the system layer comparable across the set.

Neither tool alone is sufficient. The pairing is the design, not a redundancy.

### Why L1 has to be compositor-independent

A harness that only works against sysc cannot calibrate sysc. L1 is a plain Wayland client that
talks no compositor-private protocol, so the same binary, the same scenario and the same metric
extraction run against Niri, Hyprland and Mango. It measures outcomes that are the user's, not
sysc's internals.

## 6. The harness — `sysc-bench`

A new sibling repository at `/home/nomadx/sysc-bench`, outside this one. The harness must run
against compositors that are not sysc, so it cannot live inside a sysc product repo; the precedent
is `sysc-metrics`, a standalone measurement library.

```
/home/nomadx/sysc-bench
  cmd/bench-client/    the probe and the scene generator
  cmd/bench-run/       orchestrator: compositor lifecycle, scenario, collection
  cmd/bench-report/    raw JSONL to summary tables and deltas
  internal/probe/      frame callbacks, wp_presentation feedback, timeline records
  internal/scenario/   the scenario schema and the runner
  internal/control/    one control adapter per compositor
  internal/metrics/    distribution maths, noise-floor comparison
  internal/tty/        tty1 takeover and restore
```

### The probe client

One Wayland client with two jobs.

**Probe.** A single surface that requests a frame callback every frame and listens for
`wp_presentation` feedback. It records, per frame: the commit timestamp, the frame-callback
timestamp, and the feedback's target time, actual time, refresh period and sequence. `wp_presentation`
comes from wlroots itself, so it is available under all four compositors without a sysc-specific
code path. The probe can be the only client on the screen, which is how the idle floor is measured,
or it can run alongside the workload, which is how induced jitter is measured.

**Scene generator.** N xdg-shell surfaces with shm buffers, moved along a deterministic seeded path
at a fixed cadence. The client controls the animation rate, so what gets measured is the
compositor's response to a known input, not the client's own scheduling. Parameters: window count
(1, 5, 10, 20), size, alpha, stacking order, and animation rate (60 Hz against a 120 Hz display, so
every other frame is a deliberate miss opportunity).

### Portable scenarios versus sysc-only ones

Blur, shadows, rounded corners and spring animations are **not portable** — each compositor
expresses them differently or not at all. The scenario set is therefore split:

- **Portable variants** run under all four: plain surfaces, alpha, size, position, stacking.
- **sysc-only variants** exercise sysc's own effects and animations. They produce sysc numbers and
  no peer number, and they are labelled that way.

### Control adapters

The control-plane scenario needs a way to ask each compositor to do the same thing. Each adapter
implements the intersection of actions all four support.

| Compositor | Control surface |
|---|---|
| sysc-compositor | the JSON-lines control socket at `$XDG_RUNTIME_DIR/<display>-control.sock`, and the Niri-compatible IPC for anything it does not cover |
| Niri | `niri msg` over `NIRI_SOCKET` |
| Hyprland | `hyprctl --batch` |
| Mango | `mmsg` |

Actions: switch to workspace by index, focus the next window, close the focused window, toggle
fullscreen. Any action a compositor cannot perform is dropped from the comparison and recorded as a
capability note, never silently skipped.

### Orchestration

`bench-run` owns the lifecycle: start the compositor on tty1, wait for the socket, run the scenario,
collect the raw records, stop the compositor, restore the previous session. The takeover and the
restore are one script with an abort path, and the restore is exercised on a scratch tty before
tty1 is touched at all.

Every run is one directory containing the scenario, the environment, the compositor version, the
compositor's reported adaptive-sync state, the raw JSONL and the extracted metrics.

## 7. Metrics

Definitions are fixed before the first comparison, so no metric can be redefined after seeing a
result. All times are monotonic.

| Metric | Definition |
|---|---|
| **Frame interval** | Gap between successive presentation feedbacks. Reported as p50, p95, p99 and max, plus the count of intervals over 1.5× the refresh period. |
| **Missed vblanks** | Per frame, how many refresh boundaries were skipped between the target and the actual presentation. |
| **Jitter** | Standard deviation of frame intervals after removing the linear trend, so clock drift is not counted as jitter. |
| **Presentation latency** | Feedback actual time minus the client's commit request. p50 and p95. |
| **Input latency proxy** | Pointer event arrival at the client to the presentation feedback of the frame that showed the effect. p50 and p95. **A proxy, not input-to-photon** — there is no photodiode on this bench, and every compositor gets the same proxy so the comparison stays fair. |
| **Idle floor** | Compositor process CPU time (utime + stime) per wall-clock second while the screen is static, as a percentage of one core, plus the voluntary context-switch count from `/proc/<pid>/status` so a timer-driven wakeup is visible even when it costs almost no CPU. |
| **Load CPU** | CPU time per presented frame, and CPU time per scenario. |
| **Memory** | RSS at the start and end of each scenario from `/proc/<pid>/status`. For sysc, also the Go heap in-use bytes, total allocation, GC cycle count and total pause. |
| **IPC latency** | Two numbers. The **round trip**: command written to the control socket until the response is read. The **effect latency**: command until the next presentation feedback on the output that changed. |
| **Snapshot churn (sysc only)** | `model.Snapshot()` calls per second and the cumulative time spent inside them. |
| **GC (sysc only)** | GC cycles per second, total pause, longest pause, and the longest GC-induced gap in the frame timeline. |

Effective FPS is reported only next to the distribution, never on its own. A compositor that
presents 60 evenly spaced frames and one that presents 90 frames with 30 of them late must not
read the same.

## 8. Scenarios

Five scenarios. Four run under all four compositors; one is sysc-only.

### S1 — Synthetic scene generator

N windows with a deterministic animation. Variants: window count (1, 5, 10, 20), alpha on and off,
sysc-only effect variants. Targets: frame pacing, jitter, CPU scaling with window count, and the
cost of compositing moving alpha surfaces.

### S2 — Real sysc desktop motion

The actual sysc-shell desktop: the bar, a column layout, a scripted drag, workspace switching.
Realism over comparability — this is sysc-only and produces no peer number. It is measured with the
probe running concurrently, so the desktop motion and the probe share the compositor exactly as they
would in real use.

### S3 — Idle floor

Nothing on the screen for 60 seconds. Variants: one static window, and no windows at all. This is
the scenario that tests the suspected floor from the 1-second `scheduleFrameFallback` timer in
`internal/wlr/output.go`, and the wakeup count will show whether that timer is waking the process
on every tick.

### S4 — Video playback

mpv at 3440x1440, no audio, a locally generated test clip, looping, so the run is reproducible
without a network fetch. The probe runs concurrently at a low rate to observe the jitter the
video path induces in other clients. Targets: composite cost of a scaled video surface and the
effect on presentation timing.

### S5 — Heavy application scroll

Chromium on a locally served heavy page, scrolled at a fixed rate driven through the DevTools
protocol so the input cadence is identical under every compositor. The probe runs concurrently.
Targets: jank and input latency under a realistic application load.

### P6 — Game or GL loop (deferred)

A Vulkan or GL client, added as a new scenario when the harness is stable. This is the case that
closes `sysc-2056`. It is out of scope for this round and must not be allowed to delay the rest.

## 9. Sysc in-process instrumentation

The L2 layer. All of it is **off by default** and enabled by environment or config, because
instrumentation that perturbs the frame path measures the instrumentation.

| Addition | Where | Notes |
|---|---|---|
| pprof endpoint | a new `internal/perf` package | `net/http/pprof` served over a **Unix socket**, not TCP, so nothing is exposed on the network. Disabled unless the socket path is set. |
| Execution trace | `internal/perf` | `runtime/trace` started at boot when enabled and flushed at shutdown, plus a control-socket command to start and stop one by hand. |
| Runtime statistics | `internal/perf` | `runtime.ReadMemStats` on a 1 Hz ticker **on its own goroutine**, never on the compositor thread. |
| Frame and commit counters | `internal/wlr/perf.go` | Commits per second, output presents, damage rects, direct-scanout hits and misses, pending frame callbacks, cumulative scene render time, cumulative commit time. |
| Snapshot counters | the model | `model.Snapshot()` calls per second and cumulative time, to give `sysc-1470` a number. The call sites are spread across `internal/wlr/windows.go`, `output.go`, `navigation.go`, `workspaces.go` and `switcher.go`; the counter attributes the cost rather than guessing which site is hot. |

### Constraints on the instrumentation

- **No allocation and no lock on the compositor thread.** Counters are plain atomic increments.
- **The sampler runs elsewhere.** A 1 Hz statistics goroutine reads runtime stats; the compositor
  thread never does.
- **The heap profile is stop-the-world.** Collect heap profiles at scenario boundaries, CPU profiles
  during, and never both at once. This is a measurement rule as much as an engineering rule.
- **Default off.** No flag, no socket, no goroutine, no measurable difference.

## 10. Phases and gates

Each phase ends in a gate. A phase that fails its gate is fixed before the next one starts.

### P0 — Prepare the bench

Build Mango from a scratch copy of the reference checkout and confirm it drives the X34 P. Install
`perf`, or record the fallback. Pin the CPU governor and record the machine's own idle baseline with
no compositor running. Write the tty1 takeover and restore, and exercise the restore on a scratch
tty. Confirm each of the four compositors starts natively on tty1 at 3440x1440 @ 120 with the agreed
environment. Record each one's reported adaptive-sync state.

**Gate:** all four start on tty1 at the right mode, the restore path is proven, and the adaptive-sync
report exists for each.

### P1 — Instrument sysc

The L2 additions from section 9, all default-off.

**Gate:** an idle sysc run with profiling enabled shows no new hot spot in the CPU profile compared
with profiling disabled; the counters read back over the control socket; the existing test suite
still passes.

### P2 — Build the harness

The probe, the scene generator, the scenario runner, the control adapters, the metric extraction and
the orchestrator.

**Gate:** three runs against Niri on tty1 produce a stable frame-interval distribution with no
impossible timestamps, and the orchestrator restores the previous session on abort.

### P3 — Noise floor and baselines

Three runs of S3 and S1 on one compositor to establish the noise floor, then three runs of S1, S3,
S4 and S5 under each of the four compositors.

**Gate:** every delta larger than the noise floor is reported with its margin; everything inside the
noise floor is labelled *no difference*. The raw JSONL and a summary table are committed.

### P4 — Attribution

For every scenario where sysc loses, take a Go CPU profile, an execution trace, a `perf` capture and
the counter set over the same window, and read them together. Check the known suspects specifically:

- Go GC pauses landing on the locked compositor thread.
- `model.Snapshot()` rebuild churn during motion (`sysc-1470`).
- The 1-second `scheduleFrameFallback` timer as an idle floor.
- The missing frame-callback sequence guard — Niri has one, sysc does not.
- The cost of the Go and C boundary on the per-frame path.

Where a peer avoids a mechanism, read that peer's source in the read-only reference checkout to learn
how: Niri's `frame_clock.rs` for frame scheduling, Mango's SceneFX usage for the shared substrate,
Hyprland's cached blur invalidation for the damage path.

**Gate:** every sysc deficit in the baseline table has either a root cause with evidence behind it, or
an explicit *not yet explained* entry. Nothing is left as a bare number.

### P5 — The fix list

Write the findings as a report at `docs/plans/<date>-compositor-performance-findings.md`, kind
*report*. Every finding carries symptom, evidence, root cause, proposed fix, risk, expected impact
and the verification test. Open a bead per finding under the `sysc-1133` ledger, and link `sysc-1470`
and `sysc-2056` where they apply. Rank by measured impact against fix risk.

**Gate:** the ledger holds an ordered list, and the owner decides what to schedule.

### P6 — The game case (deferred)

Add a GL or Vulkan client as a new scenario and close `sysc-2056`.

## 11. Risks

| Risk | Mitigation |
|---|---|
| The tty1 takeover ends the owner's live session | Explicit consent for each window, a scripted restore with an abort path, and the restore proven on a scratch tty first. Never automated in a loop. |
| NVIDIA and the wlroots backends disagree | One identical environment for the three wlroots compositors. A compositor needing an extra variable is a finding, but it does not get a different environment from its peers. |
| Mango may not drive the ultrawide | Build and smoke-test it in P0. If it cannot, fall back to a reduced mode set and record the limitation rather than dropping the peer. |
| `perf` may not install | P0 either installs it or records the fallback, and the plan states what is lost without it: the C-side attribution layer. |
| The `powersave` governor makes CPU numbers meaningless | Pinned for every run and recorded with the results. |
| pprof's stop-the-world heap profile perturbs the thing it measures | Heap profiles at scenario boundaries only, CPU profiles during, never both. |
| Background services on the machine | A private D-Bus for nested checks, a documented fixed set of services, and the machine's own idle baseline recorded as a floor. |
| Another agent working in the same checkout | Work in a dedicated worktree. The plan document itself is written on `docs/perf-research-20261103` off `main`. |
| Variance swamps the signal | The noise floor is measured before any comparison, three runs per cell, medians with spreads. |

## 12. What this research will not claim

Stated up front so the findings cannot drift past it.

- **Not input-to-photon.** There is no photodiode on this bench. The input latency figure is a proxy,
  identical for all four compositors, and is labelled a proxy everywhere it appears.
- **VRR is not a compared axis.** It is recorded per compositor and, if only Niri can do it, reported
  as a capability gap.
- **A Niri delta is not a diagnosis.** Niri has a different renderer in a different language. It is
  the bar and the source of ideas, not a control.
- **A Hyprland delta is not clean.** Aquamarine is a renderer fork. Hyprland numbers are context.
- **One machine, one mode, one display.** Nothing here generalises to other hardware, other modes,
  multi-GPU, or other drivers.
- **No game case.** `sysc-2056` is not closed by this plan.
- **Compositor-level cost is not the whole cost.** The shell, the terminal and the clients are not
  measured here; `sysc-1133` covers the suite.

## 13. Deliverables

| Artefact | Where |
|---|---|
| This plan | `docs/plans/2026-11-03-compositor-performance-research.md` |
| The harness | `/home/nomadx/sysc-bench`, a new sibling repository |
| The instrumentation | sysc-compositor, default-off, in `internal/perf` and `internal/wlr/perf.go` |
| The findings report | `docs/plans/<date>-compositor-performance-findings.md`, kind *report* |
| The fix list | beads under the `sysc-1133` ledger, in the `sysc-shell` database |
| The raw measurements | committed with the findings report, so every number in it can be recomputed |

## 14. Related work

- `plans/2026-10-31-frame-pacing-audit.md` — the `sysc-2056` source audit. Its "measurement still
  required" section is what P3 and P4 answer.
- `plans/2026-10-28-release-readiness.md` — lists the game frame pacing gate and `sysc-1470` as
  release gates. This plan supplies the measurement both need.
- `plans/2026-10-19-niri-mango-pleamar-capability-research.md` — the precedent for a research
  document in this repository, and the reason Niri and Mango hold the roles they hold here.
- `plans/2026-11-03-handover.md` — the standing rules: no `git add -A`, no merging `main` into a
  feature branch, the attribution guard on commit messages, and the bd database in `sysc-shell`.
