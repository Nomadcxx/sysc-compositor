// Package wm owns output, workspace, and window placement policy.
package wm

import (
	"slices"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

type workspaceState struct {
	id    uint64
	strip *layout.Workspace
}

type outputState struct {
	name        string
	bounds      layout.Bounds
	columnWidth int
	gap         int
	workspaces  []*workspaceState
	active      int
}

type windowState struct {
	output      string
	workspaceID uint64
}

type windowSize struct {
	width, height int
}

type windowSizeHints struct {
	minWidth, maxWidth   int
	minHeight, maxHeight int
}

// Model tracks M5's single-window columns and dynamic per-output workspaces.
type Model struct {
	outputs         map[string]*outputState
	windows         map[uint64]windowState
	resizedWindows  map[uint64]windowSize
	sizeHints       map[uint64]windowSizeHints
	focusedOutput   string
	nextWorkspaceID uint64
}

// WorkspaceSnapshot is the immutable Niri-facing state for one workspace.
type WorkspaceSnapshot struct {
	ID             uint64
	Index          uint8
	Output         string
	IsActive       bool
	IsFocused      bool
	ActiveWindowID *uint64
	WindowIDs      []uint64
}

// WindowSnapshot carries workspace membership and tile position.
type WindowSnapshot struct {
	ID          uint64
	Output      string
	WorkspaceID uint64
	Column      uint32
	Tile        layout.Placement
}

// Snapshot is a detached view of the compositor's layout state.
type Snapshot struct {
	FocusedOutput string
	Workspaces    []WorkspaceSnapshot
	Windows       []WindowSnapshot
}

// New creates an empty workspace model. Output order is always lexical.
func New() *Model {
	return &Model{
		outputs:         map[string]*outputState{},
		windows:         map[uint64]windowState{},
		resizedWindows:  map[uint64]windowSize{},
		sizeHints:       map[uint64]windowSizeHints{},
		nextWorkspaceID: 1,
	}
}

// AddOutput gives a connected output one active empty workspace.
func (m *Model) AddOutput(name string, bounds layout.Bounds, columnWidth, gap int) bool {
	if name == "" || m.outputs[name] != nil {
		return false
	}
	out := &outputState{name: name, bounds: bounds, columnWidth: columnWidth, gap: gap}
	out.workspaces = append(out.workspaces, m.newWorkspace(out))
	m.outputs[name] = out
	if m.focusedOutput == "" {
		m.focusedOutput = name
	}
	var orphaned []uint64
	for id, state := range m.windows {
		if state.workspaceID == 0 {
			orphaned = append(orphaned, id)
		}
	}
	slices.Sort(orphaned)
	for _, id := range orphaned {
		m.mapWindow(id, name)
	}
	return true
}

// RemoveOutput removes its workspaces and moves mapped windows to another
// connected output's active workspace when one remains.
func (m *Model) RemoveOutput(name string) {
	out := m.outputs[name]
	if out == nil {
		return
	}
	var moved []uint64
	for _, workspace := range out.workspaces {
		moved = append(moved, workspace.strip.WindowOrder()...)
	}
	slices.Sort(moved)
	delete(m.outputs, name)
	for _, id := range moved {
		delete(m.windows, id)
	}
	if m.focusedOutput == name {
		m.focusedOutput = m.firstOutput()
	}
	for _, id := range moved {
		if m.focusedOutput != "" {
			m.mapWindow(id, m.focusedOutput)
		} else {
			m.windows[id] = windowState{}
		}
	}
}

// SetOutputGeometry updates the viewport used by each workspace on an output.
func (m *Model) SetOutputGeometry(name string, bounds layout.Bounds, columnWidth, gap int) bool {
	out := m.outputs[name]
	if out == nil {
		return false
	}
	out.bounds = bounds
	out.columnWidth = columnWidth
	out.gap = gap
	for _, workspace := range out.workspaces {
		workspace.strip.SetGeometry(bounds, columnWidth, gap)
	}
	return true
}

// MapWindow places a new window on the requested output, or on the focused
// output when that name is disconnected.
func (m *Model) MapWindow(id uint64, output string) bool {
	if id == 0 || m.windows[id].workspaceID != 0 {
		return false
	}
	if m.outputs[output] == nil {
		output = m.focusedOutput
	}
	if m.outputs[output] == nil {
		output = m.firstOutput()
	}
	if output == "" {
		return false
	}
	m.mapWindow(id, output)
	m.focusedOutput = output
	return true
}

func (m *Model) mapWindow(id uint64, output string) {
	out := m.outputs[output]
	if out == nil {
		return
	}
	workspace := out.workspaces[out.active]
	workspace.strip.AddWindow(id)
	m.windows[id] = windowState{output: output, workspaceID: workspace.id}
	if hints, ok := m.sizeHints[id]; ok {
		workspace.strip.SetWindowSizeHints(id, hints.minWidth, hints.maxWidth, hints.minHeight, hints.maxHeight)
	}
	if size, ok := m.resizedWindows[id]; ok {
		if placement, ok := workspace.strip.Placement(id); ok {
			workspace.strip.ResizeWindow(id, placement.X, placement.Y, size.width, size.height)
		}
	}
	m.reconcile(out)
}

// RemoveWindow removes an unmapped window from its workspace but retains its
// resized dimensions until DestroyWindow, while the stable toplevel ID lives.
// It returns the new active window on that workspace, when one remains.
func (m *Model) RemoveWindow(id uint64) (uint64, bool) {
	state, ok := m.windows[id]
	if !ok {
		return 0, false
	}
	delete(m.windows, id)
	out := m.outputs[state.output]
	if out == nil {
		return 0, true
	}
	for _, workspace := range out.workspaces {
		if workspace.id != state.workspaceID {
			continue
		}
		workspace.strip.RemoveWindow(id)
		m.reconcile(out)
		if out.name == m.focusedOutput && out.workspaces[out.active].id == workspace.id {
			return workspace.strip.FocusedWindow()
		}
		return 0, true
	}
	return 0, true
}

// DestroyWindow drops a window's retained resize after it has unmapped.
func (m *Model) DestroyWindow(id uint64) {
	m.RemoveWindow(id)
	delete(m.resizedWindows, id)
	delete(m.sizeHints, id)
}

// SetWindowSizeHints stores the client's min/max size for its tile.
func (m *Model) SetWindowSizeHints(id uint64, minWidth, maxWidth, minHeight, maxHeight int) bool {
	if _, ok := m.windows[id]; !ok {
		return false
	}
	hints := windowSizeHints{
		minWidth: max(0, minWidth), maxWidth: max(0, maxWidth),
		minHeight: max(0, minHeight), maxHeight: max(0, maxHeight),
	}
	if current, ok := m.sizeHints[id]; ok && current == hints {
		return false
	}
	m.sizeHints[id] = hints
	state := m.windows[id]
	if out := m.outputs[state.output]; out != nil {
		for _, workspace := range out.workspaces {
			if workspace.id == state.workspaceID {
				workspace.strip.SetWindowSizeHints(id, hints.minWidth, hints.maxWidth, hints.minHeight, hints.maxHeight)
				return true
			}
		}
	}
	return true
}

// FocusWindow activates a window's output workspace and focuses its column.
func (m *Model) FocusWindow(id uint64) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	out := m.outputs[state.output]
	if out == nil {
		return false
	}
	for i, workspace := range out.workspaces {
		if workspace.id == state.workspaceID {
			out.active = i
			workspace.strip.FocusWindow(id)
			m.focusedOutput = state.output
			m.reconcile(out)
			return true
		}
	}
	return false
}

// ResizeWindow retains a pointer-resized tile's geometry in its workspace.
func (m *Model) ResizeWindow(id uint64, x, y, width, height int) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	out := m.outputs[state.output]
	if out == nil {
		return false
	}
	for _, workspace := range out.workspaces {
		if workspace.id == state.workspaceID {
			if !workspace.strip.ResizeWindow(id, x, y, width, height) {
				return false
			}
			m.resizedWindows[id] = windowSize{width: width, height: height}
			return true
		}
	}
	return false
}

// FocusOutput selects which output owns the global keyboard focus.
func (m *Model) FocusOutput(name string) bool {
	if m.outputs[name] == nil {
		return false
	}
	m.focusedOutput = name
	return true
}

// FocusWorkspace selects a one-based workspace index, clamping to the last
// existing workspace as Niri does for its dynamic workspace list.
func (m *Model) FocusWorkspace(output string, index uint8) bool {
	if output == "" {
		output = m.focusedOutput
	}
	out := m.outputs[output]
	if out == nil || len(out.workspaces) == 0 {
		return false
	}
	idx := 0
	if index > 1 {
		idx = int(index) - 1
	}
	if idx >= len(out.workspaces) {
		idx = len(out.workspaces) - 1
	}
	out.active = idx
	m.focusedOutput = output
	m.reconcile(out)
	return true
}

// FocusWorkspaceID activates a workspace resolved from the compositor's
// stable workspace IDs, as used by Niri IPC actions.
func (m *Model) FocusWorkspaceID(id uint64) bool {
	for _, output := range m.outputs {
		for index, workspace := range output.workspaces {
			if workspace.id != id {
				continue
			}
			output.active = index
			m.focusedOutput = output.name
			m.reconcile(output)
			return true
		}
	}
	return false
}

// FocusColumnLeft focuses the preceding column on the active workspace.
func (m *Model) FocusColumnLeft() bool { return m.activeStripAction((*layout.Workspace).FocusLeft) }

// FocusColumnRight focuses the following column on the active workspace.
func (m *Model) FocusColumnRight() bool { return m.activeStripAction((*layout.Workspace).FocusRight) }

func (m *Model) FocusColumnFirst() bool { return m.activeStripAction((*layout.Workspace).FocusFirst) }

func (m *Model) FocusColumnLast() bool { return m.activeStripAction((*layout.Workspace).FocusLast) }

// MoveColumnLeft moves the focused column before its left neighbor.
func (m *Model) MoveColumnLeft() bool { return m.activeStripAction((*layout.Workspace).MoveLeft) }

// MoveColumnRight moves the focused column after its right neighbor.
func (m *Model) MoveColumnRight() bool { return m.activeStripAction((*layout.Workspace).MoveRight) }

func (m *Model) MoveColumnToFirst() bool { return m.activeStripAction((*layout.Workspace).MoveFirst) }

func (m *Model) MoveColumnToLast() bool { return m.activeStripAction((*layout.Workspace).MoveLast) }

func (m *Model) activeStripAction(action func(*layout.Workspace) bool) bool {
	out := m.outputs[m.focusedOutput]
	if out == nil || len(out.workspaces) == 0 {
		return false
	}
	return action(out.workspaces[out.active].strip)
}

// ActiveWindow returns the active column window on the focused output.
func (m *Model) ActiveWindow() (uint64, bool) {
	out := m.outputs[m.focusedOutput]
	if out == nil || len(out.workspaces) == 0 {
		return 0, false
	}
	return out.workspaces[out.active].strip.FocusedWindow()
}

// Snapshot returns workspaces and windows in stable output/workspace order.
func (m *Model) Snapshot() Snapshot {
	names := m.outputNames()
	snapshot := Snapshot{FocusedOutput: m.focusedOutput}
	for _, name := range names {
		out := m.outputs[name]
		for i, workspace := range out.workspaces {
			index := i + 1
			if index > 255 {
				index = 255
			}
			placements := workspace.strip.Placements()
			windowIDs := make([]uint64, len(placements))
			for column, placement := range placements {
				windowIDs[column] = placement.ID
			}
			workspaceSnapshot := WorkspaceSnapshot{
				ID: workspace.id, Index: uint8(index), Output: name,
				IsActive:  i == out.active,
				IsFocused: name == m.focusedOutput && i == out.active,
				WindowIDs: windowIDs,
			}
			if id, ok := workspace.strip.FocusedWindow(); ok {
				workspaceSnapshot.ActiveWindowID = &id
			}
			snapshot.Workspaces = append(snapshot.Workspaces, workspaceSnapshot)
			for column, placement := range placements {
				snapshot.Windows = append(snapshot.Windows, WindowSnapshot{
					ID: placement.ID, Output: name, WorkspaceID: workspace.id,
					Column: uint32(column), Tile: placement,
				})
			}
		}
	}
	return snapshot
}

func (m *Model) reconcile(out *outputState) {
	if len(out.workspaces) == 0 {
		out.workspaces = append(out.workspaces, m.newWorkspace(out))
		out.active = 0
	}
	if out.active < 0 {
		out.active = 0
	}
	if out.active >= len(out.workspaces) {
		out.active = len(out.workspaces) - 1
	}
	for i := len(out.workspaces) - 2; i >= 0; i-- {
		if i == out.active || len(out.workspaces[i].strip.WindowOrder()) != 0 {
			continue
		}
		out.workspaces = append(out.workspaces[:i], out.workspaces[i+1:]...)
		if out.active > i {
			out.active--
		}
	}
	last := out.workspaces[len(out.workspaces)-1]
	if len(last.strip.WindowOrder()) != 0 {
		out.workspaces = append(out.workspaces, m.newWorkspace(out))
	}
}

func (m *Model) newWorkspace(out *outputState) *workspaceState {
	id := m.nextWorkspaceID
	m.nextWorkspaceID++
	return &workspaceState{
		id:    id,
		strip: layout.NewWorkspace(out.bounds, out.columnWidth, out.gap),
	}
}

func (m *Model) outputNames() []string {
	names := make([]string, 0, len(m.outputs))
	for name := range m.outputs {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (m *Model) firstOutput() string {
	names := m.outputNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}
