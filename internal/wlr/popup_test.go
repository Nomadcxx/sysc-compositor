package wlr

import (
	"testing"
	"time"
)

const btnRight = 273 // evdev BTN_RIGHT

// TestPopupDrawsBesideItsParent is report X1: weston-terminal's right-click
// menu is an xdg_popup. It must draw in its window's tree, not at the layout
// origin, and inside the output.
func TestPopupDrawsBesideItsParent(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	var (
		clicked      bool
		placed       []popupPlacement
		output, geom rect
		rootX, rootY int
	)
	c.viewMapped = func(v *toplevelView) {
		if clicked {
			return
		}
		clicked = true
		// Put the window in the output's bottom-right corner, so a popup at
		// the layout origin cannot pass by accident, and click 40 px inside
		// that corner, clear of the client's frame. The layout may move the
		// window back to its tile on a later commit, so the expected origin is
		// read with the popups below, not taken from this placement.
		out := c.layoutBox()
		geom = v.geometry()
		c.placeView(v, out.x+out.w-geom.w, out.y+out.h-geom.h)
		x, y := float64(out.x+out.w-40), float64(out.y+out.h-40)
		for _, step := range []func() error{
			func() error { return c.InjectPointerMotion("agent", x, y) },
			func() error { return c.InjectPointerButton("agent", btnRight, true) },
		} {
			if err := step(); err != nil {
				t.Errorf("injection: %v", err)
			}
		}
		go func() {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				done := make(chan bool, 1)
				if c.Post(func() {
					placed = c.popupPlacements()
					output = c.layoutBox()
					geom = v.geometry()
					x, y := v.treePosition()
					rootX, rootY = x-geom.x, y-geom.y
					done <- len(placed) > 0
				}) != nil || <-done {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			_ = c.Post(c.Terminate)
		}()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)

	if !clicked {
		t.Fatal("weston-terminal never mapped a window")
	}
	if len(placed) == 0 {
		t.Fatal("the right click opened no mapped popup")
	}
	if geom.x == 0 && geom.y == 0 {
		t.Log("weston-terminal reported no geometry offset; the origin check below is weaker")
	}
	for _, p := range placed {
		if !p.underParent {
			t.Errorf("popup tree is not a child of its parent's tree (box %+v)", p.box)
		}
		if p.box.x == 0 && p.box.y == 0 {
			t.Errorf("popup drawn at the layout origin %+v, not beside its window", p.box)
		}
		// weston-terminal sets no constraint_adjustment, so wlroots may not
		// move this popup; what the compositor owns is the box it hands over.
		// It must be the output, measured from the window's surface origin,
		// which sits the geometry offset (the client's shadow) above-left of
		// where the window is drawn.
		if !p.hasConstraint {
			t.Errorf("no constraint box for popup %+v", p.box)
			continue
		}
		if p.rootX != rootX || p.rootY != rootY {
			t.Errorf("root surface origin (%d,%d), want (%d,%d): the geometry offset was not applied",
				p.rootX, p.rootY, rootX, rootY)
		}
		got := rect{p.constraint.x + p.rootX, p.constraint.y + p.rootY, p.constraint.w, p.constraint.h}
		if got != output {
			t.Errorf("constraint box in layout coordinates %+v, want the output %+v", got, output)
		}
	}
	// The menu's grab is on the injected seat. Removing that seat with the
	// grab open, and the compositor teardown after it, must not crash.
	if err := c.RemoveInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
}
