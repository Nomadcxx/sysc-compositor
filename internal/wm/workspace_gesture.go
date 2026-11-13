package wm

import (
	"math"
	"time"
)

// Niri's WORKSPACE_GESTURE_MOVEMENT and WORKSPACE_GESTURE_RUBBER_BAND.
const (
	workspaceGesturePixels = 300.0
	workspaceRubberBand    = 0.5
	workspaceRubberLimit   = 0.05
)

// rubberBand is Niri's RubberBand: motion past [min, max] keeps moving, ever
// slower, up to limit beyond the bound.
type rubberBand struct{ stiffness, limit float64 }

func (r rubberBand) band(x float64) float64 {
	return (1 - 1/(x*r.stiffness/r.limit+1)) * r.limit
}

func (r rubberBand) clamp(lo, hi, x float64) float64 {
	clamped := min(max(x, lo), hi)
	if x < clamped {
		return clamped - r.band(clamped-x)
	}
	return clamped + r.band(x-clamped)
}

func (r rubberBand) clampDerivative(lo, hi, x float64) float64 {
	if lo <= x && x <= hi {
		return 1
	}
	d := math.Abs(x - min(max(x, lo), hi))
	return r.stiffness * r.limit * r.limit / math.Pow(r.stiffness*d+r.limit, 2)
}

// workspaceGesture is Niri's touchpad WorkspaceSwitchGesture. Positions are
// in workspaces relative to the workspace active when it began, which is
// held by ID because reconcile may insert or drop workspaces before it.
type workspaceGesture struct {
	output   string
	centerID uint64
	start    float64
	clamped  bool
	dnd      bool
	tracker  overviewGestureTracker
}

// bounds is Niri's min_max: one workspace either side of the start outside
// the overview, every workspace in it.
func (g *workspaceGesture) bounds(out *outputState, center int) (lo, hi float64) {
	lo, hi = float64(-center), float64(len(out.workspaces)-1-center)
	if g.clamped {
		lo, hi = max(lo, -1), min(hi, 1)
	}
	return lo, hi
}

// center is the index of the workspace the gesture began on, or the active
// one when that workspace is gone.
func (g *workspaceGesture) center(out *outputState) int {
	for index, workspace := range out.workspaces {
		if workspace.id == g.centerID {
			return index
		}
	}
	return out.active
}

// gestureZoom is the overview zoom a gesture scales by: 1 with it closed.
func gestureZoom(zoom float64) float64 {
	if math.IsNaN(zoom) || math.IsInf(zoom, 0) || zoom <= 0 {
		return 1
	}
	return min(zoom, 1)
}

// BeginWorkspaceGesture starts a touchpad workspace switch on output from
// where its workspaces are drawn, catching a switch that is still settling.
func (m *Model) BeginWorkspaceGesture(output string) bool {
	out := m.outputs[output]
	if out == nil || len(out.workspaces) == 0 {
		return false
	}
	start := 0.0
	if out.switching {
		start = out.switchOffset
	}
	m.workspaceGesture = &workspaceGesture{
		output: output, centerID: out.workspaces[out.active].id,
		start: start, clamped: !m.overview.open,
	}
	out.switching, out.switchOffset = true, start
	return true
}

// BeginWorkspaceDND starts a clamped, momentum-free overview strip drag.
func (m *Model) BeginWorkspaceDND(output string) bool {
	if !m.overview.open || !m.BeginWorkspaceGesture(output) {
		return false
	}
	m.workspaceGesture.dnd = true
	return true
}

// UpdateWorkspaceGesture moves the gesture by deltaY logical pixels, positive
// towards later workspaces. zoom is the overview zoom, 1 when it is closed.
func (m *Model) UpdateWorkspaceGesture(deltaY float64, timestamp time.Duration, zoom float64) bool {
	g := m.workspaceGesture
	if g == nil || math.IsNaN(deltaY) || math.IsInf(deltaY, 0) {
		return false
	}
	out := m.outputs[g.output]
	if out == nil {
		m.workspaceGesture = nil
		return false
	}
	if g.dnd {
		g.tracker.push(deltaY, timestamp)
		center := g.center(out)
		lo, hi := g.bounds(out, center)
		position := g.start + g.tracker.position/1500
		clamped := min(max(position, lo), hi)
		g.start += clamped - position
		offset := clamped + float64(center-out.active)
		changed := offset != out.switchOffset
		out.switchOffset = offset
		return changed
	}
	// Niri reduces the effect of the overview zoom on the touchpad somewhat.
	zoom = gestureZoom(zoom)
	g.tracker.push(deltaY/((zoom-1)/2.5+1), timestamp)
	center := g.center(out)
	lo, hi := g.bounds(out, center)
	band := rubberBand{workspaceRubberBand, workspaceRubberLimit / zoom}
	offset := band.clamp(lo, hi, g.start+g.tracker.position/workspaceGesturePixels) + float64(center-out.active)
	if offset == out.switchOffset {
		return false
	}
	out.switchOffset = offset
	return true
}

// EndWorkspaceGesture activates the workspace the gesture projects to and
// returns where that workspace is drawn now, in workspaces from its place,
// and the release velocity in workspaces per second, for the settling spring.
func (m *Model) EndWorkspaceGesture(timestamp time.Duration, zoom float64) (output string, from, velocity float64, ok bool) {
	g := m.workspaceGesture
	if g == nil {
		return "", 0, 0, false
	}
	m.workspaceGesture = nil
	out := m.outputs[g.output]
	if out == nil {
		return "", 0, 0, false
	}
	g.tracker.push(0, timestamp)
	center := g.center(out)
	lo, hi := g.bounds(out, center)
	band := rubberBand{workspaceRubberBand, workspaceRubberLimit / gestureZoom(zoom)}
	projected := g.start + g.tracker.projectedEndPosition()/workspaceGesturePixels
	target := center + int(math.Round(min(max(projected, lo), hi)))
	velocity = g.tracker.velocity() / workspaceGesturePixels *
		band.clampDerivative(lo, hi, g.start+g.tracker.position/workspaceGesturePixels)
	if g.dnd {
		target = center + int(math.Round(min(max(g.start+g.tracker.position/1500, lo), hi)))
		velocity = 0
	}
	drawn := float64(out.active) + out.switchOffset
	if target != out.active {
		out.active = target
		m.focusedOutput = g.output
		m.reconcile(out)
		m.clearFocusedUrgency()
		// Reconcile may have moved the start workspace; keep the drawing.
		drawn += float64(g.center(out) - center)
	}
	out.switchOffset = drawn - float64(out.active)
	return g.output, out.switchOffset, velocity, true
}

// WorkspaceGestureActive reports whether a touchpad workspace switch is held.
func (m *Model) WorkspaceGestureActive() bool { return m.workspaceGesture != nil }

// WorkspaceRenderOffset is how far, in workspaces, output's workspaces are
// drawn past the active one while a switch gesture holds or settles them.
func (m *Model) WorkspaceRenderOffset(output string) (float64, bool) {
	out := m.outputs[output]
	if out == nil || !out.switching {
		return 0, false
	}
	return out.switchOffset, true
}

// SetWorkspaceRenderOffset is the settling spring's presentation value.
func (m *Model) SetWorkspaceRenderOffset(output string, offset float64) {
	if out := m.outputs[output]; out != nil {
		out.switching, out.switchOffset = true, offset
	}
}

// ClearWorkspaceRenderOffset ends a settled switch's presentation.
func (m *Model) ClearWorkspaceRenderOffset(output string) {
	if out := m.outputs[output]; out != nil && (m.workspaceGesture == nil || m.workspaceGesture.output != output) {
		out.switching, out.switchOffset = false, 0
	}
}
