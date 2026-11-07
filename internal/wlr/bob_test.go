package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// TestBobOffsetFollowsNiri checks Niri's baba_is_float_offset on a 960 px
// output: an amplitude of 10 px, a 3.6 s cycle, never below the place.
func TestBobOffsetFollowsNiri(t *testing.T) {
	for _, tc := range []struct {
		at   time.Duration
		want int
	}{
		{0, -10},
		{900 * time.Millisecond, 0},
		{2700 * time.Millisecond, -20},
		{3600 * time.Millisecond, -10},
	} {
		if got := bobOffset(time.Unix(0, int64(tc.at)), 960); got != tc.want {
			t.Errorf("bobOffset(%v) = %d, want %d", tc.at, got, tc.want)
		}
	}
}

// TestBabaIsFloatBobsTheWindow: a window with baba-is-float is drawn at its
// bob offset, the layout keeps its place, and the window goes back to its
// place when the rule stops matching.
func TestBabaIsFloatBobsTheWindow(t *testing.T) {
	on := true
	opts := Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{BabaIsFloat: &on}}}
	c := newNestedWith(t, opts)
	c.viewMapped = func(view *toplevelView) {
		defer c.Terminate()
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			return
		}
		bounds, _ := c.outputLayoutBounds(window.Output)
		now := time.Unix(0, int64(2700*time.Millisecond))
		want := bobOffset(now, bounds.Height)
		if want == 0 {
			t.Fatalf("bob offset at the bottom of the cycle is 0 on a %d px output", bounds.Height)
		}
		placeX, placeY := view.layoutPosition()
		c.syncBobbing(now)
		if x, y := view.treePosition(); x != placeX || y != placeY+want {
			t.Errorf("tree at (%d,%d), want (%d,%d): the place plus the bob", x, y, placeX, placeY+want)
		}
		if x, y := view.layoutPosition(); x != placeX || y != placeY {
			t.Errorf("layout position (%d,%d), want the place (%d,%d)", x, y, placeX, placeY)
		}
		opts.WindowRules = nil
		if err := c.Reconfigure(opts); err != nil {
			t.Error(err)
			return
		}
		if x, y := view.treePosition(); view.bobY != 0 || x != placeX || y != placeY {
			t.Errorf("after the rule went: tree at (%d,%d) with bob %d, want the place (%d,%d)", x, y, view.bobY, placeX, placeY)
		}
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}
