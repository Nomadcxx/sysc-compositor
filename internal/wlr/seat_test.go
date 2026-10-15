package wlr

import (
	"errors"
	"runtime"
	"testing"
)

// keyA is evdev KEY_A; tests cannot import linux/input-event-codes.h.
const keyA = 30

// TestSeatTableOrder pins the ordering contract the shell relies on: the
// user's seat is created at New and advertised before any injected seat, so a
// client that binds the first wl_seat always gets the user's. It also pins the
// owner's answer to D8.5: one injected seat, never named seat0.
func TestSeatTableOrder(t *testing.T) {
	c := newNested(t)

	if got := c.seatNames(); len(got) != 1 || got[0] != "seat0" {
		t.Fatalf("seats after New = %v, want [seat0]", got)
	}
	if err := c.AddInjectedSeat("seat0"); !errors.Is(err, ErrSeatName) {
		t.Fatalf("AddInjectedSeat(seat0) = %v, want ErrSeatName", err)
	}
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatalf("AddInjectedSeat(agent) = %v", err)
	}
	if got := c.seatNames(); len(got) != 2 || got[0] != "seat0" || got[1] != "agent" {
		t.Fatalf("seats = %v, want [seat0 agent]", got)
	}
	user, injected := c.seatGlobal("seat0"), c.seatGlobal("agent")
	if user == 0 || injected <= user {
		t.Fatalf("wl_global names seat0=%d agent=%d: the user's seat must be advertised first", user, injected)
	}
	if err := c.AddInjectedSeat("agent2"); !errors.Is(err, ErrSeatLimit) {
		t.Fatalf("second AddInjectedSeat = %v, want ErrSeatLimit (one injected seat)", err)
	}
	if err := c.RemoveInjectedSeat("agent"); err != nil {
		t.Fatalf("RemoveInjectedSeat = %v", err)
	}
	if err := c.RemoveInjectedSeat("agent"); !errors.Is(err, ErrUnknownSeat) {
		t.Fatalf("removing twice = %v, want ErrUnknownSeat", err)
	}
	if got := c.seatNames(); len(got) != 1 || got[0] != "seat0" {
		t.Fatalf("seats after removal = %v, want [seat0]", got)
	}
}

// TestInjectedDevicesStayOnTheirSeat: the injected seat carries its own
// keyboard and pointer capabilities, and nothing about it reaches the user's
// seat or the drawn cursor.
func TestInjectedDevicesStayOnTheirSeat(t *testing.T) {
	c := newNested(t)
	userCaps := c.SeatCapabilities()
	x, y := c.cursorPosition()

	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	if got := c.injectedCapabilities("agent"); got != CKeyboard|CPointer {
		t.Fatalf("injected seat capabilities = %d, want keyboard|pointer", got)
	}
	if got := c.SeatCapabilities(); got != userCaps {
		t.Fatalf("user seat capabilities changed from %d to %d", userCaps, got)
	}
	if err := c.InjectPointerMotion("agent", 300, 200); err != nil {
		t.Fatal(err)
	}
	if err := c.InjectKey("agent", keyA, true); err != nil {
		t.Fatal(err)
	}
	if err := c.InjectKey("agent", keyA, false); err != nil {
		t.Fatal(err)
	}
	if gx, gy := c.cursorPosition(); gx != x || gy != y {
		t.Fatalf("drawn cursor moved from (%v,%v) to (%v,%v): injected motion reached it", x, y, gx, gy)
	}
	if err := c.InjectKey("nobody", keyA, true); !errors.Is(err, ErrUnknownSeat) {
		t.Fatalf("injecting into an unknown seat = %v, want ErrUnknownSeat", err)
	}
}

// TestInjectedSeatTeardown: a seat created and removed repeatedly, then a
// Destroy with one still present, leaves no Go handle behind.
func TestInjectedSeatTeardown(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	before := LiveHandles()
	c, err := New()
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	for range 3 {
		if err := c.AddInjectedSeat("agent"); err != nil {
			t.Fatal(err)
		}
		if err := c.InjectKey("agent", keyA, true); err != nil {
			t.Fatal(err)
		}
		if err := c.RemoveInjectedSeat("agent"); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	c.Destroy()
	if after := LiveHandles(); after != before {
		t.Fatalf("LiveHandles() = %d, want %d", after, before)
	}
}
