package wm

import (
	"math"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

const (
	overviewZoomMin       = 0.0001
	overviewZoomMax       = 0.75
	overviewWorkspaceGap  = 0.1
	overviewGesturePixels = 300.0
	swipeAxisThreshold    = 16.0
)

type overviewState struct {
	open          bool
	progress      float64
	gestureActive bool
}

// OverviewFrame contains the output's workspace thumbnails in layout coordinates.
type OverviewFrame struct {
	Output     layout.Bounds
	Workspaces []OverviewWorkspace
}

// OverviewWorkspace is one workspace's projected rectangle and visible windows.
type OverviewWorkspace struct {
	ID      uint64
	Active  bool
	Bounds  layout.Bounds
	Windows []OverviewWindow
}

// OverviewWindow is a scaled window rectangle within an overview frame.
type OverviewWindow struct {
	ID      uint64
	Bounds  layout.Bounds
	Visible bool
}

// OverviewHit identifies a workspace and, when WindowID is nonzero, a window.
type OverviewHit struct {
	WorkspaceID uint64
	WindowID    uint64
}

// OpenOverview opens the overview and reports whether its IPC state changed.
func (m *Model) OpenOverview() bool {
	return m.setOverviewOpen(true)
}

// CloseOverview closes the overview and reports whether its IPC state changed.
func (m *Model) CloseOverview() bool {
	return m.setOverviewOpen(false)
}

// ToggleOverview switches the overview's open state.
func (m *Model) ToggleOverview() bool {
	return m.setOverviewOpen(!m.overview.open)
}

func (m *Model) setOverviewOpen(open bool) bool {
	changed := m.overview.open != open
	m.overview.open = open
	m.overview.progress = 0
	if open {
		m.overview.progress = 1
	}
	m.overview.gestureActive = false
	return changed
}

// OverviewProgress returns the current direct-render progress in [0, 1].
func (m *Model) OverviewProgress() float64 {
	return m.overview.progress
}

// BeginOverviewGesture starts a four-finger overview gesture at the current progress.
func (m *Model) BeginOverviewGesture() {
	m.overview.gestureActive = true
	m.overview.open = true
}

// UpdateOverviewGesture applies vertical logical pixels to the active gesture.
func (m *Model) UpdateOverviewGesture(deltaY float64) bool {
	if !m.overview.gestureActive || math.IsNaN(deltaY) || math.IsInf(deltaY, 0) {
		return false
	}
	progress := clampOverviewProgress(m.overview.progress + deltaY/overviewGesturePixels)
	if progress == m.overview.progress {
		return false
	}
	m.overview.progress = progress
	return true
}

// EndOverviewGesture settles to the nearest open/closed state by gesture progress.
func (m *Model) EndOverviewGesture() bool {
	if !m.overview.gestureActive {
		return false
	}
	m.overview.gestureActive = false
	m.overview.open = m.overview.progress >= 0.5
	if m.overview.open {
		m.overview.progress = 1
	} else {
		m.overview.progress = 0
	}
	return true
}

// OverviewFrame projects an output's workspaces and window placements at zoom.
func (m *Model) OverviewFrame(output string, zoom float64) (OverviewFrame, bool) {
	out := m.outputs[output]
	if out == nil {
		return OverviewFrame{}, false
	}
	zoom = validOverviewZoom(zoom)
	outputBounds := out.bounds
	outputBounds.Width = max(0, outputBounds.Width)
	outputBounds.Height = max(0, outputBounds.Height)
	workspaceWidth := max(0, int(math.Round(float64(outputBounds.Width)*zoom)))
	workspaceHeight := max(0, int(math.Round(float64(outputBounds.Height)*zoom)))
	gap := max(1, int(math.Round(float64(outputBounds.Height)*overviewWorkspaceGap*zoom)))
	workspaceX := outputBounds.X + (outputBounds.Width-workspaceWidth)/2
	activeY := outputBounds.Y + (outputBounds.Height-workspaceHeight)/2

	frame := OverviewFrame{Output: outputBounds, Workspaces: make([]OverviewWorkspace, 0, len(out.workspaces))}
	for index, workspace := range out.workspaces {
		bounds := layout.Bounds{
			X:     workspaceX,
			Y:     activeY + (index-out.active)*(workspaceHeight+gap),
			Width: workspaceWidth, Height: workspaceHeight,
		}
		projected := OverviewWorkspace{ID: workspace.id, Active: index == out.active, Bounds: bounds}
		for _, placement := range workspace.strip.Placements() {
			projected.Windows = append(projected.Windows, OverviewWindow{
				ID: placement.ID, Visible: placement.Visible,
				Bounds: layout.Bounds{
					X:      bounds.X + int(math.Round(float64(placement.X-outputBounds.X)*zoom)),
					Y:      bounds.Y + int(math.Round(float64(placement.Y-outputBounds.Y)*zoom)),
					Width:  max(0, int(math.Round(float64(placement.Width)*zoom))),
					Height: max(0, int(math.Round(float64(placement.Height)*zoom))),
				},
			})
		}
		frame.Workspaces = append(frame.Workspaces, projected)
	}
	return frame, true
}

// OverviewHitTest returns the topmost visible window or empty workspace under a point.
func (m *Model) OverviewHitTest(output string, x, y, zoom float64) (OverviewHit, bool) {
	frame, ok := m.OverviewFrame(output, zoom)
	if !ok || !pointInBounds(x, y, frame.Output) {
		return OverviewHit{}, false
	}
	for workspaceIndex := len(frame.Workspaces) - 1; workspaceIndex >= 0; workspaceIndex-- {
		workspace := frame.Workspaces[workspaceIndex]
		if !pointInBounds(x, y, workspace.Bounds) {
			continue
		}
		for windowIndex := len(workspace.Windows) - 1; windowIndex >= 0; windowIndex-- {
			window := workspace.Windows[windowIndex]
			if window.Visible && pointInBounds(x, y, window.Bounds) {
				return OverviewHit{WorkspaceID: workspace.ID, WindowID: window.ID}, true
			}
		}
		return OverviewHit{WorkspaceID: workspace.ID}, true
	}
	return OverviewHit{}, false
}

// SelectOverviewAt focuses the hit window or workspace and closes overview.
func (m *Model) SelectOverviewAt(output string, x, y, zoom float64) bool {
	if !m.overview.open {
		return false
	}
	hit, ok := m.OverviewHitTest(output, x, y, zoom)
	if !ok {
		return false
	}
	if hit.WindowID != 0 {
		if !m.FocusWindow(hit.WindowID) {
			return false
		}
	} else if !m.FocusWorkspaceID(hit.WorkspaceID) {
		return false
	}
	m.CloseOverview()
	return true
}

// SwipeAxis is the dominant direction selected for a three-finger swipe.
type SwipeAxis uint8

const (
	SwipeAxisNone SwipeAxis = iota
	SwipeAxisHorizontal
	SwipeAxisVertical
)

// SwipeAxisSelector waits for a 16 logical pixel displacement before locking an axis.
type SwipeAxisSelector struct {
	x, y float64
	axis SwipeAxis
}

// Add accumulates updates until one axis is selected, then returns that axis's delta.
func (s *SwipeAxisSelector) Add(dx, dy float64) (axis SwipeAxis, delta float64, selected bool) {
	if s.axis == SwipeAxisNone {
		s.x += dx
		s.y += dy
		if math.Max(math.Abs(s.x), math.Abs(s.y)) < swipeAxisThreshold {
			return SwipeAxisNone, 0, false
		}
		if math.Abs(s.x) >= math.Abs(s.y) {
			s.axis = SwipeAxisHorizontal
			return s.axis, s.x, true
		}
		s.axis = SwipeAxisVertical
		return s.axis, s.y, true
	}
	if s.axis == SwipeAxisHorizontal {
		return s.axis, dx, true
	}
	return s.axis, dy, true
}

// HotCorners is the set of configured one-logical-pixel overview trigger corners.
type HotCorners struct {
	TopLeft, TopRight, BottomLeft, BottomRight bool
	Off                                        bool
}

// HotCorner identifies a corner region on one output.
type HotCorner uint8

const (
	HotCornerNone HotCorner = iota
	HotCornerTopLeft
	HotCornerTopRight
	HotCornerBottomLeft
	HotCornerBottomRight
)

// HotCornerTracker reports a single transition when the pointer enters a configured corner.
type HotCornerTracker struct {
	output string
	corner HotCorner
	inside bool
}

// Update detects corner entry using output-global logical coordinates.
func (t *HotCornerTracker) Update(output string, bounds layout.Bounds, x, y float64, configured HotCorners) bool {
	corner := hotCornerAt(bounds, x, y)
	if configured.Off || !hotCornerEnabled(configured, corner) || corner == HotCornerNone {
		t.output, t.corner, t.inside = "", HotCornerNone, false
		return false
	}
	if t.inside && t.output == output && t.corner == corner {
		return false
	}
	t.output, t.corner, t.inside = output, corner, true
	return true
}

func hotCornerEnabled(corners HotCorners, corner HotCorner) bool {
	selected := corners.TopLeft || corners.TopRight || corners.BottomLeft || corners.BottomRight
	if !selected {
		return corner == HotCornerTopLeft
	}
	switch corner {
	case HotCornerTopLeft:
		return corners.TopLeft
	case HotCornerTopRight:
		return corners.TopRight
	case HotCornerBottomLeft:
		return corners.BottomLeft
	case HotCornerBottomRight:
		return corners.BottomRight
	default:
		return false
	}
}

func hotCornerAt(bounds layout.Bounds, x, y float64) HotCorner {
	if bounds.Width <= 0 || bounds.Height <= 0 ||
		x < float64(bounds.X) || x >= float64(bounds.X+bounds.Width) ||
		y < float64(bounds.Y) || y >= float64(bounds.Y+bounds.Height) {
		return HotCornerNone
	}
	left := x < float64(bounds.X)+1
	right := x >= float64(bounds.X+bounds.Width)-1
	top := y < float64(bounds.Y)+1
	bottom := y >= float64(bounds.Y+bounds.Height)-1
	switch {
	case top && left:
		return HotCornerTopLeft
	case top && right:
		return HotCornerTopRight
	case bottom && left:
		return HotCornerBottomLeft
	case bottom && right:
		return HotCornerBottomRight
	default:
		return HotCornerNone
	}
}

func validOverviewZoom(zoom float64) float64 {
	if math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return 0.5
	}
	return min(max(zoom, overviewZoomMin), overviewZoomMax)
}

func clampOverviewProgress(progress float64) float64 {
	return min(max(progress, 0), 1)
}

func pointInBounds(x, y float64, bounds layout.Bounds) bool {
	return x >= float64(bounds.X) && x < float64(bounds.X+bounds.Width) &&
		y >= float64(bounds.Y) && y < float64(bounds.Y+bounds.Height)
}
