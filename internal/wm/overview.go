package wm

import (
	"math"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

const (
	overviewZoomMin        = 0.0001
	overviewZoomMax        = 0.75
	overviewWorkspaceGap   = 0.1
	overviewGesturePixels  = 300.0
	overviewGestureDecay   = 0.997
	overviewGestureHistory = 150 * time.Millisecond
	swipeAxisThreshold     = 16.0
)

type overviewState struct {
	open          bool
	progress      float64
	gestureActive bool
	gestureStart  float64
	gesture       overviewGestureTracker
}

type overviewGestureEvent struct {
	delta float64
	time  time.Duration
}

type overviewGestureTracker struct {
	history  []overviewGestureEvent
	position float64
}

func (t *overviewGestureTracker) push(delta float64, timestamp time.Duration) {
	if len(t.history) != 0 && timestamp < t.history[len(t.history)-1].time {
		return
	}
	t.history = append(t.history, overviewGestureEvent{delta: delta, time: timestamp})
	t.position += delta
	for len(t.history) > 1 && timestamp > t.history[0].time+overviewGestureHistory {
		t.history = t.history[1:]
	}
}

func (t *overviewGestureTracker) velocity() float64 {
	if len(t.history) < 2 {
		return 0
	}
	first, last := t.history[0], t.history[len(t.history)-1]
	seconds := (last.time - first.time).Seconds()
	if seconds <= 0 {
		return 0
	}
	var delta float64
	for _, event := range t.history {
		delta += event.delta
	}
	return delta / seconds
}

func (t *overviewGestureTracker) projectedEndPosition() float64 {
	return t.position - t.velocity()/(1000*math.Log(overviewGestureDecay))
}

const windowDragThreshold = 8.0

// WindowDragTarget describes the proposed insertion point and its hint bounds.
type WindowDragTarget struct {
	WorkspaceID uint64
	Position    layout.InsertPosition
	Hint        layout.Bounds
}

// WindowDrag is transient pointer state; placements remain committed until release.
type WindowDrag struct {
	WindowID       uint64
	StartX, StartY float64
	Started        bool
	Target         *WindowDragTarget
}

// BeginWindowDrag starts recognizing a tiled move without changing placement.
func (m *Model) BeginWindowDrag(id uint64, x, y float64) bool {
	state, exists := m.windows[id]
	if !exists || m.windowDrag != nil {
		return false
	}
	workspace := m.workspaceByID(state.workspaceID)
	if workspace == nil {
		return false
	}
	placement, ok := workspace.strip.Placement(id)
	if !ok || placement.Floating {
		return false
	}
	m.windowDrag = &WindowDrag{WindowID: id, StartX: x, StartY: y}
	return true
}

// UpdateWindowDrag recognizes movement after eight logical pixels and tracks its target.
func (m *Model) UpdateWindowDrag(x, y float64, output string) (WindowDrag, bool) {
	if m.windowDrag == nil {
		return WindowDrag{}, false
	}
	drag := m.windowDrag
	dx, dy := x-drag.StartX, y-drag.StartY
	if !drag.Started && dx*dx+dy*dy < windowDragThreshold*windowDragThreshold {
		return cloneWindowDrag(*drag), false
	}
	drag.Started = true
	state := m.windows[drag.WindowID]
	if output == "" {
		output = state.output
	}
	targetOutput := m.outputs[output]
	if targetOutput == nil || len(targetOutput.workspaces) == 0 {
		drag.Target = nil
		return cloneWindowDrag(*drag), true
	}
	workspace := targetOutput.workspaces[targetOutput.active]
	position := workspace.strip.InsertionPositionAt(x, y)
	target := WindowDragTarget{WorkspaceID: workspace.id, Position: position,
		Hint: workspace.strip.InsertHintBounds(position)}
	drag.Target = &target
	return cloneWindowDrag(*drag), true
}

// WindowDragState returns a copy of current transient drag state.
func (m *Model) WindowDragState() (WindowDrag, bool) {
	if m.windowDrag == nil {
		return WindowDrag{}, false
	}
	return cloneWindowDrag(*m.windowDrag), true
}

// CancelWindowDrag discards the proposed target and leaves committed placements untouched.
func (m *Model) CancelWindowDrag() bool {
	if m.windowDrag == nil {
		return false
	}
	m.windowDrag = nil
	return true
}

// CommitWindowDrag applies the proposed tiled insertion on release.
func (m *Model) CommitWindowDrag() bool {
	drag := m.windowDrag
	m.windowDrag = nil
	if drag == nil || !drag.Started || drag.Target == nil {
		return false
	}
	state, exists := m.windows[drag.WindowID]
	if !exists {
		return false
	}
	sourceOutput := m.outputs[state.output]
	source := m.workspaceByID(state.workspaceID)
	targetOutput, targetIndex, target := m.workspaceLocation(drag.Target.WorkspaceID)
	if sourceOutput == nil || source == nil || targetOutput == nil || target == nil ||
		!source.strip.MoveWindowToPosition(target.strip, drag.WindowID, drag.Target.Position) {
		return false
	}
	m.windows[drag.WindowID] = windowState{output: targetOutput.name, workspaceID: target.id}
	targetOutput.active = targetIndex
	m.focusedOutput = targetOutput.name
	m.reconcile(sourceOutput)
	if sourceOutput != targetOutput {
		m.reconcile(targetOutput)
	}
	m.clearFocusedUrgency()
	return true
}

func cloneWindowDrag(drag WindowDrag) WindowDrag {
	if drag.Target != nil {
		target := *drag.Target
		drag.Target = &target
	}
	return drag
}

// OverviewFrame contains the output's workspace thumbnails in layout coordinates.
type OverviewFrame struct {
	Output     layout.Bounds
	Zoom       float64
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
	m.overview.gesture = overviewGestureTracker{}
	return changed
}

// OverviewProgress returns the current direct-render progress in [0, 1].
func (m *Model) OverviewProgress() float64 {
	return m.overview.progress
}

// OverviewOpen reports the overview's current target state.
func (m *Model) OverviewOpen() bool { return m.overview.open }

// OverviewZoom returns the configured zoom interpolated by the current gesture progress.
func (m *Model) OverviewZoom(zoom float64) float64 {
	zoom = validOverviewZoom(zoom)
	progress := clampOverviewProgress(m.overview.progress)
	return 1 - progress*(1-zoom)
}

// BeginOverviewGesture starts a four-finger overview gesture at the current progress.
func (m *Model) BeginOverviewGesture() {
	m.overview.gestureActive = true
	m.overview.gestureStart = m.overview.progress
	m.overview.gesture = overviewGestureTracker{}
	m.overview.open = true
}

// UpdateOverviewGesture applies vertical logical pixels to the active gesture.
func (m *Model) UpdateOverviewGesture(deltaY float64, timestamp time.Duration) bool {
	if !m.overview.gestureActive || math.IsNaN(deltaY) || math.IsInf(deltaY, 0) {
		return false
	}
	m.overview.gesture.push(deltaY, timestamp)
	progress := clampOverviewProgress(m.overview.gestureStart + m.overview.gesture.position/overviewGesturePixels)
	if progress == m.overview.progress {
		return false
	}
	m.overview.progress = progress
	return true
}

// EndOverviewGesture settles to the open or closed state projected from recent swipe velocity.
func (m *Model) EndOverviewGesture(timestamp time.Duration) bool {
	if !m.overview.gestureActive {
		return false
	}
	m.overview.gesture.push(0, timestamp)
	m.overview.gestureActive = false
	projected := m.overview.gestureStart + m.overview.gesture.projectedEndPosition()/overviewGesturePixels
	m.overview.open = clampOverviewProgress(projected) >= 0.5
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

	frame := OverviewFrame{Output: outputBounds, Zoom: zoom, Workspaces: make([]OverviewWorkspace, 0, len(out.workspaces))}
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

// OverviewWorkspaceUnder returns the overview workspace row under a point, extending its horizontal hit region to the output width.
func (m *Model) OverviewWorkspaceUnder(output string, x, y, zoom float64) (uint64, layout.Bounds, bool) {
	frame, ok := m.OverviewFrame(output, zoom)
	if !ok || !pointInBounds(x, y, frame.Output) {
		return 0, layout.Bounds{}, false
	}
	for index := len(frame.Workspaces) - 1; index >= 0; index-- {
		workspace := frame.Workspaces[index]
		bounds := workspace.Bounds
		bounds.X, bounds.Width = frame.Output.X, frame.Output.Width
		if pointInBounds(x, y, bounds) {
			return workspace.ID, bounds, true
		}
	}
	return 0, layout.Bounds{}, false
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
		if math.Hypot(s.x, s.y) < swipeAxisThreshold {
			return SwipeAxisNone, 0, false
		}
		if math.Abs(s.x) > math.Abs(s.y) {
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

// DNDEdgeScrollTracker enforces edge dwell and returns signed logical pixels.
type DNDEdgeScrollTracker struct {
	direction int
	speed     float64
	started   time.Time
	last      time.Time
	active    bool
	scrolling bool
}

// Update reports the scroll distance since its previous call after edge dwell.
func (t *DNDEdgeScrollTracker) Update(now time.Time, bounds layout.Bounds, x, y, width float64, delay time.Duration, maxSpeed float64) float64 {
	direction, speed := DNDEdgeScrollAt(bounds, x, y, width, maxSpeed)
	if direction == 0 || speed <= 0 {
		t.Reset()
		return 0
	}
	if !t.active || direction != t.direction {
		*t = DNDEdgeScrollTracker{direction: direction, speed: speed, started: now, last: now, active: true}
		return 0
	}
	t.speed = speed
	if now.Sub(t.started) < delay {
		return 0
	}
	if !t.scrolling {
		t.scrolling = true
		t.last = now
		return 0
	}
	elapsed := now.Sub(t.last).Seconds()
	t.last = now
	return float64(t.direction) * t.speed * elapsed
}

// Reset ends the current edge dwell and scroll sequence.
func (t *DNDEdgeScrollTracker) Reset() { *t = DNDEdgeScrollTracker{} }

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

// DNDEdgeScrollAt returns signed direction and speed for a horizontal edge zone.
func DNDEdgeScrollAt(bounds layout.Bounds, x, y, width, maxSpeed float64) (int, float64) {
	if bounds.Width <= 0 || bounds.Height <= 0 || width <= 0 || maxSpeed <= 0 ||
		x < float64(bounds.X) || x >= float64(bounds.X+bounds.Width) ||
		y < float64(bounds.Y) || y >= float64(bounds.Y+bounds.Height) {
		return 0, 0
	}
	left, right := x-float64(bounds.X), float64(bounds.X+bounds.Width)-x
	if left < width && left <= right {
		return -1, maxSpeed * clampOverviewProgress((width-left)/width)
	}
	if right <= width {
		return 1, maxSpeed * clampOverviewProgress((width-right)/width)
	}
	return 0, 0
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
