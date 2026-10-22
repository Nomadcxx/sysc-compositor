package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/ui"
)

const keyEnter = 28 // evdev KEY_ENTER

// checkExitDialogUp: the dialog covers every output with a panel of its drawn
// size, holds the keyboard, and the pointer reaches no window.
func checkExitDialogUp(t *testing.T, c *Compositor, window *toplevelView, label string) {
	t.Helper()
	if !c.ExitDialogOpen() || c.exitDialog.tree == nil {
		t.Fatalf("%s: the exit dialog is not open", label)
	}
	enabled := 0
	for pointer := range c.sceneOutputs {
		if c.outputIsEnabled(pointer) {
			enabled++
		}
	}
	if len(c.exitDialog.outputs) != enabled {
		t.Errorf("%s: dialog on %d outputs, want %d", label, len(c.exitDialog.outputs), enabled)
	}
	for _, out := range c.exitDialog.outputs {
		img, err := ui.ExitDialog(1)
		if err != nil {
			t.Fatal(err)
		}
		if out.panel == nil || out.backdrop == nil {
			t.Errorf("%s: an output has no panel or backdrop", label)
			continue
		}
		if w, h := int(out.panel.buffer.width), int(out.panel.buffer.height); w < img.Rect.Dx() || h < img.Rect.Dy() {
			t.Errorf("%s: panel buffer %dx%d is smaller than the scale-1 panel %v", label, w, h, img.Rect)
		}
	}
	if c.seat.keyboard_state.focused_surface != nil {
		t.Errorf("%s: a client holds keyboard focus under the dialog", label)
	}
	c.focusView(window)
	if c.seat.keyboard_state.focused_surface != nil {
		t.Errorf("%s: focusing the window gave it the keyboard under the dialog", label)
	}
	for _, p := range outputProbePoints(c) {
		if surface, _, _ := c.surfaceAt(p[0], p[1]); surface != nil {
			t.Errorf("%s: the pointer at (%v,%v) reaches a client", label, p[0], p[1])
		}
	}
}

// TestExitDialogAsksBeforeQuitting: Quit opens the dialog instead of quitting,
// and a key or a button press closes it and gives the window its focus back.
func TestExitDialogAsksBeforeQuitting(t *testing.T) {
	c := newNested(t)
	lockTestWindow(t, c, func(window *toplevelView) {
		var keyboard *fakeKeyboard
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			keyboard.remove()
			pointer.remove()
		})
		onCompositor(t, c, func() {
			keyboard = c.addFakeKeyboard()
			pointer = c.addFakePointer()
			c.focusView(window)
			c.Quit(false)
			checkExitDialogUp(t, c, window, "after Quit")

			keyboard.key(keyEsc, true)
			keyboard.key(keyEsc, false)
			if c.ExitDialogOpen() || c.exitDialog.tree != nil {
				t.Fatal("Escape did not close the dialog")
			}
			c.focusView(window)
			if c.keyboardFocusedView() != window {
				t.Error("the window does not get the keyboard back")
			}

			c.executeBindAction(bind.Action{Name: bind.ActionQuit})
			checkExitDialogUp(t, c, window, "after the quit bind")
			x, y := c.viewCenter(window)
			pointer.moveTo(x, y)
			pointer.button(btnLeft, true)
			if c.ExitDialogOpen() {
				t.Fatal("a button press did not close the dialog")
			}
			pointer.button(btnLeft, false)
			if focus := c.userPointerFocus(); focus == nil || c.viewForSurface(c.seat.pointer_state.focused_surface) != window {
				t.Error("the pointer does not reach the window after the dialog closed")
			}
		})
	})
}

// quitsWithin runs the compositor until it stops or 5 s pass, with steps on
// the compositor thread; it reports whether something other than the
// watchdog stopped it.
func quitsWithin(t *testing.T, c *Compositor, steps func()) bool {
	t.Helper()
	timedOut := make(chan struct{})
	watchdog := time.AfterFunc(5*time.Second, func() { close(timedOut); c.Terminate() })
	t.Cleanup(func() { watchdog.Stop() })
	if err := c.Post(steps); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-timedOut:
		return false
	default:
		return true
	}
}

func TestExitDialogEnterQuits(t *testing.T) {
	c := newNested(t)
	quit := quitsWithin(t, c, func() {
		keyboard := c.addFakeKeyboard()
		defer keyboard.remove()
		c.Quit(false)
		if !c.ExitDialogOpen() {
			t.Error("Quit did not ask first")
		}
		keyboard.key(keyEnter, true)
	})
	if !quit {
		t.Error("Enter in the dialog did not quit")
	}
}

func TestQuitSkipConfirmationQuitsAtOnce(t *testing.T) {
	c := newNested(t)
	quit := quitsWithin(t, c, func() {
		c.Quit(true)
		if c.ExitDialogOpen() {
			t.Error("skip-confirmation opened the dialog")
		}
	})
	if !quit {
		t.Error("Quit with skip-confirmation did not quit")
	}
}

// TestQuitBindRunsWhileLocked is Niri's allowed_when_locked for quit alone.
func TestQuitBindRunsWhileLocked(t *testing.T) {
	quit := bind.Binding{Actions: []bind.Action{{Name: bind.ActionQuit}}}
	mixed := bind.Binding{Actions: []bind.Action{{Name: bind.ActionQuit}, {Name: bind.ActionCloseWindow}}}
	if !allowedWhenLocked(quit) || allowedWhenLocked(mixed) || allowedWhenLocked(bind.Binding{}) {
		t.Error("allowedWhenLocked should take a bind of quit alone")
	}
}
