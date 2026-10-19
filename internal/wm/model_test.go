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
