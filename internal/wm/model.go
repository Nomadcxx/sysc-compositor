// Package wm owns output, workspace, and window placement policy.
package wm

import (
	"slices"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

type workspaceState struct {
	id    uint64
	name  *string
	strip *layout.Workspace
}

// WorkspaceConfig declares one named workspace from the Niri KDL config.
type WorkspaceConfig struct {
	Name         string
	OpenOnOutput string
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

type fullscreenRestore struct {
	bounds layout.Bounds
	area   layout.Bounds
}

// Model tracks windows, layouts, and dynamic per-output workspaces.
type Model struct {
	outputs         map[string]*outputState
	windows         map[uint64]windowState
	overview        overviewState
	windowDrag      *WindowDrag
	resizedWindows  map[uint64]windowSize
	floatingWindows map[uint64]layout.Bounds
	fullscreenFloat map[uint64]fullscreenRestore
	fullscreen      map[uint64]bool
	windowedFull    map[uint64]bool
	urgent          map[uint64]bool
	sizeHints       map[uint64]windowSizeHints
	focusedOutput   string
	nextWorkspaceID uint64
	workspaceConfig []WorkspaceConfig
}

// WorkspaceSnapshot is the immutable Niri-facing state for one workspace.
type WorkspaceSnapshot struct {
	ID             uint64
	Index          uint8
	Output         string
	Name           *string
	IsActive       bool
	IsFocused      bool
	ActiveWindowID *uint64
	WindowIDs      []uint64
	IsUrgent       bool
}

// WindowSnapshot carries workspace membership and tile position.
type WindowSnapshot struct {
	ID                 uint64
	Output             string
	WorkspaceID        uint64
	Column             uint32
	Fullscreen         bool
	WindowedFullscreen bool
	IsUrgent           bool
	Tile               layout.Placement
}

// Snapshot is a detached view of the compositor's layout state.
type Snapshot struct {
	FocusedOutput string
	OverviewOpen  bool
	Workspaces    []WorkspaceSnapshot
	Windows       []WindowSnapshot
}

// New creates an empty workspace model. Output order is always lexical.
func New(workspaces ...WorkspaceConfig) *Model {
	return &Model{
		outputs:         map[string]*outputState{},
		windows:         map[uint64]windowState{},
		resizedWindows:  map[uint64]windowSize{},
		floatingWindows: map[uint64]layout.Bounds{},
		fullscreenFloat: map[uint64]fullscreenRestore{},
		fullscreen:      map[uint64]bool{},
		windowedFull:    map[uint64]bool{},
		urgent:          map[uint64]bool{},
		sizeHints:       map[uint64]windowSizeHints{},
		nextWorkspaceID: 1,
		workspaceConfig: append([]WorkspaceConfig(nil), workspaces...),
	}
}

// AddOutput gives a connected output one active empty workspace.
func (m *Model) AddOutput(name string, bounds layout.Bounds, columnWidth, gap int) bool {
	if name == "" || m.outputs[name] != nil {
		return false
	}
	hadOutputs := len(m.outputs) != 0
	out := &outputState{name: name, bounds: bounds, columnWidth: columnWidth, gap: gap}
	out.workspaces = append(out.workspaces, m.newWorkspace(out))
	m.outputs[name] = out
	m.addConfiguredWorkspaces(out, !hadOutputs)
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
		m.remapFullscreenFloating(id, out.bounds)
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
	floating := make(map[uint64]layout.Bounds)
	for _, workspace := range out.workspaces {
		moved = append(moved, workspace.strip.WindowOrder()...)
		for _, placement := range workspace.strip.Placements() {
			if placement.Floating {
				floating[placement.ID] = layout.Bounds{
					X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height,
				}
			}
		}
	}
	slices.Sort(moved)
	delete(m.outputs, name)
	for _, id := range moved {
		delete(m.windows, id)
	}
	if m.focusedOutput == name {
		m.focusedOutput = m.firstOutput()
	}
	target := m.outputs[m.focusedOutput]
	if target == nil {
		target = m.outputs[m.firstOutput()]
	}
	for _, id := range moved {
		if m.focusedOutput != "" {
			if target != nil {
				m.remapFullscreenFloating(id, target.bounds)
			}
			if bounds, ok := floating[id]; ok && target != nil {
				m.floatingWindows[id] = layout.RemapFloatingBounds(out.bounds, target.bounds, bounds)
			}
			m.mapWindow(id, m.focusedOutput)
		} else {
			m.windows[id] = windowState{}
		}
	}
	m.clearFocusedUrgency()
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
	for id, state := range m.windows {
		if state.output == name {
			m.remapFullscreenFloating(id, bounds)
			m.syncFullscreenFloatingGeometry(id)
		}
	}
	return true
}

// SetWorkspaceConfig adds newly configured named workspaces to live outputs.
// Existing workspaces keep their identity and placement across config reloads.
func (m *Model) SetWorkspaceConfig(workspaces []WorkspaceConfig) bool {
	if slices.Equal(m.workspaceConfig, workspaces) {
		return false
	}
	m.workspaceConfig = slices.Clone(workspaces)
	first := m.firstOutput()
	for _, name := range m.outputNames() {
		m.addConfiguredWorkspaces(m.outputs[name], name == first)
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
	m.clearFocusedUrgency()
	return true
}

func (m *Model) mapWindow(id uint64, output string) {
	out := m.outputs[output]
	if out == nil {
		return
	}
	workspace := out.workspaces[out.active]
	m.mapWindowOnWorkspace(id, out, workspace)
	m.reconcile(out)
}

func (m *Model) mapWindowOnWorkspace(id uint64, out *outputState, workspace *workspaceState) {
	if out == nil || workspace == nil {
		return
	}
	workspace.strip.AddWindow(id)
	m.windows[id] = windowState{output: out.name, workspaceID: workspace.id}
	if hints, ok := m.sizeHints[id]; ok {
		workspace.strip.SetWindowSizeHints(id, hints.minWidth, hints.maxWidth, hints.minHeight, hints.maxHeight)
	}
	if size, ok := m.resizedWindows[id]; ok {
		if placement, ok := workspace.strip.Placement(id); ok && (size.width > 0 || size.height > 0) {
			workspace.strip.ResizeWindow(id, placement.X, placement.Y, size.width, size.height, layout.ResizeOptions{
				ResizeWidth: size.width > 0, ResizeHeight: size.height > 0,
			})
		}
	}
	if bounds, ok := m.floatingWindows[id]; ok {
		workspace.strip.SetWindowFloating(id, true, bounds)
	}
	if restore, ok := m.fullscreenFloat[id]; ok {
		workspace.strip.SetSavedFloatingGeometry(id, restore.bounds)
		if !m.fullscreen[id] && !m.windowedFull[id] && workspace.strip.SetWindowFloating(id, true, restore.bounds) {
			if placement, exists := workspace.strip.Placement(id); exists {
				m.floatingWindows[id] = layout.Bounds{
					X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height,
				}
			}
			delete(m.fullscreenFloat, id)
		}
	}
}

// RemapWindow moves an unmapped toplevel's retained policy to its fresh mapping
// ID, retiring the old key before adding it to an output again. The caller owns
// ID allocation and must supply a new, unused ID.
func (m *Model) RemapWindow(oldID, newID uint64, output string) bool {
	if oldID == 0 || newID == 0 || oldID == newID {
		return false
	}
	if _, mapped := m.windows[oldID]; mapped {
		return false
	}
	if _, mapped := m.windows[newID]; mapped {
		return false
	}
	if value, ok := m.resizedWindows[oldID]; ok {
		m.resizedWindows[newID] = value
	}
	if value, ok := m.floatingWindows[oldID]; ok {
		m.floatingWindows[newID] = value
	}
	if value, ok := m.fullscreenFloat[oldID]; ok {
		m.fullscreenFloat[newID] = value
	}
	if value, ok := m.fullscreen[oldID]; ok {
		m.fullscreen[newID] = value
	}
	if value, ok := m.windowedFull[oldID]; ok {
		m.windowedFull[newID] = value
	}
	if value, ok := m.urgent[oldID]; ok {
		m.urgent[newID] = value
	}
	if value, ok := m.sizeHints[oldID]; ok {
		m.sizeHints[newID] = value
	}
	m.DestroyWindow(oldID)
	return m.MapWindow(newID, output)
}

// MapWindowToWorkspace places a new toplevel on a specific stable workspace.
func (m *Model) MapWindowToWorkspace(id, workspaceID uint64) bool {
	if id == 0 || m.windows[id].workspaceID != 0 {
		return false
	}
	out, index, workspace := m.workspaceLocation(workspaceID)
	if workspace == nil {
		return false
	}
	m.mapWindowOnWorkspace(id, out, workspace)
	out.active = index
	m.focusedOutput = out.name
	m.reconcile(out)
	m.clearFocusedUrgency()
	return true
}

func (m *Model) WorkspaceIDByName(name string) (uint64, bool) {
	workspace := m.workspaceByName(name)
	if workspace == nil {
		return 0, false
	}
	return workspace.id, true
}

// RemoveWindow removes an unmapped window from its workspace but retains its
// resized dimensions until DestroyWindow, while the stable toplevel ID lives.
// It returns the new active window on that workspace, when one remains.
func (m *Model) RemoveWindow(id uint64) (uint64, bool) {
	state, ok := m.windows[id]
	if !ok {
		return 0, false
	}
	if m.windowDrag != nil && m.windowDrag.WindowID == id {
		m.CancelWindowDrag()
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
		if placement, ok := workspace.strip.Placement(id); ok && placement.Floating {
			m.floatingWindows[id] = layout.Bounds{
				X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height,
			}
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
	delete(m.floatingWindows, id)
	delete(m.fullscreenFloat, id)
	delete(m.fullscreen, id)
	delete(m.windowedFull, id)
	delete(m.urgent, id)
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
			m.clearFocusedUrgency()
			return true
		}
	}
	return false
}

// ResizeWindow retains a pointer-resized tile's geometry in its workspace.
func (m *Model) ResizeWindow(id uint64, x, y, width, height int, options layout.ResizeOptions) bool {
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
			placement, exists := workspace.strip.Placement(id)
			if !exists {
				return false
			}
			if !workspace.strip.ResizeWindow(id, x, y, width, height, options) {
				return false
			}
			if placement.Floating {
				if resized, exists := workspace.strip.Placement(id); exists {
					m.floatingWindows[id] = layout.Bounds{
						X: resized.X, Y: resized.Y, Width: resized.Width, Height: resized.Height,
					}
				}
				return true
			}
			size := m.resizedWindows[id]
			if options.ResizeWidth {
				size.width = width
			}
			if options.ResizeHeight {
				size.height = height
			}
			if size.width > 0 || size.height > 0 {
				m.resizedWindows[id] = size
			}
			return true
		}
	}
	return false
}

// SetWindowFloating changes a mapped window's placement while preserving its
// floating rectangle and tiled resize state across unmap/remap.
func (m *Model) SetWindowFloating(id uint64, floating bool, bounds layout.Bounds) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	out := m.outputs[state.output]
	if out == nil {
		return false
	}
	for _, workspace := range out.workspaces {
		if workspace.id != state.workspaceID {
			continue
		}
		if !workspace.strip.SetWindowFloating(id, floating, bounds) {
			return false
		}
		if floating {
			if placement, exists := workspace.strip.Placement(id); exists {
				m.floatingWindows[id] = layout.Bounds{
					X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height,
				}
			}
		} else {
			delete(m.floatingWindows, id)
		}
		return true
	}
	return false
}

// SetFloatingGeometry updates one mapped floating window's retained rectangle.
func (m *Model) SetFloatingGeometry(id uint64, bounds layout.Bounds) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	out := m.outputs[state.output]
	if out == nil {
		return false
	}
	for _, workspace := range out.workspaces {
		if workspace.id != state.workspaceID {
			continue
		}
		if !workspace.strip.SetFloatingGeometry(id, bounds) {
			return false
		}
		placement, exists := workspace.strip.Placement(id)
		if !exists || !placement.Floating {
			return false
		}
		m.floatingWindows[id] = layout.Bounds{
			X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height,
		}
		return true
	}
	return false
}

// MoveFloatingWindow changes a window's saved floating position in its workspace.
func (m *Model) MoveFloatingWindow(id uint64, x, y layout.PositionChange) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	out := m.outputs[state.output]
	if out == nil {
		return false
	}
	for _, workspace := range out.workspaces {
		if workspace.id != state.workspaceID {
			continue
		}
		if !workspace.strip.MoveFloatingWindow(id, x, y) {
			return false
		}
		if placement, exists := workspace.strip.Placement(id); exists && placement.Floating {
			m.floatingWindows[id] = layout.Bounds{
				X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height,
			}
		}
		return true
	}
	return false
}

// SetFullscreen records a mapped client's requested fullscreen state without
// replacing its saved workspace placement.
func (m *Model) SetFullscreen(id uint64, fullscreen bool) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	if fullscreen {
		if m.fullscreen[id] && !m.windowedFull[id] {
			return false
		}
		if _, restoring := m.fullscreenFloat[id]; !restoring {
			out, _, workspace := m.workspaceLocation(state.workspaceID)
			if workspace != nil {
				if placement, exists := workspace.strip.Placement(id); exists && placement.Floating {
					bounds := layout.Bounds{X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height}
					if !workspace.strip.SetWindowFloating(id, false, layout.Bounds{}) {
						return false
					}
					area := layout.Bounds{}
					if out != nil {
						area = out.bounds
					}
					m.fullscreenFloat[id] = fullscreenRestore{bounds: bounds, area: area}
					delete(m.floatingWindows, id)
					m.reconcile(out)
				}
			}
		}
		m.fullscreen[id] = true
		delete(m.windowedFull, id)
		return true
	}
	if m.windowedFull[id] {
		delete(m.windowedFull, id)
		m.restoreFloatingAfterFullscreen(id)
		return true
	}
	if !m.fullscreen[id] {
		return false
	}
	delete(m.fullscreen, id)
	m.restoreFloatingAfterFullscreen(id)
	return true
}

func (m *Model) restoreFloatingAfterFullscreen(id uint64) {
	restore, ok := m.fullscreenFloat[id]
	if !ok {
		return
	}
	if _, mapped := m.windows[id]; !mapped {
		return
	}
	m.syncFullscreenFloatingGeometry(id)
	if m.SetWindowFloating(id, true, restore.bounds) {
		delete(m.fullscreenFloat, id)
	}
}

func (m *Model) syncFullscreenFloatingGeometry(id uint64) {
	restore, ok := m.fullscreenFloat[id]
	if !ok {
		return
	}
	state, ok := m.windows[id]
	if !ok {
		return
	}
	if workspace := m.workspaceByID(state.workspaceID); workspace != nil {
		workspace.strip.SetSavedFloatingGeometry(id, restore.bounds)
	}
}

func (m *Model) remapFullscreenFloating(id uint64, bounds layout.Bounds) {
	restore, ok := m.fullscreenFloat[id]
	if !ok || restore.area == bounds {
		return
	}
	restore.bounds = layout.RemapFloatingBounds(restore.area, bounds, restore.bounds)
	restore.area = bounds
	m.fullscreenFloat[id] = restore
}

// SetWindowedFullscreen records windowed fullscreen separately from real
// client fullscreen while retaining the window's ordinary placement.
func (m *Model) SetWindowedFullscreen(id uint64, fullscreen bool) bool {
	if _, ok := m.windows[id]; !ok || m.windowedFull[id] == fullscreen {
		return false
	}
	if fullscreen {
		delete(m.fullscreen, id)
		m.windowedFull[id] = true
		m.restoreFloatingAfterFullscreen(id)
	} else {
		delete(m.windowedFull, id)
		m.restoreFloatingAfterFullscreen(id)
	}
	return true
}

// IsWindowedFullscreen reports the retained presentation mode, including while
// the stable toplevel ID is unmapped.
func (m *Model) IsWindowedFullscreen(id uint64) bool {
	return m.windowedFull[id]
}

// SetWindowUrgent marks an unfocused mapped window urgent. Focusing a window
// clears its urgency, as Niri does when a toplevel receives focus.
func (m *Model) SetWindowUrgent(id uint64, urgent bool) bool {
	if _, ok := m.windows[id]; !ok || (urgent && m.isFocusedWindow(id)) || m.urgent[id] == urgent {
		return false
	}
	if urgent {
		m.urgent[id] = true
	} else {
		delete(m.urgent, id)
	}
	return true
}

func (m *Model) ToggleWindowUrgent(id uint64) bool {
	if _, ok := m.windows[id]; !ok {
		return false
	}
	return m.SetWindowUrgent(id, !m.urgent[id])
}

func (m *Model) isFocusedWindow(id uint64) bool {
	out := m.outputs[m.focusedOutput]
	if out == nil || out.active < 0 || out.active >= len(out.workspaces) {
		return false
	}
	focused, ok := out.workspaces[out.active].strip.FocusedWindow()
	return ok && focused == id
}

func (m *Model) clearFocusedUrgency() {
	if id, ok := m.ActiveWindow(); ok {
		delete(m.urgent, id)
	}
}

// FocusOutput selects which output owns the global keyboard focus.
func (m *Model) FocusOutput(name string) bool {
	if m.outputs[name] == nil {
		return false
	}
	m.focusedOutput = name
	m.clearFocusedUrgency()
	return true
}

// FocusOutputDirection selects the nearest output in the requested direction.
func (m *Model) FocusOutputDirection(direction string) bool {
	return m.FocusOutput(m.outputInDirection(m.focusedOutput, direction))
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
	m.clearFocusedUrgency()
	return true
}

// FocusWorkspaceByDelta switches an output's active workspace by a relative index.
func (m *Model) FocusWorkspaceByDelta(output string, delta int) bool {
	state := m.outputs[output]
	if state == nil || delta == 0 || len(state.workspaces) == 0 {
		return false
	}
	index := min(max(state.active+delta, 0), len(state.workspaces)-1)
	if index == state.active {
		return false
	}
	state.active = index
	m.focusedOutput = output
	m.reconcile(state)
	m.clearFocusedUrgency()
	return true
}

// ScrollWorkspaceView moves one workspace's horizontal viewport without changing focus.
func (m *Model) ScrollWorkspaceView(id uint64, delta int) bool {
	workspace := m.workspaceByID(id)
	return workspace != nil && workspace.strip.ScrollBy(delta)
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
			m.clearFocusedUrgency()
			return true
		}
	}
	return false
}

// FocusWorkspaceName activates a named workspace, matching Niri's
// case-insensitive ASCII name lookup.
func (m *Model) FocusWorkspaceName(name string) bool {
	for _, output := range m.outputNames() {
		state := m.outputs[output]
		for index, workspace := range state.workspaces {
			if workspace.name == nil || !equalName(*workspace.name, name) {
				continue
			}
			state.active = index
			m.focusedOutput = output
			m.reconcile(state)
			m.clearFocusedUrgency()
			return true
		}
	}
	return false
}

// SetWorkspaceName assigns name to the workspace with id unless another
// workspace already owns that name.
func (m *Model) SetWorkspaceName(id uint64, name string) bool {
	target := m.workspaceByID(id)
	if target == nil {
		return false
	}
	if m.workspaceByName(name) != nil {
		return false
	}
	value := name
	target.name = &value
	for _, output := range m.outputs {
		m.reconcile(output)
	}
	return true
}

// UnsetWorkspaceName removes the name from the workspace with id.
func (m *Model) UnsetWorkspaceName(id uint64) bool {
	target := m.workspaceByID(id)
	if target == nil || target.name == nil {
		return false
	}
	target.name = nil
	for _, output := range m.outputs {
		m.reconcile(output)
	}
	return true
}

// MoveWindowToWorkspace moves one window to an existing stable workspace ID.
// Focus follows only when the moved window was focused and follow is enabled.
func (m *Model) MoveWindowToWorkspace(id, targetID uint64, follow bool) bool {
	state, exists := m.windows[id]
	if !exists {
		return false
	}
	targetOutput, _, target := m.workspaceLocation(targetID)
	if target == nil || target.id == state.workspaceID {
		return false
	}
	sourceOutput := m.outputs[state.output]
	source := m.workspaceByID(state.workspaceID)
	if sourceOutput == nil || source == nil {
		return false
	}
	focused := false
	if sourceOutput.name == m.focusedOutput && sourceOutput.workspaces[sourceOutput.active] == source {
		if active, ok := source.strip.FocusedWindow(); ok {
			focused = active == id
		}
	}
	destinationFocus, destinationHadFocus := target.strip.FocusedWindow()
	activate := follow && focused
	if !source.strip.MoveWindowTo(target.strip, id) {
		return false
	}
	if !activate && destinationHadFocus {
		target.strip.FocusWindow(destinationFocus)
	}
	m.windows[id] = windowState{output: targetOutput.name, workspaceID: target.id}
	m.remapFullscreenFloating(id, targetOutput.bounds)
	m.syncFullscreenFloatingGeometry(id)
	m.reconcile(sourceOutput)
	if targetOutput != sourceOutput {
		m.reconcile(targetOutput)
	}
	if activate {
		targetOutput.active = m.workspaceIndex(targetOutput, target.id)
		target.strip.FocusWindow(id)
		m.focusedOutput = targetOutput.name
		m.reconcile(targetOutput)
	}
	m.clearFocusedUrgency()
	return true
}

// MoveFocusedColumnToWorkspace transfers the focused tiled column, keeping
// its tabs and ordering within the target workspace.
func (m *Model) MoveFocusedColumnToWorkspace(targetID uint64, follow bool) bool {
	sourceOutput := m.outputs[m.focusedOutput]
	if sourceOutput == nil || sourceOutput.active < 0 || sourceOutput.active >= len(sourceOutput.workspaces) {
		return false
	}
	source := sourceOutput.workspaces[sourceOutput.active]
	targetOutput, _, target := m.workspaceLocation(targetID)
	if target == nil || target == source {
		return false
	}
	if id, ok := source.strip.FocusedWindow(); ok {
		if placement, exists := source.strip.Placement(id); exists && placement.Floating {
			return m.MoveWindowToWorkspace(id, target.id, follow)
		}
	}
	ids := source.strip.FocusedColumnWindowIDs()
	destinationFocus, destinationHadFocus := target.strip.FocusedWindow()
	if len(ids) == 0 || !source.strip.MoveFocusedColumnTo(target.strip) {
		return false
	}
	for _, id := range ids {
		m.remapFullscreenFloating(id, targetOutput.bounds)
	}
	if !follow && destinationHadFocus {
		target.strip.FocusWindow(destinationFocus)
	}
	for _, id := range ids {
		m.windows[id] = windowState{output: targetOutput.name, workspaceID: target.id}
		m.syncFullscreenFloatingGeometry(id)
	}
	m.reconcile(sourceOutput)
	if targetOutput != sourceOutput {
		m.reconcile(targetOutput)
	}
	if follow {
		targetOutput.active = m.workspaceIndex(targetOutput, target.id)
		m.focusedOutput = targetOutput.name
		m.reconcile(targetOutput)
	}
	m.clearFocusedUrgency()
	return true
}

// MoveWindowToWorkspaceDown moves the active window to the next workspace on
// the focused output and optionally follows it.
func (m *Model) MoveWindowToWorkspaceDown(follow bool) bool {
	return m.moveActiveWindowToAdjacentWorkspace(1, follow)
}

// MoveWindowToWorkspaceUp moves the active window to the previous workspace
// on the focused output and optionally follows it.
func (m *Model) MoveWindowToWorkspaceUp(follow bool) bool {
	return m.moveActiveWindowToAdjacentWorkspace(-1, follow)
}

func (m *Model) moveActiveWindowToAdjacentWorkspace(direction int, follow bool) bool {
	out, target := m.adjacentWorkspace(direction)
	if out == nil || target == nil || out.active < 0 || out.active >= len(out.workspaces) {
		return false
	}
	id, ok := out.workspaces[out.active].strip.FocusedWindow()
	return ok && m.MoveWindowToWorkspace(id, target.id, follow)
}

// MoveColumnToWorkspaceDown moves the focused tiled column to the next
// workspace. A focused floating window moves alone, as it does in Niri.
func (m *Model) MoveColumnToWorkspaceDown(follow bool) bool {
	return m.moveFocusedColumnToAdjacentWorkspace(1, follow)
}

// MoveColumnToWorkspaceUp moves the focused tiled column to the previous
// workspace. A focused floating window moves alone, as it does in Niri.
func (m *Model) MoveColumnToWorkspaceUp(follow bool) bool {
	return m.moveFocusedColumnToAdjacentWorkspace(-1, follow)
}

func (m *Model) moveFocusedColumnToAdjacentWorkspace(direction int, follow bool) bool {
	out, target := m.adjacentWorkspace(direction)
	if out == nil || target == nil || out.active < 0 || out.active >= len(out.workspaces) {
		return false
	}
	source := out.workspaces[out.active]
	if id, ok := source.strip.FocusedWindow(); ok {
		if placement, exists := source.strip.Placement(id); exists && placement.Floating {
			return m.MoveWindowToWorkspace(id, target.id, follow)
		}
	}
	return m.MoveFocusedColumnToWorkspace(target.id, follow)
}

func (m *Model) adjacentWorkspace(direction int) (*outputState, *workspaceState) {
	out := m.outputs[m.focusedOutput]
	if out == nil || out.active < 0 || out.active >= len(out.workspaces) {
		return nil, nil
	}
	index := out.active + direction
	if index < 0 || index >= len(out.workspaces) {
		return nil, nil
	}
	return out, out.workspaces[index]
}

// MoveWindowToOutput moves a window to the output's active workspace.
func (m *Model) MoveWindowToOutput(id uint64, output string, follow bool) bool {
	target := m.outputs[output]
	if target == nil || target.active < 0 || target.active >= len(target.workspaces) {
		return false
	}
	return m.MoveWindowToWorkspace(id, target.workspaces[target.active].id, follow)
}

// MoveFocusedColumnToOutput moves the focused tiled column to the output's
// active workspace.
func (m *Model) MoveFocusedColumnToOutput(output string, follow bool) bool {
	target := m.outputs[output]
	if target == nil || target.active < 0 || target.active >= len(target.workspaces) {
		return false
	}
	return m.MoveFocusedColumnToWorkspace(target.workspaces[target.active].id, follow)
}

// MoveWorkspaceToOutput transfers one workspace to another output. The target
// inserts it after its active workspace; moving the focused workspace also
// activates it on the target, matching Niri.
func (m *Model) MoveWorkspaceToOutput(id uint64, output string) bool {
	source, sourceIndex, workspace := m.workspaceLocation(id)
	target := m.outputs[output]
	if source == nil || target == nil || source == target {
		return false
	}
	activate := source.name == m.focusedOutput && sourceIndex == source.active
	workspace.strip.SetGeometry(target.bounds, target.columnWidth, target.gap)

	source.workspaces = append(source.workspaces[:sourceIndex], source.workspaces[sourceIndex+1:]...)
	if source.active > sourceIndex {
		source.active--
	} else if source.active >= len(source.workspaces) {
		source.active = len(source.workspaces) - 1
	}
	if len(source.workspaces) == 0 {
		source.workspaces = append(source.workspaces, m.newWorkspace(source))
		source.active = 0
	}

	targetIndex := target.active + 1
	if targetIndex >= len(target.workspaces) {
		targetIndex = len(target.workspaces) - 1
	}
	target.workspaces = append(target.workspaces, nil)
	copy(target.workspaces[targetIndex+1:], target.workspaces[targetIndex:len(target.workspaces)-1])
	target.workspaces[targetIndex] = workspace
	if targetIndex <= target.active {
		target.active++
	}
	if activate {
		target.active = targetIndex
		m.focusedOutput = target.name
	}
	for _, windowID := range workspace.strip.WindowOrder() {
		m.remapFullscreenFloating(windowID, target.bounds)
		m.windows[windowID] = windowState{output: target.name, workspaceID: workspace.id}
		m.syncFullscreenFloatingGeometry(windowID)
	}
	m.reconcile(source)
	m.reconcile(target)
	m.clearFocusedUrgency()
	return true
}

// MoveFocusedWorkspaceToMonitorDirection transfers the focused workspace to
// the nearest output in the requested direction.
func (m *Model) MoveFocusedWorkspaceToMonitorDirection(direction string) bool {
	source := m.outputs[m.focusedOutput]
	if source == nil || source.active < 0 || source.active >= len(source.workspaces) {
		return false
	}
	return m.MoveWorkspaceToOutput(source.workspaces[source.active].id,
		m.outputInDirection(m.focusedOutput, direction))
}

// MoveWindowToMonitorDirection selects the nearest output in the requested
// direction from the window's current output.
func (m *Model) MoveWindowToMonitorDirection(id uint64, direction string, follow bool) bool {
	state, ok := m.windows[id]
	if !ok {
		return false
	}
	return m.MoveWindowToOutput(id, m.outputInDirection(state.output, direction), follow)
}

// MoveFocusedColumnToMonitorDirection selects the nearest output in the
// requested direction from the focused output.
func (m *Model) MoveFocusedColumnToMonitorDirection(direction string, follow bool) bool {
	return m.MoveFocusedColumnToOutput(m.outputInDirection(m.focusedOutput, direction), follow)
}

func (m *Model) workspaceLocation(id uint64) (*outputState, int, *workspaceState) {
	for _, output := range m.outputs {
		for index, workspace := range output.workspaces {
			if workspace.id == id {
				return output, index, workspace
			}
		}
	}
	return nil, -1, nil
}

func (m *Model) workspaceIndex(output *outputState, id uint64) int {
	for index, workspace := range output.workspaces {
		if workspace.id == id {
			return index
		}
	}
	return 0
}

// OutputInDirection is the output next to from in direction (left, right,
// up, down, previous or next), or "" when there is none.
func (m *Model) OutputInDirection(from, direction string) string {
	return m.outputInDirection(from, direction)
}

func (m *Model) outputInDirection(from, direction string) string {
	current := m.outputs[from]
	if current == nil || len(m.outputs) < 2 {
		return ""
	}
	names := m.outputNames()
	if direction == "previous" || direction == "next" {
		for index, name := range names {
			if name != from {
				continue
			}
			step := -1
			if direction == "next" {
				step = 1
			}
			return names[(index+step+len(names))%len(names)]
		}
		return ""
	}
	currentX, currentY := int64(current.bounds.X), int64(current.bounds.Y)
	currentRight, currentBottom := currentX+int64(current.bounds.Width), currentY+int64(current.bounds.Height)
	cx, cy := currentX+int64(current.bounds.Width)/2, currentY+int64(current.bounds.Height)/2
	best, bestDistance := "", int64(1<<62)
	for _, name := range names {
		candidate := m.outputs[name]
		if candidate == current {
			continue
		}
		left, top := int64(candidate.bounds.X), int64(candidate.bounds.Y)
		right, bottom := left+int64(candidate.bounds.Width), top+int64(candidate.bounds.Height)
		x, y := left+int64(candidate.bounds.Width)/2, top+int64(candidate.bounds.Height)/2
		dx, dy := x-cx, y-cy
		overlapX := left < currentRight && right > currentX
		overlapY := top < currentBottom && bottom > currentY
		distance := int64(1 << 62)
		switch direction {
		case "left":
			if dx >= 0 || !overlapY {
				continue
			}
			distance = -dx
		case "right":
			if dx <= 0 || !overlapY {
				continue
			}
			distance = dx
		case "up":
			if dy >= 0 || !overlapX {
				continue
			}
			distance = -dy
		case "down":
			if dy <= 0 || !overlapX {
				continue
			}
			distance = dy
		default:
			return ""
		}
		if distance < bestDistance {
			best, bestDistance = name, distance
		}
	}
	return best
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

// ConsumeWindowIntoColumn moves the first window from the adjacent right column into focus.
func (m *Model) ConsumeWindowIntoColumn() bool {
	return m.activeStripAction((*layout.Workspace).ConsumeWindowIntoColumn)
}

// ExpelWindowFromColumn moves the bottom window of the focused column to a new column on its right.
func (m *Model) ExpelWindowFromColumn() bool {
	return m.activeStripAction((*layout.Workspace).ExpelWindowFromColumn)
}

func (m *Model) ConsumeOrExpelWindowRight(id *uint64) bool {
	return m.consumeOrExpelWindow(id, true)
}

func (m *Model) ConsumeOrExpelWindowLeft(id *uint64) bool {
	return m.consumeOrExpelWindow(id, false)
}

func (m *Model) consumeOrExpelWindow(id *uint64, right bool) bool {
	action := func(workspace *layout.Workspace) bool {
		if right {
			return workspace.ConsumeOrExpelWindowRight(id)
		}
		return workspace.ConsumeOrExpelWindowLeft(id)
	}
	if id == nil {
		return m.activeStripAction(action)
	}
	state, ok := m.windows[*id]
	if !ok {
		return false
	}
	out := m.outputs[state.output]
	workspace := m.workspaceByID(state.workspaceID)
	if out == nil || workspace == nil || !action(workspace.strip) {
		return false
	}
	m.clearFocusedUrgency()
	m.reconcile(out)
	return true
}

func (m *Model) ToggleColumnTabbedDisplay() bool {
	return m.activeStripAction((*layout.Workspace).ToggleColumnTabbedDisplay)
}

func (m *Model) SetColumnDisplay(display layout.ColumnDisplay) bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.SetColumnDisplay(display)
	})
}

func (m *Model) FocusWindowInColumn(index uint8) bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.FocusWindowInColumn(index)
	})
}

func (m *Model) FocusWindowDown() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowDown)
}

func (m *Model) FocusWindowUp() bool { return m.activeStripAction((*layout.Workspace).FocusWindowUp) }

func (m *Model) MoveWindowDown() bool { return m.activeStripAction((*layout.Workspace).MoveWindowDown) }

func (m *Model) MoveWindowUp() bool { return m.activeStripAction((*layout.Workspace).MoveWindowUp) }

func (m *Model) FocusFloating() bool {
	return m.activeStripAction((*layout.Workspace).FocusFloating)
}

func (m *Model) FocusTiling() bool {
	return m.activeStripAction((*layout.Workspace).FocusTiling)
}

func (m *Model) SwitchFocusFloatingAndTiling() bool {
	return m.activeStripAction((*layout.Workspace).SwitchFocusFloatingAndTiling)
}

func (m *Model) activeStripAction(action func(*layout.Workspace) bool) bool {
	out := m.outputs[m.focusedOutput]
	if out == nil || len(out.workspaces) == 0 {
		return false
	}
	changed := action(out.workspaces[out.active].strip)
	if changed {
		m.clearFocusedUrgency()
	}
	return changed
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
	snapshot := Snapshot{FocusedOutput: m.focusedOutput, OverviewOpen: m.overview.open}
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
				Name:      cloneName(workspace.name),
				IsActive:  i == out.active,
				IsFocused: name == m.focusedOutput && i == out.active,
				WindowIDs: windowIDs,
			}
			for _, id := range windowIDs {
				workspaceSnapshot.IsUrgent = workspaceSnapshot.IsUrgent || m.urgent[id]
			}
			if id, ok := workspace.strip.FocusedWindow(); ok {
				workspaceSnapshot.ActiveWindowID = &id
			}
			snapshot.Workspaces = append(snapshot.Workspaces, workspaceSnapshot)
			for _, placement := range placements {
				snapshot.Windows = append(snapshot.Windows, WindowSnapshot{
					ID: placement.ID, Output: name, WorkspaceID: workspace.id,
					Column: placement.Column, Fullscreen: m.fullscreen[placement.ID],
					WindowedFullscreen: m.windowedFull[placement.ID],
					IsUrgent:           m.urgent[placement.ID], Tile: placement,
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
		if i == out.active || out.workspaces[i].name != nil || len(out.workspaces[i].strip.WindowOrder()) != 0 {
			continue
		}
		out.workspaces = append(out.workspaces[:i], out.workspaces[i+1:]...)
		if out.active > i {
			out.active--
		}
	}
	last := out.workspaces[len(out.workspaces)-1]
	if last.name != nil || len(last.strip.WindowOrder()) != 0 {
		out.workspaces = append(out.workspaces, m.newWorkspace(out))
	}
}

func (m *Model) addConfiguredWorkspaces(out *outputState, firstOutput bool) {
	for _, config := range m.workspaceConfig {
		if m.workspaceByName(config.Name) != nil {
			continue
		}
		if config.OpenOnOutput != "" && !equalName(config.OpenOnOutput, out.name) {
			continue
		}
		if config.OpenOnOutput == "" && !firstOutput {
			continue
		}
		workspace := m.newWorkspace(out)
		name := config.Name
		workspace.name = &name
		out.workspaces = append(out.workspaces, nil)
		copy(out.workspaces[1:], out.workspaces[:len(out.workspaces)-1])
		out.workspaces[0] = workspace
		out.active++
	}
}

func (m *Model) workspaceByID(id uint64) *workspaceState {
	for _, output := range m.outputs {
		for _, workspace := range output.workspaces {
			if workspace.id == id {
				return workspace
			}
		}
	}
	return nil
}

func (m *Model) workspaceByName(name string) *workspaceState {
	for _, output := range m.outputs {
		for _, workspace := range output.workspaces {
			if workspace.name != nil && equalName(*workspace.name, name) {
				return workspace
			}
		}
	}
	return nil
}

func cloneName(name *string) *string {
	if name == nil {
		return nil
	}
	copy := *name
	return &copy
}

func equalName(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		left, right := a[i], b[i]
		if left >= 'A' && left <= 'Z' {
			left += 'a' - 'A'
		}
		if right >= 'A' && right <= 'Z' {
			right += 'a' - 'A'
		}
		if left != right {
			return false
		}
	}
	return true
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
