// Package wm owns output, workspace, and window placement policy.
package wm

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"reflect"
	"slices"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

type workspaceState struct {
	originalOutput string
	outputLayout   *WorkspaceLayout
	id             uint64
	name           *string
	strip          *layout.Workspace
}

// occupied reports whether the workspace holds windows or a name, which keeps
// it alive while it is not active: Niri's has_windows_or_name.
func (w *workspaceState) occupied() bool {
	return w.name != nil || len(w.strip.WindowOrder()) != 0
}

// WorkspaceConfig declares one named workspace from the Niri KDL config.
type WorkspaceConfig struct {
	Name         string
	OpenOnOutput string
	Layout       *WorkspaceLayout
}

// WorkspaceLayout holds the layout overrides a workspace sets. A nil field
// means the workspace keeps the value of its output.
type WorkspaceLayout struct {
	ConfigTabIndicator    *config.TabIndicator
	TabIndicatorPart      config.TabIndicatorPart
	Gaps                  *int
	ColumnWidthProportion *float64
	FixedColumnWidth      *int
	// A nil preset list keeps the config's list, an empty one resets to
	// Niri's default presets.
	PresetColumnWidths   []layout.PresetSize
	PresetWindowHeights  []layout.PresetSize
	DefaultColumnDisplay *layout.ColumnDisplay
	// A nil mode keeps the config's policy.
	CenterFocusedColumn *layout.CenterFocusedColumn
	// A nil flag keeps the config's always-center-single-column.
	AlwaysCenterSingleColumn *bool
	// A nil strut set keeps the config's. When present it replaces the
	// whole set, Niri's merge_clone, not one edge at a time.
	Struts *layout.Struts
	// A nil tab space keeps the config's.
	TabSpace *layout.TabSpace
}

type outputState struct {
	layout      *WorkspaceLayout
	name        string
	bounds      layout.Bounds
	columnWidth int
	gap         int
	workspaces  []*workspaceState
	active      int
	// activeID and previousWorkspace follow Niri's Monitor
	// previous_workspace_id: reconcile records the old active workspace
	// whenever the active one changes.
	activeID          uint64
	previousWorkspace uint64
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
	detached            *outputState
	lastActiveWorkspace map[string]uint64
	outputs             map[string]*outputState
	windows             map[uint64]windowState
	overview            overviewState
	windowDrag          *WindowDrag
	resizedWindows      map[uint64]windowSize
	floatingWindows     map[uint64]layout.Bounds
	fullscreenFloat     map[uint64]fullscreenRestore
	fullscreen          map[uint64]bool
	windowedFull        map[uint64]bool
	urgent              map[uint64]bool
	sizeHints           map[uint64]windowSizeHints
	focusedOutput       string
	nextWorkspaceID     uint64
	workspaceConfig     []WorkspaceConfig
	// presetColumnWidths, presetWindowHeights, defaultDisplay and
	// centerFocusedColumn are the config-wide layout values; the workspaces
	// without their own override use them.
	presetColumnWidths  []layout.PresetSize
	presetWindowHeights []layout.PresetSize
	defaultDisplay      layout.ColumnDisplay
	centerFocusedColumn layout.CenterFocusedColumn
	// alwaysCenterSingleColumn is the config-wide
	// always-center-single-column.
	alwaysCenterSingleColumn bool
	// emptyWorkspaceAboveFirst keeps an empty workspace above the first one
	// of every output, Niri's empty-workspace-above-first.
	emptyWorkspaceAboveFirst bool
	// struts is the config-wide layout.struts; the zero value reserves
	// nothing and leaves each workspace on its output's full area.
	struts layout.Struts
	// tabSpace is the room tabbed columns keep for their tab indicator.
	tabSpace layout.TabSpace
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
	m.outputs[name] = out
	if m.detached != nil {
		out.workspaces = append(out.workspaces, m.detached.workspaces...)
		m.detached = nil
	} else {
		for _, sourceName := range m.outputNames() {
			if sourceName == name {
				continue
			}
			source := m.outputs[sourceName]
			for i := 0; i < len(source.workspaces); {
				workspace := source.workspaces[i]
				if !equalName(workspace.originalOutput, name) || !workspace.occupied() {
					i++
					continue
				}
				out.workspaces = append(out.workspaces, workspace)
				source.workspaces = append(source.workspaces[:i], source.workspaces[i+1:]...)
				if i <= source.active {
					source.active--
				}
			}
			m.reconcile(source)
		}
	}
	for _, workspace := range out.workspaces {
		m.attachWorkspace(out, workspace)
	}
	out.workspaces = append(out.workspaces, m.newWorkspace(out))
	for i, workspace := range out.workspaces {
		if workspace.id == m.lastActiveWorkspace[name] {
			out.active = i
		}
	}
	m.addConfiguredWorkspaces(out, !hadOutputs)
	m.reconcile(out)
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

// RemoveOutput moves whole workspaces to the focused surviving output. The
// original output and active workspace survive until that output returns.
func (m *Model) RemoveOutput(name string) {
	out := m.outputs[name]
	if out == nil {
		return
	}
	if m.lastActiveWorkspace == nil {
		m.lastActiveWorkspace = map[string]uint64{}
	}
	m.lastActiveWorkspace[name] = out.workspaces[out.active].id
	delete(m.outputs, name)
	if m.focusedOutput == name {
		m.focusedOutput = m.firstOutput()
	}
	target := m.outputs[m.focusedOutput]
	if target == nil {
		target = &outputState{bounds: out.bounds, columnWidth: out.columnWidth, gap: out.gap}
		m.detached = target
	}
	insert := max(0, len(target.workspaces)-1)
	for _, workspace := range out.workspaces {
		if !workspace.occupied() {
			continue
		}
		m.attachWorkspace(target, workspace)
		target.workspaces = slices.Insert(target.workspaces, insert, workspace)
		if insert <= target.active && len(target.workspaces) > 1 {
			target.active++
		}
		insert++
	}
	m.reconcile(target)
	m.clearFocusedUrgency()
}

// attachWorkspace transfers geometry and the window membership together.
func (m *Model) attachWorkspace(out *outputState, workspace *workspaceState) {
	m.applyWorkspaceLayout(out, workspace)
	for _, placement := range workspace.strip.Placements() {
		id := placement.ID
		m.windows[id] = windowState{output: out.name, workspaceID: workspace.id}
		if placement.Floating {
			m.floatingWindows[id] = layout.Bounds{X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height}
		}
		m.remapFullscreenFloating(id, out.bounds)
		m.syncFullscreenFloatingGeometry(id)
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
		m.applyWorkspaceLayout(out, workspace)
	}
	for id, state := range m.windows {
		if state.output == name {
			m.remapFullscreenFloating(id, bounds)
			m.syncFullscreenFloatingGeometry(id)
		}
	}
	return true
}

// workspaceGeometry returns the column width and the gap of a workspace: the
// values of its output, with the workspace's own layout merged over them.
func (m *Model) workspaceGeometry(out *outputState, workspace *workspaceState) (int, int) {
	width, gap := out.columnWidth, out.gap
	override := m.workspaceLayout(workspace)
	if override == nil {
		return width, gap
	}
	if override.Gaps != nil {
		gap = *override.Gaps
	}
	switch {
	case override.FixedColumnWidth != nil:
		width = *override.FixedColumnWidth
	case override.ColumnWidthProportion != nil:
		// Niri computes the proportion on the working area, so the
		// workspace's struts are already removed from the width.
		width = layout.ColumnWidthPixels(m.workspaceBounds(out, workspace).Width, gap, *override.ColumnWidthProportion)
	}
	return width, gap
}

// workspaceBounds returns the layout area of a workspace: its output's full
// area minus the struts it keeps. Only the strip lays out inside this area;
// fullscreen windows and the overview still use the whole output.
func (m *Model) workspaceBounds(out *outputState, workspace *workspaceState) layout.Bounds {
	return m.strutsFor(workspace).Inset(out.bounds)
}

// applyWorkspaceLayout pushes the merged layout of a workspace into its strip:
// the geometry of its output with its own overrides, the preset sizes, the
// default column display and the centering policy.
func (m *Model) applyWorkspaceLayout(out *outputState, workspace *workspaceState) {
	workspace.outputLayout = out.layout
	width, gap := m.workspaceGeometry(out, workspace)
	workspace.strip.SetGeometry(m.workspaceBounds(out, workspace), width, gap)
	widths, heights := m.presetSizes(workspace)
	workspace.strip.SetPresetSizes(widths, heights)
	workspace.strip.SetDefaultColumnDisplay(m.columnDisplay(workspace))
	workspace.strip.SetCenterFocusedColumn(m.centerFocusedColumnFor(workspace))
	workspace.strip.SetAlwaysCenterSingleColumn(m.alwaysCenterSingleColumnFor(workspace))
	workspace.strip.SetTabSpace(m.tabSpaceFor(workspace))
}

// tabSpaceFor returns the room a workspace's tabbed columns keep for their
// tab indicator: its own when it sets one, otherwise the config-wide one.
func (m *Model) tabSpaceFor(workspace *workspaceState) layout.TabSpace {
	if override := m.workspaceLayout(workspace); override != nil && override.TabSpace != nil {
		return *override.TabSpace
	}
	return m.tabSpace
}

// SetTabSpace sets the room the tabbed columns of the workspaces without
// their own keep for their tab indicator.
func (m *Model) SetTabSpace(space layout.TabSpace) {
	if m.tabSpace == space {
		return
	}
	m.tabSpace = space
	m.reapplyWorkspaceLayout()
}

// presetSizes returns the preset sizes a workspace cycles through: its own
// lists when it sets them, otherwise the config-wide lists.
func (m *Model) presetSizes(workspace *workspaceState) ([]layout.PresetSize, []layout.PresetSize) {
	widths, heights := m.presetColumnWidths, m.presetWindowHeights
	if override := m.workspaceLayout(workspace); override != nil {
		if override.PresetColumnWidths != nil {
			widths = override.PresetColumnWidths
		}
		if override.PresetWindowHeights != nil {
			heights = override.PresetWindowHeights
		}
	}
	return widths, heights
}

// columnDisplay returns the display mode the new columns of a workspace start
// in: its own when it sets one, otherwise the config-wide one.
func (m *Model) columnDisplay(workspace *workspaceState) layout.ColumnDisplay {
	if override := m.workspaceLayout(workspace); override != nil && override.DefaultColumnDisplay != nil {
		return *override.DefaultColumnDisplay
	}
	return m.defaultDisplay
}

// centerFocusedColumnFor returns the centering policy of a workspace: its own
// when it sets one, otherwise the config-wide one.
func (m *Model) centerFocusedColumnFor(workspace *workspaceState) layout.CenterFocusedColumn {
	if override := m.workspaceLayout(workspace); override != nil && override.CenterFocusedColumn != nil {
		return *override.CenterFocusedColumn
	}
	return m.centerFocusedColumn
}

// alwaysCenterSingleColumnFor returns whether a workspace centers a lone
// column: its own flag when it sets one, otherwise the config-wide one.
func (m *Model) alwaysCenterSingleColumnFor(workspace *workspaceState) bool {
	if override := m.workspaceLayout(workspace); override != nil && override.AlwaysCenterSingleColumn != nil {
		return *override.AlwaysCenterSingleColumn
	}
	return m.alwaysCenterSingleColumn
}

// strutsFor returns the struts a workspace lays out inside: its own when it
// sets them, otherwise the config-wide ones.
func (m *Model) strutsFor(workspace *workspaceState) layout.Struts {
	if override := m.workspaceLayout(workspace); override != nil && override.Struts != nil {
		return *override.Struts
	}
	return m.struts
}

// SetPresetSizes sets the preset sizes the workspaces without their own lists
// cycle through.
func (m *Model) SetPresetSizes(columnWidths, windowHeights []layout.PresetSize) {
	if slices.Equal(m.presetColumnWidths, columnWidths) && slices.Equal(m.presetWindowHeights, windowHeights) {
		return
	}
	m.presetColumnWidths = slices.Clone(columnWidths)
	m.presetWindowHeights = slices.Clone(windowHeights)
	m.reapplyWorkspaceLayout()
}

// SetDefaultColumnDisplay sets the display mode new columns start in when
// their workspace does not set one.
func (m *Model) SetDefaultColumnDisplay(display layout.ColumnDisplay) {
	if m.defaultDisplay == display {
		return
	}
	m.defaultDisplay = display
	m.reapplyWorkspaceLayout()
}

// SetCenterFocusedColumn sets the centering policy of the workspaces that do
// not set one.
func (m *Model) SetCenterFocusedColumn(mode layout.CenterFocusedColumn) {
	if m.centerFocusedColumn == mode {
		return
	}
	m.centerFocusedColumn = mode
	m.reapplyWorkspaceLayout()
}

// SetAlwaysCenterSingleColumn sets whether the workspaces that do not set the
// flag center a lone column.
func (m *Model) SetAlwaysCenterSingleColumn(on bool) {
	if m.alwaysCenterSingleColumn == on {
		return
	}
	m.alwaysCenterSingleColumn = on
	m.reapplyWorkspaceLayout()
}

// SetEmptyWorkspaceAboveFirst sets whether every output keeps an empty
// workspace above its first one. The outputs add or drop that workspace at
// once, as Niri's Monitor::update_config does.
func (m *Model) SetEmptyWorkspaceAboveFirst(on bool) {
	if m.emptyWorkspaceAboveFirst == on {
		return
	}
	m.emptyWorkspaceAboveFirst = on
	for _, name := range m.outputNames() {
		m.reconcile(m.outputs[name])
	}
}

// SetStruts sets the reserved edges of the layout area for the workspaces
// that do not set their own struts.
func (m *Model) SetStruts(struts layout.Struts) {
	if m.struts == struts {
		return
	}
	m.struts = struts
	m.reapplyWorkspaceLayout()
}

// reapplyWorkspaceLayout pushes the merged layout of every workspace into its
// strip after the config-wide layout changed.
func (m *Model) reapplyWorkspaceLayout() {
	for _, name := range m.outputNames() {
		out := m.outputs[name]
		for _, workspace := range out.workspaces {
			m.applyWorkspaceLayout(out, workspace)
		}
	}
}

// workspaceLayout returns the layout override of a named workspace.
func (m *Model) workspaceLayout(workspace *workspaceState) *WorkspaceLayout {
	if workspace == nil {
		return nil
	}
	if workspace.name == nil {
		return workspace.outputLayout
	}
	for _, config := range m.workspaceConfig {
		if equalName(config.Name, *workspace.name) {
			return workspace.outputLayout.merged(config.Layout)
		}
	}
	return workspace.outputLayout
}

// SetWorkspaceConfig adds newly configured named workspaces to live outputs.
// Existing workspaces keep their identity and placement across config reloads.
func (m *Model) SetWorkspaceConfig(workspaces []WorkspaceConfig) bool {
	if sameWorkspaceConfig(m.workspaceConfig, workspaces) {
		return false
	}
	m.workspaceConfig = slices.Clone(workspaces)
	first := m.firstOutput()
	for _, name := range m.outputNames() {
		m.addConfiguredWorkspaces(m.outputs[name], name == first)
	}
	// A workspace that already exists keeps its identity, but its own layout
	// may have changed.
	for _, name := range m.outputNames() {
		out := m.outputs[name]
		for _, workspace := range out.workspaces {
			m.applyWorkspaceLayout(out, workspace)
		}
	}
	return true
}

// sameWorkspaceConfig compares two workspace configurations by value.
func sameWorkspaceConfig(a, b []WorkspaceConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].OpenOnOutput != b[i].OpenOnOutput {
			return false
		}
		if !reflect.DeepEqual(a[i].Layout, b[i].Layout) {
			return false
		}
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
		if m.detached == nil {
			return false
		}
		m.mapWindowOnWorkspace(id, m.detached, m.detached.workspaces[m.detached.active])
		m.reconcile(m.detached)
		return true
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
		out = m.detached
	}
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
	return m.switchWorkspaceBy(output, delta, true)
}

// FocusWorkspaceDownOn and FocusWorkspaceUpOn are Niri's
// FocusWorkspace{Down,Up}UnderMouse: the output's monitor switches workspace
// and the focused output stays where it is.
func (m *Model) FocusWorkspaceDownOn(output string) bool {
	return m.switchWorkspaceBy(output, 1, false)
}

func (m *Model) FocusWorkspaceUpOn(output string) bool { return m.switchWorkspaceBy(output, -1, false) }

func (m *Model) switchWorkspaceBy(output string, delta int, focus bool) bool {
	state := m.outputs[output]
	if state == nil || delta == 0 || len(state.workspaces) == 0 {
		return false
	}
	index := min(max(state.active+delta, 0), len(state.workspaces)-1)
	if index == state.active {
		return false
	}
	state.active = index
	if focus {
		m.focusedOutput = output
	}
	m.reconcile(state)
	m.clearFocusedUrgency()
	return true
}

// FocusWorkspaceDown is Niri's switch_workspace_down on the focused output.
func (m *Model) FocusWorkspaceDown() bool { return m.FocusWorkspaceByDelta(m.focusedOutput, 1) }

// FocusWorkspaceUp is Niri's switch_workspace_up on the focused output.
func (m *Model) FocusWorkspaceUp() bool { return m.FocusWorkspaceByDelta(m.focusedOutput, -1) }

// FocusWorkspacePrevious returns the focused output to the workspace that
// was active before the current one, if it is still on that output.
func (m *Model) FocusWorkspacePrevious() bool {
	out := m.outputs[m.focusedOutput]
	if out == nil || out.previousWorkspace == 0 {
		return false
	}
	owner, index, _ := m.workspaceLocation(out.previousWorkspace)
	if owner != out || index == out.active {
		return false
	}
	out.active = index
	m.reconcile(out)
	m.clearFocusedUrgency()
	return true
}

// MoveWorkspaceDown swaps the focused output's active workspace with the
// one below it, as Niri's Monitor::move_workspace_down; it stays active.
func (m *Model) MoveWorkspaceDown() bool { return m.moveActiveWorkspace(1) }

// MoveWorkspaceUp swaps the active workspace with the one above it.
func (m *Model) MoveWorkspaceUp() bool { return m.moveActiveWorkspace(-1) }

func (m *Model) moveActiveWorkspace(delta int) bool {
	out := m.outputs[m.focusedOutput]
	if out == nil || len(out.workspaces) == 0 {
		return false
	}
	target := min(max(out.active+delta, 0), len(out.workspaces)-1)
	if target == out.active {
		return false
	}
	out.workspaces[out.active], out.workspaces[target] = out.workspaces[target], out.workspaces[out.active]
	out.active = target
	m.reconcile(out)
	return true
}

// MoveWorkspaceToIndex moves a workspace within its output to the 1-based
// index (0 counts as 1; past the end clamps), as Niri's
// Monitor::move_workspace_to_idx. workspaceID 0 is the focused output's
// active workspace. The active workspace stays active wherever it lands.
func (m *Model) MoveWorkspaceToIndex(index int, workspaceID uint64) bool {
	out := m.outputs[m.focusedOutput]
	oldIndex := -1
	if workspaceID == 0 {
		if out != nil && len(out.workspaces) != 0 {
			oldIndex = out.active
		}
	} else {
		out, oldIndex, _ = m.workspaceLocation(workspaceID)
	}
	if out == nil || oldIndex < 0 {
		return false
	}
	newIndex := min(max(index-1, 0), len(out.workspaces)-1)
	if newIndex == oldIndex {
		return false
	}
	active := out.workspaces[out.active].id
	moved := out.workspaces[oldIndex]
	out.workspaces = append(out.workspaces[:oldIndex], out.workspaces[oldIndex+1:]...)
	out.workspaces = append(out.workspaces[:newIndex], append([]*workspaceState{moved}, out.workspaces[newIndex:]...)...)
	out.active = m.workspaceIndex(out, active)
	m.reconcile(out)
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
	// Route the move through the same merged layout a reconfigure uses, so a
	// workspace that carries its own overrides keeps them on the new output
	// instead of reverting to the target's raw defaults.
	m.applyWorkspaceLayout(target, workspace)
	workspace.originalOutput = target.name

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
	if m.detached != nil {
		for i, workspace := range m.detached.workspaces {
			if workspace.id == id {
				return m.detached, i, workspace
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

// FocusColumnLeftOnWorkspace and FocusColumnRightOnWorkspace are Niri's
// FocusColumn{Left,Right}UnderMouse: they act on the workspace under the
// pointer, which in the overview need not be an active one.
func (m *Model) FocusColumnLeftOnWorkspace(id uint64) bool {
	return m.workspaceStripAction(id, (*layout.Workspace).FocusLeft)
}

func (m *Model) FocusColumnRightOnWorkspace(id uint64) bool {
	return m.workspaceStripAction(id, (*layout.Workspace).FocusRight)
}

func (m *Model) workspaceStripAction(id uint64, action func(*layout.Workspace) bool) bool {
	workspace := m.workspaceByID(id)
	if workspace == nil || !action(workspace.strip) {
		return false
	}
	m.clearFocusedUrgency()
	return true
}

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

// The column width and window height actions of Niri, on the focused output.
func (m *Model) SetColumnWidth(change layout.SizeChange) bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.SetColumnWidth(change)
	})
}

func (m *Model) SwitchPresetColumnWidth(backwards bool) bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.SwitchPresetColumnWidth(backwards)
	})
}

func (m *Model) ToggleMaximizeColumn() bool {
	return m.activeStripAction((*layout.Workspace).ToggleMaximizeColumn)
}

func (m *Model) ToggleMaximizeWindowToEdges() bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.ToggleMaximizeWindowToEdges(0)
	})
}

func (m *Model) CenterColumn() bool {
	return m.activeStripAction((*layout.Workspace).CenterColumn)
}

func (m *Model) CenterVisibleColumns() bool {
	return m.activeStripAction((*layout.Workspace).CenterVisibleColumns)
}

func (m *Model) ExpandColumnToAvailableWidth() bool {
	return m.activeStripAction((*layout.Workspace).ExpandColumnToAvailableWidth)
}

func (m *Model) SetWindowHeight(change layout.SizeChange) bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.SetWindowHeight(0, change)
	})
}

func (m *Model) ResetWindowHeight() bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.ResetWindowHeight(0)
	})
}

func (m *Model) SwitchPresetWindowHeight(backwards bool) bool {
	return m.activeStripAction(func(workspace *layout.Workspace) bool {
		return workspace.SwitchPresetWindowHeight(0, backwards)
	})
}

// SetWindowHeightFor sets the height of a window, wherever it sits. The
// window id of zero means the focused window.
func (m *Model) SetWindowHeightFor(id uint64, change layout.SizeChange) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.SetWindowHeight(windowID, change)
	})
}

// ResetWindowHeightFor gives a window its automatic height back. The window id
// of zero means the focused window.
func (m *Model) ResetWindowHeightFor(id uint64) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.ResetWindowHeight(windowID)
	})
}

// SwitchPresetWindowHeightFor cycles a window through the preset heights. The
// window id of zero means the focused window.
func (m *Model) SwitchPresetWindowHeightFor(id uint64, backwards bool) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.SwitchPresetWindowHeight(windowID, backwards)
	})
}

// ToggleMaximizeWindowToEdgesFor maximizes a window to the edges of its
// column. The window id of zero means the focused window.
func (m *Model) ToggleMaximizeWindowToEdgesFor(id uint64) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.ToggleMaximizeWindowToEdges(windowID)
	})
}

// MaximizeColumnFor makes the column that holds a window take the whole
// working width, the way Niri's open-maximized does.
func (m *Model) MaximizeColumnFor(id uint64) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.MaximizeColumn(windowID)
	})
}

// MaximizeWindowToEdgesFor maximizes a window to the edges of the working
// area, the way Niri's open-maximized-to-edges does.
func (m *Model) MaximizeWindowToEdgesFor(id uint64) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.MaximizeWindowToEdges(windowID)
	})
}

// SetColumnDisplayFor sets the display mode of the column that holds a
// window, the way Niri's default-column-display does.
func (m *Model) SetColumnDisplayFor(id uint64, display layout.ColumnDisplay) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.SetColumnDisplayFor(windowID, display)
	})
}

// SetFloatingPositionFor puts a window at a position inside the working area,
// the way Niri's default-floating-position does.
func (m *Model) SetFloatingPositionFor(id uint64, position layout.FloatingPosition) bool {
	return m.windowStripAction(id, func(workspace *layout.Workspace, windowID uint64) bool {
		return workspace.SetFloatingPosition(windowID, position)
	})
}

// windowStripAction runs an action on the workspace that holds a window. The
// window id of zero means the focused output's active workspace.
func (m *Model) windowStripAction(id uint64, action func(*layout.Workspace, uint64) bool) bool {
	if id == 0 {
		out := m.outputs[m.focusedOutput]
		if out == nil || len(out.workspaces) == 0 {
			return false
		}
		changed := action(out.workspaces[out.active].strip, 0)
		if changed {
			m.clearFocusedUrgency()
		}
		return changed
	}
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
		changed := action(workspace.strip, id)
		if changed {
			m.clearFocusedUrgency()
		}
		return changed
	}
	return false
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

// The strip actions of Niri's Workspace, on the focused output.
func (m *Model) FocusColumn(index int) bool {
	return m.activeStripAction(func(w *layout.Workspace) bool { return w.FocusColumn(index) })
}
func (m *Model) FocusColumnRightOrFirst() bool {
	return m.activeStripAction((*layout.Workspace).FocusColumnRightOrFirst)
}
func (m *Model) FocusColumnLeftOrLast() bool {
	return m.activeStripAction((*layout.Workspace).FocusColumnLeftOrLast)
}
func (m *Model) FocusWindowTop() bool { return m.activeStripAction((*layout.Workspace).FocusWindowTop) }
func (m *Model) FocusWindowBottom() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowBottom)
}
func (m *Model) FocusWindowDownOrTop() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowDownOrTop)
}
func (m *Model) FocusWindowUpOrBottom() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowUpOrBottom)
}
func (m *Model) FocusWindowDownOrColumnLeft() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowDownOrColumnLeft)
}
func (m *Model) FocusWindowDownOrColumnRight() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowDownOrColumnRight)
}
func (m *Model) FocusWindowUpOrColumnLeft() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowUpOrColumnLeft)
}
func (m *Model) FocusWindowUpOrColumnRight() bool {
	return m.activeStripAction((*layout.Workspace).FocusWindowUpOrColumnRight)
}
func (m *Model) MoveColumnToIndex(index int) bool {
	return m.activeStripAction(func(w *layout.Workspace) bool { return w.MoveColumnToIndex(index) })
}
func (m *Model) SwapWindowLeft() bool { return m.activeStripAction((*layout.Workspace).SwapWindowLeft) }
func (m *Model) SwapWindowRight() bool {
	return m.activeStripAction((*layout.Workspace).SwapWindowRight)
}

// The or-monitor composites: the plain action, or at the strip's edge the
// output in that direction, as Niri's Action::FocusColumnOrMonitorLeft and
// friends. With no output there they are the plain action.
func (m *Model) FocusColumnOrMonitorLeft() bool {
	return m.orMonitor("left", m.FocusColumnLeft, m.FocusOutputDirection)
}
func (m *Model) FocusColumnOrMonitorRight() bool {
	return m.orMonitor("right", m.FocusColumnRight, m.FocusOutputDirection)
}
func (m *Model) FocusWindowOrMonitorUp() bool {
	return m.orMonitor("up", m.FocusWindowUp, m.FocusOutputDirection)
}
func (m *Model) FocusWindowOrMonitorDown() bool {
	return m.orMonitor("down", m.FocusWindowDown, m.FocusOutputDirection)
}
func (m *Model) MoveColumnLeftOrToMonitorLeft() bool {
	return m.orMonitor("left", m.MoveColumnLeft, m.moveColumnToMonitorFollowing)
}
func (m *Model) MoveColumnRightOrToMonitorRight() bool {
	return m.orMonitor("right", m.MoveColumnRight, m.moveColumnToMonitorFollowing)
}

func (m *Model) moveColumnToMonitorFollowing(direction string) bool {
	return m.MoveFocusedColumnToMonitorDirection(direction, true)
}

func (m *Model) orMonitor(direction string, plain func() bool, cross func(string) bool) bool {
	return plain() || (m.outputInDirection(m.focusedOutput, direction) != "" && cross(direction))
}

// FocusWindowOrWorkspaceDown focuses the tile below, or else the workspace
// below (Niri's Monitor::focus_window_or_workspace_down).
func (m *Model) FocusWindowOrWorkspaceDown() bool {
	return m.FocusWindowDown() || m.FocusWorkspaceDown()
}

// FocusWindowOrWorkspaceUp focuses the tile above, or else the workspace above.
func (m *Model) FocusWindowOrWorkspaceUp() bool { return m.FocusWindowUp() || m.FocusWorkspaceUp() }

// MoveWindowDownOrToWorkspaceDown moves the window down its column, or else
// to the workspace below and follows it (monitor.rs move_down_or_to_workspace_down).
func (m *Model) MoveWindowDownOrToWorkspaceDown() bool {
	return m.MoveWindowDown() || m.MoveWindowToWorkspaceDown(true)
}

// MoveWindowUpOrToWorkspaceUp moves the window up, or else to the workspace above.
func (m *Model) MoveWindowUpOrToWorkspaceUp() bool {
	return m.MoveWindowUp() || m.MoveWindowToWorkspaceUp(true)
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
	if m.detached != nil {
		names = append(names, "")
	}
	snapshot := Snapshot{FocusedOutput: m.focusedOutput, OverviewOpen: m.overview.open}
	for _, name := range names {
		out := m.outputs[name]
		if out == nil {
			out = m.detached
		}
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
				IsActive:  name != "" && i == out.active,
				IsFocused: name != "" && name == m.focusedOutput && i == out.active,
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
	// The empty workspace above the first one is kept, as Niri's
	// clean_up_workspaces starts after it.
	first := 0
	if m.emptyWorkspaceAboveFirst {
		first = 1
	}
	for i := len(out.workspaces) - 2; i >= first; i-- {
		if i == out.active || out.workspaces[i].occupied() {
			continue
		}
		out.workspaces = append(out.workspaces[:i], out.workspaces[i+1:]...)
		if out.active > i {
			out.active--
		}
	}
	if out.workspaces[len(out.workspaces)-1].occupied() {
		out.workspaces = append(out.workspaces, m.newWorkspace(out))
	}
	if m.emptyWorkspaceAboveFirst {
		switch {
		case out.workspaces[0].occupied():
			out.workspaces = slices.Insert(out.workspaces, 0, m.newWorkspace(out))
			out.active++
		case len(out.workspaces) == 2 && !out.workspaces[1].occupied():
			// Both are empty: one empty workspace is enough.
			out.workspaces = out.workspaces[:1]
			out.active = 0
		}
	}
	if id := out.workspaces[out.active].id; id != out.activeID {
		if out.activeID != 0 {
			out.previousWorkspace = out.activeID
		}
		out.activeID = id
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
		m.applyWorkspaceLayout(out, workspace)
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
	if m.detached != nil {
		for _, workspace := range m.detached.workspaces {
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
	if m.detached != nil {
		for _, workspace := range m.detached.workspaces {
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
	workspace := &workspaceState{
		originalOutput: out.name,
		id:             id,
		strip:          layout.NewWorkspace(out.bounds, out.columnWidth, out.gap),
	}
	// The config-wide layout may have been set before this output existed and
	// a plain workspace never goes through SetWorkspaceConfig, so apply the
	// merged layout now: for an unnamed workspace the merged values are
	// exactly the config-wide ones.
	m.applyWorkspaceLayout(out, workspace)
	return workspace
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
