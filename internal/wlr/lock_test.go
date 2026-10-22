package wlr

import (
	"errors"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// onCompositor runs fn on the compositor thread and waits for it.
func onCompositor(t *testing.T, c *Compositor, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if err := c.Post(func() { fn(); close(done) }); err != nil {
		t.Errorf("post to the compositor thread: %v", err)
		return
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("the compositor thread did not run the check")
	}
}

// lockTestWindow maps weston-terminal, then runs drive on its own goroutine
// with the mapped window. The compositor stops when drive returns.
func lockTestWindow(t *testing.T, c *Compositor, drive func(window *toplevelView)) {
	t.Helper()
	var window *toplevelView
	driven := make(chan struct{})
	c.viewMapped = func(view *toplevelView) {
		if window != nil {
			return
		}
		window = view
		c.focusView(view)
		go func() {
			defer close(driven)
			defer c.Terminate()
			drive(view)
		}()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if window == nil {
		t.Fatal("weston-terminal never mapped")
	}
	<-driven
}

// lockSurfaceSet is every lock surface the live locker has, by wl_surface,
// with whether it is mapped.
func lockSurfaceSet(c *Compositor) map[unsafe.Pointer]bool {
	set := map[unsafe.Pointer]bool{}
	if c.lock == nil {
		return set
	}
	for _, s := range c.lock.surfaces {
		set[unsafe.Pointer(s.surface.surface)] = bool(s.surface.surface.mapped)
	}
	return set
}

// outputProbePoints are the centre and the four inset corners of every
// output's logical box.
func outputProbePoints(c *Compositor) [][2]float64 {
	var points [][2]float64
	for _, out := range c.OutputSnapshot() {
		if out.Logical == nil {
			continue
		}
		x, y := float64(out.Logical.X), float64(out.Logical.Y)
		w, h := float64(out.Logical.Width), float64(out.Logical.Height)
		points = append(points, [2]float64{x + w/2, y + h/2},
			[2]float64{x + 1, y + 1}, [2]float64{x + w - 2, y + 1},
			[2]float64{x + 1, y + h - 2}, [2]float64{x + w - 2, y + h - 2})
	}
	return points
}

// checkSealed is what a sealed session guarantees whether or not a locker is
// alive: a backdrop on every output, nothing but lock surfaces under the
// pointer or holding the keyboard, focus requests ignored, injection refused.
func checkSealed(t *testing.T, c *Compositor, window *toplevelView, label string) {
	t.Helper()
	if !c.sessionLocked {
		t.Errorf("%s: session is not sealed", label)
	}
	for output := range c.sceneOutputs {
		if c.lockBackdrops[output] == nil {
			t.Errorf("%s: an output has no black backdrop", label)
		}
	}
	lockSurfaces := lockSurfaceSet(c)
	for _, p := range outputProbePoints(c) {
		surface, _, _ := c.surfaceAt(p[0], p[1])
		if surface != nil && !lockSurfaces[unsafe.Pointer(surface)] {
			t.Errorf("%s: the pointer at (%v,%v) reaches a surface that is not a mapped lock surface", label, p[0], p[1])
		}
	}
	x, y := c.viewCenter(window)
	if surface, _, _ := c.surfaceAt(x, y); c.viewForSurface(surface) == window {
		t.Errorf("%s: the pointer reaches the window under the lock", label)
	}
	c.focusView(window)
	if focused := unsafe.Pointer(c.seat.keyboard_state.focused_surface); focused != nil && !lockSurfaces[focused] {
		t.Errorf("%s: keyboard focus is on a surface that is not a mapped lock surface", label)
	}
	if err := c.InjectKey("agent", keyA, true); !errors.Is(err, ErrInjectedInputBlocked) {
		t.Errorf("%s: injected key press error = %v, want ErrInjectedInputBlocked", label, err)
	}
}

// checkUnsealed is the session back to normal after a valid unlock.
func checkUnsealed(t *testing.T, c *Compositor, window *toplevelView) {
	t.Helper()
	if c.sessionLocked {
		t.Error("session still sealed after unlock_and_destroy")
	}
	if len(c.lockBackdrops) != 0 {
		t.Errorf("%d black backdrops left after unlock", len(c.lockBackdrops))
	}
	c.focusView(window)
	if c.keyboardFocusedView() != window {
		t.Error("the window does not get keyboard focus back after unlock")
	}
	x, y := c.viewCenter(window)
	if surface, _, _ := c.surfaceAt(x, y); c.viewForSurface(surface) != window {
		t.Error("the pointer does not reach the window after unlock")
	}
	if err := c.InjectKey("agent", keyA, true); err != nil {
		t.Errorf("injected key press after unlock: %v", err)
	} else if err := c.InjectKey("agent", keyA, false); err != nil {
		t.Errorf("injected key release after unlock: %v", err)
	}
}

// TestSessionLockCoversEveryOutputAndRoutesInputOnlyToLockSurfaces: once
// `locked` arrives, every output is covered, the pointer and keyboard reach
// only lock surfaces, a second locker is refused, and injected input is
// blocked. unlock_and_destroy gives the window its input back (sysc-1260).
func TestSessionLockCoversEveryOutputAndRoutesInputOnlyToLockSurfaces(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	lockTestWindow(t, c, func(window *toplevelView) {
		lock, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Errorf("lock: %v", err)
			return
		}
		defer lock.Close()
		if second, err := testclient.StartLockClient(c.SocketName(), false); !errors.Is(err, testclient.ErrLockFinished) {
			t.Errorf("a second locker while the first holds the lock: error %v, want finished", err)
			second.Close()
		}
		onCompositor(t, c, func() {
			surfaces := lockSurfaceSet(c)
			if len(surfaces) != len(c.sceneOutputs) {
				t.Errorf("lock surfaces = %d, want one per output (%d)", len(surfaces), len(c.sceneOutputs))
			}
			for _, mapped := range surfaces {
				if !mapped {
					t.Error("a lock surface is not mapped after its first buffer")
				}
			}
			if !surfaces[unsafe.Pointer(c.seat.keyboard_state.focused_surface)] {
				t.Error("keyboard focus is not on a lock surface")
			}
			checkSealed(t, c, window, "locked")
		})
		if err := lock.Unlock(); err != nil {
			t.Errorf("unlock: %v", err)
			return
		}
		onCompositor(t, c, func() { checkUnsealed(t, c, window) })
	})
}

// TestLockerCrashKeepsSessionSealed: a locker that disconnects without
// unlock_and_destroy leaves the session sealed behind black backdrops, with
// no surface holding input. A new locker can take over at once and unlock.
func TestLockerCrashKeepsSessionSealed(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	lockTestWindow(t, c, func(window *toplevelView) {
		lock, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Errorf("lock: %v", err)
			return
		}
		onCompositor(t, c, func() { checkSealed(t, c, window, "locked") })
		lock.Close()
		gone := false
		for deadline := time.Now().Add(2 * time.Second); !gone && time.Now().Before(deadline); {
			onCompositor(t, c, func() { gone = c.lock == nil })
		}
		if !gone {
			t.Error("the compositor still holds the crashed locker's lock")
			return
		}
		onCompositor(t, c, func() {
			checkSealed(t, c, window, "after the locker crashed")
			if c.seat.keyboard_state.focused_surface != nil {
				t.Error("a surface holds keyboard focus with no locker")
			}
		})

		next, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Errorf("a new locker could not take over the sealed session: %v", err)
			return
		}
		defer next.Close()
		onCompositor(t, c, func() { checkSealed(t, c, window, "relocked") })
		if err := next.Unlock(); err != nil {
			t.Errorf("unlock: %v", err)
			return
		}
		onCompositor(t, c, func() { checkUnsealed(t, c, window) })
	})
}
