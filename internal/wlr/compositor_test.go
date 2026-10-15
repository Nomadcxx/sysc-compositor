package wlr

import (
	"runtime"
	"testing"
	"unsafe"
)

// newNested builds a compositor against the live session's Wayland display.
// Every test here reaches wlroots, so each one owns the OS thread the way
// cmd/sysc-compositor does.
func newNested(t *testing.T) *Compositor {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	c, err := New()
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	t.Cleanup(c.Destroy)
	return c
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
