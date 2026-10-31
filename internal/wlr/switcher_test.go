package wlr

import (
	"math"
	"os/exec"
	"reflect"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

const keyTab = 15 // evdev KEY_TAB

// keyLeftAlt is a literal Alt: the recent-windows binds are Alt+Tab, not Mod.
const keyLeftAlt = 56 // evdev KEY_LEFTALT

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
    Mod+Shift+WheelScrollDown { focus-column-right-or-first; }
    Mod+L { focus-column-or-monitor-left; }
    Mod+J { focus-window-down-or-column-right; }
    Mod+K { focus-window-up-or-column-left; }
    Mod+Tab { focus-column-left-or-last; }
    Mod+O { focus-column-or-monitor-right; }
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
		// Niri's mru.rs make_dynamic_opened_binds maps the or-actions too,
		// on any trigger.
		"WheelScrollDown": {Name: bind.ActionMRUAdvance},
		"L":               {Name: bind.ActionMRUAdvance, Backward: true},
		"J":               {Name: bind.ActionMRUAdvance},
		"K":               {Name: bind.ActionMRUAdvance, Backward: true},
		"Tab":             {Name: bind.ActionMRUAdvance, Backward: true},
		"O":               {Name: bind.ActionMRUAdvance},
	}
	if len(got) != len(want) {
		t.Fatalf("dynamic binds = %+v", got)
	}
	for _, b := range got {
		chord, _ := bind.ParseChord(b.Chord.Key)
		if b.Chord != (bind.Chord{Key: b.Chord.Key, Trigger: chord.Trigger}) || len(b.Actions) != 1 || !reflect.DeepEqual(b.Actions[0], want[b.Chord.Key]) {
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

const keyEnd = 107 // evdev KEY_END

// When the strip overflows the output, moving the selection out of view
// springs the view with window-movement instead of jumping; the first
// layout does not animate.
func TestWindowSwitcherSpringsTheViewToTheSelection(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	recent := config.Default().RecentWindows
	recent.OpenDelayMS = 0
	// Full-size previews of half-width tiles overflow any output.
	recent.Previews = config.MRUPreviews{MaxHeight: 65535, MaxScale: 1}
	c := newNestedWith(t, Options{RecentWindows: &recent, Animations: config.Default().Animations})
	var views []*toplevelView
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 3 {
			return
		}
		defer c.Terminate()
		for _, view := range views {
			c.focusView(view)
			c.applyPendingWindowFocus()
		}
		keyboard := c.addFakeKeyboard()
		defer keyboard.remove()
		keyboard.key(keyLeftAlt, true)
		keyboard.key(keyTab, true)
		keyboard.key(keyTab, false)
		s := &c.switcher
		if !s.isOpen() || c.animations[switcherViewKey] != nil || s.viewPos != s.viewTarget {
			t.Fatalf("the first layout animated: open %v, view %v heading for %v", s.isOpen(), s.viewPos, s.viewTarget)
		}
		before := s.viewPos
		keyboard.key(keyEnd, true)
		keyboard.key(keyEnd, false)
		if s.viewTarget == before {
			t.Fatalf("selecting the last window left the view at %v", before)
		}
		if c.animations[switcherViewKey] == nil || s.viewPos == s.viewTarget {
			t.Errorf("the view jumped to %v instead of springing there", s.viewTarget)
		}
		keyboard.key(keyEsc, true)
		keyboard.key(keyEsc, false)
		keyboard.key(keyLeftAlt, false)
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 3 {
		t.Fatalf("mapped windows = %d, want three", len(views))
	}
}

const keyGrave = 41 // evdev KEY_GRAVE

// TestWindowSwitcherPreviewsMoveAsNiri: previews slide with window-movement
// from where they were drawn when a filter change relists them (Niri's
// animate_scope_filter_change) and when a listed window goes (Niri's
// remove_window), instead of jumping.
func TestWindowSwitcherPreviewsMoveAsNiri(t *testing.T) {
	for _, client := range []string{"weston-terminal", "weston-flower"} {
		if _, err := exec.LookPath(client); err != nil {
			t.Skipf("%s unavailable: %v", client, err)
		}
	}
	recent := config.Default().RecentWindows
	recent.OpenDelayMS = 0
	c := newNestedWith(t, Options{RecentWindows: &recent, Animations: config.Default().Animations})
	var views []*toplevelView
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 3 {
			return
		}
		defer c.Terminate()
		var terminals []*toplevelView
		var flower *toplevelView
		for _, v := range views {
			if cString(v.toplevel.app_id) == "org.freedesktop.weston.wayland-terminal" {
				terminals = append(terminals, v)
			} else {
				flower = v
			}
		}
		if len(terminals) != 2 || flower == nil {
			t.Fatalf("mapped %d terminals and flower %v", len(terminals), flower != nil)
		}
		// Newest first: terminal, flower, terminal.
		for _, v := range []*toplevelView{terminals[1], flower, terminals[0]} {
			c.focusView(v)
			c.applyPendingWindowFocus()
		}
		keyboard := c.addFakeKeyboard()
		defer keyboard.remove()
		keyboard.key(keyLeftAlt, true)
		for range 2 {
			keyboard.key(keyTab, true)
			keyboard.key(keyTab, false)
		}
		s := &c.switcher
		if id, _ := s.selected(); id != terminals[1].id {
			t.Fatalf("selected %d, want the oldest terminal %d", id, terminals[1].id)
		}
		strip, _ := c.switcherStrip()
		hole := func(v *toplevelView) float64 { return c.switcherPreviewWidth(strip, v.id) + strip.Gap() }

		// Alt+grave filters by the terminal's app id: the newest terminal,
		// left of the selection, starts where it was, past the flower.
		keyboard.key(keyGrave, true)
		keyboard.key(keyGrave, false)
		if got, want := c.switcherMoveOffset(terminals[0].id), -hole(flower); got != want {
			t.Errorf("after filtering the newest terminal is drawn %v from its place, want %v", got, want)
		}
		if c.animations[switcherMoveKey(terminals[0].id)] == nil {
			t.Errorf("the newest terminal's preview does not move with window-movement")
		}

		// The newest terminal goes: the selection's preview slides into its place.
		c.switcherWindowGone(terminals[0].id)
		if got, want := c.switcherMoveOffset(terminals[1].id), hole(terminals[0]); got != want {
			t.Errorf("after a removal the remaining terminal is drawn %v from its place, want %v", got, want)
		}
		keyboard.key(keyEsc, true)
		keyboard.key(keyEsc, false)
		keyboard.key(keyLeftAlt, false)
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & weston-flower & wait"}, nil, nil)
	if len(views) != 3 {
		t.Fatalf("mapped windows = %d, want three", len(views))
	}
}

// TestWindowSwitcherClipsPreviewsToGeometry: as Niri's Thumbnail::render, a
// preview shows the window's geometry only, with its corner radius scaled to
// the preview, even when the window itself is not clipped.
func TestWindowSwitcherClipsPreviewsToGeometry(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	recent := config.Default().RecentWindows
	recent.OpenDelayMS = 0
	radius := config.CornerRadius{24, 24, 24, 24}
	c := newNestedWith(t, Options{
		RecentWindows: &recent,
		WindowRules:   []config.WindowRule{{GeometryCornerRadius: &radius}},
	})
	var views []*toplevelView
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 {
			return
		}
		defer c.Terminate()
		for _, v := range views {
			c.focusView(v)
			c.applyPendingWindowFocus()
		}
		keyboard := c.addFakeKeyboard()
		defer keyboard.remove()
		keyboard.key(keyLeftAlt, true)
		keyboard.key(keyTab, true)
		keyboard.key(keyTab, false)
		strip, _ := c.switcherStrip()
		for _, v := range views {
			preview := c.switcher.previews[v.id]
			if preview == nil {
				t.Fatalf("window %d has no preview", v.id)
			}
			g := v.geometry()
			w, h := strip.PreviewSize(float64(g.w), float64(g.h))
			bounds, ok := sceneFXTreeBounds(preview)
			if !ok || bounds.w > int(math.Round(w)) || bounds.h > int(math.Round(h)) {
				t.Errorf("window %d (geometry %+v) preview covers %+v, want within %vx%v", v.id, g, bounds, w, h)
			}
			zoom := w / float64(g.w)
			var want config.CornerRadius
			for i, r := range v.cornerRadii {
				want[i] = r * zoom
			}
			if got := sceneFXTreeRoundedBufferCornerRadii(preview); got != sceneFXRadii(want) {
				t.Errorf("window %d preview corners %v, want %v", v.id, got, sceneFXRadii(want))
			}
		}
		keyboard.key(keyEsc, true)
		keyboard.key(keyEsc, false)
		keyboard.key(keyLeftAlt, false)
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 2 {
		t.Fatalf("mapped windows = %d, want two", len(views))
	}
}
