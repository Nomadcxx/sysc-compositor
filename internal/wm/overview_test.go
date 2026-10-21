package wm

import (
	"math"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestOverviewOpenCloseState(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)

	if m.Snapshot().OverviewOpen {
		t.Fatal("new model reports overview open")
	}
	if !m.OpenOverview() || !m.Snapshot().OverviewOpen {
		t.Fatal("OpenOverview() did not open overview")
	}
	if m.OpenOverview() {
		t.Fatal("OpenOverview() reported a second state change")
	}
	if !m.CloseOverview() || m.Snapshot().OverviewOpen {
		t.Fatal("CloseOverview() did not close overview")
	}
	if m.CloseOverview() {
		t.Fatal("CloseOverview() reported a second state change")
	}
	if !m.ToggleOverview() || !m.Snapshot().OverviewOpen {
		t.Fatal("ToggleOverview() did not open overview")
	}
}

func TestOverviewFrameCentersActiveWorkspaceAndScalesWindows(t *testing.T) {
	m := New()
	output := layout.Bounds{X: -1920, Y: 100, Width: 1200, Height: 800}
	m.AddOutput("DP-1", output, 500, 16)
	if !m.MapWindow(1, "DP-1") || !m.FocusWorkspace("DP-1", 2) || !m.MapWindow(2, "DP-1") {
		t.Fatal("could not create two populated workspaces")
	}

	frame, ok := m.OverviewFrame("DP-1", 0.5)
	if !ok {
		t.Fatal("OverviewFrame() failed for connected output")
	}
	if len(frame.Workspaces) != 3 {
		t.Fatalf("overview workspaces = %d, want 3 including the trailing empty workspace", len(frame.Workspaces))
	}

	active := frame.Workspaces[1]
	wantActive := layout.Bounds{X: -1620, Y: 300, Width: 600, Height: 400}
	if active.Bounds != wantActive || !active.Active {
		t.Fatalf("active workspace = %+v active=%v, want %+v centered on output", active.Bounds, active.Active, wantActive)
	}
	if frame.Workspaces[0].Bounds.Y != -140 || frame.Workspaces[2].Bounds.Y != 740 {
		t.Fatalf("neighbor workspace y positions = %d,%d, want -140,740", frame.Workspaces[0].Bounds.Y, frame.Workspaces[2].Bounds.Y)
	}
	if len(active.Windows) != 1 {
		t.Fatalf("active workspace windows = %d, want 1", len(active.Windows))
	}
	window := active.Windows[0]
	if window.ID != 2 || window.Bounds.X != -1612 || window.Bounds.Y != 308 || window.Bounds.Width != 250 {
		t.Fatalf("projected window = %+v, want window 2 at (-1612,308) with width 250", window)
	}
}

func TestOverviewHitTestFindsWindowAndEmptyWorkspace(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{X: -1000, Y: -200, Width: 1000, Height: 800}, 500, 16)
	if !m.MapWindow(1, "DP-1") || !m.FocusWorkspace("DP-1", 2) || !m.MapWindow(2, "DP-1") {
		t.Fatal("could not create overview test scene")
	}
	if !m.OpenOverview() {
		t.Fatal("could not open overview")
	}

	frame, _ := m.OverviewFrame("DP-1", 0.5)
	workspace := frame.Workspaces[1]
	window := workspace.Windows[0]
	hit, ok := m.OverviewHitTest("DP-1", float64(window.Bounds.X+window.Bounds.Width/2), float64(window.Bounds.Y+window.Bounds.Height/2), 0.5)
	if !ok || hit.WorkspaceID != workspace.ID || hit.WindowID != 2 {
		t.Fatalf("window hit = %+v,%v, want workspace %d window 2", hit, ok, workspace.ID)
	}

	empty := frame.Workspaces[2]
	x := float64(empty.Bounds.X + empty.Bounds.Width - 10)
	y := float64(min(empty.Bounds.Y+empty.Bounds.Height/2, -200+800-1))
	hit, ok = m.OverviewHitTest("DP-1", x, y, 0.5)
	if !ok || hit.WorkspaceID != empty.ID || hit.WindowID != 0 {
		t.Fatalf("empty-space hit = %+v,%v, want workspace %d without a window", hit, ok, empty.ID)
	}
	if !m.SelectOverviewAt("DP-1", x, y, 0.5) || m.Snapshot().OverviewOpen {
		t.Fatal("empty-space selection did not focus its workspace and close overview")
	}
	if !m.Snapshot().Workspaces[2].IsActive {
		t.Fatal("empty-space selection did not activate the hit workspace")
	}
}

func TestOverviewWindowSelectionFocusesWindowAndCloses(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.MapWindow(1, "DP-1") || !m.FocusWorkspace("DP-1", 2) || !m.MapWindow(2, "DP-1") || !m.OpenOverview() {
		t.Fatal("could not create overview test scene")
	}
	frame, _ := m.OverviewFrame("DP-1", 0.5)
	window := frame.Workspaces[1].Windows[0]
	x := float64(window.Bounds.X + window.Bounds.Width/2)
	y := float64(window.Bounds.Y + window.Bounds.Height/2)
	if !m.SelectOverviewAt("DP-1", x, y, 0.5) {
		t.Fatal("SelectOverviewAt() failed for a projected window")
	}
	snapshot := m.Snapshot()
	if snapshot.OverviewOpen || !snapshot.Workspaces[1].IsActive || snapshot.Workspaces[1].ActiveWindowID == nil || *snapshot.Workspaces[1].ActiveWindowID != 2 {
		t.Fatalf("state after window selection = overview %v, workspaces %+v", snapshot.OverviewOpen, snapshot.Workspaces)
	}
}

func TestThreeFingerSwipeSelectsDominantAxisAtSixteenPixels(t *testing.T) {
	var swipe SwipeAxisSelector
	if axis, delta, selected := swipe.Add(8, 5); selected || axis != SwipeAxisNone || delta != 0 {
		t.Fatalf("first swipe update = %v,%v,%v, want undecided", axis, delta, selected)
	}
	if axis, delta, selected := swipe.Add(8, 2); !selected || axis != SwipeAxisHorizontal || delta != 16 {
		t.Fatalf("threshold swipe update = %v,%v,%v, want horizontal 16", axis, delta, selected)
	}
	if axis, delta, selected := swipe.Add(-3, 40); !selected || axis != SwipeAxisHorizontal || delta != -3 {
		t.Fatalf("locked-axis update = %v,%v,%v, want horizontal -3", axis, delta, selected)
	}

	var vertical SwipeAxisSelector
	if axis, delta, selected := vertical.Add(3, -16); !selected || axis != SwipeAxisVertical || delta != -16 {
		t.Fatalf("vertical threshold update = %v,%v,%v, want vertical -16", axis, delta, selected)
	}
}

func TestOverviewGestureTracksProgressAndSettles(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.BeginOverviewGesture()
	if !m.Snapshot().OverviewOpen {
		t.Fatal("overview should be open while a four-finger gesture is active")
	}
	if !m.UpdateOverviewGesture(150) || m.OverviewProgress() != 0.5 {
		t.Fatalf("overview progress = %v, want 0.5", m.OverviewProgress())
	}
	if !m.EndOverviewGesture() || !m.Snapshot().OverviewOpen || m.OverviewProgress() != 1 {
		t.Fatalf("gesture did not settle open: open=%v progress=%v", m.Snapshot().OverviewOpen, m.OverviewProgress())
	}

	m.BeginOverviewGesture()
	if !m.UpdateOverviewGesture(-151) || math.Abs(m.OverviewProgress()-0.4966666666666667) > 1e-9 {
		t.Fatalf("closing gesture progress = %v, want about 0.4967", m.OverviewProgress())
	}
	if !m.EndOverviewGesture() || m.Snapshot().OverviewOpen || m.OverviewProgress() != 0 {
		t.Fatalf("gesture did not settle closed: open=%v progress=%v", m.Snapshot().OverviewOpen, m.OverviewProgress())
	}
}

func TestHotCornerEntryDefaultsExplicitOffAndReentry(t *testing.T) {
	left := layout.Bounds{X: -1000, Y: 0, Width: 1000, Height: 800}
	right := layout.Bounds{X: 0, Y: 0, Width: 1000, Height: 800}
	var tracker HotCornerTracker

	if !tracker.Update("left", left, -1000, 0, HotCorners{}) || tracker.Update("left", left, -999.5, 0.5, HotCorners{}) {
		t.Fatal("default top-left corner should trigger only once while entered")
	}
	if tracker.Update("left", left, -998, 0, HotCorners{}) || !tracker.Update("left", left, -1000, 0, HotCorners{}) {
		t.Fatal("leaving and re-entering the default corner should trigger again")
	}

	selected := HotCorners{TopRight: true}
	if tracker.Update("right", right, 0, 0, selected) {
		t.Fatal("explicit top-right selection also triggered the top-left corner")
	}
	if !tracker.Update("right", right, 999.5, 0.5, selected) {
		t.Fatal("explicit top-right corner did not trigger")
	}
	if tracker.Update("right", right, 999.5, 0.5, HotCorners{TopRight: true, Off: true}) {
		t.Fatal("off did not disable the explicitly selected corner")
	}
	if tracker.Update("right", right, 999.5, 0.5, HotCorners{Off: true}) {
		t.Fatal("off did not disable the default top-left corner")
	}
}

func TestHotCornerEntryOnAnotherOutputIsNewEntry(t *testing.T) {
	var tracker HotCornerTracker
	first := layout.Bounds{X: 0, Width: 1000, Height: 800}
	second := layout.Bounds{X: 1000, Width: 1000, Height: 800}
	if !tracker.Update("DP-1", first, 0, 0, HotCorners{}) {
		t.Fatal("first output corner did not trigger")
	}
	if !tracker.Update("DP-2", second, 1000, 0, HotCorners{}) {
		t.Fatal("entering the same corner on a different output did not trigger")
	}
}
