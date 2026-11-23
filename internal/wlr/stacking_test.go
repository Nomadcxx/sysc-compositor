package wlr

import (
	"runtime"
	"testing"
)

// A resize, open or close snapshot stands in for a window hidden underneath,
// so it must draw over the border fill of a client-decorated window.
func TestAnimationSnapshotsDrawAboveBorders(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c, destroy := newStackingTestCompositor()
	if c == nil {
		t.Fatal("creating the stacking trees")
	}
	defer destroy()
	if !c.windowTreesStackedForTest() {
		t.Fatal("want shadows, borders, animation snapshots, then windows, bottom first")
	}
}
