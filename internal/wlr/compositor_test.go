package wlr

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// newNested builds a compositor against the live session's Wayland display.
// Every test here reaches wlroots, so each one owns the OS thread the way
// cmd/sysc-compositor does.
func newNested(t *testing.T) *Compositor {
	return newNestedWith(t, Options{})
}

func newNestedWith(t *testing.T, opts Options) *Compositor {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	if opts.Animations == (config.Animations{}) {
		// Existing nested tests focus on lifecycle/layout behavior; M8 animation
		// checks opt into spring values explicitly.
		opts.Animations.Off = true
	}
	c, err := NewWith(opts)
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	t.Cleanup(c.Destroy)
	return c
}

// TestNestedNeedsConsent: without SYSC_NESTED_TESTS=1 no test can build a
// compositor, so none puts a window on the owner's desktop by default.
func TestNestedNeedsConsent(t *testing.T) {
	t.Setenv("SYSC_NESTED_TESTS", "")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c, err := New()
	if c != nil {
		c.Destroy()
	}
	if !errors.Is(err, ErrNestedTestNeedsConsent) {
		t.Fatalf("New() error = %v, want ErrNestedTestNeedsConsent", err)
	}
}

// TestFailedStartReleasesTheSocket: a NewWith that fails after taking a
// Wayland socket gives it back, so one failing test cannot use up every
// wayland-N name for the tests after it. WLR_BACKENDS names no real backend,
// so no compositor is built and nothing reaches the desktop.
func TestFailedStartReleasesTheSocket(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("SYSC_NESTED_TESTS", "1")
	t.Setenv("WLR_BACKENDS", "sysc-no-such-backend")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if c, err := New(); err == nil {
		c.Destroy()
		t.Fatal("New() succeeded with no usable backend")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("failed start left %s in XDG_RUNTIME_DIR", entry.Name())
	}
}

// TestTerminateAfterDestroyIsSafe: a watchdog timer that outlives its test
// calls Terminate after Destroy has freed the display. That must do nothing,
// not hand a nil display to libwayland.
func TestTerminateAfterDestroyIsSafe(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c, err := New()
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	c.Destroy()
	c.Terminate()
	done := make(chan struct{})
	go func() {
		c.Terminate()
		close(done)
	}()
	<-done
}

// TestOutputsEnabled is the check that foot already failed for once: a client
// refuses to start when the compositor advertises no output, so a build that
// compiles and connects is still not a build a client can use.
func TestOutputsEnabled(t *testing.T) {
	c := newNested(t)
	if got := c.EnabledOutputs(); got == 0 {
		t.Fatal("EnabledOutputs() = 0: clients exit with \"no monitors available\"")
	}
}

func TestNestedSessionEndsWhenLastOutputCloses(t *testing.T) {
	c := newNested(t)
	if got := c.EnabledOutputs(); got == 0 {
		t.Fatal("nested session started without an output")
	}
	var output unsafe.Pointer
	for output = range c.sceneOutputs {
		break
	}

	runFinished := make(chan struct{})
	timedOut := make(chan struct{})
	if err := c.Post(func() { destroyOutput(output) }); err != nil {
		t.Fatal(err)
	}
	go func() {
		select {
		case <-runFinished:
		case <-time.After(2 * time.Second):
			close(timedOut)
			c.Terminate()
		}
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	close(runFinished)
	select {
	case <-timedOut:
		t.Fatal("Run remained active after the nested backend's last output closed")
	default:
	}
	if got := c.EnabledOutputs(); got != 0 {
		t.Fatalf("EnabledOutputs() after output destroy = %d, want 0", got)
	}
}

// TestListenerTokensBalance is the M0 exit gate for handle ownership: New
// registers listeners, so if any token outlived its wl_signal the process would
// leak a Go handle per run.
//
// This one owns its own lifecycle rather than deferring Destroy to t.Cleanup,
// because the assertion has to run after teardown to mean anything.
func TestListenerTokensBalance(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	before := LiveHandles()
	c, err := New()
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}

	l := AddListener(c.newSurfaceSignal(), func(unsafe.Pointer) {})
	l.Destroy()
	c.Destroy()

	if after := LiveHandles(); after != before {
		t.Fatalf("LiveHandles() = %d, want %d: a listener token outlived its wl_signal", after, before)
	}
}

func TestXDGInitialConfigureAfterInitialCommit(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	configured := make(chan string, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "xdg_surface#") && strings.Contains(line, ".configure(") {
				configured <- line
				c.Terminate()
				return
			}
		}
	}()

	cmd := exec.Command(terminal)
	cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName(), "WAYLAND_DEBUG=client")
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()

	stopTimeout := make(chan struct{})
	timeoutDone := make(chan struct{})
	go func() {
		defer close(timeoutDone)
		select {
		case <-time.After(5 * time.Second):
			c.Terminate()
		case <-stopTimeout:
		}
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	close(stopTimeout)
	<-timeoutDone
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	_ = reader.Close()
	<-readerDone

	select {
	case line := <-configured:
		t.Logf("received %s", line)
	default:
		t.Fatal("weston-terminal did not receive an initial xdg_surface.configure")
	}
}
