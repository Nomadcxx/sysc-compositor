package wm

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestOutputsKeepStableWorkspaceIDs(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	first := m.Snapshot().Workspaces[0].ID
	m.AddOutput("DP-1", layout.Bounds{X: 1000, Width: 1200, Height: 800}, 600, 16)

	snapshot := m.Snapshot()
	if len(snapshot.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want one per output", len(snapshot.Workspaces))
	}
	for _, workspace := range snapshot.Workspaces {
		if workspace.Output == "eDP-1" && workspace.ID != first {
			t.Fatalf("eDP-1 workspace ID = %d, want stable ID %d", workspace.ID, first)
		}
	}
}

func TestMappingWindowCreatesTrailingWorkspace(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.MapWindow(1, "eDP-1") {
		t.Fatal("MapWindow() = false, want true")
	}

	snapshot := m.Snapshot()
	if len(snapshot.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want active and trailing empty", len(snapshot.Workspaces))
	}
	if got := snapshot.Workspaces[0].ActiveWindowID; got == nil || *got != 1 {
		t.Fatalf("active window = %v, want window 1", got)
	}
	if got := snapshot.Workspaces[1].ActiveWindowID; got != nil {
		t.Fatalf("trailing workspace active window = %v, want nil", got)
	}
	if got := snapshot.Windows[0].WorkspaceID; got != snapshot.Workspaces[0].ID {
		t.Fatalf("window workspace ID = %d, want %d", got, snapshot.Workspaces[0].ID)
	}
}

func TestFocusWorkspaceClampsToTrailingEmpty(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "eDP-1")
	if !m.FocusWorkspace("eDP-1", 9) {
		t.Fatal("FocusWorkspace() = false, want true")
	}

	snapshot := m.Snapshot()
	if !snapshot.Workspaces[1].IsActive || !snapshot.Workspaces[1].IsFocused {
		t.Fatalf("workspace 2 = %+v, want active and focused", snapshot.Workspaces[1])
	}
	if snapshot.Workspaces[0].IsActive || snapshot.Workspaces[0].IsFocused {
		t.Fatalf("workspace 1 = %+v, want inactive and unfocused", snapshot.Workspaces[0])
	}
}

func TestWorkspaceIndexClampsToIPCMaximum(t *testing.T) {
	m := New()
	if !m.AddOutput("DP-1", layout.Bounds{Width: 800, Height: 600}, 400, 16) {
		t.Fatal("AddOutput failed")
	}
	for i := 1; i <= 255; i++ {
		if !m.MapWindow(uint64(i), "DP-1") {
			t.Fatalf("MapWindow(%d) failed", i)
		}
		if i < 255 && !m.FocusWorkspace("DP-1", uint8(i+1)) {
			t.Fatalf("FocusWorkspace(%d) failed", i+1)
		}
	}

	workspaces := m.Snapshot().Workspaces
	if got := workspaces[len(workspaces)-1].Index; got != 255 {
		t.Fatalf("last workspace index = %d, want saturated 255", got)
	}
}

func TestFocusWorkspaceByStableID(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	snapshot := m.Snapshot()
	workspaceID := snapshot.Workspaces[1].ID
	if !m.FocusWorkspaceID(workspaceID) {
		t.Fatal("FocusWorkspaceID() = false, want true")
	}
	snapshot = m.Snapshot()
	if snapshot.Workspaces[0].IsActive || !snapshot.Workspaces[1].IsActive || !snapshot.Workspaces[1].IsFocused {
		t.Fatalf("workspaces after ID focus = %+v, want workspace %d active and focused", snapshot.Workspaces, workspaceID)
	}
	if m.FocusWorkspaceID(workspaceID + 100) {
		t.Fatal("FocusWorkspaceID() accepted an unknown ID")
	}
}

func TestColumnActionsChangeWindowOrder(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "eDP-1")
	m.MapWindow(2, "eDP-1")
	if !m.FocusColumnLeft() || !m.MoveColumnRight() {
		t.Fatal("focus and move of the first column failed")
	}

	snapshot := m.Snapshot()
	if got := snapshot.Windows[0].ID; got != 2 {
		t.Fatalf("first window after move = %d, want 2", got)
	}
	if got := snapshot.Windows[1].ID; got != 1 {
		t.Fatalf("second window after move = %d, want 1", got)
	}
	if got := snapshot.Workspaces[0].ActiveWindowID; got == nil || *got != 1 {
		t.Fatalf("active window = %v, want moved window 1", got)
	}
}

func TestSnapshotKeepsColumnAndTileIndexesForTabbedWindows(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1200, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2, 3} {
		if !m.MapWindow(id, "eDP-1") {
			t.Fatalf("MapWindow(%d) failed", id)
		}
	}
	if !m.FocusWindow(1) || !m.ConsumeWindowIntoColumn() {
		t.Fatal("could not consume window 2 into window 1's column")
	}

	snapshot := m.Snapshot()
	if len(snapshot.Windows) != 3 {
		t.Fatalf("windows = %d, want 3", len(snapshot.Windows))
	}
	if snapshot.Windows[0].Column != 0 || snapshot.Windows[0].Tile.Tile != 0 ||
		snapshot.Windows[1].Column != 0 || snapshot.Windows[1].Tile.Tile != 1 ||
		snapshot.Windows[2].Column != 1 || snapshot.Windows[2].Tile.Tile != 0 {
		t.Errorf("column/tile positions = (%d,%d), (%d,%d), (%d,%d), want (0,0), (0,1), (1,0)",
			snapshot.Windows[0].Column, snapshot.Windows[0].Tile.Tile,
			snapshot.Windows[1].Column, snapshot.Windows[1].Tile.Tile,
			snapshot.Windows[2].Column, snapshot.Windows[2].Tile.Tile)
	}
}

func TestMoveFloatingWindowRetainsPositionAcrossUnmapAndRemap(t *testing.T) {
	m := New()
	if !m.AddOutput("DP-1", layout.Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 500, 16) || !m.MapWindow(1, "DP-1") {
		t.Fatal("could not create output and map window 1")
	}
	set := layout.PositionChange{Kind: layout.PositionSetFixed, Value: 120}
	if !m.MoveFloatingWindow(1, set, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 80}) {
		t.Fatal("MoveFloatingWindow() = false, want to set a tiled window's saved floating position")
	}
	if !m.SetWindowFloating(1, true, layout.Bounds{X: 700, Y: 600, Width: 300, Height: 200}) {
		t.Fatal("SetWindowFloating() = false, want window 1 floating")
	}
	if !m.MoveFloatingWindow(1,
		layout.PositionChange{Kind: layout.PositionAdjustFixed, Value: 40},
		layout.PositionChange{Kind: layout.PositionAdjustFixed, Value: -25},
	) {
		t.Fatal("MoveFloatingWindow() = false, want to move a floating window")
	}
	m.RemoveWindow(1)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("could not unmap and remap window 1")
	}
	windows := m.Snapshot().Windows
	if len(windows) != 1 || !windows[0].Tile.Floating || [2]int{windows[0].Tile.X, windows[0].Tile.Y} != [2]int{260, 105} {
		t.Fatalf("window after remap = %+v, want floating position (260,105)", windows)
	}
	if m.MoveFloatingWindow(99, set, set) {
		t.Fatal("MoveFloatingWindow() accepted an unmapped window")
	}
}

func TestWorkspaceNamesAreCaseInsensitiveAndUnique(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1200, Height: 800}, 500, 16)
	m.MapWindow(1, "eDP-1")
	workspaces := m.Snapshot().Workspaces
	firstID, secondID := workspaces[0].ID, workspaces[1].ID
	if !m.SetWorkspaceName(firstID, "Work") {
		t.Fatal("SetWorkspaceName() = false, want to name the first workspace")
	}
	if m.SetWorkspaceName(secondID, "WORK") {
		t.Fatal("SetWorkspaceName() accepted a case-insensitive duplicate")
	}
	if !m.FocusWorkspaceName("work") {
		t.Fatal("FocusWorkspaceName() = false, want case-insensitive name lookup")
	}
	snapshot := m.Snapshot()
	if !snapshot.Workspaces[0].IsFocused || snapshot.Workspaces[0].Name == nil || *snapshot.Workspaces[0].Name != "Work" {
		t.Fatalf("workspace after name focus = %+v, want named workspace Work focused", snapshot.Workspaces[0])
	}
}

func TestFloatingGeometryRetainedAcrossWindowUnmapAndRemap(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.MapWindow(1, "DP-1") || !m.MapWindow(2, "DP-1") || !m.FocusWindow(1) {
		t.Fatal("could not map and focus the test windows")
	}
	want := layout.Bounds{X: 100, Y: 80, Width: 420, Height: 280}
	if !m.SetWindowFloating(1, true, want) {
		t.Fatal("SetWindowFloating() = false, want window 1 to enter floating placement")
	}
	m.RemoveWindow(1)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow(1) = false, want the stable window to remap")
	}
	for _, window := range m.Snapshot().Windows {
		if window.ID != 1 {
			continue
		}
		got := window.Tile
		if !got.Floating || [4]int{got.X, got.Y, got.Width, got.Height} != [4]int{want.X, want.Y, want.Width, want.Height} {
			t.Fatalf("remapped placement = %+v, want floating rectangle %+v", got, want)
		}
		return
	}
	t.Fatal("remapped floating window is missing from the snapshot")
}

func TestFocusFloatingAndTilingChangesActiveWorkspaceWindow(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.MapWindow(2, "DP-1")
	if !m.FocusWindow(1) || !m.SetWindowFloating(1, true, layout.Bounds{X: 80, Y: 60, Width: 300, Height: 200}) {
		t.Fatal("could not focus and float window 1")
	}
	if !m.FocusTiling() {
		t.Fatal("FocusTiling() = false, want tiled layer selected")
	}
	if got, ok := m.ActiveWindow(); !ok || got != 2 {
		t.Fatalf("tiled active window = %d, %t, want 2", got, ok)
	}
	if !m.FocusFloating() {
		t.Fatal("FocusFloating() = false, want floating layer selected")
	}
	if got, ok := m.ActiveWindow(); !ok || got != 1 {
		t.Errorf("floating active window = %d, %t, want 1", got, ok)
	}
}

func TestFullscreenModesRemainDistinctAndKeepTiledPlacement(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.MapWindow(2, "DP-1")
	var original layout.Placement
	for _, window := range m.Snapshot().Windows {
		if window.ID == 1 {
			original = window.Tile
		}
	}
	if !m.SetFullscreen(1, true) {
		t.Fatal("SetFullscreen() = false, want client fullscreen recorded")
	}
	if !m.SetWindowedFullscreen(1, true) {
		t.Fatal("SetWindowedFullscreen() = false, want windowed fullscreen recorded")
	}
	for _, window := range m.Snapshot().Windows {
		if window.ID != 1 {
			continue
		}
		if window.Fullscreen || !window.WindowedFullscreen {
			t.Errorf("fullscreen states = (client %t, windowed %t), want (false, true)", window.Fullscreen, window.WindowedFullscreen)
		}
		if window.Tile != original {
			t.Errorf("tile after fullscreen = %+v, want original placement %+v", window.Tile, original)
		}
		return
	}
	t.Fatal("fullscreen window missing from snapshot")
}

func TestClientFullscreenTemporarilyTilesFloatingWindow(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	floatBounds := layout.Bounds{X: 180, Y: 130, Width: 420, Height: 280}
	if !m.SetWindowFloating(1, true, floatBounds) {
		t.Fatal("SetWindowFloating() = false, want window floated")
	}

	if !m.SetFullscreen(1, true) {
		t.Fatal("SetFullscreen(true) = false, want fullscreen state set")
	}
	fullscreen, ok := windowSnapshot(m, 1)
	if !ok || !fullscreen.Fullscreen || fullscreen.Tile.Floating {
		t.Fatalf("fullscreen placement = %+v, %t, want tiled fullscreen window", fullscreen, ok)
	}

	if !m.SetFullscreen(1, false) {
		t.Fatal("SetFullscreen(false) = false, want fullscreen exited")
	}
	restored, ok := windowSnapshot(m, 1)
	if !ok || restored.Fullscreen || !restored.Tile.Floating {
		t.Fatalf("restored placement = %+v, %t, want floating window", restored, ok)
	}
	if got := [4]int{restored.Tile.X, restored.Tile.Y, restored.Tile.Width, restored.Tile.Height}; got != [4]int{floatBounds.X, floatBounds.Y, floatBounds.Width, floatBounds.Height} {
		t.Errorf("restored floating bounds = %v, want %v", got, floatBounds)
	}
}

func TestClientFullscreenToWindowedFullscreenRestoresFloatingWindow(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	floatBounds := layout.Bounds{X: 180, Y: 130, Width: 420, Height: 280}
	if !m.SetWindowFloating(1, true, floatBounds) || !m.SetFullscreen(1, true) {
		t.Fatal("could not float and fullscreen the window")
	}
	if !m.SetWindowedFullscreen(1, true) {
		t.Fatal("SetWindowedFullscreen(true) = false, want transition from client fullscreen")
	}

	window, ok := windowSnapshot(m, 1)
	if !ok || window.Fullscreen || !window.WindowedFullscreen || !window.Tile.Floating {
		t.Fatalf("windowed fullscreen placement = %+v, %t, want floating windowed fullscreen", window, ok)
	}
	if got := [4]int{window.Tile.X, window.Tile.Y, window.Tile.Width, window.Tile.Height}; got != [4]int{floatBounds.X, floatBounds.Y, floatBounds.Width, floatBounds.Height} {
		t.Errorf("windowed floating bounds = %v, want %v", got, floatBounds)
	}
	if !m.SetWindowedFullscreen(1, false) {
		t.Fatal("SetWindowedFullscreen(false) = false, want windowed fullscreen exited")
	}
	window, ok = windowSnapshot(m, 1)
	if !ok || window.WindowedFullscreen || !window.Tile.Floating {
		t.Fatalf("window after windowed fullscreen exit = %+v, %t, want floating placement", window, ok)
	}
}

func TestFullscreenFloatingWindowRestoresOnMovedOutput(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Y: 300, Width: 2000, Height: 1200}, 1000, 16)
	m.MapWindow(1, "DP-1")
	if !m.SetWindowFloating(1, true, layout.Bounds{X: 200, Y: 200, Width: 300, Height: 200}) || !m.SetFullscreen(1, true) {
		t.Fatal("could not float and fullscreen the window")
	}
	if !m.MoveWindowToOutput(1, "HDMI-A-1", true) {
		t.Fatal("MoveWindowToOutput() = false, want fullscreen window moved")
	}
	if !m.SetFullscreen(1, false) {
		t.Fatal("SetFullscreen(false) = false, want fullscreen exited")
	}
	window, ok := windowSnapshot(m, 1)
	if !ok || window.Output != "HDMI-A-1" || !window.Tile.Floating {
		t.Fatalf("window after fullscreen exit = %+v, %t, want floating on HDMI-A-1", window, ok)
	}
	if got, want := [4]int{window.Tile.X, window.Tile.Y, window.Tile.Width, window.Tile.Height}, [4]int{1400, 600, 300, 200}; got != want {
		t.Errorf("restored floating bounds = %v, want remapped bounds %v", got, want)
	}
}

func windowSnapshot(m *Model, id uint64) (WindowSnapshot, bool) {
	for _, window := range m.Snapshot().Windows {
		if window.ID == id {
			return window, true
		}
	}
	return WindowSnapshot{}, false
}

func TestWindowUrgencyIsDerivedAndClearsOnFocus(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.MapWindow(2, "DP-1")

	if !m.SetWindowUrgent(1, true) {
		t.Fatal("SetWindowUrgent(1, true) = false, want non-focused window marked urgent")
	}
	if m.SetWindowUrgent(1, true) {
		t.Fatal("SetWindowUrgent(1, true) changed an already urgent window")
	}
	snapshot := m.Snapshot()
	if !snapshot.Workspaces[0].IsUrgent {
		t.Fatal("workspace urgency = false, want true while it contains an urgent window")
	}
	for _, window := range snapshot.Windows {
		if window.ID == 1 && !window.IsUrgent {
			t.Fatal("window urgency = false, want true")
		}
	}
	if !m.FocusWindow(1) {
		t.Fatal("FocusWindow(1) = false")
	}
	snapshot = m.Snapshot()
	if snapshot.Workspaces[0].IsUrgent {
		t.Fatal("workspace urgency stayed true after its urgent window received focus")
	}
	for _, window := range snapshot.Windows {
		if window.ID == 1 && window.IsUrgent {
			t.Fatal("window stayed urgent after receiving focus")
		}
	}
	if m.SetWindowUrgent(1, true) {
		t.Fatal("focused window became urgent")
	}
	if m.SetWindowUrgent(99, false) {
		t.Fatal("unknown window urgency changed")
	}
}

func TestConfiguredWorkspaceIsCreatedOnItsNamedOutput(t *testing.T) {
	m := New(WorkspaceConfig{Name: "chat", OpenOnOutput: "DP-1"})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("DP-1", layout.Bounds{X: 1000, Width: 1200, Height: 800}, 600, 16)

	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Name != nil && *workspace.Name == "chat" {
			if workspace.Output != "DP-1" {
				t.Fatalf("configured workspace output = %q, want DP-1", workspace.Output)
			}
			return
		}
	}
	t.Fatal("configured workspace chat was not created")
}

func TestSetWorkspaceConfigAddsConfiguredWorkspaceToExistingOutput(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.SetWorkspaceConfig([]WorkspaceConfig{{Name: "chat", OpenOnOutput: "DP-1"}}) {
		t.Fatal("SetWorkspaceConfig() reported no change")
	}
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Name != nil && *workspace.Name == "chat" {
			return
		}
	}
	t.Fatal("reloaded workspace config was not applied to the existing output")
}

func TestMoveWindowToWorkspacePreservesIDAndFollowsWhenFocused(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.MapWindow(2, "DP-1")
	if !m.FocusWindow(1) {
		t.Fatal("could not focus source window")
	}
	targetID := uint64(0)
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == "HDMI-A-1" && workspace.IsActive {
			targetID = workspace.ID
		}
	}
	if targetID == 0 {
		t.Fatal("target output has no active workspace")
	}
	if !m.MoveWindowToWorkspace(1, targetID, true) {
		t.Fatal("MoveWindowToWorkspace() = false, want move")
	}
	snapshot := m.Snapshot()
	if snapshot.FocusedOutput != "HDMI-A-1" {
		t.Errorf("focused output = %q, want HDMI-A-1", snapshot.FocusedOutput)
	}
	for _, window := range snapshot.Windows {
		if window.ID == 1 && (window.Output != "HDMI-A-1" || window.WorkspaceID != targetID) {
			t.Errorf("moved window = %+v, want stable ID in target output/workspace", window)
		}
		if window.ID == 2 && window.Output != "DP-1" {
			t.Errorf("unmoved window = %+v, want to remain on DP-1", window)
		}
	}
}

func TestMoveWindowToWorkspaceWithoutFollowPreservesDestinationFocus(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2} {
		m.MapWindow(id, "DP-1")
	}
	for _, id := range []uint64{3, 4} {
		m.MapWindow(id, "HDMI-A-1")
	}
	if !m.FocusOutput("DP-1") || !m.FocusWindow(1) {
		t.Fatal("could not focus source window")
	}

	var targetID, targetFocus uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == "HDMI-A-1" && workspace.IsActive {
			targetID = workspace.ID
			if workspace.ActiveWindowID != nil {
				targetFocus = *workspace.ActiveWindowID
			}
		}
	}
	if targetID == 0 || targetFocus == 0 {
		t.Fatal("target workspace has no active window")
	}
	if !m.MoveWindowToWorkspace(1, targetID, false) {
		t.Fatal("MoveWindowToWorkspace() = false, want move")
	}

	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.ID != targetID {
			continue
		}
		if workspace.ActiveWindowID == nil || *workspace.ActiveWindowID != targetFocus {
			t.Errorf("destination active window = %v, want preserved window %d", workspace.ActiveWindowID, targetFocus)
		}
	}
	if got := m.Snapshot().FocusedOutput; got != "DP-1" {
		t.Errorf("focused output = %q, want DP-1 when focus does not follow", got)
	}
}

func TestMoveFocusedColumnToWorkspaceWithoutFollowPreservesDestinationFocus(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2} {
		m.MapWindow(id, "DP-1")
	}
	for _, id := range []uint64{3, 4} {
		m.MapWindow(id, "HDMI-A-1")
	}
	if !m.FocusOutput("DP-1") || !m.FocusWindow(1) {
		t.Fatal("could not focus source column")
	}

	var targetID, targetFocus uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == "HDMI-A-1" && workspace.IsActive {
			targetID = workspace.ID
			if workspace.ActiveWindowID != nil {
				targetFocus = *workspace.ActiveWindowID
			}
		}
	}
	if targetID == 0 || targetFocus == 0 {
		t.Fatal("target workspace has no active window")
	}
	if !m.MoveFocusedColumnToWorkspace(targetID, false) {
		t.Fatal("MoveFocusedColumnToWorkspace() = false, want move")
	}

	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.ID != targetID {
			continue
		}
		if workspace.ActiveWindowID == nil || *workspace.ActiveWindowID != targetFocus {
			t.Errorf("destination active window = %v, want preserved window %d", workspace.ActiveWindowID, targetFocus)
		}
	}
	if got := m.Snapshot().FocusedOutput; got != "DP-1" {
		t.Errorf("focused output = %q, want DP-1 when focus does not follow", got)
	}
}

func TestMoveTabbedColumnToWorkspaceKeepsTileOrder(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2, 3} {
		m.MapWindow(id, "DP-1")
	}
	if !m.FocusWindow(1) || !m.ConsumeWindowIntoColumn() {
		t.Fatal("could not create a two-window column")
	}
	var targetID uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == "HDMI-A-1" && workspace.IsActive {
			targetID = workspace.ID
		}
	}
	if !m.MoveFocusedColumnToWorkspace(targetID, true) {
		t.Fatal("MoveFocusedColumnToWorkspace() = false, want move")
	}
	var moved []WindowSnapshot
	for _, window := range m.Snapshot().Windows {
		if window.ID == 1 || window.ID == 2 {
			moved = append(moved, window)
		}
	}
	if len(moved) != 2 || moved[0].Output != "HDMI-A-1" || moved[1].Output != "HDMI-A-1" {
		t.Fatalf("moved tab group = %+v, want two windows on target output", moved)
	}
	if moved[0].Column != moved[1].Column || moved[0].Tile.Tile != 0 || moved[1].Tile.Tile != 1 {
		t.Errorf("moved tab layout = (%+v, %+v), want ordered tiles in one column", moved[0].Tile, moved[1].Tile)
	}
}

func TestMoveFocusedColumnToWorkspaceMovesOnlyFocusedFloatingWindow(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.MapWindow(2, "DP-1")
	if !m.SetWindowFloating(1, true, layout.Bounds{X: 50, Y: 60, Width: 400, Height: 300}) {
		t.Fatal("SetWindowFloating() = false, want window 1 floating")
	}
	if !m.FocusWindow(1) {
		t.Fatal("could not focus floating window 1")
	}

	var targetID uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == "HDMI-A-1" && workspace.IsActive {
			targetID = workspace.ID
		}
	}
	if !m.MoveFocusedColumnToWorkspace(targetID, true) {
		t.Fatal("MoveFocusedColumnToWorkspace() = false, want focused floating window moved")
	}

	snapshot := m.Snapshot()
	for _, window := range snapshot.Windows {
		switch window.ID {
		case 1:
			if window.Output != "HDMI-A-1" || window.WorkspaceID != targetID || !window.Tile.Floating {
				t.Errorf("focused floating window = %+v, want floating on target workspace", window)
			}
		case 2:
			if window.Output != "DP-1" {
				t.Errorf("unfocused tiled window = %+v, want it to stay on DP-1", window)
			}
		}
	}
}

func TestMoveWindowToMonitorUsesNearestOutputInDirection(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	if !m.MoveWindowToMonitorDirection(1, "right", true) {
		t.Fatal("MoveWindowToMonitorDirection() = false, want move to HDMI-A-1")
	}
	for _, window := range m.Snapshot().Windows {
		if window.ID == 1 && window.Output != "HDMI-A-1" {
			t.Errorf("window output = %q, want HDMI-A-1", window.Output)
		}
	}
}

func TestMonitorDirectionSkipsOutputsWithoutOrthogonalOverlap(t *testing.T) {
	m := New()
	m.AddOutput("source", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("diagonal", layout.Bounds{X: 1000, Y: 900, Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("aligned", layout.Bounds{X: 3000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "source")

	if !m.MoveWindowToMonitorDirection(1, "right", true) {
		t.Fatal("MoveWindowToMonitorDirection() = false, want aligned output selected")
	}
	for _, window := range m.Snapshot().Windows {
		if window.ID == 1 && window.Output != "aligned" {
			t.Fatalf("window output = %q, want nearest output sharing vertical span", window.Output)
		}
	}
}

func TestMoveFocusedWorkspaceToMonitorMovesStableWorkspaceAndActivatesTarget(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")

	var movedWorkspaceID, targetWorkspaceID uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == "DP-1" && workspace.IsActive {
			movedWorkspaceID = workspace.ID
		}
		if workspace.Output == "HDMI-A-1" && workspace.IsActive {
			targetWorkspaceID = workspace.ID
		}
	}
	if !m.MoveFocusedWorkspaceToMonitorDirection("right") {
		t.Fatal("MoveFocusedWorkspaceToMonitorDirection() = false, want move to HDMI-A-1")
	}

	snapshot := m.Snapshot()
	if snapshot.FocusedOutput != "HDMI-A-1" {
		t.Errorf("focused output = %q, want HDMI-A-1", snapshot.FocusedOutput)
	}
	var movedWorkspace, originalTarget, sourceWorkspace *WorkspaceSnapshot
	for i := range snapshot.Workspaces {
		workspace := &snapshot.Workspaces[i]
		switch workspace.ID {
		case movedWorkspaceID:
			movedWorkspace = workspace
		case targetWorkspaceID:
			originalTarget = workspace
		default:
			if workspace.Output == "DP-1" && workspace.IsActive {
				sourceWorkspace = workspace
			}
		}
	}
	if movedWorkspace == nil || movedWorkspace.Output != "HDMI-A-1" || !movedWorkspace.IsActive || !movedWorkspace.IsFocused {
		t.Errorf("moved workspace = %+v, want stable ID active and focused on HDMI-A-1", movedWorkspace)
	}
	if originalTarget == nil || originalTarget.Output != "HDMI-A-1" || originalTarget.IsActive || originalTarget.Index <= movedWorkspace.Index {
		t.Errorf("preexisting target workspace = %+v, want it after the inserted active workspace", originalTarget)
	}
	if sourceWorkspace == nil || sourceWorkspace.Output != "DP-1" {
		t.Errorf("source replacement workspace = %+v, want an active DP-1 workspace", sourceWorkspace)
	}
	if len(snapshot.Windows) != 1 || snapshot.Windows[0].ID != 1 || snapshot.Windows[0].WorkspaceID != movedWorkspaceID || snapshot.Windows[0].Output != "HDMI-A-1" {
		t.Errorf("window after workspace move = %+v, want stable window and workspace IDs on HDMI-A-1", snapshot.Windows)
	}
}

func TestMoveWorkspaceToOutputRemapsFloatingWindowPosition(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 2000, Height: 1200}, 1000, 16)
	m.MapWindow(1, "DP-1")
	if !m.SetWindowFloating(1, true, layout.Bounds{X: 200, Y: 200, Width: 300, Height: 200}) {
		t.Fatal("SetWindowFloating() = false, want window floating")
	}

	var workspaceID uint64
	for _, window := range m.Snapshot().Windows {
		if window.ID == 1 {
			workspaceID = window.WorkspaceID
		}
	}
	if workspaceID == 0 || !m.MoveWorkspaceToOutput(workspaceID, "HDMI-A-1") {
		t.Fatal("MoveWorkspaceToOutput() = false, want workspace transferred")
	}

	for _, window := range m.Snapshot().Windows {
		if window.ID != 1 {
			continue
		}
		if window.Output != "HDMI-A-1" || !window.Tile.Floating {
			t.Fatalf("moved floating window = %+v, want floating on HDMI-A-1", window)
		}
		if got, want := [4]int{window.Tile.X, window.Tile.Y, window.Tile.Width, window.Tile.Height}, [4]int{1400, 300, 300, 200}; got != want {
			t.Errorf("moved floating bounds = %v, want relative position and size %v", got, want)
		}
		return
	}
	t.Fatal("moved floating window missing from workspace snapshot")
}

func TestRemoveOutputRemapsFloatingWindowPositionToFallback(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Y: 300, Width: 2000, Height: 1200}, 1000, 16)
	m.MapWindow(1, "DP-1")
	if !m.SetWindowFloating(1, true, layout.Bounds{X: 200, Y: 200, Width: 300, Height: 200}) {
		t.Fatal("SetWindowFloating() = false, want window floating")
	}

	m.RemoveOutput("DP-1")

	for _, window := range m.Snapshot().Windows {
		if window.ID != 1 {
			continue
		}
		if window.Output != "HDMI-A-1" || !window.Tile.Floating {
			t.Fatalf("window after output removal = %+v, want floating on HDMI-A-1", window)
		}
		if got, want := [4]int{window.Tile.X, window.Tile.Y, window.Tile.Width, window.Tile.Height}, [4]int{1400, 600, 300, 200}; got != want {
			t.Errorf("fallback floating bounds = %v, want remapped bounds %v", got, want)
		}
		return
	}
	t.Fatal("floating window missing after output removal")
}

func TestMoveNamedInactiveWorkspaceToMonitorKeepsCurrentFocus(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	workspaceID := m.Snapshot().Workspaces[0].ID
	if !m.SetWorkspaceName(workspaceID, "Work") || !m.FocusWorkspace("DP-1", 2) || !m.FocusOutput("HDMI-A-1") {
		t.Fatal("could not set up an inactive named workspace and focus HDMI-A-1")
	}
	if !m.MoveWorkspaceToOutput(workspaceID, "HDMI-A-1") {
		t.Fatal("MoveWorkspaceToOutput() = false, want named workspace moved")
	}

	snapshot := m.Snapshot()
	if snapshot.FocusedOutput != "HDMI-A-1" {
		t.Errorf("focused output = %q, want focus to stay on HDMI-A-1", snapshot.FocusedOutput)
	}
	for _, workspace := range snapshot.Workspaces {
		if workspace.ID == workspaceID && (workspace.Output != "HDMI-A-1" || workspace.Name == nil || *workspace.Name != "Work" || workspace.IsFocused) {
			t.Errorf("moved named workspace = %+v, want inactive Work on HDMI-A-1", workspace)
		}
	}
}

func TestFocusOutputDirectionSelectsAdjacentMonitor(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	if !m.FocusOutputDirection("right") {
		t.Fatal("FocusOutputDirection(right) = false, want HDMI-A-1")
	}
	if got := m.Snapshot().FocusedOutput; got != "HDMI-A-1" {
		t.Fatalf("focused output = %q, want HDMI-A-1", got)
	}
}

func TestFirstAndLastColumnActions(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "eDP-1")
	m.MapWindow(2, "eDP-1")
	m.MapWindow(3, "eDP-1")
	if !m.FocusColumnFirst() || !m.FocusColumnLast() || !m.MoveColumnToFirst() {
		t.Fatal("first/last focus and move-to-first actions failed")
	}
	snapshot := m.Snapshot()
	if got, want := []uint64{snapshot.Windows[0].ID, snapshot.Windows[1].ID, snapshot.Windows[2].ID}, []uint64{3, 1, 2}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("column order = %v, want %v", got, want)
	}
	if active := snapshot.Workspaces[0].ActiveWindowID; active == nil || *active != 3 {
		t.Fatalf("focused window = %v, want 3", active)
	}
	if !m.MoveColumnToLast() {
		t.Fatal("MoveColumnToLast() = false, want true")
	}
	snapshot = m.Snapshot()
	if got, want := []uint64{snapshot.Windows[0].ID, snapshot.Windows[1].ID, snapshot.Windows[2].ID}, []uint64{1, 2, 3}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("column order after MoveColumnToLast() = %v, want %v", got, want)
	}
}

func TestRemovingOutputMovesItsWindowsToRemainingOutput(t *testing.T) {
	m := New()
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("DP-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.RemoveOutput("DP-1")

	snapshot := m.Snapshot()
	if len(snapshot.Workspaces) != 2 || snapshot.Workspaces[0].Output != "eDP-1" || snapshot.Workspaces[1].Output != "eDP-1" {
		t.Fatalf("workspaces after output removal = %+v, want active and trailing eDP-1 workspaces", snapshot.Workspaces)
	}
	if len(snapshot.Windows) != 1 || snapshot.Windows[0].Output != "eDP-1" {
		t.Fatalf("windows after output removal = %+v, want window on eDP-1", snapshot.Windows)
	}
	if snapshot.Workspaces[1].ActiveWindowID != nil {
		t.Fatalf("trailing workspace active window = %v, want nil", snapshot.Workspaces[1].ActiveWindowID)
	}
}

func TestReconnectedOutputRemapsWindowsLeftWithoutAnOutput(t *testing.T) {
	m := New()
	if !m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16) {
		t.Fatal("AddOutput(DP-1) failed")
	}
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow() failed")
	}
	oldWorkspaceID := m.Snapshot().Workspaces[0].ID
	m.RemoveOutput("DP-1")
	if !m.AddOutput("HDMI-A-1", layout.Bounds{Width: 1200, Height: 900}, 600, 16) {
		t.Fatal("AddOutput(HDMI-A-1) failed")
	}

	snapshot := m.Snapshot()
	if len(snapshot.Windows) != 1 || snapshot.Windows[0].ID != 1 {
		t.Fatalf("windows after output reconnect = %+v, want window 1 mapped", snapshot.Windows)
	}
	if snapshot.Windows[0].WorkspaceID == oldWorkspaceID || snapshot.Windows[0].Output != "HDMI-A-1" {
		t.Fatalf("reconnected window = %+v, want a new HDMI-A-1 workspace", snapshot.Windows[0])
	}
	if len(snapshot.Workspaces) != 2 || snapshot.Workspaces[1].IsActive || len(snapshot.Workspaces[1].WindowIDs) != 0 {
		t.Fatalf("workspaces after remap = %+v, want a trailing empty workspace", snapshot.Workspaces)
	}
}

func TestResizeRetainsTileSizeAcrossWindowUnmapAndRemap(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow() failed")
	}
	if !m.ResizeWindow(1, 16, 16, 560, 420, layout.ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow() failed")
	}

	m.RemoveWindow(1)
	if snapshot := m.Snapshot(); len(snapshot.Windows) != 0 {
		t.Fatalf("windows after unmap = %+v, want none", snapshot.Windows)
	}
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow() after remap failed")
	}

	window := m.Snapshot().Windows[0]
	if window.Tile.Width != 560 || window.Tile.Height != 420 {
		t.Fatalf("remapped tile = %+v, want retained size 560x420", window.Tile)
	}
}

func TestVerticalResizePreservesConstrainedWidthAcrossRemap(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1200, Height: 800}, 400, 16)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow() failed")
	}
	if !m.ResizeWindow(1, 16, 16, 900, 500, layout.ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("initial ResizeWindow() failed")
	}
	if !m.SetWindowSizeHints(1, 0, 500, 0, 0) {
		t.Fatal("setting max-width hint failed")
	}

	placement := m.Snapshot().Windows[0].Tile
	if placement.Width != 500 {
		t.Fatalf("constrained width = %d, want 500", placement.Width)
	}
	if !m.ResizeWindow(1, placement.X, placement.Y, placement.Width, 420, layout.ResizeOptions{ResizeHeight: true}) {
		t.Fatal("vertical ResizeWindow() failed")
	}
	m.RemoveWindow(1)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow() after unmap failed")
	}
	if !m.SetWindowSizeHints(1, 0, 0, 0, 0) {
		t.Fatal("removing max-width hint failed")
	}

	placement = m.Snapshot().Windows[0].Tile
	if placement.Width != 900 || placement.Height != 420 {
		t.Fatalf("remapped placement after vertical resize = %+v, want 900x420", placement)
	}
}

func TestDestroyDropsRetainedResizeState(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	if !m.ResizeWindow(1, 16, 16, 560, 420, layout.ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow() failed")
	}
	m.RemoveWindow(1)
	m.DestroyWindow(1)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("MapWindow() after destroy failed")
	}

	window := m.Snapshot().Windows[0]
	if window.Tile.Width != 500 || window.Tile.Height != 768 {
		t.Fatalf("tile after destroyed ID was mapped again = %+v, want defaults 500x768", window.Tile)
	}
}

// activeIndex is the active workspace's position on output.
func activeIndex(t *testing.T, m *Model, output string) int {
	t.Helper()
	i := 0
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output != output {
			continue
		}
		if workspace.IsActive {
			return i
		}
		i++
	}
	t.Fatalf("output %s has no active workspace", output)
	return -1
}

// workspaceIDs lists output's workspace IDs in order.
func workspaceIDs(m *Model, output string) []uint64 {
	var ids []uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Output == output {
			ids = append(ids, workspace.ID)
		}
	}
	return ids
}

// threeWorkspaces maps windows 1-3 onto workspaces 1-3 of output, leaving
// the trailing empty workspace as the fourth.
func threeWorkspaces(t *testing.T, m *Model, output string) {
	t.Helper()
	for i := 1; i <= 3; i++ {
		if !m.FocusWorkspace(output, uint8(i)) || !m.MapWindow(uint64(i), output) {
			t.Fatalf("mapping window %d failed", i)
		}
	}
}

func TestFocusWorkspaceUpDownAndPreviousFollowNiri(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "DP-1")
	m.FocusWorkspace("", 1)
	if !m.FocusWorkspaceDown() || activeIndex(t, m, "DP-1") != 1 {
		t.Fatalf("FocusWorkspaceDown: active %d, want 1", activeIndex(t, m, "DP-1"))
	}
	if !m.FocusWorkspaceDown() || activeIndex(t, m, "DP-1") != 2 {
		t.Fatalf("second FocusWorkspaceDown: active %d, want 2", activeIndex(t, m, "DP-1"))
	}
	if !m.FocusWorkspacePrevious() || activeIndex(t, m, "DP-1") != 1 {
		t.Fatalf("FocusWorkspacePrevious: active %d, want 1", activeIndex(t, m, "DP-1"))
	}
	if !m.FocusWorkspacePrevious() || activeIndex(t, m, "DP-1") != 2 {
		t.Fatalf("second FocusWorkspacePrevious: active %d, want 2", activeIndex(t, m, "DP-1"))
	}
	m.FocusWorkspace("", 1)
	if m.FocusWorkspaceUp() {
		t.Fatal("FocusWorkspaceUp on the first workspace reported a change")
	}

	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	if !m.FocusOutput("HDMI-A-1") {
		t.Fatal("FocusOutput(HDMI-A-1) failed")
	}
	if m.FocusWorkspacePrevious() {
		t.Fatal("a new output with no history went to a previous workspace")
	}
	m.FocusOutput("DP-1")
	if !m.FocusWorkspacePrevious() || activeIndex(t, m, "DP-1") != 2 {
		t.Fatalf("DP-1's previous after a monitor change: active %d, want 2", activeIndex(t, m, "DP-1"))
	}
}

func TestMoveWorkspaceDownKeepsPreviousAndTrailingEmpty(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.MapWindow(1, "DP-1")
	m.FocusWorkspace("", 2)
	m.MapWindow(2, "DP-1")
	ids := workspaceIDs(m, "DP-1")
	a, b := ids[0], ids[1]
	m.FocusWorkspace("", 1) // previous is now B
	if !m.MoveWorkspaceDown() {
		t.Fatal("MoveWorkspaceDown() = false")
	}
	got := workspaceIDs(m, "DP-1")
	if len(got) != 3 || got[0] != b || got[1] != a || activeIndex(t, m, "DP-1") != 1 {
		t.Fatalf("after MoveWorkspaceDown: order %v active %d, want [%d %d empty] active 1", got, activeIndex(t, m, "DP-1"), b, a)
	}
	if !m.FocusWorkspacePrevious() || workspaceIDs(m, "DP-1")[activeIndex(t, m, "DP-1")] != b {
		t.Fatal("MoveWorkspaceDown lost the previous workspace")
	}
	m.FocusWorkspaceID(a)
	if !m.MoveWorkspaceUp() {
		t.Fatal("MoveWorkspaceUp() = false")
	}
	if got := workspaceIDs(m, "DP-1"); got[0] != a || got[1] != b || len(got) != 3 {
		t.Fatalf("after MoveWorkspaceUp: order %v, want [%d %d empty]", got, a, b)
	}
	m.FocusWorkspace("", 3)
	if m.MoveWorkspaceDown() {
		t.Fatal("MoveWorkspaceDown on the trailing empty workspace reported a change")
	}
	m.FocusWorkspaceID(b)
	if !m.MoveWorkspaceDown() {
		t.Fatal("MoveWorkspaceDown into the trailing position failed")
	}
	got = workspaceIDs(m, "DP-1")
	if len(got) != 3 || got[1] != b {
		t.Fatalf("moving B last: order %v, want [%d %d new-empty]", got, a, b)
	}
	if windows := m.Snapshot().Workspaces[2].WindowIDs; len(windows) != 0 {
		t.Fatalf("trailing workspace has windows %v, want one empty trailing workspace", windows)
	}
}

func TestMoveWorkspaceToIndexClampsAndResolvesReference(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "DP-1")
	ids := workspaceIDs(m, "DP-1")
	if !m.MoveWorkspaceToIndex(0, 0) {
		t.Fatal("MoveWorkspaceToIndex(0) = false")
	}
	if got := workspaceIDs(m, "DP-1"); got[0] != ids[2] || activeIndex(t, m, "DP-1") != 0 {
		t.Fatalf("index 0: order %v active %d, want %d first and active", got, activeIndex(t, m, "DP-1"), ids[2])
	}
	if !m.MoveWorkspaceToIndex(99, 0) {
		t.Fatal("MoveWorkspaceToIndex(99) = false")
	}
	got := workspaceIDs(m, "DP-1")
	if got[2] != ids[2] || activeIndex(t, m, "DP-1") != 2 || len(got) != 4 {
		t.Fatalf("index 99: order %v active %d, want %d last before the trailing empty", got, activeIndex(t, m, "DP-1"), ids[2])
	}
	// A referenced inactive workspace moves; the active one keeps focus.
	if !m.MoveWorkspaceToIndex(3, ids[0]) {
		t.Fatal("MoveWorkspaceToIndex by reference = false")
	}
	got = workspaceIDs(m, "DP-1")
	if got[2] != ids[0] || got[activeIndex(t, m, "DP-1")] != ids[2] {
		t.Fatalf("by reference: order %v, active %d; want %d third and %d still active", got, activeIndex(t, m, "DP-1"), ids[0], ids[2])
	}
	if m.MoveWorkspaceToIndex(1, 9999) {
		t.Fatal("an unknown workspace reference moved something")
	}
}

func focusedWindowID(t *testing.T, m *Model) uint64 {
	t.Helper()
	id, ok := m.ActiveWindow()
	if !ok {
		t.Fatal("no active window")
	}
	return id
}

func TestOrMonitorCompositesCrossOnlyAtTheEdge(t *testing.T) {
	m := New()
	m.AddOutput("L", layout.Bounds{Width: 1000, Height: 800}, 400, 16)
	m.AddOutput("R", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 400, 16)
	m.FocusOutput("R")
	m.MapWindow(3, "R")
	m.MapWindow(4, "R")
	m.FocusOutput("L")
	m.MapWindow(1, "L")
	m.MapWindow(2, "L")
	m.FocusWindow(1)
	if !m.FocusColumnOrMonitorRight() || focusedWindowID(t, m) != 2 || m.Snapshot().FocusedOutput != "L" {
		t.Fatalf("mid-strip FocusColumnOrMonitorRight: output %s window %d, want L/2 and no output change",
			m.Snapshot().FocusedOutput, focusedWindowID(t, m))
	}
	if !m.FocusColumnOrMonitorRight() || m.Snapshot().FocusedOutput != "R" {
		t.Fatalf("edge FocusColumnOrMonitorRight: output %s, want R", m.Snapshot().FocusedOutput)
	}
	if !m.FocusColumnOrMonitorLeft() || m.Snapshot().FocusedOutput != "R" || focusedWindowID(t, m) != 3 {
		t.Fatalf("FocusColumnOrMonitorLeft on R's last column: output %s window %d, want R/3",
			m.Snapshot().FocusedOutput, focusedWindowID(t, m))
	}
	m.FocusOutput("L")
	m.FocusWindow(2)
	if !m.MoveColumnRightOrToMonitorRight() || m.Snapshot().FocusedOutput != "R" || focusedWindowID(t, m) != 2 {
		t.Fatalf("edge MoveColumnRightOrToMonitorRight: output %s window %d, want window 2 on R",
			m.Snapshot().FocusedOutput, focusedWindowID(t, m))
	}

	single := New()
	single.AddOutput("L", layout.Bounds{Width: 1000, Height: 800}, 400, 16)
	single.MapWindow(1, "L")
	single.MapWindow(2, "L")
	single.FocusWindow(1)
	if !single.FocusColumnOrMonitorRight() || focusedWindowID(t, single) != 2 {
		t.Fatalf("single-output FocusColumnOrMonitorRight focused %d, want plain focus-column-right to 2", focusedWindowID(t, single))
	}
	if !single.MoveColumnLeftOrToMonitorLeft() || !slicesEqual(single.Snapshot().Workspaces[0].WindowIDs, []uint64{2, 1}) {
		t.Fatalf("single-output MoveColumnLeftOrToMonitorLeft: %v, want plain move-column-left", single.Snapshot().Workspaces[0].WindowIDs)
	}

	vertical := New()
	vertical.AddOutput("T", layout.Bounds{Width: 1000, Height: 800}, 400, 16)
	vertical.AddOutput("B", layout.Bounds{Y: 800, Width: 1000, Height: 800}, 400, 16)
	vertical.FocusOutput("T")
	vertical.MapWindow(1, "T")
	if !vertical.FocusWindowOrMonitorDown() || vertical.Snapshot().FocusedOutput != "B" {
		t.Fatalf("FocusWindowOrMonitorDown on a lone tile: output %s, want B", vertical.Snapshot().FocusedOutput)
	}
	if !vertical.FocusWindowOrMonitorUp() || vertical.Snapshot().FocusedOutput != "T" {
		t.Fatalf("FocusWindowOrMonitorUp from an empty B: output %s, want T", vertical.Snapshot().FocusedOutput)
	}
}

func slicesEqual(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOrWorkspaceCompositesFallBackToWorkspace(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1400, Height: 900}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		m.MapWindow(id, "DP-1")
	}
	m.FocusWindow(1)
	m.ConsumeWindowIntoColumn()
	m.ConsumeWindowIntoColumn() // one column [1, 2, 3]
	m.FocusWindow(2)
	if !m.FocusWindowOrWorkspaceDown() || focusedWindowID(t, m) != 3 || activeIndex(t, m, "DP-1") != 0 {
		t.Fatalf("mid-column FocusWindowOrWorkspaceDown: window %d workspace %d, want 3 on workspace 0",
			focusedWindowID(t, m), activeIndex(t, m, "DP-1"))
	}
	if !m.FocusWindowOrWorkspaceDown() || activeIndex(t, m, "DP-1") != 1 {
		t.Fatalf("bottom-tile FocusWindowOrWorkspaceDown: workspace %d, want 1", activeIndex(t, m, "DP-1"))
	}
	if !m.FocusWindowOrWorkspaceUp() || activeIndex(t, m, "DP-1") != 0 {
		t.Fatalf("FocusWindowOrWorkspaceUp from an empty workspace: workspace %d, want 0", activeIndex(t, m, "DP-1"))
	}

	m.FocusWindow(2)
	if !m.MoveWindowDownOrToWorkspaceDown() || activeIndex(t, m, "DP-1") != 0 ||
		!slicesEqual(m.Snapshot().Workspaces[0].WindowIDs, []uint64{1, 3, 2}) {
		t.Fatalf("mid-column MoveWindowDownOrToWorkspaceDown: %v, want a plain move down", m.Snapshot().Workspaces[0].WindowIDs)
	}
	if !m.MoveWindowDownOrToWorkspaceDown() || activeIndex(t, m, "DP-1") != 1 || focusedWindowID(t, m) != 2 {
		t.Fatalf("bottom MoveWindowDownOrToWorkspaceDown: workspace %d, want window 2 followed to workspace 1",
			activeIndex(t, m, "DP-1"))
	}
	if !m.MoveWindowUpOrToWorkspaceUp() || activeIndex(t, m, "DP-1") != 0 || focusedWindowID(t, m) != 2 {
		t.Fatalf("lone-tile MoveWindowUpOrToWorkspaceUp: workspace %d, want window 2 back on workspace 0",
			activeIndex(t, m, "DP-1"))
	}
}

func TestUnderMouseActionsLeaveTheFocusedOutput(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("HDMI-A-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "HDMI-A-1")
	if !m.FocusWorkspace("HDMI-A-1", 1) || !m.MapWindow(4, "HDMI-A-1") {
		t.Fatal("could not give HDMI-A-1's first workspace a second column")
	}
	m.FocusWorkspace("DP-1", 1)
	first := workspaceIDs(m, "HDMI-A-1")[0]
	activeWindow := func() uint64 {
		for _, workspace := range m.Snapshot().Workspaces {
			if workspace.ID == first && workspace.ActiveWindowID != nil {
				return *workspace.ActiveWindowID
			}
		}
		return 0
	}
	if !m.FocusColumnLeftOnWorkspace(first) || activeWindow() != 1 {
		t.Fatalf("FocusColumnLeftOnWorkspace: active window %d, want 1", activeWindow())
	}
	if !m.FocusColumnRightOnWorkspace(first) || activeWindow() != 4 {
		t.Fatalf("FocusColumnRightOnWorkspace: active window %d, want 4", activeWindow())
	}
	if !m.FocusWorkspaceDownOn("HDMI-A-1") || activeIndex(t, m, "HDMI-A-1") != 1 {
		t.Fatalf("FocusWorkspaceDownOn: active %d, want 1", activeIndex(t, m, "HDMI-A-1"))
	}
	if !m.FocusWorkspaceUpOn("HDMI-A-1") || activeIndex(t, m, "HDMI-A-1") != 0 {
		t.Fatalf("FocusWorkspaceUpOn: active %d, want 0", activeIndex(t, m, "HDMI-A-1"))
	}
	if m.FocusWorkspaceUpOn("HDMI-A-1") {
		t.Fatal("FocusWorkspaceUpOn on the first workspace reported a change")
	}
	if got := m.Snapshot().FocusedOutput; got != "DP-1" {
		t.Fatalf("focused output %q after under-mouse actions on HDMI-A-1, want DP-1", got)
	}
}
