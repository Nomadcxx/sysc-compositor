package wlr

import (
	"bytes"
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestWorkspaceSwitchStaysOnItsOutput(t *testing.T) {
	c := newNested(t)
	var primary unsafe.Pointer
	for primary = range c.sceneOutputs {
		break
	}
	name := addConfigurableTestOutput(t, c)
	bounds, _ := c.outputLayoutBounds(outputName(primary))
	if err := c.Reconfigure(Options{Animations: config.Animations{Off: true}, Outputs: []config.OutputConfig{
		{Name: outputName(primary), Position: &config.OutputPosition{X: 0, Y: 0}},
		{Name: name, Position: &config.OutputPosition{X: 0, Y: bounds.Height}},
	}}); err != nil {
		t.Fatal(err)
	}
	c.model.FocusOutput(outputName(primary))
	runTouchClients(t, c, 1, func(_ []*testclient.ToplevelClient, views []*toplevelView) {
		onCompositor(t, c, func() {
			var neighbor unsafe.Pointer
			for pointer := range c.sceneOutputs {
				if outputName(pointer) == name {
					neighbor = pointer
				}
			}
			view := views[0]
			c.SetFullscreen(view.id, true)
			before, err := c.captureOutputForTarget(neighbor, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return
			}
			c.model.SetWorkspaceRenderOffset(outputName(primary), -0.9)
			c.presentWorkspaceSwitch()
			c.syncOverviewScene()
			after, err := c.captureOutputForTarget(neighbor, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(before.Pix, after.Pix) {
				t.Error("a switching fullscreen window drew onto the output below it")
			}
			if !sceneFXNodeEnabled(view.sceneNode()) {
				t.Error("rendering the neighbor left the source window disabled")
			}
		})
	})
}

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
			bounds, _ := c.outputLayoutBounds(upper.outputName)
			height := bounds.Height + int(math.Round(float64(bounds.Height)*0.1))
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

func TestDndWorkspaceSwitchDwellsAndStopsOnDrop(t *testing.T) {
	cfg, err := config.Parse("dnd.kdl", []byte(`gestures { dnd-edge-workspace-switch { trigger-height 50; delay-ms 20; max-speed 150000; }; }`))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{Gestures: cfg.Gestures, Animations: config.Animations{Off: true}, Workspaces: []wm.WorkspaceConfig{{Name: "one"}, {Name: "two"}, {Name: "three"}}})
	runTouchClients(t, c, 0, func(_ []*testclient.ToplevelClient, _ []*toplevelView) {
		var pointer *fakePointer
		onCompositor(t, c, func() { pointer = c.addFakePointer() })
		defer onCompositor(t, c, func() { pointer.remove() })
		for _, touch := range []bool{false, true} {
			var drag *fakeDrag
			var first uint64
			armed := false
			onCompositor(t, c, func() {
				c.FocusWorkspaceName("one")
				first = focusedWorkspaceID(c)
				c.OpenOverview()
				bounds, _ := c.outputLayoutBounds(c.FocusedOutputName())
				x, y := float64(bounds.X+bounds.Width/2), float64(bounds.Y+bounds.Height-1)
				if touch {
					pointer.moveTo(x, float64(bounds.Y+bounds.Height/2))
				} else {
					pointer.moveTo(x, y)
				}
				drag = newFakeDrag(c)
				if touch {
					c.touchPoints[17] = &touchPoint{id: 17, layoutX: x, layoutY: y}
					drag.drag.grab_type = 2
					drag.drag.grab_touch_id = 17
					drag.motion()
				}
				armed = c.dndScroll != nil && c.dndScroll.timer != nil
				if offset, _ := c.model.WorkspaceRenderOffset(c.FocusedOutputName()); offset != 0 {
					t.Error("DND scrolled before dwell")
				}
			})
			if !armed {
				t.Errorf("touch=%v: overview bottom edge did not arm workspace scrolling", touch)
			}
			moved := false
			deadline := time.Now().Add(2 * time.Second)
			for armed && time.Now().Before(deadline) && !moved {
				time.Sleep(20 * time.Millisecond)
				onCompositor(t, c, func() { offset, ok := c.model.WorkspaceRenderOffset(c.FocusedOutputName()); moved = ok && offset > .5 })
			}
			onCompositor(t, c, func() {
				if !moved || focusedWorkspaceID(c) != first {
					t.Errorf("touch=%v: DND did not move the strip with activation deferred until drop", touch)
				}
				drag.drop()
				if focusedWorkspaceID(c) == first {
					t.Errorf("touch=%v: drop did not activate the nearest workspace", touch)
				}
				if c.dndScroll.timer != nil {
					t.Error("drop left the scroll timer running")
				}
				drag.destroy()
				delete(c.touchPoints, 17)
				if c.dndScroll != nil {
					t.Error("destroy left drag state installed")
				}
			})
		}
	})
}

// TestNativeTouchDragEndsAfterItsSurfaceIsGone: the surface a drag finger went
// down on can go while its client stays; the finger keeps moving the drag and
// its up over a client still ends the source-less grab, which wlroots 0.20.2
// would otherwise crash on.
func TestNativeTouchDragEndsAfterItsSurfaceIsGone(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		view := views[0]
		var screen *fakeTouch
		onCompositor(t, c, func() { screen = c.addFakeTouch() })
		defer onCompositor(t, c, func() { screen.remove() })
		if err := clients[0].BindTouch(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			lx, ly := c.viewCenter(view)
			dx, dy := touchDevicePoint(c, lx, ly)
			screen.down(17, dx, dy)
			screen.frame()
			if !c.requestTestDrag(view, uint32(c.seat.touch_state.grab_serial)) {
				t.Error("valid touch serial did not start the native drag")
				screen.up(17)
				return
			}
			// What the surface's destroy listener runs.
			c.touchSurfaceGone(c.touchPoints[17])
			dx, dy = touchDevicePoint(c, lx+20, ly)
			screen.motion(17, dx, dy)
			screen.frame()
			if point := c.touchPoints[17]; point == nil || point.layoutX != lx+20 {
				t.Error("the drag finger stopped moving the drag when its surface went")
			}
			screen.up(17)
			screen.frame()
			if c.seat.drag != nil || c.dndScroll != nil || c.touchPoints[17] != nil {
				t.Errorf("lifting the finger left drag=%v dnd=%v point=%v", c.seat.drag != nil, c.dndScroll != nil, c.touchPoints[17] != nil)
			}
		})
	})
}

func TestNativeDragRequestsValidateSerialAndPreserveTouchThroughOverview(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		view := views[0]
		var pointer *fakePointer
		var screen *fakeTouch
		onCompositor(t, c, func() { pointer = c.addFakePointer(); screen = c.addFakeTouch() })
		defer onCompositor(t, c, func() { pointer.remove(); screen.remove() })
		if err := clients[0].BindTouch(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			pointer.moveTo(c.viewCenter(view))
			pointer.button(buttonLeft, true)
			serial := uint32(c.seat.pointer_state.grab_serial)
			if c.requestTestDrag(view, serial+1) {
				t.Error("invalid pointer serial started a native drag")
			}
			if !c.requestTestDrag(view, serial) || c.dndScroll == nil {
				t.Error("valid pointer serial did not start the native drag")
			}
			c.setSessionLocked(true)
			if c.seat.drag != nil || c.dndScroll != nil {
				t.Error("locking retained a native client drag")
			}
			c.setSessionLocked(false)
			pointer.button(buttonLeft, false)
			lx, ly := c.viewCenter(view)
			dx, dy := touchDevicePoint(c, lx, ly)
			screen.down(17, dx, dy)
			screen.frame()
			serial = uint32(c.seat.touch_state.grab_serial)
			if !c.requestTestDrag(view, serial) || c.dndScroll == nil {
				t.Error("valid touch serial did not start the native drag")
				screen.up(17)
				return
			}
			c.OpenOverview()
			if c.touchPoints[17] == nil || c.seat.drag == nil {
				t.Error("opening the overview ended the active touch drag")
				return
			}
			bounds, _ := c.outputLayoutBounds(view.outputName)
			dx, dy = touchDevicePoint(c, float64(bounds.X+bounds.Width/2), float64(bounds.Y+bounds.Height-1))
			screen.motion(17, dx, dy)
			screen.frame()
			if c.dndScroll == nil || c.dndScroll.timer == nil {
				t.Error("native drag finger did not arm workspace edge scrolling")
			}
			screen.up(17)
			screen.frame()
			if c.dndScroll != nil && c.dndScroll.timer != nil {
				t.Error("lifting the drag finger left workspace scrolling active")
			}
		})
	})
}
