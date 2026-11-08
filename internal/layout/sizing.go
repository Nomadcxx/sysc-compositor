package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SizeChange is a change to a column width or a window height. A fixed size
// is given in pixels, a proportion as a percentage of the working area. A
// relative change adjusts the current size, an absolute one replaces it.
type SizeChange struct {
	// Proportion is a percentage of the working area.
	Proportion float64
	// Fixed is a size in pixels.
	Fixed int
	// Relative adjusts the current size instead of replacing it.
	Relative bool
}

// ParseSizeChange parses a size argument the way Niri does: a leading + or -
// makes the change relative, a number with a percent sign is a share of the
// working area and a bare number is a fixed pixel size.
func ParseSizeChange(arg string) (SizeChange, error) {
	text := strings.TrimSpace(arg)
	if text == "" {
		return SizeChange{}, fmt.Errorf("size must not be empty")
	}
	relative := false
	switch text[0] {
	case '+', '-':
		relative = true
	}
	if strings.HasSuffix(text, "%") {
		value, err := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
		if err != nil {
			return SizeChange{}, fmt.Errorf("invalid size %q", arg)
		}
		return SizeChange{Proportion: value, Relative: relative}, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return SizeChange{}, fmt.Errorf("invalid size %q", arg)
	}
	return SizeChange{Fixed: value, Relative: relative}, nil
}

// PresetSize is one size of a preset cycle: a fixed pixel size, or a share of
// the working size.
type PresetSize struct {
	Fixed bool
	Value float64
}

// defaultPresetSizes are the presets Niri cycles through when the config does
// not list any, as a share of the working size.
var defaultPresetSizes = []PresetSize{
	{Value: 1.0 / 3.0},
	{Value: 0.5},
	{Value: 2.0 / 3.0},
}

// SetPresetSizes sets the preset sizes that SwitchPresetColumnWidth and
// SwitchPresetWindowHeight cycle through. An empty list keeps Niri's default
// presets.
func (w *Workspace) SetPresetSizes(columnWidths, windowHeights []PresetSize) {
	w.presetColumnWidths = columnWidths
	w.presetWindowHeights = windowHeights
}

// columnPresets returns the preset column widths of the workspace, falling back
// to the default presets when the config does not list any.
func (w *Workspace) columnPresets() []PresetSize {
	if len(w.presetColumnWidths) == 0 {
		return defaultPresetSizes
	}
	return w.presetColumnWidths
}

// windowPresets returns the preset window heights of the workspace, falling
// back to the default presets when the config does not list any.
func (w *Workspace) windowPresets() []PresetSize {
	if len(w.presetWindowHeights) == 0 {
		return defaultPresetSizes
	}
	return w.presetWindowHeights
}

// presetPixels turns a preset into pixels against the working size. A fixed
// preset is its own pixel size, a proportional one takes the same share math as
// a proportional column width.
func (w *Workspace) presetPixels(preset PresetSize, working int) int {
	target := int(preset.Value)
	if !preset.Fixed {
		target = w.proportionPixels(preset.Value, working)
	}
	if working <= 0 {
		return target
	}
	return min(max(1, target), working)
}

// ColumnWidthPixels returns the width of a column that takes the given share
// of the available width, with the gaps around it.
func ColumnWidthPixels(available, gap int, proportion float64) int {
	if available <= 0 {
		return 0
	}
	if proportion <= 0 {
		proportion = 0.5
	}
	width := (float64(available-gap) * proportion) - float64(gap)
	return max(1, int(math.Round(width)))
}

// MaximizeColumn makes the column that holds a window take the whole working
// width, the way Niri's open-maximized does. The window id of zero means the
// focused window.
func (w *Workspace) MaximizeColumn(id uint64) bool {
	columnIndex, _, ok := w.windowTile(id)
	if !ok {
		return false
	}
	column := &w.columns[columnIndex]
	if column.fullWidth {
		return false
	}
	column.fullWidth = true
	column.presetWidth = false
	w.revealFocused()
	return true
}

// MaximizeWindowToEdges makes a window fill the height of its column and its
// column the whole working width, the way Niri's open-maximized-to-edges
// does. The window id of zero means the focused window.
func (w *Workspace) MaximizeWindowToEdges(id uint64) bool {
	columnIndex, tileIndex, ok := w.windowTile(id)
	if !ok {
		return false
	}
	column := &w.columns[columnIndex]
	tile := &column.tiles[tileIndex]
	if tile.maximized {
		return false
	}
	tile.maximized = true
	column.presetWidth = false
	w.revealFocused()
	return true
}

// SetColumnDisplayFor sets the display mode of the column that holds a
// window, the way Niri's default-column-display does. The window id of zero
// means the focused window.
func (w *Workspace) SetColumnDisplayFor(id uint64, display ColumnDisplay) bool {
	columnIndex, _, ok := w.windowTile(id)
	if !ok || display > ColumnDisplayTabbed {
		return false
	}
	column := &w.columns[columnIndex]
	if column.display == display {
		return false
	}
	column.display = display
	return true
}

// SetFloatingPosition puts a window at a position inside the working area,
// the way Niri's default-floating-position does. A window that is not
// floating yet keeps the position for the day it floats.
func (w *Workspace) SetFloatingPosition(id uint64, position FloatingPosition) bool {
	floatIndex := w.floatingIndexOf(id)
	var tile *tileState
	var bounds Bounds
	if floatIndex >= 0 {
		bounds = w.floating[floatIndex].bounds
		tile = &w.floating[floatIndex].tile
	} else {
		columnIndex, tileIndex, ok := w.locationOf(id)
		if !ok {
			return false
		}
		tile = &w.columns[columnIndex].tiles[tileIndex]
		if tile.hasFloatingBounds {
			bounds = tile.floatingBounds
		} else {
			placement := w.placementAt(columnIndex, tileIndex)
			bounds.Width, bounds.Height = placement.Width, placement.Height
		}
	}
	x, y := position.resolve(w.bounds, bounds.Width, bounds.Height)
	bounds.X, bounds.Y = x, y
	if floatIndex >= 0 {
		return w.SetFloatingGeometry(id, bounds)
	}
	if tile.hasFloatingBounds && tile.floatingBounds == bounds {
		return false
	}
	tile.floatingBounds, tile.hasFloatingBounds = bounds, true
	return true
}

// SetColumnWidth sets the focused column's width, the way Niri's
// set-column-width does.
func (w *Workspace) SetColumnWidth(change SizeChange) bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	current := w.columnWidthAt(w.focused)
	target, ok := w.resolveSize(change, current, w.bounds.Width)
	if !ok {
		return false
	}
	column := &w.columns[w.focused]
	column.width = target
	if target == w.columnWidth {
		column.width = 0
	}
	column.fullWidth = false
	column.presetWidth = false
	for index := range column.tiles {
		column.tiles[index].maximized = false
	}
	w.revealFocused()
	return true
}

// SwitchPresetColumnWidth cycles the focused column through the preset
// widths.
func (w *Workspace) SwitchPresetColumnWidth(backwards bool) bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	presets := w.columnPresets()
	column := &w.columns[w.focused]
	current := w.columnWidthAt(w.focused)
	index := -1
	if column.presetWidth && !column.fullWidth && !column.hasMaximizedTile() {
		index = column.presetWidthIdx
	}
	var next int
	if index >= 0 {
		if backwards {
			next = (index + len(presets) - 1) % len(presets)
		} else {
			next = (index + 1) % len(presets)
		}
	} else {
		next = w.presetIndex(presets, current, w.bounds.Width, backwards)
	}
	column.presetWidth = true
	column.presetWidthIdx = next
	column.fullWidth = false
	target := w.presetPixels(presets[next], w.bounds.Width)
	column.width = target
	if target == w.columnWidth {
		column.width = 0
	}
	for index := range column.tiles {
		column.tiles[index].maximized = false
	}
	w.revealFocused()
	return true
}

// ToggleMaximizeColumn makes the focused column take the whole working width
// and back again. A column with a window maximized to the edges loses that
// instead, the way Niri treats the two as one state.
func (w *Workspace) ToggleMaximizeColumn() bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	column := &w.columns[w.focused]
	if column.hasMaximizedTile() {
		for index := range column.tiles {
			column.tiles[index].maximized = false
		}
		column.fullWidth = false
	} else {
		column.fullWidth = !column.fullWidth
		if column.fullWidth {
			column.presetWidth = false
		}
	}
	w.revealFocused()
	return true
}

// ToggleMaximizeWindowToEdges makes a window fill the working area and back
// again. The window id of zero means the focused window.
func (w *Workspace) ToggleMaximizeWindowToEdges(id uint64) bool {
	columnIndex, tileIndex, ok := w.windowTile(id)
	if !ok {
		return false
	}
	column := &w.columns[columnIndex]
	tile := &column.tiles[tileIndex]
	tile.maximized = !tile.maximized
	if tile.maximized {
		column.presetWidth = false
	}
	w.revealFocused()
	return true
}

// CenterColumn scrolls the view so the focused column sits in the middle of
// the working area.
func (w *Workspace) CenterColumn() bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	width := w.columnWidthAt(w.focused)
	left := w.columnStart(w.focused)
	w.viewOffset = left - (w.bounds.Width-width)/2
	return true
}

// CenterVisibleColumns scrolls the view so the columns that are fully visible
// sit in the middle of the working area.
func (w *Workspace) CenterVisibleColumns() bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	visible := w.visibleColumns()
	if len(visible) == 0 {
		return false
	}
	taken := 0
	left := 0
	found := false
	for position, index := range visible {
		if position == 0 {
			left = w.columnStart(index)
		}
		taken += w.columnWidthAt(index) + w.gap
		if index == w.focused {
			found = true
		}
	}
	if !found {
		return false
	}
	free := w.bounds.Width - taken + w.gap
	w.viewOffset = left - free/2
	return true
}

// ExpandColumnToAvailableWidth grows the focused column into the space the
// other visible columns do not use.
func (w *Workspace) ExpandColumnToAvailableWidth() bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	column := &w.columns[w.focused]
	if column.fullWidth || column.hasMaximizedTile() {
		// A column that already fills the width has nothing to grow into.
		return false
	}
	visible := w.visibleColumns()
	if len(visible) == 0 {
		return false
	}
	taken := 0
	left := 0
	countedOther := false
	found := false
	for position, index := range visible {
		if position == 0 {
			left = w.columnStart(index)
		}
		taken += w.columnWidthAt(index) + w.gap
		if index == w.focused {
			found = true
		} else {
			countedOther = true
		}
	}
	if !found {
		return false
	}
	if !countedOther {
		// The focused column is the only visible one, so it takes the whole
		// working width.
		column.fullWidth = true
		column.presetWidth = false
		w.revealFocused()
		return true
	}
	available := w.bounds.Width - w.gap - taken
	if available <= 0 {
		return false
	}
	target := w.columnWidthAt(w.focused) + available
	if target > w.bounds.Width {
		target = w.bounds.Width
	}
	column.width = target
	if target == w.columnWidth {
		column.width = 0
	}
	column.fullWidth = false
	column.presetWidth = false
	w.viewOffset = left - w.gap
	w.revealFocused()
	return true
}

// SetWindowHeight sets the height of a window inside its column, the way
// Niri's set-window-height does. The window id of zero means the focused
// window.
func (w *Workspace) SetWindowHeight(id uint64, change SizeChange) bool {
	columnIndex, tileIndex, ok := w.windowTile(id)
	if !ok {
		return false
	}
	current := w.windowHeightAt(columnIndex, tileIndex)
	target, ok := w.resolveSize(change, current, w.bounds.Height)
	if !ok {
		return false
	}
	column := &w.columns[columnIndex]
	tile := &column.tiles[tileIndex]
	// Niri keeps the height it was given, even when it happens to match the
	// automatic height, so that the other windows of the column keep their
	// own share of the space.
	tile.height = target
	tile.maximized = false
	tile.presetHeight = false
	return true
}

// ResetWindowHeight gives a window its automatic height back. The window id of
// zero means the focused window.
func (w *Workspace) ResetWindowHeight(id uint64) bool {
	columnIndex, tileIndex, ok := w.windowTile(id)
	if !ok {
		return false
	}
	tile := &w.columns[columnIndex].tiles[tileIndex]
	tile.height = 0
	tile.maximized = false
	tile.presetHeight = false
	return true
}

// SwitchPresetWindowHeight cycles a window through the preset heights. The
// window id of zero means the focused window.
func (w *Workspace) SwitchPresetWindowHeight(id uint64, backwards bool) bool {
	columnIndex, tileIndex, ok := w.windowTile(id)
	if !ok {
		return false
	}
	current := w.windowHeightAt(columnIndex, tileIndex)
	tile := &w.columns[columnIndex].tiles[tileIndex]
	presets := w.windowPresets()
	index := -1
	if tile.presetHeight && !tile.maximized {
		index = tile.presetHeightIdx
	}
	var next int
	if index >= 0 {
		if backwards {
			next = (index + len(presets) - 1) % len(presets)
		} else {
			next = (index + 1) % len(presets)
		}
	} else {
		next = w.presetIndex(presets, current, w.bounds.Height, backwards)
	}
	tile.presetHeight = true
	tile.presetHeightIdx = next
	tile.maximized = false
	target := w.presetPixels(presets[next], w.bounds.Height)
	tile.height = target
	return true
}

// resolveSize turns a size change into a pixel size against the working size.
func (w *Workspace) resolveSize(change SizeChange, current, working int) (int, bool) {
	if working <= 0 || working <= w.gap {
		return 0, false
	}
	var target int
	if change.Fixed != 0 {
		if change.Relative {
			target = current + change.Fixed
		} else {
			target = change.Fixed
		}
	} else {
		share := change.Proportion / 100
		if change.Relative {
			// Niri measures a relative change against the share of the
			// working area that the column or the window already takes.
			currentShare := (float64(current) + float64(w.gap)) / float64(working-w.gap)
			share = currentShare + change.Proportion/100
		}
		target = w.proportionPixels(share, working)
	}
	if target < 1 {
		target = 1
	}
	if target > working {
		target = working
	}
	return target, true
}

// presetIndex picks the preset to use for a column or window that is not on a
// preset yet: the first preset larger than the current size, or the last one
// smaller than it, wrapping around. Presets compare in pixels so a cycle can
// mix fixed and proportional sizes, and Niri allows one pixel of slack for
// fractional scaling.
func (w *Workspace) presetIndex(presets []PresetSize, current, working int, backwards bool) int {
	if backwards {
		for index := len(presets) - 1; index >= 0; index-- {
			if w.presetPixels(presets[index], working)+1 < current {
				return index
			}
		}
		return len(presets) - 1
	}
	for index, preset := range presets {
		if current+1 < w.presetPixels(preset, working) {
			return index
		}
	}
	return 0
}

// proportionPixels turns a share of the working area into pixels, the way Niri
// does: the share applies to the space the gap leaves, and the gap comes off
// again.
func (w *Workspace) proportionPixels(share float64, working int) int {
	return int(math.Round((float64(working-w.gap) * share) - float64(w.gap)))
}

// focusedTile returns the focused column and the focused window inside it.
func (w *Workspace) focusedTile() (int, int, bool) {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return 0, 0, false
	}
	column := w.columns[w.focused]
	if len(column.tiles) == 0 {
		return 0, 0, false
	}
	return w.focused, column.active, true
}

// windowTile returns the column and the tile of a window. The window id of
// zero means the focused window.
func (w *Workspace) windowTile(id uint64) (int, int, bool) {
	if id == 0 {
		return w.focusedTile()
	}
	for columnIndex := range w.columns {
		for tileIndex, tile := range w.columns[columnIndex].tiles {
			if tile.id == id {
				return columnIndex, tileIndex, true
			}
		}
	}
	return 0, 0, false
}

// visibleColumns returns the columns that are fully inside the working area.
func (w *Workspace) visibleColumns() []int {
	var visible []int
	for index := range w.columns {
		left := w.columnStart(index)
		right := left + w.columnWidthAt(index)
		if left < w.viewOffset+w.gap {
			// The column runs off the left edge.
			continue
		}
		if right+w.gap > w.viewOffset+w.bounds.Width {
			// The column runs off the right edge.
			break
		}
		visible = append(visible, index)
	}
	return visible
}
