package wlr

import (
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
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
		if window.Tile.Height != bounds.h {
			t.Errorf("maximized tile height = %d, want %d", window.Tile.Height, bounds.h)
		}
		if window.Tile.Width != bounds.w {
			t.Errorf("maximized tile width = %d, want %d", window.Tile.Width, bounds.w)
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

// TestWindowRuleIsWindowCastTargetFollowsTheCasts: a window is the cast
// target while a cast shows it, as Niri's refresh_mapped_cast_window_rules
// reads it, and the rules are resolved again when the casts change. The
// cast stands in for a PipeWire one; its stream is not needed for the match.
func TestWindowRuleIsWindowCastTargetFollowsTheCasts(t *testing.T) {
	c := newNested(t)
	target := true
	opacity := 0.5
	c.windowRules = []config.WindowRule{{
		Matches: []config.RuleMatch{{IsWindowCastTarget: &target}},
		Opacity: &opacity,
	}}
	c.viewMapped = func(view *toplevelView) {
		defer c.Terminate()
		if view.appearance.Opacity != nil {
			t.Errorf("an uncast window matched is-window-cast-target=true")
		}
		id := view.id
		c.casts = map[uint64]*screenCast{1: {c: c, state: ipc.Cast{SessionID: 1, StreamID: 1, Target: ipc.CastTarget{WindowID: &id}}}}
		c.castStateChanged()
		if got := view.appearance.Opacity; got == nil || *got != opacity {
			t.Errorf("cast window opacity = %v, want the rule's %v", got, opacity)
		}
		c.casts = nil
		c.castStateChanged()
		if view.appearance.Opacity != nil {
			t.Errorf("the rule still matched after the cast stopped")
		}
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

// TestWindowRuleOpacityComposesWithOpeningAndResizeSnapshots covers opening
// opacity, reload/removal, resize snapshots and hit testing while transparent.
func TestWindowRuleOpacityComposesWithOpeningAndResizeSnapshots(t *testing.T) {
	normalOpacity := 0.65
	rules := []config.WindowRule{{Opacity: &normalOpacity}}
	opts := Options{Animations: laptopAnimations(), WindowRules: rules}
	c := newNestedWith(t, opts)
	var mapped, finished bool

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

	var waitForSettledOpacity func(*toplevelView, float64, string, time.Time, func())
	waitForSettledOpacity = func(view *toplevelView, want float64, stage string, started time.Time, done func()) {
		time.AfterFunc(25*time.Millisecond, func() {
			_ = c.Post(func() {
				if time.Since(started) > 5*time.Second {
					t.Errorf("%s did not settle", stage)
					c.Terminate()
					return
				}
				if view.resizeSnapshot != nil {
					if !assertOpacity(view, 0, stage+" live tree during resize") {
						return
					}
					_, _, snapshotOpacity, found := sceneFXTreeFirstBuffer(view.resizeSnapshot)
					if !found || math.Abs(snapshotOpacity-want) > 0.01 {
						t.Errorf("%s resize snapshot opacity = %v (found %v), want %v", stage, snapshotOpacity, found, want)
						c.Terminate()
						return
					}
					waitForSettledOpacity(view, want, stage, started, done)
					return
				}
				wKey, hKey := c.resizeKeys(view)
				if c.hasAnimation(wKey) || c.hasAnimation(hKey) {
					waitForSettledOpacity(view, want, stage, started, done)
					return
				}
				if !assertOpacity(view, want, stage) {
					return
				}
				done()
			})
		})
	}

	var startResize func(*toplevelView)
	startResize = func(view *toplevelView) {
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
		if !assertOpacity(view, 0, "live tree during resize") {
			return
		}
		if !sceneFXNodeEnabled(view.sceneNode()) {
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
		waitForSettledOpacity(view, normalOpacity, "settled resize", time.Now(), func() {
			finished = true
			c.Terminate()
		})
	}

	var waitForOpen func(*toplevelView)
	waitForOpen = func(view *toplevelView) {
		time.AfterFunc(25*time.Millisecond, func() {
			_ = c.Post(func() {
				if !view.openAnimationFinished {
					if !assertOpacity(view, clampOpacity(view.presentationOpacity)*normalOpacity, "opening animation") {
						return
					}
					waitForOpen(view)
					return
				}
				waitForSettledOpacity(view, normalOpacity, "open window", time.Now(), func() {
					opts.WindowRules = nil
					if err := c.Reconfigure(opts); err != nil {
						t.Errorf("remove opacity rule: %v", err)
						c.Terminate()
						return
					}
					waitForSettledOpacity(view, 1, "opacity rule removal", time.Now(), func() {
						opts.WindowRules = rules
						if err := c.Reconfigure(opts); err != nil {
							t.Errorf("restore opacity rule: %v", err)
							c.Terminate()
							return
						}
						waitForSettledOpacity(view, normalOpacity, "opacity rule restore", time.Now(), func() {
							startResize(view)
						})
					})
				})
			})
		})
	}

	c.viewMapped = func(view *toplevelView) {
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
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !mapped || !finished {
		t.Fatalf("opacity regression did not finish: mapped=%v finished=%v", mapped, finished)
	}
}

// TestWindowRuleOpacityTracksUrgencyAndFullscreen covers state-rule
// reapplication and the fullscreen opaque override.
func TestWindowRuleOpacityTracksUrgencyAndFullscreen(t *testing.T) {
	notUrgent, urgent := false, true
	normalOpacity, urgentOpacity := 0.65, -0.5
	animations := laptopAnimations()
	animations.WindowOpen.Off = true
	animations.WindowResize.Off = true
	c := newNestedWith(t, Options{
		Animations: animations,
		WindowRules: []config.WindowRule{
			{Matches: []config.RuleMatch{{IsUrgent: &notUrgent}}, Opacity: &normalOpacity},
			{Matches: []config.RuleMatch{{IsUrgent: &urgent}}, Opacity: &urgentOpacity},
		},
	})
	var views []*toplevelView
	checked := false

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

	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) < 2 {
			return
		}
		target := views[0]
		if !assertOpacity(target, normalOpacity, "open window") {
			return
		}
		if !c.SetWindowUrgent(target.id, true) {
			t.Error("SetWindowUrgent(true) failed")
			c.Terminate()
			return
		}
		if !assertOpacity(target, 0, "transparent urgent rule") {
			c.Terminate()
			return
		}
		x, y := c.viewCenter(target)
		surface, _, _ := c.surfaceAt(x, y)
		if c.viewForSurface(surface) != target || !sceneFXNodeEnabled(target.sceneNode()) {
			t.Error("transparent window stopped receiving hit tests or was disabled")
			c.Terminate()
			return
		}
		if !c.SetWindowUrgent(target.id, false) {
			t.Error("SetWindowUrgent(false) failed")
			c.Terminate()
			return
		}
		if !assertOpacity(target, normalOpacity, "urgent rule removal") {
			return
		}
		if !c.SetFullscreen(target.id, true) {
			t.Error("SetFullscreen(true) failed")
			c.Terminate()
			return
		}
		if !assertOpacity(target, 1, "fullscreen") {
			return
		}
		if !c.SetFullscreen(target.id, false) {
			t.Error("SetFullscreen(false) failed")
			c.Terminate()
			return
		}
		if !assertOpacity(target, normalOpacity, "fullscreen exit") {
			return
		}
		checked = true
		c.Terminate()
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 2 || !checked {
		t.Fatalf("opacity state regression did not finish: mapped=%d checked=%v", len(views), checked)
	}
}

// TestWindowRuleOpacityChangesRenderedPixels checks opacity in the rendered
// output, not only the scene buffer metadata.
func TestWindowRuleOpacityChangesRenderedPixels(t *testing.T) {
	notUrgent, urgent := false, true
	normalOpacity, urgentOpacity := 0.5, 0.0
	animations := config.Animations{Off: true}
	rules := []config.WindowRule{
		{Matches: []config.RuleMatch{{IsUrgent: &notUrgent}}, Opacity: &normalOpacity},
		{Matches: []config.RuleMatch{{IsUrgent: &urgent}}, Opacity: &urgentOpacity},
	}
	c := newNestedWith(t, Options{Animations: animations, WindowRules: rules})
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		var output unsafe.Pointer
		var outputBox image.Rectangle
		var backgroundNode unsafe.Pointer
		onCompositor(t, c, func() {
			for output = range c.sceneOutputs {
				break
			}
			if output == nil {
				t.Error("no output for rendered opacity test")
				return
			}
			outputBox = c.outputLayoutBox(output)
			backgroundNode = sceneFXCreateRect(unsafe.Pointer(c.layerTrees[layerBackground]),
				outputBox.Dx(), outputBox.Dy(), [4]float32{0.2, 0.4, 0.6, 1})
			if backgroundNode == nil {
				t.Error("create background for rendered opacity test")
				return
			}
			sceneFXNodeSetPosition(backgroundNode, outputBox.Min.X, outputBox.Min.Y)
		})
		if output == nil || backgroundNode == nil {
			return
		}
		defer onCompositor(t, c, func() {
			if backgroundNode != nil {
				sceneFXNodeDestroy(backgroundNode)
			}
		})

		var background color.NRGBA
		onCompositor(t, c, func() {
			frame, err := c.captureOutput(output)
			if err != nil {
				t.Error(err)
				return
			}
			background = frame.NRGBAAt(1, 1)
		})
		if want := (color.NRGBA{R: 51, G: 102, B: 153, A: 255}); background != want {
			t.Errorf("test background pixel = %v, want %v", background, want)
			return
		}

		targetClient, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer targetClient.Close()
		if err := targetClient.SetIdentity("opacity target", "opacity-target"); err != nil {
			t.Error(err)
			return
		}
		const sourceARGB = uint32(0xff3498db)
		if err := targetClient.Paint(sourceARGB, 200, 150); err != nil {
			t.Error(err)
			return
		}
		focusClient, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer focusClient.Close()
		if err := focusClient.SetIdentity("opacity focus", "opacity-focus"); err != nil {
			t.Error(err)
			return
		}
		if err := focusClient.Paint(sourceARGB, 200, 150); err != nil {
			t.Error(err)
			return
		}

		var view *toplevelView
		onCompositor(t, c, func() {
			for _, candidate := range c.views {
				if cString(candidate.toplevel.app_id) == "opacity-target" {
					view = candidate
					break
				}
			}
			if view == nil {
				t.Error("painted window did not map")
			}
		})
		if view == nil {
			return
		}

		readWindowPixel := func() color.NRGBA {
			var pixel color.NRGBA
			onCompositor(t, c, func() {
				frame, err := c.captureOutput(output)
				if err != nil {
					t.Error(err)
					return
				}
				x, y := c.viewCenter(view)
				px := int((x - float64(outputBox.Min.X)) * float64(frame.Bounds().Dx()) / float64(outputBox.Dx()))
				py := int((y - float64(outputBox.Min.Y)) * float64(frame.Bounds().Dy()) / float64(outputBox.Dy()))
				if !image.Pt(px, py).In(frame.Bounds()) {
					t.Errorf("window center pixel (%d,%d) outside rendered output %v", px, py, frame.Bounds())
					return
				}
				pixel = frame.NRGBAAt(px, py)
			})
			return pixel
		}
		assertPixel := func(stage string, got, want color.NRGBA) {
			t.Helper()
			const tolerance = 3
			if math.Abs(float64(int(got.R)-int(want.R))) > tolerance ||
				math.Abs(float64(int(got.G)-int(want.G))) > tolerance ||
				math.Abs(float64(int(got.B)-int(want.B))) > tolerance || got.A != 255 {
				t.Errorf("%s rendered pixel = %v, want %v (±%d per color channel)", stage, got, want, tolerance)
			}
		}
		blend := func(foreground, under color.NRGBA, opacity float64) color.NRGBA {
			return color.NRGBA{
				R: uint8(math.Round(float64(foreground.R)*opacity + float64(under.R)*(1-opacity))),
				G: uint8(math.Round(float64(foreground.G)*opacity + float64(under.G)*(1-opacity))),
				B: uint8(math.Round(float64(foreground.B)*opacity + float64(under.B)*(1-opacity))),
				A: 255,
			}
		}
		foreground := color.NRGBA{R: 0x34, G: 0x98, B: 0xdb, A: 255}
		assertPixel("rule opacity", readWindowPixel(), blend(foreground, background, normalOpacity))

		onCompositor(t, c, func() {
			if !c.SetWindowUrgent(view.id, true) {
				t.Error("SetWindowUrgent(true) failed")
			}
		})
		assertPixel("transparent urgent rule", readWindowPixel(), background)

		onCompositor(t, c, func() {
			if !c.SetWindowUrgent(view.id, false) {
				t.Error("SetWindowUrgent(false) failed")
			}
		})
		assertPixel("urgent rule removal", readWindowPixel(), blend(foreground, background, normalOpacity))

		onCompositor(t, c, func() {
			if !c.SetFullscreen(view.id, true) {
				t.Error("SetFullscreen(true) failed")
			}
		})
		assertPixel("fullscreen opacity override", readWindowPixel(), foreground)

		onCompositor(t, c, func() {
			if !c.SetFullscreen(view.id, false) {
				t.Error("SetFullscreen(false) failed")
			}
		})
		assertPixel("fullscreen exit opacity", readWindowPixel(), blend(foreground, background, normalOpacity))

		opts := Options{Animations: animations}
		onCompositor(t, c, func() {
			if err := c.Reconfigure(opts); err != nil {
				t.Errorf("remove opacity rule: %v", err)
			}
		})
		assertPixel("opacity rule removal", readWindowPixel(), foreground)
	})
}

// TestWindowRuleScrollFactorScalesTheWindowUnderThePointer: a wheel detent
// over a window with scroll-factor 2 reaches it doubled, while the keyboard
// focus is on another window, as Niri's on_pointer_axis takes the factor of
// the window under the pointer.
func TestWindowRuleScrollFactorScalesTheWindowUnderThePointer(t *testing.T) {
	two := 2.0
	rules := []config.WindowRule{{
		Matches:      []config.RuleMatch{{AppID: regexp.MustCompile(`^fast$`)}},
		ScrollFactor: &two,
	}}
	c := newNestedWith(t, Options{WindowRules: rules})
	runTouchClients(t, c, 2, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			if pointer != nil {
				pointer.remove()
			}
		})
		if err := clients[0].SetIdentity("fast", "fast"); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() { pointer = c.addFakePointer() })
		for _, client := range clients {
			if err := client.SyncInput(); err != nil {
				t.Fatal(err)
			}
		}
		onCompositor(t, c, func() {
			if views[0].appearance.ScrollFactor == nil {
				t.Fatal("the scroll-factor rule did not match the window")
			}
			c.focusView(views[1])
			c.applyPendingWindowFocus()
			pointer.moveTo(c.viewCenter(views[0]))
			pointer.wheelAt(1000, false, 120)
			pointer.moveTo(c.viewCenter(views[1]))
			pointer.wheelAt(1100, false, 120)
		})
		for i, want := range []float64{30, 15} {
			if err := clients[i].SyncInput(); err != nil {
				t.Fatal(err)
			}
			if got := clients[i].InputEvents().PointerAxisSum; got != want {
				t.Errorf("client %d axis = %v, want %v", i, got, want)
			}
		}
	})
}
