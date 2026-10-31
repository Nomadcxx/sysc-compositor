package wlr

import (
	"fmt"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

const keyCapsLock = 58 // evdev KEY_CAPSLOCK

// attachKeyboardMonitor routes the user seat's keys through m, as the session
// does with the D-Bus service.
func attachKeyboardMonitor(c *Compositor, m *a11y.Monitor) {
	c.SetKeyboardMonitor(func(key a11y.Key, repeatDelay time.Duration) a11y.Decision {
		decision, _ := m.ProcessKey(key, repeatDelay)
		return decision
	})
}

// withFocusedClient runs check against a mapped, focused toplevel client and
// fails the test on its error.
func withFocusedClient(t *testing.T, c *Compositor, check func(*testclient.ToplevelClient) error) {
	t.Helper()
	done := make(chan error, 1)
	guard := time.AfterFunc(15*time.Second, c.Terminate)
	defer guard.Stop()
	go func() {
		defer c.Terminate()
		done <- func() error {
			client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer client.Close()
			return check(client)
		}()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("the check did not finish")
	}
}

func typeKeys(t *testing.T, c *Compositor, kb *fakeKeyboard, client *testclient.ToplevelClient, keys ...[2]uint32) error {
	t.Helper()
	onCompositor(t, c, func() {
		for _, key := range keys {
			kb.key(key[0], key[1] == 1)
		}
	})
	return client.SyncInput()
}

// TestKeyboardMonitorBlocksGrabbedKeysBeforeBinds: Orca's grabbed Ctrl+A
// reaches neither the Ctrl+A bind nor the focused client, while the
// ungrabbed Ctrl it is typed with does; without the grab the bind runs.
func TestKeyboardMonitorBlocksGrabbedKeysBeforeBinds(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{keyBind(t, "Ctrl+A", bind.ActionSpawn, "bound")}})
	kb := attachFakeKeyboard(t, c)
	var spawned int
	c.SetSpawnHandlers(func([]string) error { spawned++; return nil }, nil)
	m := a11y.NewMonitor()
	m.SetKeyGrabs(":1.5", nil, [][2]uint32{{0x61, 4}}) // a with XKB Control
	attachKeyboardMonitor(c, m)
	ctrlA := [][2]uint32{{keyLeftCtrl, 1}, {keyA, 1}, {keyA, 0}, {keyLeftCtrl, 0}}
	withFocusedClient(t, c, func(client *testclient.ToplevelClient) error {
		if err := typeKeys(t, c, kb, client, ctrlA...); err != nil {
			return err
		}
		onCompositor(t, c, func() {
			if spawned != 0 {
				t.Errorf("grabbed Ctrl+A ran its bind %d times", spawned)
			}
		})
		if events := client.InputEvents(); events.Presses != 1 || events.Releases != 1 || events.LastKey != keyLeftCtrl {
			return fmt.Errorf("client keys %+v, want only Ctrl's press and release", events)
		}
		m.SetKeyGrabs(":1.5", nil, nil)
		if err := typeKeys(t, c, kb, client, ctrlA...); err != nil {
			return err
		}
		onCompositor(t, c, func() {
			if spawned != 1 {
				t.Errorf("ungrabbed Ctrl+A ran its bind %d times, want 1", spawned)
			}
		})
		return nil
	})
}

// TestKeyboardMonitorFirstModifierPressKeepsXKBState: one tap of Orca's
// Caps Lock leaves the lock and its LED alone; a double tap toggles it.
func TestKeyboardMonitorFirstModifierPressKeepsXKBState(t *testing.T) {
	c := newNested(t)
	kb := attachFakeKeyboard(t, c)
	m := a11y.NewMonitor()
	m.SetKeyGrabs(":1.5", []uint32{0xffe5}, nil) // Caps_Lock
	attachKeyboardMonitor(c, m)
	const lockMask, capsLED = 2, 1 << 1
	tap := func() (locked, leds uint32) {
		onCompositor(t, c, func() {
			kb.key(keyCapsLock, true)
			kb.key(keyCapsLock, false)
			locked, leds = uint32(c.keyboard.modifiers.locked), uint32(c.keyboard.leds)
		})
		return locked, leds
	}
	go func() {
		defer c.Terminate()
		if locked, leds := tap(); locked&lockMask != 0 || leds&capsLED != 0 {
			t.Errorf("one Caps Lock tap: locked %#x LEDs %#x, want Caps Lock off", locked, leds)
		}
		if locked, leds := tap(); locked&lockMask == 0 || leds&capsLED == 0 {
			t.Errorf("double Caps Lock tap: locked %#x LEDs %#x, want Caps Lock on", locked, leds)
		}
	}()
	guard := time.AfterFunc(10*time.Second, c.Terminate)
	defer guard.Stop()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}

// TestKeyboardMonitorKeepsGrabbedKeysFromClients: under a full keyboard
// grab no key reaches a client, and a grabbed Shift still held when another
// window takes focus is not in that client's wl_keyboard.enter keys. (Niri
// forwards grabbed modifier releases to undo exactly that enter.)
func TestKeyboardMonitorKeepsGrabbedKeysFromClients(t *testing.T) {
	c := newNested(t)
	kb := attachFakeKeyboard(t, c)
	m := a11y.NewMonitor()
	m.Grab(":1.5")
	attachKeyboardMonitor(c, m)
	withFocusedClient(t, c, func(first *testclient.ToplevelClient) error {
		if err := typeKeys(t, c, kb, first, [2]uint32{keyA, 1}, [2]uint32{keyA, 0}, [2]uint32{keyLeftShift, 1}); err != nil {
			return err
		}
		second, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			return err
		}
		defer second.Close()
		var focused bool
		for deadline := time.Now().Add(2 * time.Second); !focused && time.Now().Before(deadline); {
			if err := second.SyncInput(); err != nil {
				return err
			}
			focused = second.InputEvents().KeyboardEnters > 0
		}
		if err := typeKeys(t, c, kb, second, [2]uint32{keyLeftShift, 0}); err != nil {
			return err
		}
		if events := second.InputEvents(); !focused || len(events.EnterKeys) != 0 || events.Presses != 0 || events.Releases != 0 {
			return fmt.Errorf("second client keys %+v (focused %v), want focus with no held or sent keys", events, focused)
		}
		if err := first.SyncInput(); err != nil {
			return err
		}
		if events := first.InputEvents(); events.Presses != 0 || events.Releases != 0 {
			return fmt.Errorf("first client keys %+v, want none", events)
		}
		return nil
	})
}
