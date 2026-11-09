package wlr

import (
	"math"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestWorkspaceSwipeFollowsTheFingers: Niri's three-finger vertical swipe
// moves the workspaces with the fingers, 300 px a workspace, draws the next
// workspace as it comes into view, and on release settles on the workspace
// it is nearest: a short swipe returns, a longer one switches.
func TestWorkspaceSwipeFollowsTheFingers(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	runTouchClients(t, c, 2, func(_ []*testclient.ToplevelClient, views []*toplevelView) {
		upper, lower := views[0], views[1]
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			if pointer != nil {
				pointer.remove()
			}
		})
		// rest is where view sits while its workspace is active.
		rest := func(view *toplevelView) int {
			for _, window := range c.model.Snapshot().Windows {
				if window.ID == view.id {
					_, y, _, _ := c.windowSceneTarget(view, window)
					return y
				}
			}
			t.Fatalf("window %d missing from the model", view.id)
			return 0
		}
		y := func(view *toplevelView) int {
			_, y := view.layoutPosition()
			return y
		}
		drawn := func(view *toplevelView) bool { return bool(view.tree.node.enabled) && view.presentationVisible }
		onCompositor(t, c, func() {
			if !c.FocusWindow(lower.id) || !c.MoveWindowToWorkspaceDown(false) {
				t.Fatal("could not move the second window to the next workspace")
			}
			first := focusedWorkspaceID(c)
			height := c.workspaceBounds(upper.outputName).Height
			upperRest, lowerRest := rest(upper), rest(lower)
			pointer = c.addFakePointer()
			pointer.moveTo(c.viewCenter(upper))

			pointer.swipeBeginAt(1000, 3)
			pointer.swipeUpdateAt(1010, 3, 0, 30)
			if want := upperRest - int(math.Round(0.1*float64(height))); y(upper) != want {
				t.Errorf("a tenth of a workspace in: upper at %d, want %d", y(upper), want)
			}
			if want := lowerRest + int(math.Round(0.9*float64(height))); !drawn(lower) || y(lower) != want {
				t.Errorf("next workspace drawn %v at %d, want at %d", drawn(lower), y(lower), want)
			}
			if focusedWorkspaceID(c) != first {
				t.Error("the workspace switched before the fingers lifted")
			}
			pointer.swipeUpdateAt(1020, 3, 0, 60)
			pointer.swipeEndAt(3000, false)
			if focusedWorkspaceID(c) != first || y(upper) != upperRest || !drawn(upper) || drawn(lower) {
				t.Errorf("a short swipe: switched %v, upper at %d (want %d), lower drawn %v",
					focusedWorkspaceID(c) != first, y(upper), upperRest, drawn(lower))
			}

			pointer.swipeBeginAt(4000, 3)
			pointer.swipeUpdateAt(4010, 3, 0, 30)
			pointer.swipeUpdateAt(4020, 3, 0, 150)
			pointer.swipeEndAt(6000, false)
			if focusedWorkspaceID(c) == first || y(lower) != lowerRest || !drawn(lower) || drawn(upper) {
				t.Errorf("a swipe past halfway: switched %v, lower at %d (want %d), upper drawn %v",
					focusedWorkspaceID(c) != first, y(lower), lowerRest, drawn(upper))
			}
			if _, switching := c.model.WorkspaceRenderOffset(upper.outputName); switching {
				t.Error("the settled switch kept its render offset")
			}
		})
	})
}
