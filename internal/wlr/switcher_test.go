package wlr

import (
	"os/exec"
	"reflect"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

const keyTab = 15 // evdev KEY_TAB

// The open switcher's keys taken from general binds: Niri's mapped actions
// only, matching under any modifiers, the bind with the fewest modifiers per
// key.
func TestSwitcherDynamicBindsFollowNiri(t *testing.T) {
	cfg, err := config.Parse("binds.kdl", []byte(`binds {
    Mod+Right { focus-column-right; }
    Mod+Ctrl+Right { focus-column-right; }
    Ctrl+Right { focus-column-left; }
    Mod+Home { focus-column-first; }
    Mod+Q { close-window; }
    Print { screenshot; }
    Mod+T { spawn "foot"; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Binds {
		cfg.Binds[i].Chord = cfg.Binds[i].Chord.ResolveMod(bind.ModifierSuper)
	}
	// Mod+Right and Ctrl+Right tie on one modifier: the first wins, as
	// Niri's min_by_key keeps it.
	got := switcherDynamicBinds(cfg.Binds)
	want := map[string]bind.Action{
		"Right": {Name: bind.ActionMRUAdvance},
		"Home":  {Name: bind.ActionMRUFirst},
		"Q":     {Name: bind.ActionMRUCloseCurrentWindow},
		"Print": {Name: bind.ActionScreenshot, ShowPointer: true, WriteToDisk: true},
	}
	if len(got) != len(want) {
		t.Fatalf("dynamic binds = %+v", got)
	}
	for _, b := range got {
		if b.Chord != (bind.Chord{Key: b.Chord.Key}) || len(b.Actions) != 1 || !reflect.DeepEqual(b.Actions[0], want[b.Chord.Key]) {
			t.Errorf("dynamic bind %+v, want key alone running %+v", b, want[b.Chord.Key])
		}
	}
}

// Alt+Tab opens the switcher on the next most recently used window and
// holds the keyboard; Tab and the arrow keys move the selection; releasing
// Alt focuses it; Escape cancels; a click on a preview focuses its window.
func TestWindowSwitcherSelectsConfirmsAndCancels(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	recent := config.Default().RecentWindows
	recent.OpenDelayMS = 0
	c := newNestedWith(t, Options{RecentWindows: &recent})
	var views []*toplevelView
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 3 {
			return
		}
		defer c.Terminate()
		a, b, third := views[0], views[1], views[2]
		for _, view := range views {
			c.focusView(view)
			c.applyPendingWindowFocus()
		}
		keyboard := c.addFakeKeyboard()
		defer keyboard.remove()
		pointer := c.addFakePointer()
		defer pointer.remove()
		press := func(key uint32) {
			keyboard.key(key, true)
			keyboard.key(key, false)
		}
		selected := func() uint64 {
			id, open := c.SwitcherOpen()
			if !open {
				return 0
			}
			return id
		}

		keyboard.key(keyLeftAlt, true)
		press(keyTab)
		if selected() != b.id {
			t.Fatalf("Alt+Tab selected %d, want the previous window %d", selected(), b.id)
		}
		if c.keyboardFocusedView() != nil {
			t.Error("a client kept the keyboard while the switcher is open")
		}
		if c.switcher.tree == nil || len(c.switcher.hits) != 3 {
			t.Errorf("the switcher shows %d previews, want 3", len(c.switcher.hits))
		}
		press(keyTab)
		if selected() != a.id {
			t.Errorf("a second Tab selected %d, want the oldest window %d", selected(), a.id)
		}
		press(keyTab)
		if selected() != third.id {
			t.Errorf("Tab from the last window selected %d, want it to wrap to %d", selected(), third.id)
		}
		press(keyLeft)
		if selected() != a.id {
			t.Errorf("Left from the first window selected %d, want it to wrap to %d", selected(), a.id)
		}
		keyboard.key(keyLeftAlt, false)
		if _, open := c.SwitcherOpen(); open {
			t.Fatal("releasing Alt did not close the switcher")
		}
		if c.focusedView != a || c.keyboardFocusedView() != a {
			t.Fatalf("releasing Alt focused %v, want the selected window %d", c.focusedView.id, a.id)
		}

		keyboard.key(keyLeftAlt, true)
		press(keyTab)
		if selected() != third.id {
			t.Errorf("reopening selected %d, want %d after the confirmed window", selected(), third.id)
		}
		press(keyEsc)
		if _, open := c.SwitcherOpen(); open || c.focusedView != a || c.keyboardFocusedView() != a {
			t.Error("Escape did not close the switcher and keep the focus")
		}
		keyboard.key(keyLeftAlt, false)

		keyboard.key(keyLeftAlt, true)
		press(keyTab)
		var target switcherHit
		for _, hit := range c.switcher.hits {
			if hit.id == b.id {
				target = hit
			}
		}
		box := c.outputLayoutBox(c.switcher.output)
		pointer.moveTo(float64(box.Min.X)+target.box.X+target.box.W/2, float64(box.Min.Y)+target.box.Y+target.box.H/2)
		if selected() != b.id {
			t.Errorf("the pointer over a preview selected %d, want %d", selected(), b.id)
		}
		pointer.button(btnLeft, true)
		pointer.button(btnLeft, false)
		if _, open := c.SwitcherOpen(); open || c.focusedView != b {
			t.Error("a click on a preview did not focus its window")
		}
		keyboard.key(keyLeftAlt, false)
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 3 {
		t.Fatalf("mapped windows = %d, want three", len(views))
	}
}
