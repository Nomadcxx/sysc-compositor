package wlr

import (
	"os"
	"os/exec"
	"slices"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestDefaultColumnWidthUsesNiriGapSizing(t *testing.T) {
	c := &Compositor{layoutOptions: LayoutOptions{Gaps: 16, ColumnWidthProportion: 0.5}}
	if got, want := c.defaultColumnWidth(1000), 476; got != want {
		t.Fatalf("defaultColumnWidth(1000) = %d, want %d", got, want)
	}
	c.layoutOptions = LayoutOptions{FixedColumnWidth: 1200}
	if got, want := c.defaultColumnWidth(1000), 1200; got != want {
		t.Fatalf("fixed defaultColumnWidth(1000) = %d, want oversized fixed width %d", got, want)
	}
}

func TestUrgencyActionsUpdateModelAndNotifyIPC(t *testing.T) {
	model := wm.New()
	model.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	model.MapWindow(1, "DP-1")
	model.MapWindow(2, "DP-1")
	changed := 0
	c := &Compositor{model: model, stateChanged: func() { changed++ }}
	if !c.SetWindowUrgent(1, true) {
		t.Fatal("SetWindowUrgent() = false, want urgent transition")
	}
	if !model.Snapshot().Windows[0].IsUrgent || changed != 1 {
		t.Fatalf("after set urgent: snapshot=%+v notifications=%d", model.Snapshot().Windows, changed)
	}
	if !c.ToggleWindowUrgent(1) {
		t.Fatal("ToggleWindowUrgent() = false, want urgency cleared")
	}
	if model.Snapshot().Windows[0].IsUrgent || changed != 2 {
		t.Fatalf("after toggle urgent: snapshot=%+v notifications=%d", model.Snapshot().Windows, changed)
	}
}

func TestActivationWithoutInputSerialMarksUnfocusedWindowUrgent(t *testing.T) {
	model := wm.New()
	model.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	model.MapWindow(1, "DP-1")
	model.MapWindow(2, "DP-1")
	c := &Compositor{model: model}
	view := &toplevelView{id: 1, mapped: true}
	if !c.applyActivationRequest(view, false) {
		t.Fatal("no-serial activation request = false, want urgency set")
	}
	if !model.Snapshot().Windows[0].IsUrgent {
		t.Fatal("no-serial activation request did not mark its target urgent")
	}
	if c.applyActivationRequest(&toplevelView{id: 2, mapped: true}, false) {
		t.Fatal("focused target became urgent")
	}
	if c.applyActivationRequest(view, true) {
		t.Fatal("serial-backed activation request entered the M6 urgency path")
	}
	if c.applyActivationRequest(&toplevelView{id: 1}, false) {
		t.Fatal("unmapped target became urgent")
	}
}

func TestApplyingEmptyWorkspaceActivationNotifiesState(t *testing.T) {
	model := wm.New()
	model.AddOutput("test", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	model.MapWindow(1, "test")
	if !model.FocusWorkspace("test", 2) {
		t.Fatal("could not activate the trailing empty workspace")
	}

	var got wm.Snapshot
	c := &Compositor{model: model, stateChanged: func() { got = model.Snapshot() }}
	c.applyWorkspaceModel()
	if len(got.Workspaces) != 2 || !got.Workspaces[1].IsActive || !got.Workspaces[1].IsFocused {
		t.Fatalf("state callback snapshot = %+v, want empty workspace 2 active and focused", got.Workspaces)
	}
}

func TestConstrainedToplevelSizeUpdatesTileAndWindowOffset(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		view.toplevel.current.min_width = 700
		view.toplevel.current.max_width = 700
		view.toplevel.current.min_height = 300
		view.toplevel.current.max_height = 400
		c.applyWorkspaceModel()

		tile := c.WorkspaceSnapshot().Windows[0].Tile
		if tile.Width != 700 || tile.Height != 400 {
			t.Errorf("constrained tile = %dx%d, want 700x400", tile.Width, tile.Height)
		}
		if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{700, 400} {
			t.Errorf("scheduled client size = %v, want [700 400]", got)
		}
		geometry := view.geometry()
		wantX := tile.X + (tile.Width-geometry.w)/2
		wantY := tile.Y + (tile.Height-geometry.h)/2
		if x, y := view.treePosition(); x != wantX || y != wantY {
			t.Errorf("constrained window position = (%d,%d), want centered (%d,%d)", x, y, wantX, wantY)
		}
		view.toplevel.current.max_width = 720
		c.applyWorkspaceModel()
		if !c.model.ResizeWindow(view.id, tile.X, tile.Y, 900, 200, layout.ResizeOptions{
			ResizeWidth: true, ResizeHeight: true,
		}) {
			t.Error("could not resize the constrained tile")
		} else {
			c.applyWorkspaceModel()
			tile = c.WorkspaceSnapshot().Windows[0].Tile
			if tile.Width != 720 || tile.Height != 300 {
				t.Errorf("resized constrained tile = %dx%d, want 720x300", tile.Width, tile.Height)
			}
			if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{720, 300} {
				t.Errorf("resized scheduled client size = %v, want [720 300]", got)
			}
		}
		view.toplevel.current.min_width = 750
		view.toplevel.current.max_width = 650
		view.toplevel.current.min_height = 350
		view.toplevel.current.max_height = 250
		c.applyWorkspaceModel()
		tile = c.WorkspaceSnapshot().Windows[0].Tile
		if tile.Width != 750 || tile.Height != 350 {
			t.Errorf("tile with inconsistent client bounds = %dx%d, want min bounds 750x350", tile.Width, tile.Height)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestFloatingWindowUsesItsStoredSceneRectangle(t *testing.T) {
	c := newNested(t)
	want := layout.Bounds{X: 123, Y: 87, Width: 420, Height: 280}
	c.viewMapped = func(view *toplevelView) {
		if !c.model.SetWindowFloating(view.id, true, want) {
			t.Error("SetWindowFloating() = false, want window to float")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		if x, y := view.treePosition(); x != want.X || y != want.Y {
			t.Errorf("floating tree position = (%d,%d), want (%d,%d)", x, y, want.X, want.Y)
		}
		if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{want.Width, want.Height} {
			t.Errorf("floating client size = %v, want [%d %d]", got, want.Width, want.Height)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowRulePlacesNewWindowOnNamedWorkspaceAndFloating(t *testing.T) {
	cfg, err := config.Parse("rules.kdl", []byte(`
workspace "Work";
window-rule {
    match app-id="."
    open-on-workspace "Work"
    open-floating true
}
`))
	if err != nil {
		t.Fatal(err)
	}
	workspaces := make([]wm.WorkspaceConfig, len(cfg.Workspaces))
	for i, workspace := range cfg.Workspaces {
		workspaces[i] = wm.WorkspaceConfig{Name: workspace.Name, OpenOnOutput: workspace.OpenOnOutput}
	}
	c := newNestedWith(t, Options{Workspaces: workspaces, WindowRules: cfg.WindowRules})
	c.viewMapped = func(view *toplevelView) {
		snapshot := c.WorkspaceSnapshot()
		for _, window := range snapshot.Windows {
			if window.ID != view.id {
				continue
			}
			if !window.Tile.Floating {
				t.Error("open-floating window mapped as tiled")
			}
			for _, workspace := range snapshot.Workspaces {
				if workspace.ID == window.WorkspaceID && workspace.Name != nil && *workspace.Name == "Work" {
					c.Terminate()
					return
				}
			}
			t.Errorf("window %+v did not map to the named Work workspace", window)
			c.Terminate()
			return
		}
		t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowRulesApplyDefaultSizesAndFullscreen(t *testing.T) {
	cfg, err := config.Parse("sizes-fullscreen.kdl", []byte(`
window-rule {
    match app-id="."
    default-column-width { fixed 420; }
    default-window-height { fixed 260; }
    open-fullscreen true
}
`))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{WindowRules: cfg.WindowRules})
	c.viewMapped = func(view *toplevelView) {
		var window wm.WindowSnapshot
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id {
				window = candidate
				break
			}
		}
		if !window.Fullscreen {
			t.Error("open-fullscreen rule did not set the model fullscreen state")
		}
		if window.Tile.Width != 420 || window.Tile.Height != 260 {
			t.Errorf("retained tile after rules = %dx%d, want 420x260", window.Tile.Width, window.Tile.Height)
		}
		bounds := c.layoutBox()
		if x, y := view.treePosition(); x != bounds.x || y != bounds.y {
			t.Errorf("fullscreen position = (%d,%d), want output origin (%d,%d)", x, y, bounds.x, bounds.y)
		}
		if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{bounds.w, bounds.h} {
			t.Errorf("fullscreen configure size = %v, want [%d %d]", got, bounds.w, bounds.h)
		}
		if !c.SetFullscreen(view.id, false) {
			t.Error("SetFullscreen(false) = false, want rule fullscreen exited")
		} else if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{420, 260} {
			t.Errorf("restored configure size = %v, want [420 260]", got)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowRuleFullscreenRestoresOpenFloatingWindow(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		openFloating, openFullscreen := true, true
		c.windowRules = []config.WindowRule{{OpenFloating: &openFloating, OpenFullscreen: &openFullscreen}}
		c.model.RemoveWindow(view.id)
		c.mapWindowWithRules(view, false)

		fullscreen, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok || !fullscreen.Fullscreen || fullscreen.Tile.Floating {
			t.Errorf("rule fullscreen placement = %+v, %t, want tiled fullscreen window", fullscreen, ok)
		}
		if !c.SetFullscreen(view.id, false) {
			t.Error("SetFullscreen(false) = false, want fullscreen exited")
		} else if restored, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id); !ok || restored.Fullscreen || !restored.Tile.Floating {
			t.Errorf("window after fullscreen exit = %+v, %t, want floating", restored, ok)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func findWindowSnapshot(windows []wm.WindowSnapshot, id uint64) (wm.WindowSnapshot, bool) {
	for _, window := range windows {
		if window.ID == id {
			return window, true
		}
	}
	return wm.WindowSnapshot{}, false
}

func TestFullscreenWindowDoesNotOverrideAnotherWorkspace(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var first, second *toplevelView
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			var target uint64
			for _, workspace := range c.WorkspaceSnapshot().Workspaces {
				if workspace.Output != "" && workspace.ID != 0 && !workspace.IsActive {
					target = workspace.ID
				}
			}
			if target == 0 || !c.FocusWorkspaceID(target) {
				t.Error("could not activate the trailing workspace before mapping the second window")
				c.Terminate()
			}
			return
		}
		second = view
		if !c.SetFullscreen(first.id, true) {
			t.Error("SetFullscreen() = false, want first workspace window fullscreen")
			c.Terminate()
			return
		}
		x, y := second.treePosition()
		geometry := second.geometry()
		surface, _, _ := c.surfaceAt(float64(x+geometry.w/2), float64(y+geometry.h/2))
		if got := c.viewForSurface(surface); got != second {
			if got == nil {
				t.Error("active workspace has no visible window at its center")
			} else {
				t.Errorf("active workspace hit window %d, want window %d", got.id, second.id)
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
		t.Fatal("both workspace windows did not map")
	}
}

func TestFullscreenRequestMovesWindowToRequestedOutput(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	var targetOutput string
	c.viewMapped = func(view *toplevelView) {
		var sourceOutput string
		for _, window := range c.WorkspaceSnapshot().Windows {
			if window.ID == view.id {
				sourceOutput = window.Output
				break
			}
		}
		for _, output := range c.OutputSnapshot() {
			if output.Name != sourceOutput {
				targetOutput = output.Name
				break
			}
		}
		if sourceOutput == "" || targetOutput == "" {
			t.Error("could not resolve distinct source and target outputs")
			c.Terminate()
			return
		}
		if !requestFullscreenOnOutputForTest(c, view, targetOutput) {
			t.Errorf("could not request fullscreen on output %q", targetOutput)
			c.Terminate()
			return
		}
		defer clearFullscreenOutputForTest(view)
		c.applyFullscreen(view)

		var window *wm.WindowSnapshot
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id {
				window = &candidate
				break
			}
		}
		if window == nil {
			t.Error("fullscreen window missing from workspace snapshot")
		} else if !window.Fullscreen || window.Output != targetOutput {
			t.Errorf("fullscreen window state = %+v, want fullscreen on output %q", *window, targetOutput)
		}
		var outputOrigin [2]int
		for _, output := range c.OutputSnapshot() {
			if output.Name == targetOutput && output.Logical != nil {
				outputOrigin = [2]int{int(output.Logical.X), int(output.Logical.Y)}
				break
			}
		}
		if x, y := view.treePosition(); x != outputOrigin[0] || y != outputOrigin[1] {
			t.Errorf("fullscreen scene position = (%d,%d), want target output origin %v", x, y, outputOrigin)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestFullscreenRequestWithoutOutputKeepsWindowOnItsWorkspace(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		var window wm.WindowSnapshot
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id {
				window = candidate
				break
			}
		}
		if window.ID == 0 {
			t.Error("mapped window missing from workspace snapshot")
			c.Terminate()
			return
		}
		var inactiveWorkspace uint64
		for _, workspace := range c.WorkspaceSnapshot().Workspaces {
			if workspace.Output == window.Output && !workspace.IsActive {
				inactiveWorkspace = workspace.ID
				break
			}
		}
		if inactiveWorkspace == 0 || !c.model.MoveWindowToWorkspace(view.id, inactiveWorkspace, false) {
			t.Error("could not move mapped window to an inactive workspace")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		focusedOutput := c.model.Snapshot().FocusedOutput
		if !requestFullscreenWithoutOutputForTest(view) {
			t.Error("could not request fullscreen without an output")
			c.Terminate()
			return
		}
		c.applyFullscreen(view)

		var after wm.WindowSnapshot
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id {
				after = candidate
				break
			}
		}
		if !after.Fullscreen || after.WorkspaceID != inactiveWorkspace {
			t.Errorf("fullscreen state after output-less request = %+v, want fullscreen in workspace %d", after, inactiveWorkspace)
		}
		if got := c.model.Snapshot().FocusedOutput; got != focusedOutput {
			t.Errorf("focused output after output-less request = %q, want unchanged %q", got, focusedOutput)
		}
		if bool(view.tree.node.enabled) {
			t.Error("output-less request activated the window's inactive workspace")
		}
		if !requestFullscreenOnOutputForTest(c, view, window.Output) {
			t.Error("could not request fullscreen on the current output")
			c.Terminate()
			return
		}
		defer clearFullscreenOutputForTest(view)
		c.applyFullscreen(view)
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id && candidate.WorkspaceID != inactiveWorkspace {
				t.Errorf("same-output fullscreen request moved window to workspace %d, want %d", candidate.WorkspaceID, inactiveWorkspace)
			}
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestFullscreenRequestReconcilesOutputSceneAndFocus(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	var views []*toplevelView
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 {
			return
		}
		first, sibling := views[0], views[1]
		var sourceOutput string
		for _, window := range c.WorkspaceSnapshot().Windows {
			if window.ID == first.id {
				sourceOutput = window.Output
				break
			}
		}
		targetOutput := ""
		for _, output := range c.OutputSnapshot() {
			if output.Name != sourceOutput {
				targetOutput = output.Name
				break
			}
		}
		if targetOutput == "" {
			t.Error("could not resolve target output")
			c.Terminate()
			return
		}
		if !c.model.MoveWindowToOutput(sibling.id, targetOutput, false) {
			t.Error("could not move sibling window to target output")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		if !c.FocusWindow(first.id) {
			t.Error("could not focus fullscreen request window")
			c.Terminate()
			return
		}
		if !requestFullscreenOnOutputForTest(c, first, targetOutput) {
			t.Errorf("could not request fullscreen on output %q", targetOutput)
			c.Terminate()
			return
		}
		defer clearFullscreenOutputForTest(first)
		c.applyFullscreen(first)

		var fullscreenWindow *wm.WindowSnapshot
		for _, window := range c.WorkspaceSnapshot().Windows {
			if window.ID == first.id {
				fullscreenWindow = &window
				break
			}
		}
		if fullscreenWindow == nil || !fullscreenWindow.Fullscreen || fullscreenWindow.Output != targetOutput {
			t.Errorf("fullscreen model state = %+v, want first window fullscreen on %q", fullscreenWindow, targetOutput)
		}
		if got := c.FocusedOutputName(); got != targetOutput {
			t.Errorf("focused output after fullscreen request = %q, want %q", got, targetOutput)
		}
		if bool(sibling.tree.node.enabled) {
			t.Error("target sibling scene node remained enabled beneath fullscreen window")
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
	if len(views) != 2 {
		t.Fatalf("mapped windows = %d, want two", len(views))
	}
}

func TestFullscreenWindowTracksAssignedOutputAfterMove(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	c.viewMapped = func(view *toplevelView) {
		sourceOutput := c.outputNameForView(view)
		if !c.SetFullscreen(view.id, true) {
			t.Error("SetFullscreen() = false, want fullscreen enabled")
			c.Terminate()
			return
		}
		var target *OutputSnapshot
		for _, output := range c.OutputSnapshot() {
			if output.Name != c.outputNameForView(view) && output.Logical != nil {
				copy := output
				target = &copy
				break
			}
		}
		if target == nil || !c.model.MoveWindowToOutput(view.id, target.Name, true) {
			t.Error("could not move fullscreen window to the second output")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		if gotX, gotY := view.treePosition(); gotX != int(target.Logical.X) || gotY != int(target.Logical.Y) {
			t.Errorf("fullscreen scene origin = (%d,%d), want assigned output origin (%d,%d)", gotX, gotY, target.Logical.X, target.Logical.Y)
		}
		if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{int(target.Logical.Width), int(target.Logical.Height)} {
			t.Errorf("fullscreen size after output move = %v, want assigned output size [%d %d]", got, target.Logical.Width, target.Logical.Height)
		}

		headless.destroy()
		var fallback *OutputSnapshot
		for _, output := range c.OutputSnapshot() {
			if output.Name == sourceOutput && output.Logical != nil {
				copy := output
				fallback = &copy
			}
		}
		window, mapped := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if fallback == nil || !mapped || window.Output != sourceOutput {
			t.Errorf("fullscreen window after output disconnect = %+v, %t; fallback output = %+v", window, mapped, fallback)
		} else {
			if x, y := view.treePosition(); x != int(fallback.Logical.X) || y != int(fallback.Logical.Y) {
				t.Errorf("fullscreen scene origin after disconnect = (%d,%d), want fallback origin (%d,%d)", x, y, fallback.Logical.X, fallback.Logical.Y)
			}
			if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{int(fallback.Logical.Width), int(fallback.Logical.Height)} {
				t.Errorf("fullscreen size after disconnect = %v, want fallback size [%d %d]", got, fallback.Logical.Width, fallback.Logical.Height)
			}
		}
		c.Terminate()
	}
	client := exec.Command(terminal)
	client.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = client.Process.Kill()
		_ = client.Wait()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMoveAndMonitorFocusUseConnectedOutputs(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	c.viewMapped = func(view *toplevelView) {
		var window wm.WindowSnapshot
		found := false
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id {
				window, found = candidate, true
				break
			}
		}
		if !found {
			t.Error("mapped window missing from workspace snapshot")
			c.Terminate()
			return
		}
		targetOutput := ""
		for _, output := range c.OutputSnapshot() {
			if output.Name != window.Output {
				targetOutput = output.Name
				break
			}
		}
		if targetOutput == "" {
			t.Error("no distinct target output connected")
			c.Terminate()
			return
		}
		c.executeBindAction(bind.Action{Name: bind.ActionMoveWorkspaceToMonitor, Output: targetOutput})
		for _, moved := range c.WorkspaceSnapshot().Windows {
			if moved.ID == view.id && (moved.Output != targetOutput || moved.WorkspaceID != window.WorkspaceID) {
				t.Errorf("window after workspace move = %+v, want stable IDs on %q", moved, targetOutput)
			}
		}
		if snapshot := c.WorkspaceSnapshot(); snapshot.FocusedOutput != targetOutput {
			t.Errorf("focused output after moving focused workspace = %q, want %q", snapshot.FocusedOutput, targetOutput)
		}
		c.executeBindAction(bind.Action{Name: bind.ActionFocusMonitor, Output: window.Output})
		if got := c.WorkspaceSnapshot().FocusedOutput; got != window.Output {
			t.Errorf("focused output after FocusMonitor = %q, want %q", got, window.Output)
		}
		var focusedOutput *OutputSnapshot
		for _, output := range c.OutputSnapshot() {
			if output.Name == window.Output {
				focusedOutput = &output
				break
			}
		}
		if focusedOutput == nil || focusedOutput.Logical == nil {
			t.Errorf("focused output %q has no logical geometry", window.Output)
		} else {
			wantX := float64(focusedOutput.Logical.X) + float64(focusedOutput.Logical.Width)/2
			wantY := float64(focusedOutput.Logical.Y) + float64(focusedOutput.Logical.Height)/2
			if dx, dy := float64(c.cursor.x)-wantX, float64(c.cursor.y)-wantY; dx < -1 || dx > 1 || dy < -1 || dy > 1 {
				t.Errorf("cursor after FocusMonitor = (%f,%f), want output center (%f,%f)", c.cursor.x, c.cursor.y, wantX, wantY)
			}
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestEmptyDefaultSizeRulesKeepClientGeometry(t *testing.T) {
	cfg, err := config.Parse("window-chooses-size.kdl", []byte(`
window-rule {
    match app-id="."
    default-column-width {}
    default-window-height {}
}
`))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{
		Layout:      &LayoutOptions{Gaps: 16, ColumnWidthProportion: 0.5},
		WindowRules: cfg.WindowRules,
	})
	c.viewMapped = func(view *toplevelView) {
		geometry := view.geometry()
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Windows) != 1 {
			t.Errorf("mapped windows = %+v, want one", snapshot.Windows)
		} else if tile := snapshot.Windows[0].Tile; tile.Width != geometry.w || tile.Height != geometry.h {
			t.Errorf("empty size-rule tile = %dx%d, client chose %dx%d", tile.Width, tile.Height, geometry.w, geometry.h)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowRuleFallsBackWhenNamedWorkspaceIsUnavailable(t *testing.T) {
	cfg, err := config.Parse("missing-workspace.kdl", []byte(`
window-rule {
    match app-id="."
    open-on-workspace "Missing"
}
`))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{WindowRules: cfg.WindowRules})
	c.viewMapped = func(view *toplevelView) {
		snapshot := c.WorkspaceSnapshot()
		for _, window := range snapshot.Windows {
			if window.ID != view.id {
				continue
			}
			if window.Output != snapshot.FocusedOutput || window.WorkspaceID == 0 {
				t.Errorf("fallback placement = %+v, want connected focused output workspace", window)
			}
			for _, workspace := range snapshot.Workspaces {
				if workspace.ID == window.WorkspaceID && workspace.Name != nil && *workspace.Name == "Missing" {
					t.Error("unavailable named workspace was synthesized from the rule")
				}
			}
			c.Terminate()
			return
		}
		t.Errorf("mapped window %d missing from snapshot", view.id)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestSetWindowFloatingStartsFromCurrentSceneRectangle(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		wantX, wantY := view.treePosition()
		geometry := view.geometry()
		if !c.SetWindowFloating(view.id, true) {
			t.Error("SetWindowFloating() = false, want mapped window to float")
			c.Terminate()
			return
		}
		placement := c.WorkspaceSnapshot().Windows[0].Tile
		if !placement.Floating || placement.X != wantX || placement.Y != wantY ||
			placement.Width != geometry.w || placement.Height != geometry.h {
			t.Errorf("floating placement = %+v, want current scene rectangle (%d,%d,%d,%d)",
				placement, wantX, wantY, geometry.w, geometry.h)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestMoveFloatingWindowUpdatesSceneAndRestoresSavedPosition(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		id := view.id
		if !c.SetWindowFloating(id, true) {
			t.Error("SetWindowFloating() = false, want mapped window to float")
			c.Terminate()
			return
		}
		x, y := view.treePosition()
		if !c.MoveFloatingWindow(&id,
			layout.PositionChange{Kind: layout.PositionAdjustFixed, Value: 40},
			layout.PositionChange{Kind: layout.PositionAdjustFixed, Value: -25},
		) {
			t.Error("MoveFloatingWindow() = false, want mapped window to move")
			c.Terminate()
			return
		}
		wantX, wantY := x+40, y-25
		if gotX, gotY := view.treePosition(); gotX != wantX || gotY != wantY {
			t.Errorf("moved scene position = (%d,%d), want (%d,%d)", gotX, gotY, wantX, wantY)
		}
		if !c.SetWindowFloating(id, false) || !c.SetWindowFloating(id, true) {
			t.Error("could not toggle floating window out and back")
		} else if gotX, gotY := view.treePosition(); gotX != wantX || gotY != wantY {
			t.Errorf("scene position after float toggle = (%d,%d), want saved position (%d,%d)", gotX, gotY, wantX, wantY)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowedFullscreenKeepsTileBoundsAndRestoresTile(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		before := c.model.Snapshot().Windows
		if len(before) == 0 {
			t.Fatal("mapped window has no tiled placement")
		}
		original := before[0].Tile
		if !c.SetWindowedFullscreen(view.id, true) {
			t.Error("SetWindowedFullscreen(true) = false, want fullscreen enabled")
			c.Terminate()
			return
		}
		if !bool(view.toplevel.scheduled.fullscreen) {
			t.Error("windowed fullscreen did not set the client xdg fullscreen state")
		}
		wantX, wantY := original.X, original.Y
		if !original.Floating {
			geometry := view.geometry()
			wantX += (original.Width - geometry.w) / 2
			wantY += (original.Height - geometry.h) / 2
		}
		if x, y := view.treePosition(); x != wantX || y != wantY {
			t.Errorf("windowed fullscreen position = (%d,%d), want window placement (%d,%d)", x, y, wantX, wantY)
		}
		if got := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}; got != [2]int{original.Width, original.Height} {
			t.Errorf("windowed fullscreen size = %v, want tile size [%d %d]", got, original.Width, original.Height)
		}
		if !c.SetWindowedFullscreen(view.id, false) {
			t.Error("SetWindowedFullscreen(false) = false, want tiled placement restored")
		}
		if bool(view.toplevel.scheduled.fullscreen) {
			t.Error("windowed fullscreen exit kept the client xdg fullscreen state")
		}
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Windows) != 1 || snapshot.Windows[0].WindowedFullscreen || snapshot.Windows[0].Tile != original {
			t.Errorf("placement after exit = %+v, want original tile %+v", snapshot.Windows, original)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowedFullscreenFromClientFullscreenRestoresFloatingPlacement(t *testing.T) {
	c := newNested(t)
	c.viewMapped = func(view *toplevelView) {
		if !c.SetWindowFloating(view.id, true) {
			t.Error("SetWindowFloating() = false, want floating window")
			c.Terminate()
			return
		}
		floating, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok || !floating.Tile.Floating {
			t.Fatalf("floating placement = %+v, %t, want floating window", floating, ok)
		}
		if !c.SetFullscreen(view.id, true) {
			t.Error("SetFullscreen(true) = false, want client fullscreen")
			c.Terminate()
			return
		}
		if !c.SetWindowedFullscreen(view.id, true) {
			t.Error("SetWindowedFullscreen(true) = false, want transition from client fullscreen")
			c.Terminate()
			return
		}
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok || window.Fullscreen || !window.WindowedFullscreen || !window.Tile.Floating {
			t.Errorf("windowed fullscreen placement = %+v, %t, want floating windowed fullscreen", window, ok)
		} else if got, want := [4]int{window.Tile.X, window.Tile.Y, window.Tile.Width, window.Tile.Height}, [4]int{floating.Tile.X, floating.Tile.Y, floating.Tile.Width, floating.Tile.Height}; got != want {
			t.Errorf("windowed floating bounds = %v, want restored bounds %v", got, want)
		}
		if !bool(view.toplevel.scheduled.fullscreen) {
			t.Error("windowed fullscreen did not set the client xdg fullscreen state")
		}
		if got, want := [2]int{int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)}, [2]int{floating.Tile.Width, floating.Tile.Height}; got != want {
			t.Errorf("windowed floating size = %v, want restored size %v", got, want)
		}
		if x, y := view.treePosition(); x != floating.Tile.X || y != floating.Tile.Y {
			t.Errorf("windowed floating origin = (%d,%d), want restored origin (%d,%d)", x, y, floating.Tile.X, floating.Tile.Y)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWindowedFullscreenSurvivesUnmapAndRemap(t *testing.T) {
	model := wm.New()
	model.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	model.MapWindow(1, "DP-1")
	bounds := layout.Bounds{X: 180, Y: 130, Width: 420, Height: 280}
	if !model.SetWindowFloating(1, true, bounds) || !model.SetWindowedFullscreen(1, true) {
		t.Fatal("could not set up floating windowed fullscreen")
	}
	model.RemoveWindow(1)

	c := &Compositor{model: model}
	c.mapWindowAfterUnmap(&toplevelView{id: 1}, "DP-1", true)

	window, ok := findWindowSnapshot(model.Snapshot().Windows, 1)
	if !ok || window.Fullscreen || !window.WindowedFullscreen || !window.Tile.Floating {
		t.Fatalf("remapped window = %+v, %t, want floating windowed fullscreen", window, ok)
	}
	if got := [4]int{window.Tile.X, window.Tile.Y, window.Tile.Width, window.Tile.Height}; got != [4]int{bounds.X, bounds.Y, bounds.Width, bounds.Height} {
		t.Errorf("remapped floating bounds = %v, want %v", got, bounds)
	}
}

func TestWindowedFullscreenExitRepositionsAfterClientConfigure(t *testing.T) {
	c := newNested(t)
	stage := 0
	c.viewMapped = func(view *toplevelView) {
		if !c.SetWindowedFullscreen(view.id, true) {
			t.Error("SetWindowedFullscreen(true) = false, want fullscreen enabled")
			c.Terminate()
			return
		}
		stage = 1
		var listener *Listener
		listener = AddListener(unsafe.Pointer(&view.toplevel.base.surface.events.commit), func(unsafe.Pointer) {
			if stage == 1 {
				stage = 2
				if !c.SetWindowedFullscreen(view.id, false) {
					t.Error("SetWindowedFullscreen(false) = false, want fullscreen exited")
					c.Terminate()
				}
				return
			}
			if stage != 2 {
				return
			}
			stage = 3
			c.drop(listener)
			geometry := view.geometry()
			var tileFound bool
			for _, window := range c.model.Snapshot().Windows {
				if window.ID != view.id {
					continue
				}
				tileFound = true
				wantX, wantY := window.Tile.X, window.Tile.Y
				if !window.Tile.Floating {
					wantX += (window.Tile.Width - geometry.w) / 2
					wantY += (window.Tile.Height - geometry.h) / 2
				}
				if gotX, gotY := view.treePosition(); gotX != wantX || gotY != wantY {
					t.Errorf("scene position after restore configure = (%d,%d), want (%d,%d) for committed geometry %dx%d",
						gotX, gotY, wantX, wantY, geometry.w, geometry.h)
				}
				break
			}
			if !tileFound {
				t.Error("window placement disappeared while exiting windowed fullscreen")
			}
			c.Terminate()
		})
		c.keep(listener)
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if stage != 3 {
		t.Fatalf("windowed fullscreen exit reached stage %d, want committed restore configure at stage 3", stage)
	}
}

func TestOutputReconnectRestoresFocusToRemappedWindow(t *testing.T) {
	c := newNested(t)
	var mapped *toplevelView
	c.viewMapped = func(view *toplevelView) {
		mapped = view
		name := c.FocusedOutputName()
		if !c.model.FocusWorkspace(name, 2) {
			t.Error("could not focus the trailing empty workspace")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		c.activateWorkspaceFocus()
		if c.focusedView != nil {
			t.Error("focused view remained while the active workspace was empty")
		}
		for output, sceneOutput := range c.sceneOutputs {
			delete(c.sceneOutputs, output)
			delete(c.usable, output)
			c.removeWorkspaceOutput(name)
			if c.keyboardFocusedView() != nil {
				t.Error("keyboard focus remained after removing the last output")
			}

			c.sceneOutputs[output] = sceneOutput
			c.syncWorkspaceOutput(output)
			if c.keyboardFocusedView() != view {
				t.Errorf("keyboard focus after output reconnect = %p, want window %p", c.keyboardFocusedView(), view)
			}
			c.Terminate()
			return
		}
		t.Error("nested compositor has no output")
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if mapped == nil {
		t.Fatal("window did not map")
	}
}

func TestMappedWindowsUseScrollingColumnPlacements(t *testing.T) {
	c := newNested(t)
	var first, second *toplevelView
	var secondClient *exec.Cmd
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Workspaces) != 2 || !snapshot.Workspaces[0].IsActive || snapshot.Workspaces[1].IsActive {
			t.Errorf("workspace snapshot = %+v, want occupied active workspace and trailing empty workspace", snapshot.Workspaces)
		}
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].ID != first.id || snapshot.Windows[1].ID != second.id {
			t.Errorf("window snapshot = %+v, want two windows in column order", snapshot.Windows)
		} else {
			left, right := snapshot.Windows[0], snapshot.Windows[1]
			if left.Column != 0 || right.Column != 1 || right.Tile.X <= left.Tile.X {
				t.Errorf("column placements = %+v and %+v, want ordered visible columns", left, right)
			}
			firstGeometry := first.geometry()
			firstX := left.Tile.X + (left.Tile.Width-firstGeometry.w)/2
			firstY := left.Tile.Y + (left.Tile.Height-firstGeometry.h)/2
			if x, y := first.treePosition(); x != firstX || y != firstY {
				t.Errorf("first scene position = (%d,%d), want centered (%d,%d)", x, y, firstX, firstY)
			}
			secondGeometry := second.geometry()
			secondX := right.Tile.X + (right.Tile.Width-secondGeometry.w)/2
			secondY := right.Tile.Y + (right.Tile.Height-secondGeometry.h)/2
			if x, y := second.treePosition(); x != secondX || y != secondY {
				t.Errorf("second scene position = (%d,%d), want centered (%d,%d)", x, y, secondX, secondY)
			}
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestTabbedColumnHitTestIgnoresInactiveWindow(t *testing.T) {
	c := newNested(t)
	var first, second *toplevelView
	var secondClient *exec.Cmd
	var hit *toplevelView
	t.Cleanup(func() {
		if secondClient != nil && secondClient.Process != nil {
			_ = secondClient.Process.Kill()
			_ = secondClient.Wait()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		c.focusView(first)
		if !c.model.ConsumeWindowIntoColumn() || !c.model.ToggleColumnTabbedDisplay() {
			t.Error("could not create and activate a tabbed column")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()

		firstGeometry, secondGeometry := first.geometry(), second.geometry()
		firstX, firstY := first.treePosition()
		secondX, secondY := second.treePosition()
		left := max(firstX+firstGeometry.x, secondX+secondGeometry.x)
		top := max(firstY+firstGeometry.y, secondY+secondGeometry.y)
		right := min(firstX+firstGeometry.x+firstGeometry.w, secondX+secondGeometry.x+secondGeometry.w)
		bottom := min(firstY+firstGeometry.y+firstGeometry.h, secondY+secondGeometry.y+secondGeometry.h)
		if right <= left || bottom <= top {
			t.Errorf("tab window surfaces do not overlap: first=%+v at (%d,%d), second=%+v at (%d,%d)",
				firstGeometry, firstX, firstY, secondGeometry, secondX, secondY)
			c.Terminate()
			return
		}
		surface, _, _ := c.surfaceAt(float64((left+right)/2), float64((top+bottom)/2))
		hit = c.viewForSurface(surface)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
	if hit != first {
		t.Errorf("scene hit in active tab overlap = %p, want active window %p (inactive window %p)", hit, first, second)
	}
}

func TestPointerResizeSurvivesWorkspaceModelReapplication(t *testing.T) {
	c := newNested(t)
	var keyboard *fakeKeyboard
	var pointer *fakePointer
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		keyboard = c.addFakeKeyboard()
		pointer = c.addFakePointer()
		if keyboard == nil || pointer == nil {
			t.Error("could not attach the test input devices")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		x, y := view.treePosition()
		pointer.moveTo(float64(x+1), float64(y+1))
		pointer.button(buttonRight, true)
		if c.pointerGrab == nil || c.pointerGrab.mode != grabResize {
			t.Error("nested Mod+RMB did not begin a resize grab")
			c.Terminate()
			return
		}
		grab := *c.pointerGrab
		pointer.moveTo(60, 60)
		want := grab.geometry(float64(c.cursor.x), float64(c.cursor.y))
		positionAfterResize := c.WorkspaceSnapshot().Windows[0].Tile
		pointer.button(buttonRight, false)
		c.applyWorkspaceModel()

		got := c.WorkspaceSnapshot().Windows[0].Tile
		if got.Width != want.width || got.Height != want.height {
			t.Errorf("workspace size after resize/reapply = %dx%d, want %dx%d", got.Width, got.Height, want.width, want.height)
		}
		if got.X != positionAfterResize.X || got.Y != positionAfterResize.Y {
			t.Errorf("workspace position after resize/reapply = (%d,%d), want retained (%d,%d)",
				got.X, got.Y, positionAfterResize.X, positionAfterResize.Y)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWorkspaceBindActivatesDynamicWorkspace(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+9", bind.ActionFocusWorkspace, "9"),
	}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			keyboard = c.addFakeKeyboard()
			if keyboard == nil {
				t.Error("could not attach fake keyboard")
				c.Terminate()
				return
			}
			keyboard.key(c.modKeycode(), true)
			// Which key Mod is per backend is TestResolveModKeyByBackend's.
			if got := c.activeBindModifiers(); got != bind.Modifiers(c.modKey) {
				t.Errorf("active modifiers after Mod = %b, want %b", got, c.modKey)
			}
			if c.bindings[0].Chord.Modifiers != bind.Modifiers(c.modKey) {
				t.Errorf("chord = %+v, want it resolved to Mod %v", c.bindings[0].Chord, c.modKey)
			}
			keyboard.key(10, true) // Top row 9.
			keyboard.key(10, false)
			keyboard.key(c.modKeycode(), false)
			snapshot := c.WorkspaceSnapshot()
			if len(snapshot.Workspaces) != 2 || snapshot.Workspaces[0].IsActive || !snapshot.Workspaces[1].IsActive {
				t.Errorf("after Mod+9, workspaces = %+v; want workspace 2 active", snapshot.Workspaces)
			}
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Workspaces) != 3 || !snapshot.Workspaces[1].IsActive || snapshot.Workspaces[2].IsActive {
			t.Errorf("after second map, workspaces = %+v; want active workspace 2 and trailing workspace 3", snapshot.Workspaces)
		}
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].WorkspaceID != snapshot.Workspaces[0].ID ||
			snapshot.Windows[1].WorkspaceID != snapshot.Workspaces[1].ID {
			t.Errorf("windows = %+v; want one window in each occupied workspace", snapshot.Windows)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestNestedKeyboardBindNamesAndMovesWindowToNamedWorkspace(t *testing.T) {
	move, supported, err := bind.ParseAction(bind.ActionMoveWindowToWorkspace, []string{"Work"})
	if err != nil || !supported {
		t.Fatalf("ParseAction(move-window-to-workspace) = (%+v, %v, %v)", move, supported, err)
	}
	c := newNestedWith(t, Options{
		Workspaces: []wm.WorkspaceConfig{{Name: "Work"}},
		Binds: []bind.Binding{
			keyBind(t, "Mod+N", bind.ActionSetWorkspaceName, "Build"),
			{Chord: bind.Chord{Key: "W", UsesMod: true}, Actions: []bind.Action{move}},
		},
	})
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(*toplevelView) {
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(49, true) // N: name the active workspace.
		keyboard.key(49, false)
		keyboard.key(c.modKeycode(), false)
		buildID, named := c.model.WorkspaceIDByName("Build")
		if !named {
			t.Error("set-workspace-name bind did not name the focused workspace")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(17, true) // W: move the active window to the named workspace.
		snapshot := c.WorkspaceSnapshot()
		workID, workFound := c.model.WorkspaceIDByName("Work")
		if !workFound || len(snapshot.Windows) != 1 || snapshot.Windows[0].WorkspaceID != workID {
			t.Errorf("window after move-to-workspace bind = %+v, want workspace Work (%d)", snapshot.Windows, workID)
		}
		if !snapshot.Workspaces[0].IsFocused && !snapshot.Workspaces[1].IsFocused {
			t.Errorf("focused workspace after move = %+v, want focus to follow the moved window", snapshot.Workspaces)
		}
		if buildID == workID {
			t.Fatal("new workspace name reused Work's stable ID")
		}
		keyboard.key(17, false)
		keyboard.key(c.modKeycode(), false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestFloatingBindsMoveMappedWindowBetweenLayers(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+F", bind.ActionMoveWindowToFloating),
		keyBind(t, "Mod+T", bind.ActionMoveWindowToTiling),
	}})
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(*toplevelView) {
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(33, true) // F
		keyboard.key(33, false)
		keyboard.key(c.modKeycode(), false)
		if windows := c.WorkspaceSnapshot().Windows; len(windows) != 1 || !windows[0].Tile.Floating {
			t.Errorf("after Mod+F, windows = %+v, want one floating window", windows)
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(20, true) // T
		keyboard.key(20, false)
		keyboard.key(c.modKeycode(), false)
		if windows := c.WorkspaceSnapshot().Windows; len(windows) != 1 || windows[0].Tile.Floating {
			t.Errorf("after Mod+T, windows = %+v, want one tiled window", windows)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestSpawnBindUsesServerCallback(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Return", bind.ActionSpawn, "test-command", "--flag"),
	}})
	var got []string
	c.SetSpawnHandlers(func(argv []string) error {
		got = append([]string(nil), argv...)
		return nil
	}, nil)
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(*toplevelView) {
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(28, true) // Return.
		keyboard.key(28, false)
		keyboard.key(c.modKeycode(), false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if got == nil || len(got) != 2 || got[0] != "test-command" || got[1] != "--flag" {
		t.Fatalf("spawn callback argv = %q, want %q", got, []string{"test-command", "--flag"})
	}
}

func keyBind(t *testing.T, chord, name string, args ...string) bind.Binding {
	t.Helper()
	parsedChord, err := bind.ParseChord(chord)
	if err != nil {
		t.Fatal(err)
	}
	action, supported, err := bind.ParseAction(name, args)
	if err != nil || !supported {
		t.Fatalf("ParseAction(%q, %v) = (%+v, %v, %v)", name, args, action, supported, err)
	}
	return bind.Binding{Chord: parsedChord, Actions: []bind.Action{action}}
}

func TestNestedKeyboardBindFocusesAndMovesTabbedWindowsVertically(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+J", "focus-window-down"),
		keyBind(t, "Mod+K", "focus-window-up"),
		keyBind(t, "Mod+Ctrl+J", "move-window-down"),
		keyBind(t, "Mod+Ctrl+K", "move-window-up"),
	}})
	output := c.WorkspaceSnapshot().FocusedOutput
	for id := uint64(1); id <= 3; id++ {
		c.model.MapWindow(id, output)
	}
	if !c.model.FocusWindow(1) || !c.model.ConsumeWindowIntoColumn() ||
		!c.model.ConsumeWindowIntoColumn() || !c.model.SetColumnDisplay(layout.ColumnDisplayTabbed) {
		t.Fatal("could not form a three-window tabbed column")
	}
	c.applyWorkspaceModel()

	keyboard := c.addFakeKeyboard()
	if keyboard == nil {
		t.Fatal("could not attach fake keyboard")
	}
	t.Cleanup(keyboard.remove)
	press := func(mods []uint32, key uint32) {
		for _, mod := range mods {
			keyboard.key(mod, true)
		}
		keyboard.key(key, true)
		keyboard.key(key, false)
		for i := len(mods) - 1; i >= 0; i-- {
			keyboard.key(mods[i], false)
		}
	}
	press([]uint32{c.modKeycode()}, 36) // Mod+J: focus window down.
	if got, _ := c.model.ActiveWindow(); got != 2 {
		t.Fatalf("active window after Mod+J = %d, want 2", got)
	}
	press([]uint32{c.modKeycode(), 29}, 36) // Mod+Ctrl+J: move window down.
	if got, want := windowIDs(c.WorkspaceSnapshot()), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("window order after move down = %v, want %v", got, want)
	}
	if got, _ := c.model.ActiveWindow(); got != 2 {
		t.Fatalf("active window after move down = %d, want moved window 2", got)
	}
	press([]uint32{c.modKeycode(), 29}, 37) // Mod+Ctrl+K: move window up.
	if got, want := windowIDs(c.WorkspaceSnapshot()), []uint64{1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("window order after move up = %v, want %v", got, want)
	}
	press([]uint32{c.modKeycode()}, 37) // Mod+K: focus window up.
	if got, _ := c.model.ActiveWindow(); got != 1 {
		t.Fatalf("active window after Mod+K = %d, want 1", got)
	}
	c.Terminate()
}

func TestNestedKeyboardBindMovesColumnAndWindowBetweenWorkspaces(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Ctrl+J", "move-column-to-workspace-down"),
		keyBind(t, "Mod+Ctrl+K", "move-window-to-workspace-up"),
	}})
	output := c.WorkspaceSnapshot().FocusedOutput
	c.model.MapWindow(1, output)
	c.model.MapWindow(2, output)
	initial := c.WorkspaceSnapshot()
	if len(initial.Workspaces) < 2 {
		t.Fatalf("workspaces after initial map = %+v, want trailing target workspace", initial.Workspaces)
	}
	topID, targetID := initial.Workspaces[0].ID, initial.Workspaces[1].ID
	if !c.model.SetWorkspaceName(topID, "Top") || !c.model.FocusWorkspace(output, 2) {
		t.Fatal("could not preserve workspace Top and activate workspace 2")
	}
	c.model.MapWindow(3, output)
	if !c.model.FocusWorkspace(output, 1) || !c.model.FocusWindow(1) || !c.model.ConsumeWindowIntoColumn() {
		t.Fatal("could not form a two-window column in workspace Top")
	}
	c.applyWorkspaceModel()

	keyboard := c.addFakeKeyboard()
	if keyboard == nil {
		t.Fatal("could not attach fake keyboard")
	}
	t.Cleanup(keyboard.remove)
	press := func(mods []uint32, key uint32) {
		for _, mod := range mods {
			keyboard.key(mod, true)
		}
		keyboard.key(key, true)
		keyboard.key(key, false)
		for i := len(mods) - 1; i >= 0; i-- {
			keyboard.key(mods[i], false)
		}
	}
	press([]uint32{c.modKeycode(), 29}, 36) // Mod+Ctrl+J: move the whole column down and follow it.
	snapshot := c.WorkspaceSnapshot()
	for _, id := range []uint64{1, 2} {
		var workspaceID uint64
		for _, window := range snapshot.Windows {
			if window.ID == id {
				workspaceID = window.WorkspaceID
			}
		}
		if workspaceID != targetID {
			t.Fatalf("window %d workspace after column move = %d, want %d; windows=%+v", id, workspaceID, targetID, snapshot.Windows)
		}
	}
	if got, _ := c.model.ActiveWindow(); got != 1 {
		t.Fatalf("active window after column move = %d, want 1", got)
	}
	press([]uint32{c.modKeycode(), 29}, 37) // Mod+Ctrl+K: move the focused window up and follow it.
	snapshot = c.WorkspaceSnapshot()
	for _, window := range snapshot.Windows {
		if window.ID == 1 && window.WorkspaceID != topID {
			t.Fatalf("window 1 workspace after move up = %d, want %d", window.WorkspaceID, topID)
		}
		if window.ID == 2 && window.WorkspaceID != targetID {
			t.Fatalf("window 2 workspace after move up = %d, want %d", window.WorkspaceID, targetID)
		}
	}
	if got, _ := c.model.ActiveWindow(); got != 1 {
		t.Fatalf("active window after move up = %d, want 1", got)
	}
	c.Terminate()
}

func windowIDs(snapshot wm.Snapshot) []uint64 {
	ids := make([]uint64, len(snapshot.Windows))
	for i, window := range snapshot.Windows {
		ids[i] = window.ID
	}
	return ids
}

func TestNestedKeyboardBindFocusesColumnAndConsumesKey(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{{
		Chord:   bind.Chord{Key: "Left", UsesMod: true},
		Actions: []bind.Action{{Name: bind.ActionFocusColumnLeft}},
	}}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		c.focusView(second)
		keyboard.key(c.modKeycode(), true)
		keyboard.key(105, true) // Left arrow.
		if !c.suppressedKeys[105] {
			t.Error("configured bind key was not marked consumed")
		}
		if c.focusedView != first {
			t.Errorf("focused view = %p, want first window %p", c.focusedView, first)
		}
		keyboard.key(105, false)
		if c.suppressedKeys[105] {
			t.Error("consumed key release remained suppressed after release")
		}
		keyboard.key(c.modKeycode(), false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestNestedKeyboardBindMovesFocusedColumnLeft(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Ctrl+H", bind.ActionMoveColumnLeft),
	}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		keyboard.key(29, true) // Left Ctrl.
		keyboard.key(35, true) // H.
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].ID != second.id || snapshot.Windows[1].ID != first.id {
			t.Errorf("windows after Mod+Ctrl+H = %+v; want focused column moved left", snapshot.Windows)
		}
		if !c.suppressedKeys[35] {
			t.Error("configured move bind key was not marked consumed")
		}
		keyboard.key(35, false)
		keyboard.key(29, false)
		keyboard.key(c.modKeycode(), false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestNestedKeyboardBindConsumeOrExpelWindowRight(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Ctrl+L", bind.ActionConsumeOrExpelWindowRight),
	}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
		if secondClient != nil && secondClient.Process != nil {
			_ = secondClient.Process.Kill()
			_ = secondClient.Wait()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		c.focusView(first)
		keyboard.key(c.modKeycode(), true)
		keyboard.key(29, true) // Left Ctrl.
		keyboard.key(38, true) // L.
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].Column != snapshot.Windows[1].Column {
			t.Errorf("windows after Mod+Ctrl+L = %+v; want right neighbor consumed into the focused column", snapshot.Windows)
		}
		if !c.suppressedKeys[38] {
			t.Error("configured consume-or-expel bind key was not marked consumed")
		}
		keyboard.key(38, false)
		keyboard.key(29, false)
		keyboard.key(c.modKeycode(), false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}
