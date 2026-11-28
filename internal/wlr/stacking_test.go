package wlr

import (
	"runtime"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
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

func TestFloatingDecorationsDrawWithTheirWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c, destroy := newStackingTestCompositor()
	if c == nil {
		t.Fatal("creating the stacking trees")
	}
	defer destroy()

	floating := c.newStackingTestView()
	floating.floating = true
	floating.decorations = viewDecorations{
		shadow:         config.Shadow{On: true, Softness: 8},
		ring:           config.FocusRing{Width: 3},
		border:         config.FocusRing{Off: true},
		withBackground: true,
	}
	sceneFXNodeSetPosition(floating.sceneNode(), 30, 60)
	c.createViewDecorations(floating, rect{w: 120, h: 100})

	tiled := c.newStackingTestView()
	tiledLayer := &tiled.tree.node.parent.node
	floatingLayer := &floating.tree.node.parent.node
	if tiledLayer == floatingLayer || !sceneNodeBefore(tiledLayer, floatingLayer) {
		t.Fatal("the floating window's layer must draw above tiled windows")
	}
	if !sceneFXNodeBefore(floating.shadowNode, floating.borderNode) ||
		!sceneFXNodeBefore(floating.borderNode, floating.resizeSnapshot) ||
		!sceneFXNodeBefore(floating.resizeSnapshot, floating.sceneNode()) {
		t.Fatal("want the floating shadow, border, resize snapshot, and window in the same stack, bottom first")
	}
	if x, y := sceneFXNodePosition(floating.sceneNode()); x != 30 || y != 60 {
		t.Fatalf("moving the view to the floating layer changed its position to (%d, %d)", x, y)
	}

	floating.floating = false
	c.syncViewStacking(floating)
	if floating.tree.node.parent != tiled.tree.node.parent ||
		sceneFXNodeBefore(floating.resizeSnapshot, floating.sceneNode()) {
		t.Fatal("returning to tiling must restore the tiled window and animation layers")
	}
}
