package wlr

import (
	"errors"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

const idleTestTimeout = 300 * time.Millisecond

// expectIdleEvent waits for the notification's next event and checks it; ""
// means no event within the wait.
func expectIdleEvent(t *testing.T, client *testclient.ToplevelClient, wait time.Duration, want, label string) bool {
	t.Helper()
	got, err := client.Next(wait)
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return false
	}
	if got != want {
		t.Errorf("%s: idle notification event %q, want %q", label, got, want)
		return false
	}
	return true
}

// TestIdleNotifiesOnlyUserActivityAndHonoursInhibitors: only the user seat's
// real input resumes from idle; injected input does not. A visible surface's
// idle inhibitor and a ScreenSaver D-Bus inhibition each hold idle off.
// User input wakes monitors PowerOffMonitors turned off; injected input does
// not.
func TestIdleNotifiesOnlyUserActivityAndHonoursInhibitors(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	watchdog := time.AfterFunc(20*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		client, err := testclient.StartToplevelClient(c.SocketName(), idleTestTimeout)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		var keyboard *fakeKeyboard
		defer onCompositor(t, c, func() {
			if keyboard != nil {
				keyboard.remove()
			}
		})
		userKey := func() {
			onCompositor(t, c, func() {
				if keyboard == nil {
					keyboard = c.addFakeKeyboard()
				}
				keyboard.key(keyA, true)
				keyboard.key(keyA, false)
			})
		}
		quiet := 2 * idleTestTimeout

		if !expectIdleEvent(t, client, 2*time.Second, "idled", "no input") {
			return
		}
		onCompositor(t, c, func() {
			if err := c.InjectKey("agent", keyA, true); err != nil {
				t.Errorf("injected key press: %v", err)
			}
			if err := c.InjectKey("agent", keyA, false); err != nil {
				t.Errorf("injected key release: %v", err)
			}
		})
		expectIdleEvent(t, client, quiet, "", "injected input")
		userKey()
		expectIdleEvent(t, client, time.Second, "resumed", "user input")
		expectIdleEvent(t, client, 2*time.Second, "idled", "after user input")

		// A visible window's inhibitor resumes and holds idle off.
		if err := client.Inhibit(true); err != nil {
			t.Error(err)
			return
		}
		expectIdleEvent(t, client, time.Second, "resumed", "inhibitor added")
		expectIdleEvent(t, client, quiet, "", "while inhibited")
		if err := client.Inhibit(false); err != nil {
			t.Error(err)
			return
		}
		expectIdleEvent(t, client, 2*time.Second, "idled", "inhibitor removed")

		// So does a ScreenSaver D-Bus inhibition.
		onCompositor(t, c, func() { c.SetIdleInhibitedByDBus(true) })
		expectIdleEvent(t, client, time.Second, "resumed", "D-Bus inhibition")
		expectIdleEvent(t, client, quiet, "", "while D-Bus inhibits")
		onCompositor(t, c, func() { c.SetIdleInhibitedByDBus(false) })
		expectIdleEvent(t, client, 2*time.Second, "idled", "D-Bus inhibition released")

		// Blanked monitors stay off for injected input and for releases, and
		// wake for the events Niri's should_activate_monitors lists.
		powerOff := func(label string) {
			onCompositor(t, c, func() {
				if err := c.PowerOffMonitors(); err != nil {
					t.Errorf("%s: PowerOffMonitors: %v", label, err)
				}
				if !c.allOutputsOff() {
					t.Errorf("%s: an output is on after PowerOffMonitors", label)
				}
			})
		}
		expectAwake := func(label string, want bool) {
			onCompositor(t, c, func() {
				if awake := !c.allOutputsOff() && !c.powerOffActive; awake != want {
					t.Errorf("%s: monitors awake = %v, want %v", label, awake, want)
				}
			})
		}
		var pointer *fakePointer
		var touch *fakeTouch
		defer onCompositor(t, c, func() {
			if pointer != nil {
				pointer.remove()
			}
			if touch != nil {
				touch.remove()
			}
		})

		// The key that powered them off is released while they are off.
		onCompositor(t, c, func() {
			if keyboard == nil {
				keyboard = c.addFakeKeyboard()
			}
			keyboard.key(keyA, true)
		})
		powerOff("key held")
		onCompositor(t, c, func() {
			if err := c.InjectKey("agent", keyA, true); !errors.Is(err, ErrInjectedInputBlocked) {
				t.Errorf("injected key while blanked: error %v, want ErrInjectedInputBlocked", err)
			}
			keyboard.key(keyA, false)
		})
		expectAwake("key release", false)
		userKey()
		expectAwake("key press", true)

		powerOff("pointer")
		onCompositor(t, c, func() {
			pointer = c.addFakePointer()
			pointer.moveTo(10, 10)
		})
		expectAwake("pointer motion", true)

		powerOff("swipe")
		onCompositor(t, c, func() { pointer.swipeBegin(3) })
		expectAwake("swipe begin", true)
		onCompositor(t, c, func() { pointer.swipeEnd(true) })

		powerOff("touch")
		onCompositor(t, c, func() {
			touch = c.addFakeTouch()
			touch.down(0, 0.5, 0.5)
			touch.frame()
		})
		expectAwake("touch down", true)
		onCompositor(t, c, func() {
			touch.up(0)
			touch.frame()
		})
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
