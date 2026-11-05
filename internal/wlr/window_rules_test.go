package wlr

import (
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

// TestWindowRuleOpenMaximizedFillsTheWorkingWidth: a window that opens
// maximized takes the whole working width, the way Niri's open-maximized does.
func TestWindowRuleOpenMaximizedFillsTheWorkingWidth(t *testing.T) {
	c := newNested(t)
	openMaximized := true
	c.windowRules = []config.WindowRule{{OpenMaximized: &openMaximized}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		want := bounds.w - 2*16
		if window.Tile.Width != want {
			t.Errorf("maximized tile width = %d, want %d", window.Tile.Width, want)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleOpenMaximizedToEdgesFillsTheColumn: a window that opens
// maximized to the edges fills the height of its column.
func TestWindowRuleOpenMaximizedToEdgesFillsTheColumn(t *testing.T) {
	c := newNested(t)
	openMaximizedToEdges := true
	c.windowRules = []config.WindowRule{{OpenMaximizedToEdges: &openMaximizedToEdges}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		if window.Tile.Height != bounds.h-2*16 {
			t.Errorf("maximized tile height = %d, want %d", window.Tile.Height, bounds.h-2*16)
		}
		if window.Tile.Width != bounds.w-2*16 {
			t.Errorf("maximized tile width = %d, want %d", window.Tile.Width, bounds.w-2*16)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleOpenFocusedFalseKeepsTheFocus: a window that opens without
// taking the focus leaves the focus where it was.
func TestWindowRuleOpenFocusedFalseKeepsTheFocus(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var first, second *toplevelView
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			// The window that maps next opens without taking the focus.
			openFocused := false
			c.windowRules = []config.WindowRule{{OpenFocused: &openFocused}}
			return
		}
		second = view
		if focused := c.keyboardFocusedView(); focused != first {
			if focused == nil {
				t.Error("the keyboard focus was lost when the second window mapped")
			} else {
				t.Errorf("keyboard focus after the second window mapped = window %d, want window %d",
					focused.id, first.id)
			}
		}
		c.Terminate()
	}

	clients := make([]*exec.Cmd, 0, 2)
	defer func() {
		for _, client := range clients {
			_ = client.Process.Kill()
		}
		for _, client := range clients {
			_ = client.Wait()
		}
	}()
	for range 2 {
		client := exec.Command(terminal)
		client.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
		if err := client.Start(); err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if first == nil || second == nil {
		t.Fatal("both windows did not map")
	}
}

// TestWindowRuleIsActiveInColumnMatchesTheWindowThatOpens: a window that
// opens is the active window of its column, the way Niri reads it.
func TestWindowRuleIsActiveInColumnMatchesTheWindowThatOpens(t *testing.T) {
	c := newNested(t)
	active := true
	openMaximized := true
	c.windowRules = []config.WindowRule{{
		Matches:       []config.RuleMatch{{IsActiveInColumn: &active}},
		OpenMaximized: &openMaximized,
	}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		want := bounds.w - 2*16
		if window.Tile.Width != want {
			t.Errorf("tile width = %d, want the maximized width %d", window.Tile.Width, want)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleIsFocusedDoesNotMatchAWindowThatOpens: a window that opens
// is not focused yet, so a rule that asks for the focused window does not
// apply to it.
func TestWindowRuleIsFocusedDoesNotMatchAWindowThatOpens(t *testing.T) {
	c := newNested(t)
	focused := true
	openMaximized := true
	c.windowRules = []config.WindowRule{{
		Matches:       []config.RuleMatch{{IsFocused: &focused}},
		OpenMaximized: &openMaximized,
	}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		if maximized := bounds.w - 2*16; window.Tile.Width == maximized {
			t.Errorf("tile width = %d, the focused window rule applied to the window that opened", window.Tile.Width)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleIsUrgentAppliesWhenTheWindowBecomesUrgent: a rule that asks
// for an urgent window does not apply to a window that opens, and applies
// once the window asks for attention.
func TestWindowRuleIsUrgentAppliesWhenTheWindowBecomesUrgent(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	urgent := true
	openMaximized := true
	c.windowRules = []config.WindowRule{{
		Matches:       []config.RuleMatch{{IsUrgent: &urgent}},
		OpenMaximized: &openMaximized,
	}}
	var first *toplevelView
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			return
		}
		if first.appearance.OpenMaximized != nil {
			t.Errorf("the urgent window rule applied to a window that opened: %+v", first.appearance.OpenMaximized)
		}
		// The first window lost the focus when the second one mapped, so it
		// can ask for attention.
		if !c.SetWindowUrgent(first.id, true) {
			t.Errorf("the first window could not become urgent")
		}
		if first.appearance.OpenMaximized == nil {
			t.Errorf("the urgent window rule did not apply after the window became urgent")
		}
		c.Terminate()
	}

	clients := make([]*exec.Cmd, 0, 2)
	defer func() {
		for _, client := range clients {
			_ = client.Process.Kill()
		}
		for _, client := range clients {
			_ = client.Wait()
		}
	}()
	for range 2 {
		client := exec.Command(terminal)
		client.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
		if err := client.Start(); err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if first == nil {
		t.Fatal("the windows did not map")
	}
}

// TestWindowRuleAtStartupMatchesInsideTheStartupWindow: a rule that asks
// for the startup window applies while the compositor starts, and stops
// applying when the startup window ends.
func TestWindowRuleAtStartupMatchesInsideTheStartupWindow(t *testing.T) {
	c := newNested(t)
	atStartup := true
	openMaximized := true
	c.windowRules = []config.WindowRule{{
		Matches:       []config.RuleMatch{{AtStartup: &atStartup}},
		OpenMaximized: &openMaximized,
	}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		want := bounds.w - 2*16
		if window.Tile.Width != want {
			t.Errorf("tile width = %d, want the maximized width %d", window.Tile.Width, want)
		}
		// The startup window ends: the rule stops matching.
		c.startupElapsed()
		if view.appearance.OpenMaximized != nil {
			t.Errorf("the at-startup rule still matches after the startup window: %+v", view.appearance.OpenMaximized)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleStartupTimerEndsTheStartupWindow: the clock that ends the
// startup window runs on its own.
func TestWindowRuleStartupTimerEndsTheStartupWindow(t *testing.T) {
	old := startupWindowMS
	startupWindowMS = 50
	defer func() { startupWindowMS = old }()
	c := newNested(t)
	runClientWithTimeout(t, c, []string{"weston-terminal"}, nil, nil, time.Second)
	if c.atStartup {
		t.Error("the startup window did not end")
	}
}

// TestWindowRuleIsActiveAppliesToTheActiveWindow: Niri's is-active is the
// window that has the active border. A window that opens is not active yet,
// so the rule does not change how it opens; it applies once the window is the
// active window of its workspace.
func TestWindowRuleIsActiveAppliesToTheActiveWindow(t *testing.T) {
	c := newNested(t)
	active := true
	openMaximized := true
	c.windowRules = []config.WindowRule{{
		Matches:       []config.RuleMatch{{IsActive: &active}},
		OpenMaximized: &openMaximized,
	}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		if maximized := bounds.w - 2*16; window.Tile.Width == maximized {
			t.Errorf("the active window rule changed how the window opened")
		}
		if view.appearance.OpenMaximized == nil {
			t.Errorf("the active window rule did not apply to the active window")
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleIsWindowCastTargetIsNeverTrue: the compositor has no window
// screencast protocol, so no window is the target of one.
func TestWindowRuleIsWindowCastTargetIsNeverTrue(t *testing.T) {
	c := newNested(t)
	notTarget := false
	openMaximized := true
	c.windowRules = []config.WindowRule{{
		Matches:       []config.RuleMatch{{IsWindowCastTarget: &notTarget}},
		OpenMaximized: &openMaximized,
	}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		want := bounds.w - 2*16
		if window.Tile.Width != want {
			t.Errorf("tile width = %d, want the maximized width %d", window.Tile.Width, want)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleMinWidthAmendsTheSizeHints: the window rule's min-width
// raises the minimum the window gets, the way Niri's min-width does.
func TestWindowRuleMinWidthAmendsTheSizeHints(t *testing.T) {
	c := newNested(t)
	minWidth := c.layoutBox().w - 100
	c.windowRules = []config.WindowRule{{MinWidth: &minWidth}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		if window.Tile.Width != minWidth {
			t.Errorf("tile width = %d, want the rule's min-width %d", window.Tile.Width, minWidth)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleOpacityComposesWithAnimationsFullscreenAndResize covers the
// mapped buffer, opacity-sensitive state changes, fullscreen, reload removal,
// and the snapshots that temporarily replace the live tree.
func TestWindowRuleOpacityComposesWithAnimationsFullscreenAndResize(t *testing.T) {
	notUrgent, urgent := false, true
	normalOpacity, urgentOpacity := 0.65, -0.5
	animations := laptopAnimations()
	opts := Options{
		Animations: animations,
		WindowRules: []config.WindowRule{
			{Matches: []config.RuleMatch{{IsUrgent: &notUrgent}}, Opacity: &normalOpacity},
			{Matches: []config.RuleMatch{{IsUrgent: &urgent}}, Opacity: &urgentOpacity},
		},
	}
	c := newNestedWith(t, opts)
	var mapped, otherMapped, fullscreenExitPending, finished bool

	assertOpacity := func(view *toplevelView, want float64, stage string) bool {
		_, _, got, found := sceneFXTreeFirstBuffer(view.sceneNode())
		if !found {
			t.Errorf("%s: no mapped buffer in the window tree", stage)
			c.Terminate()
			return false
		}
		if math.Abs(got-want) > 0.01 {
			t.Errorf("%s: buffer opacity = %v, want %v", stage, got, want)
			c.Terminate()
			return false
		}
		return true
	}

	var waitForOpen func(*toplevelView)
	waitForOpen = func(view *toplevelView) {
		time.AfterFunc(25*time.Millisecond, func() {
			_ = c.Post(func() {
				if !view.openAnimationFinished {
					assertOpacity(view, clampOpacity(view.presentationOpacity)*normalOpacity, "opening animation")
					waitForOpen(view)
					return
				}
				if view.resizeSnapshot != nil {
					stage := "initial resize"
					if fullscreenExitPending {
						stage = "fullscreen exit resize"
					}
					if !assertOpacity(view, 0, "live tree during "+stage) {
						return
					}
					_, _, snapshotOpacity, found := sceneFXTreeFirstBuffer(view.resizeSnapshot)
					if !found || math.Abs(snapshotOpacity-normalOpacity) > 0.01 {
						t.Errorf("%s snapshot opacity = %v (found %v), want %v", stage, snapshotOpacity, found, normalOpacity)
						c.Terminate()
						return
					}
					waitForOpen(view)
					return
				}
				if !otherMapped {
					waitForOpen(view)
					return
				}
				if fullscreenExitPending {
					if !assertOpacity(view, normalOpacity, "fullscreen exit") {
						return
					}
					fullscreenExitPending = false
				} else {
					if !assertOpacity(view, normalOpacity, "open window") {
						return
					}
					if !c.SetWindowUrgent(view.id, true) {
						t.Error("SetWindowUrgent(true) failed")
						c.Terminate()
						return
					}
					if !assertOpacity(view, 0, "transparent urgent rule") {
						return
					}
					x, y := c.viewCenter(view)
					surface, _, _ := c.surfaceAt(x, y)
					if c.viewForSurface(surface) != view || !sceneFXNodeEnabled(view.sceneNode()) {
						t.Error("transparent window stopped receiving hit tests or was disabled")
						c.Terminate()
						return
					}
					if !c.SetWindowUrgent(view.id, false) || !assertOpacity(view, normalOpacity, "urgent rule removal") {
						if !finished {
							c.Terminate()
						}
						return
					}
					if !c.SetFullscreen(view.id, true) || !assertOpacity(view, 1, "fullscreen") {
						if !finished {
							c.Terminate()
						}
						return
					}
					if !c.SetFullscreen(view.id, false) {
						t.Error("SetFullscreen(false) failed")
						c.Terminate()
						return
					}
					fullscreenExitPending = true
					waitForOpen(view)
					return
				}

				geometry := view.geometry()
				if !c.model.SetWindowFloating(view.id, true, layout.Bounds{X: 100, Y: 100, Width: geometry.w, Height: geometry.h}) {
					t.Error("SetWindowFloating() failed")
					c.Terminate()
					return
				}
				c.applyWorkspaceModel()
				target := layout.Bounds{X: 100, Y: 100, Width: geometry.w + 120, Height: geometry.h + 80}
				if !c.model.SetFloatingGeometry(view.id, target) {
					t.Error("SetFloatingGeometry() failed")
					c.Terminate()
					return
				}
				c.applyWorkspaceModel()
				if view.resizeSnapshot == nil {
					t.Error("resize did not create a snapshot")
					c.Terminate()
					return
				}
				_, _, snapshotOpacity, snapshotFound := sceneFXTreeFirstBuffer(view.resizeSnapshot)
				if !snapshotFound || math.Abs(snapshotOpacity-normalOpacity) > 0.01 {
					t.Errorf("resize snapshot opacity = %v (found %v), want %v", snapshotOpacity, snapshotFound, normalOpacity)
					c.Terminate()
					return
				}
				if !assertOpacity(view, 0, "live tree during resize") || !sceneFXNodeEnabled(view.sceneNode()) {
					t.Error("resize hid or disabled the live window tree")
					c.Terminate()
					return
				}
				x, y := c.viewCenter(view)
				surface, _, _ := c.surfaceAt(x, y)
				if c.viewForSurface(surface) != view {
					t.Error("resizing window stopped receiving hit tests")
					c.Terminate()
					return
				}

				resizeStarted := time.Now()
				var waitForResize func()
				waitForResize = func() {
					time.AfterFunc(50*time.Millisecond, func() {
						_ = c.Post(func() {
							if view.resizeSnapshot == nil {
								if !assertOpacity(view, normalOpacity, "settled resize") {
									return
								}
								opts.WindowRules = nil
								if err := c.Reconfigure(opts); err != nil {
									t.Errorf("remove opacity rule: %v", err)
									c.Terminate()
									return
								}
								if !assertOpacity(view, 1, "opacity rule removal") {
									return
								}
								finished = true
								c.Terminate()
								return
							}
							if time.Since(resizeStarted) > 5*time.Second {
								t.Error("resize snapshot did not settle")
								c.Terminate()
								return
							}
							waitForResize()
						})
					})
				}
				waitForResize()
			})
		})
	}

	c.viewMapped = func(view *toplevelView) {
		if mapped {
			otherMapped = true
			return
		}
		mapped = true
		if !view.openAnimationStarted || view.openAnimationFinished {
			t.Error("new window did not start its opening opacity animation")
			c.Terminate()
			return
		}
		if !assertOpacity(view, clampOpacity(view.presentationOpacity)*normalOpacity, "opening window") {
			return
		}
		waitForOpen(view)
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if !mapped || !otherMapped || !finished {
		t.Fatalf("opacity regression did not finish: mapped=%v otherMapped=%v finished=%v", mapped, otherMapped, finished)
	}
}
