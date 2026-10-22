package wm

import (
	"math"
	"testing"
	"time"

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
	var diagonal SwipeAxisSelector
	if axis, _, selected := diagonal.Add(12, 12); !selected || axis != SwipeAxisVertical {
		t.Fatalf("diagonal threshold update = %v,%v, want Niri's vertical tie break", axis, selected)
	}
}

func TestOverviewGestureTracksProgressAndSettles(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.BeginOverviewGesture()
	if !m.Snapshot().OverviewOpen {
		t.Fatal("overview should be open while a four-finger gesture is active")
	}
	if !m.UpdateOverviewGesture(150, 100*time.Millisecond) || m.OverviewProgress() != 0.5 {
		t.Fatalf("overview progress = %v, want 0.5", m.OverviewProgress())
	}
	if !m.EndOverviewGesture(110*time.Millisecond) || !m.Snapshot().OverviewOpen || m.OverviewProgress() != 1 {
		t.Fatalf("gesture did not settle open: open=%v progress=%v", m.Snapshot().OverviewOpen, m.OverviewProgress())
	}

	m.BeginOverviewGesture()
	if !m.UpdateOverviewGesture(-151, 200*time.Millisecond) || math.Abs(m.OverviewProgress()-0.4966666666666667) > 1e-9 {
		t.Fatalf("closing gesture progress = %v, want about 0.4967", m.OverviewProgress())
	}
	if !m.EndOverviewGesture(210*time.Millisecond) || m.Snapshot().OverviewOpen || m.OverviewProgress() != 0 {
		t.Fatalf("gesture did not settle closed: open=%v progress=%v", m.Snapshot().OverviewOpen, m.OverviewProgress())
	}
}

func TestFinishOverviewGesturePreservesProgressAndReleaseVelocity(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1920, Height: 1080}, 800, 16)
	m.BeginOverviewGesture()
	if !m.UpdateOverviewGesture(150, 100*time.Millisecond) {
		t.Fatal("UpdateOverviewGesture() = false")
	}
	from, to, velocity, ok := m.FinishOverviewGestureAnimation(110 * time.Millisecond)
	if !ok || from != 0.5 || to != 1 || velocity <= 0 {
		t.Fatalf("FinishOverviewGestureAnimation() = (%v, %v, %v, %v), want (0.5, 1, positive, true)", from, to, velocity, ok)
	}
	if got := m.OverviewProgress(); got != from {
		t.Errorf("OverviewProgress() after finish = %v, want gesture progress %v", got, from)
	}
	if !m.OverviewOpen() {
		t.Error("OverviewOpen() = false, want the target state committed")
	}
}

func TestBeginOverviewGestureAtUsesPresentedProgress(t *testing.T) {
	m := New()
	m.BeginOverviewGestureAt(0.3)
	if got := m.OverviewProgress(); got != 0.3 {
		t.Fatalf("OverviewProgress() = %v, want 0.3", got)
	}
	if !m.UpdateOverviewGesture(60, 100*time.Millisecond) || m.OverviewProgress() != 0.5 {
		t.Fatalf("OverviewProgress() after gesture update = %v, want 0.5", m.OverviewProgress())
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

func TestDndEdgeScrollHonorsDwellSpeedAndExit(t *testing.T) {
	bounds := layout.Bounds{X: -1000, Y: 100, Width: 1000, Height: 800}
	delay := 100 * time.Millisecond
	start := time.Unix(10, 0)
	var tracker DNDEdgeScrollTracker
	if delta := tracker.Update(start, bounds, -1000, 500, 30, delay, 1500); delta != 0 {
		t.Fatalf("initial edge update = %v, want no scroll", delta)
	}
	if delta := tracker.Update(start.Add(99*time.Millisecond), bounds, -1000, 500, 30, delay, 1500); delta != 0 {
		t.Fatalf("pre-dwell update = %v, want no scroll", delta)
	}
	if delta := tracker.Update(start.Add(100*time.Millisecond), bounds, -1000, 500, 30, delay, 1500); delta != 0 {
		t.Fatalf("first post-dwell update = %v, want timer baseline", delta)
	}
	if delta := tracker.Update(start.Add(120*time.Millisecond), bounds, -1000, 500, 30, delay, 1500); math.Abs(delta+30) > 1e-9 {
		t.Fatalf("left-edge delta = %v, want -30 at the speed cap", delta)
	}
	if delta := tracker.Update(start.Add(140*time.Millisecond), bounds, -990, 500, 30, delay, 1500); delta >= 0 || math.Abs(delta) >= 30 {
		t.Fatalf("near-edge delta = %v, want slower leftward scroll", delta)
	}
	if delta := tracker.Update(start.Add(160*time.Millisecond), bounds, -900, 500, 30, delay, 1500); delta != 0 {
		t.Fatalf("edge exit delta = %v, want no scroll", delta)
	}
}

func TestFocusWorkspaceByDeltaClampsToAvailableWorkspaces(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.MapWindow(1, "DP-1") {
		t.Fatal("could not map first window")
	}
	workspaces := m.Snapshot().Workspaces
	if len(workspaces) != 2 || !m.MapWindowToWorkspace(2, workspaces[1].ID) || !m.FocusWorkspace("DP-1", 1) {
		t.Fatal("could not create three workspaces")
	}
	if !m.FocusWorkspaceByDelta("DP-1", 1) {
		t.Fatal("FocusWorkspaceByDelta(+1) did not switch to the second workspace")
	}
	if !m.FocusWorkspaceByDelta("DP-1", 1) {
		t.Fatal("FocusWorkspaceByDelta(+1) did not switch to the trailing workspace")
	}
	if m.FocusWorkspaceByDelta("DP-1", 1) {
		t.Fatal("FocusWorkspaceByDelta moved beyond the trailing workspace")
	}
	if !m.FocusWorkspaceByDelta("DP-1", -2) {
		t.Fatal("FocusWorkspaceByDelta(-2) did not return to the first workspace")
	}
}

func TestWindowDragKeepsPlacementUntilReleaseAndCancellationRestoresIt(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 300, 16)
	for id := uint64(1); id <= 3; id++ {
		if !m.MapWindow(id, "DP-1") {
			t.Fatalf("MapWindow(%d) failed", id)
		}
	}
	before := m.Snapshot().Windows
	if !m.BeginWindowDrag(1, 100, 100) {
		t.Fatal("BeginWindowDrag failed")
	}
	if drag, started := m.UpdateWindowDrag(107, 100, "DP-1"); started || drag.Started {
		t.Fatalf("drag crossed threshold early: %+v", drag)
	}
	if drag, started := m.UpdateWindowDrag(108, 100, "DP-1"); !started || !drag.Started || drag.Target == nil {
		t.Fatalf("drag target after threshold = %+v, started %v", drag, started)
	}
	if got := m.Snapshot().Windows; !sameWindowPlacements(got, before) {
		t.Fatalf("placement changed before release: %+v, want %+v", got, before)
	}
	if !m.CancelWindowDrag() || !sameWindowPlacements(m.Snapshot().Windows, before) {
		t.Fatal("cancelling drag changed its committed placement")
	}
	if !m.BeginWindowDrag(1, 100, 100) {
		t.Fatal("second BeginWindowDrag failed")
	}
	if _, started := m.UpdateWindowDrag(108, 100, "DP-1"); !started {
		t.Fatal("second drag did not cross its threshold")
	}
	if _, started := m.UpdateWindowDrag(995, 400, "DP-1"); !started || !m.CommitWindowDrag() {
		t.Fatal("CommitWindowDrag failed")
	}
	after := m.Snapshot().Windows
	if len(after) != 3 || after[0].ID != 2 || after[1].ID != 3 || after[2].ID != 1 {
		t.Fatalf("window order after release = %+v, want [2,3,1]", after)
	}
}

func sameWindowPlacements(left, right []WindowSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].WorkspaceID != right[index].WorkspaceID || left[index].Tile != right[index].Tile {
			return false
		}
	}
	return true
}
