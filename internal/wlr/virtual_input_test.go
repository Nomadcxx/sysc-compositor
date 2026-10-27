package wlr

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

const keyLeftShift = 42 // evdev KEY_LEFTSHIFT

func TestIMEGrabDoesNotRetainDestroyedVirtualKeyboard(t *testing.T) {
	c := newNested(t)
	physical := attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		ime, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer ime.Close()
		if err = ime.StartTextInput(); err != nil {
			t.Error(err)
			return
		}
		if err = ime.GrabInputMethodKeyboard(); err != nil {
			t.Error(err)
			return
		}
		source, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer source.Close()
		if err = source.CreateVirtualKeyboard(false); err != nil {
			t.Error(err)
			return
		}
		if err = source.VirtualKey(keyA, true); err != nil {
			t.Error(err)
			return
		}
		var retired unsafe.Pointer
		onCompositor(t, c, func() {
			retired = unsafe.Pointer(c.inputMethod.keyboard_grab.keyboard)
			physical.key(keyA+1, true)
			physical.key(keyA+1, false)
		})
		if err = source.DestroyVirtualKeyboard(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			if unsafe.Pointer(c.inputMethod.keyboard_grab.keyboard) == retired {
				t.Error("IME grab retained a destroyed virtual keyboard")
			}
		})
		if err = ime.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if events := ime.InputEvents(); events.GrabKeys != 4 {
			t.Errorf("IME lost a held key release during keyboard destruction: %+v", events)
		}
	})
}

func TestKeyboardLateBindingIncludesPhysicalAndVirtualHeldKeys(t *testing.T) {
	c := newNested(t)
	physical := attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err = client.CreateVirtualKeyboard(false); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { physical.key(keyA, true) })
		if err = client.VirtualKey(keyA+1, true); err != nil {
			t.Error(err)
			return
		}
		if err = client.BindAdditionalKeyboard(); err != nil {
			t.Error(err)
			return
		}
		keys := client.InputEvents().EnterKeys
		if len(keys) != 2 || !(keys[0] == keyA && keys[1] == keyA+1 || keys[0] == keyA+1 && keys[1] == keyA) {
			t.Errorf("late keyboard enter keys=%v, want physical A and virtual S", keys)
		}
	})
}

func TestPhysicalKeyboardRemovalPreservesVirtualHeldKey(t *testing.T) {
	c := newNested(t)
	physical := attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err = client.CreateVirtualKeyboard(false); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { physical.key(keyA, true) })
		if err = client.VirtualKey(keyA, true); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { physical.remove() })
		if err = client.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if got := client.InputEvents(); got.Presses != 1 || got.Releases != 0 {
			t.Errorf("physical removal changed virtual held key: %+v", got)
		}
		if err = client.VirtualKey(keyA, false); err != nil {
			t.Error(err)
			return
		}
		if got := client.InputEvents(); got.Releases != 1 {
			t.Errorf("final virtual release after physical removal: %+v", got)
		}
	})
}

func TestJoinedKeyboardHeldStateBeginsAtNextFocus(t *testing.T) {
	c := newNested(t)
	attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		var joined *fakeKeyboard
		onCompositor(t, c, func() {
			joined, err = newDetachedFakeKeyboard(Keyboard{})
			if err != nil {
				return
			}
			joined.key(keyA, true)
			c.handleNewInput(unsafe.Pointer(&joined.kb.base))
		})
		if err != nil {
			t.Error(err)
			return
		}
		defer onCompositor(t, c, joined.remove)
		if err = client.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if events := client.InputEvents(); events.Presses != 0 {
			t.Errorf("joining keyboard fabricated a press: %+v", events)
		}
		other, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer other.Close()
		if keys := other.InputEvents().EnterKeys; len(keys) != 1 || keys[0] != keyA {
			t.Errorf("new focus omitted joining keyboard state: %v", keys)
		}
		onCompositor(t, c, joined.remove)
		if err = other.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if events := other.InputEvents(); events.Releases != 1 {
			t.Errorf("joining keyboard removal lost announced release: %+v", events)
		}
	})
}

func TestKeyboardUnionHonorsNativeHeldKeyCapacity(t *testing.T) {
	c := newNested(t)
	physical := attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err = client.CreateVirtualKeyboard(false); err != nil {
			t.Error(err)
			return
		}
		var capacity int
		onCompositor(t, c, func() { capacity = len(c.keyboard.keycodes); physical.key(keyA, true) })
		for i := 0; i < capacity; i++ {
			if err = client.VirtualKey(uint32(100+i), true); err != nil {
				t.Error(err)
				return
			}
		}
		if events := client.InputEvents(); events.Presses != capacity {
			t.Errorf("exported %d presses beyond native held capacity %d", events.Presses, capacity)
		}
		if err = client.DestroyVirtualKeyboard(); err != nil {
			t.Error(err)
			return
		}
		if events := client.InputEvents(); events.Releases != capacity-1 {
			t.Errorf("virtual destruction released %d keys, want %d", events.Releases, capacity-1)
		}
		onCompositor(t, c, func() { physical.key(keyA, false) })
		if err = client.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if events := client.InputEvents(); events.Releases != capacity {
			t.Errorf("final key release count=%d, want %d", events.Releases, capacity)
		}
	})
}

func TestLockKeyboardStateExcludesIMEGrabOwnedKey(t *testing.T) {
	c := newNested(t)
	physical := attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err = client.StartTextInput(); err != nil {
			t.Error(err)
			return
		}
		if err = client.EnableTextInput("hello", 0, 0, 1, 16); err != nil {
			t.Error(err)
			return
		}
		if err = client.GrabInputMethodKeyboard(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { physical.key(keyA, true) })
		locker, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Error(err)
			return
		}
		defer locker.Close()
		onCompositor(t, c, func() {
			if c.seat.keyboard_state.focused_surface == nil {
				t.Error("lock surface did not receive keyboard focus")
			}
			if keyboard := c.seat.keyboard_state.keyboard; keyboard == nil || keyboard.num_keycodes != 0 {
				t.Error("lock's exported keyboard state contains an IME-owned held key")
			}
			physical.key(keyA, false)
		})
		if err = client.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if got := client.InputEvents(); got.GrabKeys != 2 || got.Presses != 0 || got.Releases != 0 {
			t.Errorf("lock changed held key destination: %+v", got)
		}
		if err = locker.Unlock(); err != nil {
			t.Error(err)
			return
		}
	})
}

func TestScreenshotUIRetiresPreviouslyForwardedKey(t *testing.T) {
	c := newNested(t)
	physical := attachFakeKeyboard(t, c)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		onCompositor(t, c, func() {
			physical.key(keyA, true)
			if e := c.Screenshot(false, ""); e != nil {
				t.Error(e)
				return
			}
			physical.key(keyA, false)
			c.closeScreenshotUI()
			if len(c.keyboardRoutes) != 0 {
				t.Error("screenshot UI retained a released key route")
			}
		})
		if err = client.SyncInput(); err != nil {
			t.Error(err)
			return
		}
		if got := client.InputEvents(); got.Presses != 1 || got.Releases != 1 {
			t.Errorf("screenshot UI swallowed previously forwarded release: %+v", got)
		}
	})
}

func TestInputMethodGrabReturnsKeysThroughVirtualKeyboard(t *testing.T) {
	c := newNested(t)
	keyboard := attachFakeKeyboard(t, c)
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
			if err := client.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			if err := client.StartTextInput(); err != nil {
				return err
			}
			if err := client.EnableTextInput("hello", 0, 0, 1, 16); err != nil {
				return err
			}
			if err := client.GrabInputMethodKeyboard(); err != nil {
				return err
			}
			initial := client.InputEvents()
			if initial.GrabKeymaps == 0 || initial.RepeatRate != 25 || initial.RepeatDelay != 600 {
				return fmt.Errorf("grab keymap/repeat state = %+v", initial)
			}
			onCompositor(t, c, func() { keyboard.key(keyA, true); keyboard.key(keyA, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			grabbed := client.InputEvents()
			if grabbed.GrabKeys != 2 || grabbed.GrabLastKey != keyA || grabbed.GrabLastState != 0 || grabbed.Presses != 0 || grabbed.Releases != 0 {
				return fmt.Errorf("physical keys did not go only to IME grab: %+v", grabbed)
			}
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := client.VirtualKey(keyA, false); err != nil {
				return err
			}
			returned := client.InputEvents()
			if returned.GrabKeys != grabbed.GrabKeys || returned.Presses != 1 || returned.Releases != 1 || returned.LastKey != keyA {
				return fmt.Errorf("IME passthrough looped or lost its key: %+v", returned)
			}
			var focused uint64
			onCompositor(t, c, func() { focused = c.Windows()[0].ID })
			other, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer other.Close()
			if err := other.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			onCompositor(t, c, func() { c.FocusWindow(focused) })
			if err := other.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := other.VirtualKey(keyA, false); err != nil {
				return err
			}
			if err := client.SyncInput(); err != nil {
				return err
			}
			unrelated := client.InputEvents()
			if unrelated.GrabKeys != returned.GrabKeys+2 || unrelated.Presses != returned.Presses || unrelated.Releases != returned.Releases {
				return fmt.Errorf("unrelated virtual keyboard bypassed IME grab: before=%+v after=%+v", returned, unrelated)
			}
			onCompositor(t, c, func() { keyboard.key(keyLeftShift, true); keyboard.key(keyLeftShift, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			mods := client.InputEvents()
			if mods.GrabModifiers <= returned.GrabModifiers {
				return fmt.Errorf("physical modifiers bypassed IME grab: %+v", mods)
			}
			if err := client.VirtualModifiers(1); err != nil {
				return err
			}
			if err := client.VirtualModifiers(0); err != nil {
				return err
			}
			afterMods := client.InputEvents()
			if afterMods.GrabModifiers != mods.GrabModifiers || afterMods.Modifiers <= mods.Modifiers {
				return fmt.Errorf("IME modifier passthrough looped or did not reach client: before=%+v after=%+v", mods, afterMods)
			}
			onCompositor(t, c, func() {
				if err := c.reconfigureKeyboard(Keyboard{Layout: "us", RepeatRate: 37, RepeatDelay: 210}); err != nil {
					t.Error(err)
				}
				for source := range c.virtualKeyboards {
					if source.repeat_info.rate != 37 || source.repeat_info.delay != 210 {
						t.Error("virtual keyboard retained stale repeat settings")
					}
				}
			})
			if err := client.SyncInput(); err != nil {
				return err
			}
			changed := client.InputEvents()
			if changed.GrabKeymaps <= initial.GrabKeymaps || changed.RepeatRate != 37 || changed.RepeatDelay != 210 {
				return fmt.Errorf("grab did not follow keyboard reconfiguration: %+v", changed)
			}
			if err := client.ReleaseInputMethodKeyboard(); err != nil {
				return err
			}
			before := client.InputEvents()
			onCompositor(t, c, func() { keyboard.key(keyA, true); keyboard.key(keyA, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			after := client.InputEvents()
			if after.GrabKeys != before.GrabKeys || after.Presses != before.Presses+1 || after.Releases != before.Releases+1 {
				return fmt.Errorf("grab destruction did not restore client keys: before=%+v after=%+v", before, after)
			}
			return nil
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
		t.Fatal("IME grab check timed out")
	}
}

func TestVirtualKeyboardUsesBindAndInhibitionPolicy(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{keyBind(t, "Ctrl+A", bind.ActionSpawn, "bound")}})
	attachFakeKeyboard(t, c)
	var spawned int
	c.SetSpawnHandlers(func([]string) error { spawned++; return nil }, nil)
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
			if err := client.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			if err := client.VirtualModifiers(4); err != nil { // XKB Control
				return err
			}
			press := func() error {
				if err := client.VirtualKey(keyA, true); err != nil {
					return err
				}
				return client.VirtualKey(keyA, false)
			}
			if err := press(); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if spawned != 1 {
					t.Errorf("virtual bind executions = %d, want 1", spawned)
				}
			})
			if events := client.InputEvents(); events.Presses != 0 || events.Releases != 0 {
				return fmt.Errorf("consumed virtual bind reached client: %+v", events)
			}
			if err := client.InhibitShortcuts(); err != nil {
				return err
			}
			if err := press(); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if spawned != 1 {
					t.Errorf("inhibited virtual bind executed: %d", spawned)
				}
			})
			if events := client.InputEvents(); events.Presses != 1 || events.Releases != 1 {
				return fmt.Errorf("inhibited virtual bind did not reach client: %+v", events)
			}
			return nil
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
		t.Fatal("virtual bind check timed out")
	}
}

func TestVirtualInputPreservesSeatAndLockBoundaries(t *testing.T) {
	floating := true
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
	physicalKeyboard := attachFakeKeyboard(t, c)
	physicalPointer := c.addFakePointer()
	if physicalPointer == nil {
		t.Fatal("could not attach physical pointer")
	}
	t.Cleanup(physicalPointer.remove)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
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
			if err := client.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			if err := client.CreateVirtualPointer(false); err != nil {
				return err
			}
			var x, y, width, height uint32
			onCompositor(t, c, func() {
				w := c.Windows()[0]
				x, y = uint32(w.X+20), uint32(w.Y+20)
				for _, o := range c.OutputSnapshot() {
					if o.Logical != nil {
						width, height = uint32(o.Logical.Width), uint32(o.Logical.Height)
						break
					}
				}
			})
			if err := client.VirtualAbsolute(x, y, width, height); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, true); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, false); err != nil {
				return err
			}
			if err := client.VirtualAxis(15); err != nil {
				return err
			}
			before := client.InputEvents()
			if before.PointerButtons != 2 || before.PointerAxes != 1 {
				return fmt.Errorf("virtual pointer events = %+v", before)
			}
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := client.DestroyVirtualKeyboard(); err != nil {
				return err
			}
			after := client.InputEvents()
			if after.Presses != 1 || after.Releases != 1 {
				return fmt.Errorf("virtual keyboard destroy left a held key: %+v", after)
			}
			if err := client.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			onCompositor(t, c, func() { physicalKeyboard.key(keyA, true) })
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := client.DestroyVirtualKeyboard(); err != nil {
				return err
			}
			if events := client.InputEvents(); events.Presses != after.Presses+1 || events.Releases != after.Releases {
				return fmt.Errorf("virtual disconnect released a physical held key: %+v", events)
			}
			onCompositor(t, c, func() { physicalKeyboard.key(keyA, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			if events := client.InputEvents(); events.Releases != after.Releases+1 {
				return fmt.Errorf("final physical key release lost: %+v", events)
			}
			if err := client.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, true); err != nil {
				return err
			}
			if err := client.DestroyVirtualPointer(); err != nil {
				return err
			}
			if client.InputEvents().PointerButtons != 4 {
				return fmt.Errorf("virtual pointer destroy left a held button: %+v", client.InputEvents())
			}
			if err := client.CreateVirtualPointer(false); err != nil {
				return err
			}
			onCompositor(t, c, func() { physicalPointer.button(evdevButtonLeft, true) })
			if err := client.VirtualButton(evdevButtonLeft, true); err != nil {
				return err
			}
			if err := client.DestroyVirtualPointer(); err != nil {
				return err
			}
			if events := client.InputEvents(); events.PointerButtons != 5 {
				return fmt.Errorf("virtual disconnect released physical held button: %+v", events)
			}
			onCompositor(t, c, func() { physicalPointer.button(evdevButtonLeft, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			if events := client.InputEvents(); events.PointerButtons != 6 {
				return fmt.Errorf("final physical button release lost: %+v", events)
			}
			if err := client.CreateVirtualPointer(false); err != nil {
				return err
			}
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, true); err != nil {
				return err
			}
			held := client.InputEvents()
			var px, py float64
			onCompositor(t, c, func() { px, py = c.cursorPosition(); c.setSessionLocked(true) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			locked := client.InputEvents()
			if locked.Releases != held.Releases+1 || locked.PointerButtons != held.PointerButtons+1 {
				return fmt.Errorf("lock gate did not release held virtual input: before=%+v after=%+v", held, locked)
			}
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := client.VirtualKey(keyA, false); err != nil {
				return err
			}
			if err := client.VirtualMotion(50, 50); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, true); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, false); err != nil {
				return err
			}
			if err := client.VirtualAxis(10); err != nil {
				return err
			}
			if err := client.VirtualKey(keyF12, true); err != nil {
				return err
			}
			observed := client.InputEvents()
			if observed.Presses != locked.Presses || observed.Releases != locked.Releases || observed.PointerButtons != locked.PointerButtons || observed.PointerAxes != locked.PointerAxes {
				return fmt.Errorf("virtual input crossed lock gate: before=%+v after=%+v", locked, observed)
			}
			onCompositor(t, c, func() {
				if nx, ny := c.cursorPosition(); nx != px || ny != py {
					t.Error("virtual pointer moved while locked")
				}
				c.setSessionLocked(false)
				for keyboard := range c.virtualKeyboards {
					if keyboard.num_keycodes != 0 || keyboard.modifiers.depressed != 0 {
						t.Error("blocked virtual key state survived reopening the gate")
					}
				}
				if err := c.PowerOffMonitors(); err != nil {
					t.Error(err)
				}
			})
			if err := client.VirtualMotion(10, 10); err != nil {
				return err
			}
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			if err := client.VirtualKey(keyA, false); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if !c.allOutputsOff() {
					t.Error("virtual input woke blanked outputs")
				}
				if err := c.PowerOnMonitors(); err != nil {
					t.Error(err)
				}
			})
			// A protocol client cannot attach devices to the injected seat,
			// which remains controlled solely by the Stop-gated control path.
			other, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			if err := other.CreateVirtualKeyboard(true); err == nil {
				other.Close()
				return fmt.Errorf("virtual keyboard accepted injected seat")
			}
			other.Close()
			other, err = testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			if err := other.CreateVirtualPointer(true); err == nil {
				if err := other.VirtualMotion(1, 1); err == nil {
					other.Close()
					return fmt.Errorf("virtual pointer accepted injected seat")
				}
			}
			other.Close()
			return nil
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
		t.Fatal("virtual input boundary check timed out")
	}
}

func TestInputMethodGrabPreservesHeldPhysicalKeyDestination(t *testing.T) {
	c := newNested(t)
	keyboard := attachFakeKeyboard(t, c)
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
			if err := client.StartTextInput(); err != nil {
				return err
			}
			if err := client.EnableTextInput("hello", 0, 0, 1, 16); err != nil {
				return err
			}
			onCompositor(t, c, func() { keyboard.key(keyA, true) })
			if err := client.GrabInputMethodKeyboard(); err != nil {
				return err
			}
			onCompositor(t, c, func() { keyboard.key(keyA, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			first := client.InputEvents()
			if first.Presses != 1 || first.Releases != 1 || first.GrabKeys != 0 {
				return fmt.Errorf("new grab stole a client's held release: %+v", first)
			}
			if err := client.ReleaseInputMethodKeyboard(); err != nil {
				return err
			}
			onCompositor(t, c, func() { keyboard.key(keyA, true) })
			if err := client.GrabInputMethodKeyboard(); err != nil {
				return err
			}
			if err := client.ReleaseInputMethodKeyboard(); err != nil {
				return err
			}
			onCompositor(t, c, func() { keyboard.key(keyA, false) })
			if err := client.SyncInput(); err != nil {
				return err
			}
			second := client.InputEvents()
			if second.Presses != 2 || second.Releases != 2 || second.GrabKeys != 0 {
				return fmt.Errorf("grab destruction suppressed a client's held release: %+v", second)
			}
			return nil
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
		t.Fatal("held key destination check timed out")
	}
}

func TestVirtualPointerRejectsRetiredSeat(t *testing.T) {
	c := newNested(t)
	attachFakeKeyboard(t, c)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
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
			if err := client.CreateDefaultVirtualPointer(); err != nil {
				return err
			}
			if err := client.DestroyVirtualPointer(); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if err := c.RemoveInjectedSeat("agent"); err != nil {
					t.Error(err)
				}
			})
			if err := client.CreateVirtualPointer(true); err == nil {
				return fmt.Errorf("retired explicit seat became a default user-seat pointer")
			}
			return nil
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
		t.Fatal("retired seat check timed out")
	}
}

func TestVirtualInputCarriesHeldKeysAcrossFocusAndUsesPointerModifiers(t *testing.T) {
	floating := true
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
	attachFakeKeyboard(t, c)
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
			if err := client.CreateVirtualKeyboard(false); err != nil {
				return err
			}
			if err := client.CreateVirtualPointer(false); err != nil {
				return err
			}
			if err := client.VirtualKey(keyA, true); err != nil {
				return err
			}
			other, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer other.Close()
			entered := other.InputEvents()
			if entered.KeyboardEnters == 0 || len(entered.EnterKeys) != 1 || entered.EnterKeys[0] != keyA {
				return fmt.Errorf("focus enter omitted held virtual key: %+v", entered)
			}
			if err := client.VirtualKey(keyA, false); err != nil {
				return err
			}
			if err := other.SyncInput(); err != nil {
				return err
			}
			if events := other.InputEvents(); events.Presses != 0 || events.Releases != 1 {
				return fmt.Errorf("held virtual key did not release after focus change: %+v", events)
			}
			var window WindowInfo
			var width, height, mod uint32
			onCompositor(t, c, func() {
				for _, candidate := range c.Windows() {
					if candidate.IsFocused {
						window = candidate
					}
				}
				for _, o := range c.OutputSnapshot() {
					if o.Logical != nil {
						width, height = o.Logical.Width, o.Logical.Height
						break
					}
				}
				switch c.modKey {
				case bind.ModifierSuper:
					mod = 64
				case bind.ModifierAlt:
					mod = 8
				case bind.ModifierCtrl:
					mod = 4
				case bind.ModifierShift:
					mod = 1
				}
			})
			if mod == 0 {
				return fmt.Errorf("unrecognized compositor modifier")
			}
			if err := client.VirtualAbsolute(uint32(window.X+20), uint32(window.Y+20), width, height); err != nil {
				return err
			}
			if err := client.VirtualModifiers(mod); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, true); err != nil {
				return err
			}
			if err := client.VirtualMotion(25, 15); err != nil {
				return err
			}
			if err := client.VirtualButton(evdevButtonLeft, false); err != nil {
				return err
			}
			if err := client.VirtualModifiers(0); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				got := c.Windows()[0]
				if got.ID != window.ID || got.X != window.X+25 || got.Y != window.Y+15 {
					t.Errorf("virtual modifier drag = %+v, want position %d,%d", got, window.X+25, window.Y+15)
				}
			})
			return nil
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
		t.Fatal("virtual focus/modifier check timed out")
	}
}

func TestVirtualPointerStopsAtOutputRemovalAndRejectsRetiredOutput(t *testing.T) {
	c := newNested(t)
	attachFakeKeyboard(t, c)
	original := c.FocusedOutputName()
	extra, err := addHeadlessTestOutput(c, 640, 480)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(extra.destroy)
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
			if err := client.CreateVirtualPointerForOutput(true); err != nil {
				return err
			}
			if err := client.VirtualAbsolute(1, 1, 2, 2); err != nil {
				return err
			}
			var px, py float64
			onCompositor(t, c, func() {
				px, py = c.cursorPosition()
				if name := c.outputNameAt(px, py); name == "" || name == original {
					t.Errorf("virtual pointer ignored suggested output, position %v,%v on %q", px, py, name)
				}
				extra.destroy()
				px, py = c.cursorPosition()
			})
			if err := client.VirtualMotion(40, 20); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if x, y := c.cursorPosition(); x != px || y != py {
					t.Error("virtual pointer continued after its output disappeared")
				}
			})
			if err := client.DestroyVirtualPointer(); err != nil {
				return err
			}
			if err := client.CreateVirtualPointerForOutput(true); err == nil {
				return fmt.Errorf("retired explicit output became an unrestricted pointer")
			}
			return nil
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
		t.Fatal("retired output check timed out")
	}
}
