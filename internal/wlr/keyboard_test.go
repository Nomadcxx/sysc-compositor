package wlr

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
)

const keyEsc = 1 // evdev KEY_ESC

var (
	reRepeat = regexp.MustCompile(`wl_keyboard#\d+\.repeat_info\((\d+), (\d+)\)`)
	reKeymap = regexp.MustCompile(`wl_keyboard#\d+\.keymap\((\d+),`)
	reKey    = regexp.MustCompile(`wl_keyboard#\d+\.key\(\d+, \d+, (\d+), (\d+)\)`)
)

// TestUserKeyboardsShareOneKeymap is report K1–K3: clients on the user seat
// get a real xkb keymap, keys from every attached keyboard reach them, and
// Escape is an ordinary key rather than a way to end the session.
func TestUserKeyboardsShareOneKeymap(t *testing.T) {
	c := newNested(t)
	var first, second *fakeKeyboard
	t.Cleanup(func() {
		for _, kb := range []*fakeKeyboard{first, second} {
			if kb != nil {
				kb.remove()
			}
		}
	})
	acted := false
	c.viewMapped = func(v *toplevelView) {
		if acted {
			return
		}
		acted = true
		first, second = c.addFakeKeyboard(), c.addFakeKeyboard()
		if first == nil || second == nil {
			t.Error("could not attach the test keyboards")
			c.Terminate()
			return
		}
		c.focusView(v)
		first.key(keyEsc, true)
		first.key(keyEsc, false)
		// If Escape ended the session, this never runs.
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = c.Post(func() {
				second.key(keyA, true)
				second.key(keyA, false)
			})
		}()
	}
	var keys []string
	lines := runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		if m := reKey.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1]+"/"+m[2])
		}
		return len(keys) == 4
	})
	if !acted {
		t.Fatal("weston-terminal never mapped")
	}
	formats := []string{}
	for _, line := range lines {
		if m := reKeymap.FindStringSubmatch(line); m != nil {
			formats = append(formats, m[1])
		}
	}
	if len(formats) == 0 || formats[len(formats)-1] != "1" {
		t.Errorf("keymap formats %v, want the last one xkb_v1 (1)", formats)
	}
	want := []string{"1/1", "1/0", "30/1", "30/0"}
	if strings.Join(keys, " ") != strings.Join(want, " ") {
		t.Errorf("client keys %v, want Escape then A from the two keyboards %v", keys, want)
	}
}

// TestConfiguredKeyboardReachesClients is the greeter's input.keyboard: the
// xkb names build the seat's keymap and the repeat reaches clients.
func TestConfiguredKeyboardReachesClients(t *testing.T) {
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	c, err := NewWith(Options{Keyboard: Keyboard{Layout: "de", RepeatDelay: 400, RepeatRate: 40}})
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	t.Cleanup(c.Destroy)
	if got := c.userLayout(); !strings.HasPrefix(got, "German") {
		t.Errorf("seat layout %q, want German", got)
	}
	var repeat []string
	runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		if m := reRepeat.FindStringSubmatch(line); m != nil {
			repeat = m[1:]
		}
		return repeat != nil
	})
	if strings.Join(repeat, "/") != "40/400" {
		t.Errorf("client repeat_info %v, want rate 40, delay 400", repeat)
	}
}

// TestNewWindowTakesFocus: sysc-greet's kitty is typed into without a click.
func TestNewWindowTakesFocus(t *testing.T) {
	c := newNested(t)
	var focused, surface unsafe.Pointer
	var view, want *toplevelView
	c.viewMapped = func(v *toplevelView) {
		focused, view = c.userFocus()
		surface, want = unsafe.Pointer(v.toplevel.base.surface), v
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if want == nil {
		t.Fatal("weston-terminal never mapped")
	}
	if focused != surface || view != want {
		t.Errorf("keyboard focus %p (view %p) once mapped, want the new window %p", focused, view, surface)
	}
}

// TestVTSwitchChordIsConsumed is M2's hardcoded VT action: it works even when
// the config has no binds, and the switch key never reaches the focused client.
func TestVTSwitchChordIsConsumed(t *testing.T) {
	c := newNested(t)
	var kb *fakeKeyboard
	t.Cleanup(func() {
		if kb != nil {
			kb.remove()
		}
	})
	c.viewMapped = func(v *toplevelView) {
		kb = c.addFakeKeyboard()
		if kb == nil {
			t.Error("could not attach the test keyboard")
			c.Terminate()
			return
		}
		c.focusView(v)
		kb.key(29, true) // Left Ctrl
		kb.key(56, true) // Left Alt
		kb.key(59, true) // F1, whose release follows the modifier releases below
		for _, keycode := range []uint32{60, 61, 62, 63, 64, 65, 66, 67, 68, 87, 88} {
			kb.key(keycode, true) // F2 through F12
			kb.key(keycode, false)
		}
		kb.key(56, false)
		kb.key(29, false)
		kb.key(59, false)
		// F1 without Ctrl+Alt remains an ordinary client key.
		kb.key(59, true)
		kb.key(59, false)
		go func() {
			time.Sleep(100 * time.Millisecond)
			c.Terminate()
		}()
	}
	var keys []string
	runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		if m := reKey.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1]+"/"+m[2])
		}
		return false
	})
	if kb == nil {
		t.Fatal("weston-terminal never mapped")
	}
	want := []string{"29/1", "56/1", "56/0", "29/0", "59/1", "59/0"}
	if strings.Join(keys, " ") != strings.Join(want, " ") {
		t.Errorf("client received key events %v, want the chord suppressed and bare F1 forwarded %v", keys, want)
	}
}
