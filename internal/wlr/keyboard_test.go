package wlr

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
)

const keyEsc = 1 // evdev KEY_ESC

// modKeycode is the evdev key that presses c's Mod: Alt in a nested session,
// Super on DRM or headless (bind.ResolveModKey, after Niri).
func (c *Compositor) modKeycode() uint32 {
	switch c.modKey {
	case bind.ModifierAlt:
		return 56 // KEY_LEFTALT
	case bind.ModifierSuper:
		return 125 // KEY_LEFTMETA
	case bind.ModifierCtrl:
		return 29 // KEY_LEFTCTRL
	case bind.ModifierShift:
		return 42 // KEY_LEFTSHIFT
	}
	panic("modKeycode: no evdev key for this Mod")
}

// attachFakeKeyboard attaches a fake keyboard for the rest of the test. A test
// whose client must receive keys attaches it before the client starts: on a
// backend with no keyboard of its own, such as DRM in CI, the seat offers none
// until then, and keys sent before the client binds wl_keyboard are lost.
func attachFakeKeyboard(t *testing.T, c *Compositor) *fakeKeyboard {
	t.Helper()
	kb := c.addFakeKeyboard()
	if kb == nil {
		t.Fatal("could not attach a fake keyboard")
	}
	t.Cleanup(kb.remove)
	return kb
}

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
	first, second := attachFakeKeyboard(t, c), attachFakeKeyboard(t, c)
	acted := false
	c.viewMapped = func(v *toplevelView) {
		if acted {
			return
		}
		acted = true
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
	attachFakeKeyboard(t, c)
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

func TestKeyboardLayoutSwitchUpdatesActiveXKBLayout(t *testing.T) {
	c := newNestedWith(t, Options{Keyboard: Keyboard{Layout: "us,de"}})
	layouts := c.KeyboardLayoutsSnapshot()
	if layouts == nil || len(layouts.Names) != 2 || layouts.CurrentIdx != 0 {
		t.Fatalf("initial keyboard layouts = %+v, want us/de at index 0", layouts)
	}
	if !c.SwitchLayout("Next", nil) {
		t.Fatal("SwitchLayout(Next) did not change the active layout")
	}
	if got := c.KeyboardLayoutsSnapshot().CurrentIdx; got != 1 {
		t.Fatalf("active layout after Next = %d, want 1", got)
	}
	if !c.SwitchLayout("Prev", nil) || c.KeyboardLayoutsSnapshot().CurrentIdx != 0 {
		t.Fatal("SwitchLayout(Prev) did not return to layout 0")
	}
	index := uint8(1)
	if !c.SwitchLayout("Index", &index) || c.KeyboardLayoutsSnapshot().CurrentIdx != index {
		t.Fatal("SwitchLayout(Index) did not select layout 1")
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

func TestFocusingNewWindowDeactivatesPrevious(t *testing.T) {
	c := newNested(t)
	t.Cleanup(c.Destroy)
	var first, second *toplevelView
	var secondClient *exec.Cmd
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			if !bool(first.toplevel.scheduled.activated) {
				t.Error("first mapped window is not activated before the next window takes focus")
			}
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		if bool(first.toplevel.scheduled.activated) {
			t.Error("previous window remains activated after the new window takes focus")
		}
		if !bool(second.toplevel.scheduled.activated) {
			t.Error("newly focused window is not activated")
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

// TestVTSwitchChordIsConsumed is M2's hardcoded VT action: it works even when
// the config has no binds, and the switch key never reaches the focused client.
func TestVTSwitchChordIsConsumed(t *testing.T) {
	c := newNested(t)
	kb := attachFakeKeyboard(t, c)
	mapped := false
	c.viewMapped = func(v *toplevelView) {
		mapped = true
		c.focusView(v)
		kb.key(60, true) // unrelated F2 input from the nested seat
		kb.key(60, false)
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
		kb.key(60, true)
		kb.key(60, false)
		go func() {
			time.Sleep(100 * time.Millisecond)
			c.Terminate()
		}()
	}
	var keys []string
	// Host keys can arrive before or after this synchronous injected block.
	runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		if m := reKey.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1]+"/"+m[2])
		}
		return false
	})
	if !mapped {
		t.Fatal("weston-terminal never mapped")
	}
	want := []string{"29/1", "56/1", "56/0", "29/0", "59/1", "59/0"}
	wantSequence := strings.Join(want, " ")
	found := false
	for start := 0; start+len(want) <= len(keys); start++ {
		if strings.Join(keys[start:start+len(want)], " ") == wantSequence {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("client key events %v, want contiguous injected sequence %v", keys, want)
	}
}

func TestCloseWindowBindSendsCloseAndConsumesKey(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Q", bind.ActionCloseWindow),
	}})
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(*toplevelView) {
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(16, true) // Q.
		if !c.suppressedKeys[16] {
			t.Error("configured close bind key was not consumed")
		}
		keyboard.key(16, false)
		keyboard.key(c.modKeycode(), false)
	}
	lines := runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		return strings.Contains(line, ".close(")
	})
	for _, line := range lines {
		if strings.Contains(line, ".close(") {
			return
		}
	}
	t.Fatal("close bind did not send xdg_toplevel.close")
}

func TestKeyBindingRepeatHonorsDelayRateAndRelease(t *testing.T) {
	repeatable := keyBind(t, "Mod+F", bind.ActionToggleWindowFloating)
	repeatable.Repeat = true
	c := newNestedWith(t, Options{
		Keyboard: Keyboard{RepeatDelay: 300, RepeatRate: 10},
		Binds:    []bind.Binding{repeatable},
	})
	keyboard := attachFakeKeyboard(t, c)
	c.viewMapped = func(view *toplevelView) {
		c.focusView(view)
		keyboard.key(c.modKeycode(), true)
		keyboard.key(33, true) // Repeatable Mod+F.
		keyboard.key(30, true) // A is forwarded once while the bind is held.
		window, found := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !found || !window.Tile.Floating {
			t.Error("bind's initial press did not run immediately")
		}
		go func() {
			pressedAt := time.Now()
			lastFloating := true
			var transitions []time.Duration
			for time.Since(pressedAt) < 2*time.Second && len(transitions) < 2 {
				time.Sleep(10 * time.Millisecond) // Poll the compositor state without sampling a timer boundary.
				floating, found := readWindowFloating(c, view)
				if !found {
					t.Error("mapped window disappeared while repeat was held")
					break
				}
				if floating != lastFloating {
					transitions = append(transitions, time.Since(pressedAt))
					lastFloating = floating
				}
			}
			if len(transitions) != 2 {
				t.Errorf("floating state transitions while holding bind = %v, want two repeats", transitions)
			} else {
				if transitions[0] < 250*time.Millisecond {
					t.Errorf("first repeat fired after %v, before the configured 300 ms delay", transitions[0])
				}
				if interval := transitions[1] - transitions[0]; interval < 50*time.Millisecond || interval > 400*time.Millisecond {
					t.Errorf("repeat interval = %v, want the configured 100 ms interval", interval)
				}
			}
			_ = c.Post(func() { keyboard.key(33, false) })   // A release cancels bind repeat.
			stopAt := time.Now().Add(350 * time.Millisecond) // Observe more than three configured repeat intervals.
			for time.Now().Before(stopAt) {
				time.Sleep(10 * time.Millisecond)
				floating, found := readWindowFloating(c, view)
				if !found || !floating {
					t.Error("bind repeated after its key was released")
					break
				}
			}
			_ = c.Post(func() {
				keyboard.key(c.modKeycode(), false)
				keyboard.key(30, false)
				c.Terminate()
			})
		}()
	}
	var keys []string
	runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		if m := reKey.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1]+"/"+m[2])
		}
		return false
	})
	if keyboard == nil {
		t.Fatal("weston-terminal never mapped")
	}
	var aPresses, fEvents int
	for _, key := range keys {
		if key == "30/1" {
			aPresses++
		}
		if strings.HasPrefix(key, "33/") {
			fEvents++
		}
	}
	if aPresses != 1 || fEvents != 0 {
		t.Errorf("client key events = %v, want one A press and no repeatable bind key events", keys)
	}
}

func TestKeyBindingRepeatFalseRunsOnlyOnPress(t *testing.T) {
	nonRepeating := keyBind(t, "Mod+F", bind.ActionToggleWindowFloating)
	c := newNestedWith(t, Options{
		Keyboard: Keyboard{RepeatDelay: 200, RepeatRate: 20},
		Binds:    []bind.Binding{nonRepeating},
	})
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		c.focusView(view)
		keyboard.key(c.modKeycode(), true)
		keyboard.key(33, true)
		window, found := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !found || !window.Tile.Floating {
			t.Error("repeat=false bind's initial press did not run")
		}
		go func() {
			// The delay plus two repeat intervals would expose an accidental timer.
			stopAt := time.Now().Add(400 * time.Millisecond)
			for time.Now().Before(stopAt) {
				time.Sleep(10 * time.Millisecond)
				floating, found := readWindowFloating(c, view)
				if !found || !floating {
					t.Error("repeat=false bind repeated while held")
					break
				}
			}
			_ = c.Post(func() {
				keyboard.key(33, false)
				keyboard.key(c.modKeycode(), false)
				c.Terminate()
			})
		}()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if keyboard == nil {
		t.Fatal("weston-terminal never mapped")
	}
}

func readWindowFloating(c *Compositor, view *toplevelView) (bool, bool) {
	type result struct {
		floating bool
		found    bool
	}
	ready := make(chan result, 1)
	if err := c.Post(func() {
		window, found := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		ready <- result{floating: window.Tile.Floating, found: found}
	}); err != nil {
		return false, false
	}
	select {
	case got := <-ready:
		return got.floating, got.found
	case <-time.After(time.Second):
		return false, false
	}
}

// TestBindKeysIgnoreShiftAndLayoutAsNiri is Niri's find_bind trigger,
// smithay's raw_latin_sym_or_raw_current_sym: a bind names the key's
// unshifted keysym, and on a non-Latin layout the keysym of the first layout
// that gives that key an ASCII one. It needs no compositor.
func TestBindKeysIgnoreShiftAndLayoutAsNiri(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	kb, err := newDetachedFakeKeyboard(Keyboard{Layout: "us,ru"})
	if err != nil {
		t.Fatal(err)
	}
	defer kb.remove()
	const (
		keyOne   = 2  // evdev KEY_1
		keyQ     = 16 // evdev KEY_Q
		keyE     = 18 // evdev KEY_E
		keyShift = 42 // evdev KEY_LEFTSHIFT
		keySlash = 53 // evdev KEY_SLASH
	)
	expect := func(keycode uint32, name string, want bool) {
		t.Helper()
		if got := keysymMatches(kb.kb, keycode, name); got != want {
			t.Errorf("key %d matches %q = %v, want %v", keycode, name, got, want)
		}
	}

	kb.key(keyShift, true)
	expect(keySlash, "Slash", true)
	expect(keySlash, "question", false)
	expect(keyOne, "1", true)
	expect(keyOne, "exclam", false)
	expect(keyE, "E", true)
	kb.key(keyShift, false)

	kb.switchLayout(1)
	expect(keyQ, "q", true)
	expect(keyQ, "Cyrillic_shorti", false)
	// An ASCII keysym on the current layout is kept.
	expect(keySlash, "period", true)
}
