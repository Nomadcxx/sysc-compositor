package wlr

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// tapAt lands a finger at a layout point and lifts it.
func tapAt(c *Compositor, screen *fakeTouch, lx, ly float64) {
	x, y := touchDevicePoint(c, lx, ly)
	screen.down(1, x, y)
	screen.frame()
	screen.up(1)
	screen.frame()
}

// expectNoNativeTouch checks that the compositor's UI kept the finger.
func expectNoNativeTouch(t *testing.T, client *testclient.ToplevelClient, label string) {
	t.Helper()
	if err := client.SyncInput(); err != nil {
		t.Fatal(err)
	}
	if ev := client.TouchEvents(); ev.Downs != 0 {
		t.Errorf("%s: the client got touch events %+v, want none", label, ev)
	}
}

// TestTouchTapInTheOverviewSelectsTheWindow: a tap in the overview selects
// the window under it and closes the overview, as a click does and Niri's
// touch overview grab does.
func TestTouchTapInTheOverviewSelectsTheWindow(t *testing.T) {
	c := newNestedWith(t, Options{Overview: config.Overview{Zoom: 0.5}})
	runTouchClients(t, c, 2, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		var screen *fakeTouch
		defer onCompositor(t, c, func() {
			if screen != nil {
				screen.remove()
			}
		})
		onCompositor(t, c, func() { screen = c.addFakeTouch() })
		if err := clients[0].BindTouch(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			c.focusView(views[1])
			if !c.OpenOverview() {
				t.Fatal("could not open overview")
			}
			frame, ok := c.model.OverviewFrame(c.WorkspaceSnapshot().FocusedOutput, 0.5)
			if !ok || len(frame.Workspaces) == 0 {
				t.Fatal("no overview frame")
			}
			for _, window := range frame.Workspaces[0].Windows {
				if window.ID == views[0].id {
					b := window.Bounds
					tapAt(c, screen, float64(b.X+b.Width/2), float64(b.Y+b.Height/2))
				}
			}
			if c.WorkspaceSnapshot().OverviewOpen {
				t.Error("a tap on a window left the overview open")
			}
			if c.focusedView != views[0] {
				t.Error("a tap in the overview did not select the window under it")
			}
		})
		expectNoNativeTouch(t, clients[0], "overview")
	})
}

// TestTouchTapClosesTheExitDialog: a finger closes the exit dialog, as any
// press does, without reaching the window under it.
func TestTouchTapClosesTheExitDialog(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		var screen *fakeTouch
		defer onCompositor(t, c, func() {
			if screen != nil {
				screen.remove()
			}
		})
		onCompositor(t, c, func() { screen = c.addFakeTouch() })
		if err := clients[0].BindTouch(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			if !c.showExitDialog() {
				t.Fatal("the exit dialog did not open")
			}
			lx, ly := c.viewCenter(views[0])
			tapAt(c, screen, lx, ly)
			if c.exitDialog.open {
				t.Error("a tap left the exit dialog open")
			}
		})
		expectNoNativeTouch(t, clients[0], "exit dialog")
	})
}

// TestTouchDragsAScreenshotSelection: a finger drags the screenshot UI's
// selection, as Niri's pointer_down and pointer_motion with a touch slot do.
func TestTouchDragsAScreenshotSelection(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		var screen *fakeTouch
		defer onCompositor(t, c, func() {
			if screen != nil {
				screen.remove()
			}
			if c.shotUI != nil {
				c.closeScreenshotUI()
			}
		})
		onCompositor(t, c, func() {
			screen = c.addFakeTouch()
			if err := c.Screenshot(false, filepath.Join(t.TempDir(), "touch.png")); err != nil {
				t.Fatal(err)
			}
			if !c.ScreenshotUIOpen() || len(c.shotUI.frames) == 0 {
				t.Fatal("the screenshot UI did not open")
			}
			var box image.Rectangle
			for output, frame := range c.shotUI.frames {
				box = frame.box
				c.shotUI.output = output
			}
			want := image.Rect(box.Min.X+20, box.Min.Y+30, box.Min.X+120, box.Min.Y+90)
			x0, y0 := touchDevicePoint(c, float64(want.Min.X), float64(want.Min.Y))
			x1, y1 := touchDevicePoint(c, float64(want.Max.X), float64(want.Max.Y))
			screen.down(1, x0, y0)
			screen.frame()
			screen.motion(1, x1, y1)
			screen.frame()
			screen.up(1)
			screen.frame()
			if c.shotUI == nil || c.shotUI.selection != want {
				t.Errorf("selection = %v, want the dragged %v", c.shotUI.selection, want)
			}
		})
	})
}

// TestTouchTapInTheSwitcherConfirmsTheWindow: a tap on a preview confirms that
// window, as Niri's MRU UI does on touch down.
func TestTouchTapInTheSwitcherConfirmsTheWindow(t *testing.T) {
	recent := config.Default().RecentWindows
	recent.OpenDelayMS = 0
	c := newNestedWith(t, Options{RecentWindows: &recent})
	runTouchClients(t, c, 3, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		var screen *fakeTouch
		var keyboard *fakeKeyboard
		defer onCompositor(t, c, func() {
			if screen != nil {
				screen.remove()
			}
			if keyboard != nil {
				keyboard.remove()
			}
		})
		onCompositor(t, c, func() {
			for _, view := range views {
				c.focusView(view)
				c.applyPendingWindowFocus()
			}
			screen = c.addFakeTouch()
			keyboard = c.addFakeKeyboard()
			keyboard.key(keyLeftAlt, true)
			keyboard.key(keyTab, true)
			keyboard.key(keyTab, false)
			if _, open := c.SwitcherOpen(); !open || len(c.switcher.hits) != 3 {
				t.Fatalf("the switcher is not open with three previews")
			}
			var target switcherHit
			for _, hit := range c.switcher.hits {
				if hit.id == views[0].id {
					target = hit
				}
			}
			box := c.outputLayoutBox(c.switcher.output)
			tapAt(c, screen, float64(box.Min.X)+target.box.X+target.box.W/2,
				float64(box.Min.Y)+target.box.Y+target.box.H/2)
			if _, open := c.SwitcherOpen(); open {
				t.Error("a tap on a preview left the switcher open")
			}
			keyboard.key(keyLeftAlt, false)
			if c.focusedView != views[0] {
				t.Error("a tap on a preview did not confirm its window")
			}
		})
	})
}
