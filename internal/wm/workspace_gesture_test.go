package wm

import (
	"math"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func gestureOffset(t *testing.T, m *Model) float64 {
	t.Helper()
	offset, ok := m.WorkspaceRenderOffset("DP-1")
	if !ok {
		t.Fatal("no workspace render offset while switching")
	}
	return offset
}

// TestWorkspaceGestureFollowsAndSettlesAsNiri: Niri's touchpad workspace
// switch moves the workspaces with the fingers, 300 px a workspace, rubber
// banded one workspace past the start, and settles on the projected index.
func TestWorkspaceGestureFollowsAndSettlesAsNiri(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "DP-1")
	m.FocusWorkspace("", 1)

	if m.BeginWorkspaceGesture("HDMI-A-1") || m.UpdateWorkspaceGesture(10, 0, 1) {
		t.Fatal("a gesture began on a missing output")
	}
	if _, ok := m.WorkspaceRenderOffset("DP-1"); ok {
		t.Fatal("a render offset exists before any gesture")
	}
	if !m.BeginWorkspaceGesture("DP-1") {
		t.Fatal("BeginWorkspaceGesture failed")
	}
	if !m.UpdateWorkspaceGesture(150, 10*time.Millisecond, 1) || gestureOffset(t, m) != 0.5 {
		t.Fatalf("offset = %v, want half a workspace", gestureOffset(t, m))
	}
	if activeIndex(t, m, "DP-1") != 0 {
		t.Fatal("the gesture switched before it ended")
	}
	m.UpdateWorkspaceGesture(450, 20*time.Millisecond, 1)
	// Rubber band past +1: 1 + (1 - 1/(x*c/d + 1))*d with x=1, c=0.5, d=0.05.
	if got, want := gestureOffset(t, m), 1+(1-1/11.0)*0.05; math.Abs(got-want) > 1e-9 {
		t.Fatalf("rubber-banded offset = %v, want %v", got, want)
	}
	m.UpdateWorkspaceGesture(-360, 30*time.Millisecond, 1)
	if got := gestureOffset(t, m); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("offset = %v, want 0.8", got)
	}
	// A release long after the last motion has no velocity left.
	output, from, velocity, ok := m.EndWorkspaceGesture(2*time.Second, 1)
	if !ok || output != "DP-1" || math.Abs(from+0.2) > 1e-9 || velocity != 0 {
		t.Fatalf("EndWorkspaceGesture = %q, %v, %v, %v; want DP-1, -0.2, 0, true", output, from, velocity, ok)
	}
	if activeIndex(t, m, "DP-1") != 1 || math.Abs(gestureOffset(t, m)+0.2) > 1e-9 {
		t.Fatalf("active %d offset %v, want workspace 1 drawn 0.2 above", activeIndex(t, m, "DP-1"), gestureOffset(t, m))
	}

	// A new gesture catches the settling switch where it is drawn.
	if !m.BeginWorkspaceGesture("DP-1") || !m.UpdateWorkspaceGesture(60, 2100*time.Millisecond, 1) ||
		math.Abs(gestureOffset(t, m)) > 1e-9 {
		t.Fatalf("caught gesture offset = %v, want 0", gestureOffset(t, m))
	}
	// Released near its start, a gesture snaps back.
	if output, from, _, ok = m.EndWorkspaceGesture(4*time.Second, 1); !ok || from != 0 || activeIndex(t, m, "DP-1") != 1 {
		t.Fatalf("short release: %q from %v active %d", output, from, activeIndex(t, m, "DP-1"))
	}
	m.SetWorkspaceRenderOffset("DP-1", 0.25)
	if gestureOffset(t, m) != 0.25 {
		t.Fatal("SetWorkspaceRenderOffset did not apply")
	}
	m.ClearWorkspaceRenderOffset("DP-1")
	if _, ok := m.WorkspaceRenderOffset("DP-1"); ok {
		t.Fatal("render offset survived ClearWorkspaceRenderOffset")
	}
	if !m.FocusWorkspacePrevious() || activeIndex(t, m, "DP-1") != 0 {
		t.Fatal("the gesture switch did not record the previous workspace")
	}
}

func TestWorkspaceGestureFlingsAndClampsAtTheEnds(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "DP-1")
	m.FocusWorkspace("", 1)

	m.BeginWorkspaceGesture("DP-1")
	m.UpdateWorkspaceGesture(-300, 10*time.Millisecond, 1)
	if got, want := gestureOffset(t, m), -(1-1/11.0)*0.05; math.Abs(got-want) > 1e-9 {
		t.Fatalf("offset above the first workspace = %v, want %v", got, want)
	}
	m.EndWorkspaceGesture(2*time.Second, 1)
	if activeIndex(t, m, "DP-1") != 0 {
		t.Fatal("a swipe above the first workspace switched")
	}
	m.ClearWorkspaceRenderOffset("DP-1")

	// A quick flick a tenth of a workspace long projects past the middle,
	// and still moves one workspace only.
	m.BeginWorkspaceGesture("DP-1")
	for i := range 3 {
		m.UpdateWorkspaceGesture(10, time.Duration(10*i)*time.Millisecond, 1)
	}
	_, from, velocity, ok := m.EndWorkspaceGesture(20*time.Millisecond, 1)
	if !ok || activeIndex(t, m, "DP-1") != 1 || velocity <= 0 || math.Abs(from+0.9) > 1e-9 {
		t.Fatalf("flick: active %d from %v velocity %v", activeIndex(t, m, "DP-1"), from, velocity)
	}
}

// TestWorkspaceGestureInTheOverviewIsUnclamped: in the overview Niri lets a
// swipe cross several workspaces, and the overview draws the fraction.
func TestWorkspaceGestureInTheOverviewIsUnclamped(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "DP-1")
	m.FocusWorkspace("", 1)
	m.OpenOverview()

	m.BeginWorkspaceGesture("DP-1")
	m.UpdateWorkspaceGesture(450, 10*time.Millisecond, 1)
	if gestureOffset(t, m) != 1.5 {
		t.Fatalf("overview offset = %v, want 1.5", gestureOffset(t, m))
	}
	frame, ok := m.OverviewFrame("DP-1", 0.5)
	if !ok {
		t.Fatal("no overview frame")
	}
	first, third := frame.Workspaces[0].Bounds, frame.Workspaces[2].Bounds
	step := third.Y - frame.Workspaces[1].Bounds.Y
	if third.Y != first.Y+2*step {
		t.Fatalf("overview workspaces at %d and %d, want %d apart", first.Y, third.Y, 2*step)
	}
	if center := frame.Output.Y + (frame.Output.Height-third.Height)/2; third.Y-center != step/2 {
		t.Fatalf("third workspace at %d, want half a step below the centre %d", third.Y, center)
	}
	m.EndWorkspaceGesture(2*time.Second, 1)
	if activeIndex(t, m, "DP-1") != 2 {
		t.Fatalf("overview swipe settled on %d, want 2", activeIndex(t, m, "DP-1"))
	}
}

func TestDndWorkspaceGestureClampsAndUses1500Pixels(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	threeWorkspaces(t, m, "DP-1")
	m.FocusWorkspace("", 1)
	last := float64(len(m.Snapshot().Workspaces) - 1)
	m.OpenOverview()
	if !m.BeginWorkspaceDND("DP-1") {
		t.Fatal("could not begin DND")
	}
	m.UpdateWorkspaceGesture(750, time.Millisecond, .5)
	if got := gestureOffset(t, m); math.Abs(got-.5) > 1e-9 {
		t.Fatalf("750 DND pixels = %v workspaces, want 0.5", got)
	}
	m.UpdateWorkspaceGesture(100000, 2*time.Millisecond, .5)
	if got := gestureOffset(t, m); got != last {
		t.Fatalf("DND overscrolled the end: %v", got)
	}
	m.UpdateWorkspaceGesture(-150, 3*time.Millisecond, .5)
	if got := gestureOffset(t, m); math.Abs(got-(last-.1)) > 1e-9 {
		t.Fatalf("DND built up overscroll instead of reversing: %v", got)
	}
	_, _, velocity, ok := m.EndWorkspaceGesture(4*time.Millisecond, .5)
	if !ok || velocity != 0 || activeIndex(t, m, "DP-1") != int(last) {
		t.Fatalf("DND release: ok=%v velocity=%v active=%v", ok, velocity, activeIndex(t, m, "DP-1"))
	}
}

func TestDndWorkspaceEdgeUsesStripAndWorkingArea(t *testing.T) {
	bounds := layout.Bounds{X: 250, Y: 40, Width: 500, Height: 600}
	for _, tc := range []struct {
		x, y      float64
		direction int
		speed     float64
	}{
		{249, 0, 0, 0}, {750, 640, 0, 0}, {500, 0, -1, 1500}, {500, 40, -1, 1500}, {500, 65, -1, 750}, {500, 340, 0, 0}, {500, 640, 1, 1500},
	} {
		direction, speed := DNDEdgeWorkspaceSwitchAt(bounds, tc.x, tc.y, 50, 1500)
		if direction != tc.direction || speed != tc.speed {
			t.Errorf("at %v,%v got %v/%v want %v/%v", tc.x, tc.y, direction, speed, tc.direction, tc.speed)
		}
	}
	var tracker DNDEdgeScrollTracker
	start := time.Unix(0, 0)
	if got := tracker.UpdateWorkspace(start, bounds, 500, 640, 50, 20*time.Millisecond, 1500); got != 0 {
		t.Fatal(got)
	}
	if got := tracker.UpdateWorkspace(start.Add(20*time.Millisecond), bounds, 500, 640, 50, 20*time.Millisecond, 1500); got != 0 {
		t.Fatal(got)
	}
	if got := tracker.UpdateWorkspace(start.Add(40*time.Millisecond), bounds, 500, 640, 50, 20*time.Millisecond, 1500); got != 30 {
		t.Fatalf("post-dwell distance = %v want30", got)
	}
}
