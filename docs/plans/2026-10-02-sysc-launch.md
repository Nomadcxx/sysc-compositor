# L0 sysc-launch compositor PATH

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** sysc-launch activates apps through `sysc-compositor` when that binary is on PATH, and still through `niri` when only niri is installed.

**Architecture:** Keep the spawn argv shape `{bin} msg action spawn -- PROGRAM…`. Injected `ServiceConfig.Run` is unchanged (shell tests hard-code `niri`). Only the production runner resolves the binary. Scan, rank, and history do not change.

**Tech Stack:** Go 1.26.4, existing sysc-launch tests. No new dependency.

**Repo:** `/home/nomadx/sysc-launch` (not this compositor tree).

---

### Task 1: Failing tests for compositor PATH

**Files:**

- Create: `/home/nomadx/sysc-launch/compositor_test.go`
- Modify: `/home/nomadx/sysc-launch/activate_test.go` (`TestActivateWithoutNiriIsErrorNotPanic` → still valid, plus a neither-bin case if the name changes)

**Step 1: Write the failing test**

```go
package launcher

import (
	"errors"
	"testing"
)

func TestCompositorBinPrefersSysc(t *testing.T) {
	t.Parallel()
	look := func(name string) (string, error) {
		switch name {
		case "sysc-compositor":
			return "/usr/bin/sysc-compositor", nil
		case "niri":
			return "/usr/bin/niri", nil
		default:
			return "", errors.New("not found")
		}
	}
	got, err := compositorBin(look)
	if err != nil || got != "sysc-compositor" {
		t.Fatalf("got %q %v, want sysc-compositor", got, err)
	}
}

func TestCompositorBinFallsBackToNiri(t *testing.T) {
	t.Parallel()
	look := func(name string) (string, error) {
		if name == "niri" {
			return "/usr/bin/niri", nil
		}
		return "", errors.New("not found")
	}
	got, err := compositorBin(look)
	if err != nil || got != "niri" {
		t.Fatalf("got %q %v, want niri", got, err)
	}
}

func TestCompositorBinNeitherIsError(t *testing.T) {
	t.Parallel()
	look := func(string) (string, error) { return "", errors.New("not found") }
	got, err := compositorBin(look)
	if err == nil || got != "" {
		t.Fatalf("got %q %v, want error", got, err)
	}
}
```

Keep `TestActivateSpawnsThroughNiri` as-is: injected `Run` still sees `[]string{"niri", "msg", "action", "spawn", "--", "firefox"}`.

Add a production-runner test that does **not** inject `Run`:

```go
func TestActivateWithoutCompositorIsErrorNotPanic(t *testing.T) {
	t.Parallel()
	svc := NewService(ServiceConfig{
		Scan: func() []Entry {
			return []Entry{{ID: "firefox.desktop", Name: "Firefox", Argv: []string{"firefox"}}}
		},
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	defer svc.Close()
	recvResults(t, svc)
	if err := svc.Activate("firefox.desktop", ""); err == nil {
		t.Fatal("missing compositor returned nil error")
	}
}
```

Rename or keep `TestActivateWithoutNiriIsErrorNotPanic` as an alias of the same behaviour so existing names still pass.

**Step 2: Run tests to verify they fail**

```
cd /home/nomadx/sysc-launch
timeout 90s env GOMAXPROCS=2 go test -count=1 . -run TestCompositorBin
```

Expected: FAIL, `compositorBin` undefined.

**Step 3: Implement the minimal resolver**

In `/home/nomadx/sysc-launch/service.go`, next to `niriRun`:

```go
func compositorBin(lookPath func(string) (string, error)) (string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	var firstErr error
	for _, name := range []string{"sysc-compositor", "niri"} {
		if _, err := lookPath(name); err == nil {
			return name, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = errors.New("not found")
	}
	return "", fmt.Errorf("launcher: no compositor in PATH (tried sysc-compositor, niri): %w", firstErr)
}
```

Change `niriRun` to resolve then rewrite `argv[0]`:

```go
func (s *Service) niriRun(ctx context.Context, argv []string) error {
	bin, err := compositorBin(s.cfg.LookPath)
	if err != nil {
		return err
	}
	if len(argv) == 0 {
		return fmt.Errorf("launcher: empty spawn argv")
	}
	argv[0] = bin
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launcher: %s: %w: %s", argv[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
```

Leave Activate's constructed argv starting with `"niri"` so injected `Run` (sysc-shell `runningapps_click_test.go`) stays valid.

Update the comment on `ServiceConfig` ("Niri spawn" → compositor spawn) and the Activate comment (`niri msg action spawn` → compositor `msg action spawn`).

**Step 4: Run tests to verify they pass**

```
timeout 90s env GOMAXPROCS=2 go test -count=1 . -run 'TestCompositorBin|TestActivate'
```

Expected: PASS.

Also run the CLI test that injects Run:

```
timeout 90s env GOMAXPROCS=2 go test -count=1 ./cmd/sysc-launch -run Test
```

Expected: PASS. That test's argv `[0]` remains `"niri"`.

**Step 5: Docs**

Modify `/home/nomadx/sysc-launch/README.md`:

- Requirements: compositor CLI on PATH (`sysc-compositor` preferred, `niri` accepted). Query still works without one.
- `launch` via `{compositor} msg action spawn -- PROGRAM [ARG...]`.

**Step 6: Commit only if the user asked this session**

```
cd /home/nomadx/sysc-launch
git add compositor_test.go activate_test.go service.go README.md
```

Do not tag. Do not bump sysc-shell until asked (orchestration S0).

---

## Out of scope

- Talking `$NIRI_SOCKET` JSON from sysc-launch (the CLI is the contract; the compositor drop-in implements it).
- Changing `XDG_CURRENT_DESKTOP` filtering (`niri:sysc` already splits on `:` and matches `OnlyShowIn=niri`).
- Rewriting rank, scan, history, or providers.
