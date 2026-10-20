// Package layout holds compositor-independent column order and viewport geometry.
package layout

import "math"

// Bounds describes one workspace's visible output area in layout coordinates.
type Bounds struct {
	X, Y, Width, Height int
}

type PositionChangeKind string

const (
	PositionSetFixed         PositionChangeKind = "SetFixed"
	PositionSetProportion    PositionChangeKind = "SetProportion"
	PositionAdjustFixed      PositionChangeKind = "AdjustFixed"
	PositionAdjustProportion PositionChangeKind = "AdjustProportion"
)

type PositionChange struct {
	Kind  PositionChangeKind
	Value float64
}

// ColumnDisplay controls whether a column's windows are tiled vertically or
// share one tile with only the active window visible.
type ColumnDisplay uint8

const (
	ColumnDisplayNormal ColumnDisplay = iota
	ColumnDisplayTabbed
)

// Placement is a window's tile in the visible workspace view.
type Placement struct {
	ID                  uint64
	Column, Tile        uint32
	Active, Visible     bool
	Floating            bool
	Display             ColumnDisplay
	X, Y, Width, Height int
}

// SetWindowFloating moves one window between the tiled and floating layers.
func (w *Workspace) SetWindowFloating(id uint64, floating bool, bounds Bounds) bool {
	floatIndex := w.floatingIndexOf(id)
	if floating == (floatIndex >= 0) {
		return false
	}
	if floating {
		columnIndex, tileIndex, ok := w.locationOf(id)
		if !ok {
			return false
		}
		placement := w.placementAt(columnIndex, tileIndex)
		column := &w.columns[columnIndex]
		tile := column.tiles[tileIndex]
		wasFocused := w.focusedFloat == 0 && w.focused == columnIndex && column.active == tileIndex
		if tile.hasFloatingBounds {
			bounds = tile.floatingBounds
		} else if bounds.Width <= 0 || bounds.Height <= 0 {
			bounds = Bounds{X: placement.X, Y: placement.Y, Width: placement.Width, Height: placement.Height}
		}
		bounds.Width = clampSizeHint(bounds.Width, tile.minWidth, tile.maxWidth)
		bounds.Height = clampSizeHint(bounds.Height, tile.minHeight, tile.maxHeight)
		tile.floatingBounds, tile.hasFloatingBounds = bounds, true
		f := floatingState{
			tile: tile, bounds: bounds, column: columnIndex, tileIndex: tileIndex,
			columnWidth: column.width, restoreTileGroup: len(column.tiles) > 1,
		}
		column.tiles = append(column.tiles[:tileIndex], column.tiles[tileIndex+1:]...)
		if tileIndex < column.active {
			column.active--
		} else if tileIndex == column.active && column.active == len(column.tiles) {
			column.active--
		}
		if len(column.tiles) == 0 {
			w.columns = append(w.columns[:columnIndex], w.columns[columnIndex+1:]...)
			if columnIndex < w.focused {
				w.focused--
			} else if columnIndex == w.focused {
				if len(w.columns) == 0 {
					w.focused = -1
				} else if w.focused == len(w.columns) {
					w.focused--
				}
			}
		}
		w.floating = append(w.floating, f)
		if wasFocused {
			w.focusedFloat = id
		}
		w.revealFocused()
		return true
	}

	f := w.floating[floatIndex]
	w.floating = append(w.floating[:floatIndex], w.floating[floatIndex+1:]...)
	columnIndex := min(f.column, len(w.columns))
	if f.restoreTileGroup && columnIndex < len(w.columns) {
		column := &w.columns[columnIndex]
		tileIndex := min(f.tileIndex, len(column.tiles))
		column.tiles = append(column.tiles, tileState{})
		copy(column.tiles[tileIndex+1:], column.tiles[tileIndex:])
		column.tiles[tileIndex] = f.tile
		if tileIndex <= column.active && len(column.tiles) > 1 {
			column.active++
		}
	} else {
		w.columns = append(w.columns, columnState{})
		copy(w.columns[columnIndex+1:], w.columns[columnIndex:])
		w.columns[columnIndex] = columnState{tiles: []tileState{f.tile}, width: f.columnWidth}
		if w.focused >= columnIndex {
			w.focused++
		}
	}
	if w.focusedFloat == id {
		w.focusedFloat = 0
		w.focused = columnIndex
		if w.activeFloat == id {
			w.activeFloat = 0
		}
		if f.restoreTileGroup && columnIndex < len(w.columns) {
			w.columns[columnIndex].active = min(f.tileIndex, len(w.columns[columnIndex].tiles)-1)
		}
	} else if w.activeFloat == id {
		w.activeFloat = 0
	}
	w.revealFocused()
	return true
}

// SetFloatingGeometry updates a floating window's output-relative rectangle.
func (w *Workspace) SetFloatingGeometry(id uint64, bounds Bounds) bool {
	index := w.floatingIndexOf(id)
	if index < 0 || bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	floating := &w.floating[index]
	bounds.Width = clampSizeHint(bounds.Width, floating.tile.minWidth, floating.tile.maxWidth)
	bounds.Height = clampSizeHint(bounds.Height, floating.tile.minHeight, floating.tile.maxHeight)
	if floating.bounds == bounds {
		return false
	}
	floating.bounds = bounds
	floating.tile.floatingBounds = bounds
	floating.tile.hasFloatingBounds = true
	return true
}

// SetSavedFloatingGeometry updates the rectangle a tiled window will use when floated.
func (w *Workspace) SetSavedFloatingGeometry(id uint64, bounds Bounds) bool {
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	if index := w.floatingIndexOf(id); index >= 0 {
		return w.SetFloatingGeometry(id, bounds)
	}
	columnIndex, tileIndex, ok := w.locationOf(id)
	if !ok {
		return false
	}
	tile := &w.columns[columnIndex].tiles[tileIndex]
	bounds.Width = clampSizeHint(bounds.Width, tile.minWidth, tile.maxWidth)
	bounds.Height = clampSizeHint(bounds.Height, tile.minHeight, tile.maxHeight)
	if tile.hasFloatingBounds && tile.floatingBounds == bounds {
		return false
	}
	tile.floatingBounds, tile.hasFloatingBounds = bounds, true
	return true
}

// MoveFloatingWindow changes a window's floating position, retaining it when
// the window is tiled so a later move-to-floating uses the requested position.
func (w *Workspace) MoveFloatingWindow(id uint64, x, y PositionChange) bool {
	if id == 0 || !validPositionChange(x) || !validPositionChange(y) {
		return false
	}
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
			if !positionIsSet(x.Kind) || !positionIsSet(y.Kind) {
				return false
			}
			placement := w.placementAt(columnIndex, tileIndex)
			bounds.Width, bounds.Height = placement.Width, placement.Height
		}
	}

	bounds.X = applyPositionChange(x, bounds.X, w.bounds.X, w.bounds.Width)
	bounds.Y = applyPositionChange(y, bounds.Y, w.bounds.Y, w.bounds.Height)
	if floatIndex >= 0 {
		return w.SetFloatingGeometry(id, bounds)
	}
	if tile.hasFloatingBounds && tile.floatingBounds == bounds {
		return false
	}
	tile.floatingBounds, tile.hasFloatingBounds = bounds, true
	return true
}

func validPositionChange(change PositionChange) bool {
	if math.IsNaN(change.Value) || math.IsInf(change.Value, 0) {
		return false
	}
	switch change.Kind {
	case PositionSetFixed, PositionSetProportion, PositionAdjustFixed, PositionAdjustProportion:
		return true
	default:
		return false
	}
}

func positionIsSet(kind PositionChangeKind) bool {
	return kind == PositionSetFixed || kind == PositionSetProportion
}

func applyPositionChange(change PositionChange, current, origin, length int) int {
	position := float64(current)
	areaOrigin, areaLength := float64(origin), float64(length)
	switch change.Kind {
	case PositionSetFixed:
		position = areaOrigin + change.Value
	case PositionSetProportion:
		proportion := min(10000.0, max(0.0, change.Value/100))
		position = areaOrigin + areaLength*proportion
	case PositionAdjustFixed:
		position += change.Value
	case PositionAdjustProportion:
		currentProportion := (position - areaOrigin) / max(areaLength, 1)
		proportion := min(10000.0, max(0.0, currentProportion+change.Value/100))
		position = areaOrigin + areaLength*proportion
	}
	const minScenePosition = -1 << 31
	const maxScenePosition = 1<<31 - 1
	if position <= minScenePosition {
		return minScenePosition
	}
	if position >= maxScenePosition {
		return maxScenePosition
	}
	return int(math.Round(position))
}

// ResizeOptions selects the changed axes and the edges fixed by a resize.
type ResizeOptions struct {
	ResizeWidth, ResizeHeight bool
	AnchorRight, AnchorBottom bool
}

type tileState struct {
	id                   uint64
	height               int
	minWidth, maxWidth   int
	minHeight, maxHeight int
	yOffset              int
	floatingBounds       Bounds
	hasFloatingBounds    bool
}

type columnState struct {
	tiles   []tileState
	active  int
	display ColumnDisplay
	width   int
}

type floatingState struct {
	tile              tileState
	bounds            Bounds
	column, tileIndex int
	columnWidth       int
	restoreTileGroup  bool
}

// Workspace is one horizontal strip of columns. A column can contain multiple
// windows; focus is a column and an active window within that column.
type Workspace struct {
	bounds       Bounds
	columnWidth  int
	gap          int
	columns      []columnState
	floating     []floatingState
	focused      int
	focusedFloat uint64
	activeFloat  uint64
	viewOffset   int
}

const directionalMovePixels = 50

// NewWorkspace creates an empty workspace with fixed column width and gap.
func NewWorkspace(bounds Bounds, columnWidth, gap int) *Workspace {
	w := &Workspace{focused: -1}
	w.SetGeometry(bounds, columnWidth, gap)
	return w
}

// SetGeometry updates the view size and recomputes the focused column's offset.
func (w *Workspace) SetGeometry(bounds Bounds, columnWidth, gap int) {
	if bounds.Width < 0 {
		bounds.Width = 0
	}
	if bounds.Height < 0 {
		bounds.Height = 0
	}
	if columnWidth < 0 {
		columnWidth = 0
	}
	if gap < 0 {
		gap = 0
	}
	if w.bounds != bounds {
		for i := range w.floating {
			w.floating[i].bounds = RemapFloatingBounds(w.bounds, bounds, w.floating[i].bounds)
			w.floating[i].tile.floatingBounds = w.floating[i].bounds
		}
	}
	w.bounds = bounds
	w.columnWidth = columnWidth
	w.gap = gap
	w.revealFocused()
}

// AddWindow inserts a new column immediately to the right of the focused one.
func (w *Workspace) AddWindow(id uint64) {
	if id == 0 || w.indexOf(id) >= 0 || w.floatingIndexOf(id) >= 0 {
		return
	}
	idx := 0
	if w.focused >= 0 {
		idx = w.focused + 1
	}
	w.columns = append(w.columns, columnState{})
	copy(w.columns[idx+1:], w.columns[idx:])
	w.columns[idx] = columnState{tiles: []tileState{{id: id}}}
	w.focused = idx
	w.focusedFloat = 0
	w.revealFocused()
}

// RemoveWindow removes a mapped window, keeping its column while other windows remain.
func (w *Workspace) RemoveWindow(id uint64) {
	if index := w.floatingIndexOf(id); index >= 0 {
		w.floating = append(w.floating[:index], w.floating[index+1:]...)
		if w.activeFloat == id {
			w.activeFloat = 0
			if len(w.floating) != 0 {
				w.activeFloat = w.floating[min(index, len(w.floating)-1)].tile.id
			}
		}
		if w.focusedFloat == id {
			w.focusedFloat = w.activeFloat
		}
		return
	}
	columnIndex, tileIndex, ok := w.locationOf(id)
	if !ok {
		return
	}
	column := &w.columns[columnIndex]
	column.tiles = append(column.tiles[:tileIndex], column.tiles[tileIndex+1:]...)
	if tileIndex < column.active {
		column.active--
	} else if tileIndex == column.active && column.active == len(column.tiles) {
		column.active--
	}
	if len(column.tiles) != 0 {
		w.revealFocused()
		return
	}
	w.columns = append(w.columns[:columnIndex], w.columns[columnIndex+1:]...)
	if len(w.columns) == 0 {
		w.focused = -1
		w.viewOffset = 0
		return
	}
	if columnIndex < w.focused {
		w.focused--
	} else if columnIndex == w.focused && w.focused == len(w.columns) {
		w.focused--
	}
	w.revealFocused()
}

// MoveWindowTo transfers one tile to a new column on target while preserving
// its constraints, resize state, floating rectangle, and stable ID.
func (w *Workspace) MoveWindowTo(target *Workspace, id uint64) bool {
	if target == nil || target == w || target.indexOf(id) >= 0 || target.floatingIndexOf(id) >= 0 {
		return false
	}
	floatingIndex := w.floatingIndexOf(id)
	if floatingIndex >= 0 {
		floating := w.floating[floatingIndex]
		floating.bounds = RemapFloatingBounds(w.bounds, target.bounds, floating.bounds)
		floating.tile.floatingBounds = floating.bounds
		w.RemoveWindow(id)
		target.AddWindow(id)
		column, tile, ok := target.locationOf(id)
		if !ok {
			return false
		}
		target.columns[column].tiles[tile] = floating.tile
		target.columns[column].width = floating.columnWidth
		return target.SetWindowFloating(id, true, floating.bounds)
	}

	column, tile, ok := w.locationOf(id)
	if !ok {
		return false
	}
	window := w.columns[column].tiles[tile]
	width := w.columns[column].width
	w.RemoveWindow(id)
	target.AddWindow(id)
	newColumn, newTile, ok := target.locationOf(id)
	if !ok {
		return false
	}
	target.columns[newColumn].tiles[newTile] = window
	target.columns[newColumn].width = width
	target.revealFocused()
	return true
}

// RemapFloatingBounds carries a floating window's relative position to another workspace.
func RemapFloatingBounds(source, target, bounds Bounds) Bounds {
	bounds.X = remapFloatingPosition(bounds.X, source.X, source.Width, target.X, target.Width, bounds.Width)
	bounds.Y = remapFloatingPosition(bounds.Y, source.Y, source.Height, target.Y, target.Height, bounds.Height)
	return bounds
}

func remapFloatingPosition(position, sourceOrigin, sourceSize, targetOrigin, targetSize, windowSize int) int {
	fraction := float64(position-sourceOrigin) / float64(max(sourceSize, 1))
	positionInTarget := float64(targetOrigin) + fraction*float64(targetSize)
	minVisible := math.Max(10, math.Min(float64(windowSize)/4, 75))
	maxOffscreen := math.Max(0, float64(windowSize)-minVisible)
	minPosition := float64(targetOrigin) - maxOffscreen
	maxPosition := float64(targetOrigin+targetSize-windowSize) + maxOffscreen
	return int(math.Round(math.Max(minPosition, math.Min(positionInTarget, maxPosition))))
}

// MoveFocusedColumnTo transfers the focused tiled column, including all of
// its tabs, to target immediately after the target's focused column.
func (w *Workspace) MoveFocusedColumnTo(target *Workspace) bool {
	if target == nil || target == w || w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	column := w.columns[w.focused]
	for _, tile := range column.tiles {
		if target.indexOf(tile.id) >= 0 || target.floatingIndexOf(tile.id) >= 0 {
			return false
		}
	}
	index := w.focused
	w.columns = append(w.columns[:index], w.columns[index+1:]...)
	if len(w.columns) == 0 {
		w.focused = -1
		w.viewOffset = 0
	} else if index < w.focused {
		w.focused--
	} else if index == w.focused && w.focused == len(w.columns) {
		w.focused--
	}
	w.revealFocused()

	targetIndex := 0
	if target.focused >= 0 {
		targetIndex = target.focused + 1
	}
	target.columns = append(target.columns, columnState{})
	copy(target.columns[targetIndex+1:], target.columns[targetIndex:])
	target.columns[targetIndex] = column
	target.focused = targetIndex
	target.focusedFloat = 0
	target.revealFocused()
	return true
}

// FocusedColumnWindowIDs returns the focused column's tile IDs in tab order.
func (w *Workspace) FocusedColumnWindowIDs() []uint64 {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return nil
	}
	tiles := w.columns[w.focused].tiles
	ids := make([]uint64, len(tiles))
	for i, tile := range tiles {
		ids[i] = tile.id
	}
	return ids
}

// FocusLeft focuses the previous column when one exists.
func (w *Workspace) FocusLeft() bool {
	if w.focused <= 0 {
		return false
	}
	w.focused--
	w.revealFocused()
	return true
}

// FocusRight focuses the next column when one exists.
func (w *Workspace) FocusRight() bool {
	if w.focused < 0 || w.focused+1 >= len(w.columns) {
		return false
	}
	w.focused++
	w.revealFocused()
	return true
}

func (w *Workspace) FocusFirst() bool {
	if w.focused <= 0 || len(w.columns) == 0 {
		return false
	}
	w.focused = 0
	w.revealFocused()
	return true
}

func (w *Workspace) FocusLast() bool {
	last := len(w.columns) - 1
	if last < 0 || w.focused == last {
		return false
	}
	w.focused = last
	w.revealFocused()
	return true
}

// MoveLeft swaps the focused column with the one to its left.
func (w *Workspace) MoveLeft() bool {
	if w.focused <= 0 {
		return false
	}
	w.columns[w.focused], w.columns[w.focused-1] = w.columns[w.focused-1], w.columns[w.focused]
	w.focused--
	w.revealFocused()
	return true
}

// MoveRight swaps the focused column with the one to its right.
func (w *Workspace) MoveRight() bool {
	if w.focused < 0 || w.focused+1 >= len(w.columns) {
		return false
	}
	w.columns[w.focused], w.columns[w.focused+1] = w.columns[w.focused+1], w.columns[w.focused]
	w.focused++
	w.revealFocused()
	return true
}

func (w *Workspace) MoveFirst() bool {
	if w.focused <= 0 {
		return false
	}
	column := w.columns[w.focused]
	copy(w.columns[1:w.focused+1], w.columns[:w.focused])
	w.columns[0] = column
	w.focused = 0
	w.revealFocused()
	return true
}

func (w *Workspace) MoveLast() bool {
	last := len(w.columns) - 1
	if w.focused < 0 || w.focused >= last {
		return false
	}
	column := w.columns[w.focused]
	copy(w.columns[w.focused:last], w.columns[w.focused+1:])
	w.columns[last] = column
	w.focused = last
	w.revealFocused()
	return true
}

// ConsumeWindowIntoColumn moves the first window of the next column into the
// focused column. The focused window remains active, matching Niri's action.
func (w *Workspace) ConsumeWindowIntoColumn() bool {
	if w.focused < 0 || w.focused+1 >= len(w.columns) {
		return false
	}
	source := &w.columns[w.focused+1]
	if len(source.tiles) == 0 {
		return false
	}
	tile := source.tiles[0]
	source.tiles = source.tiles[1:]
	if source.active > 0 {
		source.active--
	}
	target := &w.columns[w.focused]
	target.tiles = append(target.tiles, tile)
	if len(source.tiles) == 0 {
		w.columns = append(w.columns[:w.focused+1], w.columns[w.focused+2:]...)
	}
	w.revealFocused()
	return true
}

// ExpelWindowFromColumn moves the bottom window of the focused column into a
// new column to its right without changing the focused column.
func (w *Workspace) ExpelWindowFromColumn() bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	source := &w.columns[w.focused]
	if len(source.tiles) < 2 {
		return false
	}
	last := len(source.tiles) - 1
	tile := source.tiles[last]
	source.tiles = source.tiles[:last]
	if source.active >= last {
		source.active = last - 1
	}
	idx := w.focused + 1
	w.columns = append(w.columns, columnState{})
	copy(w.columns[idx+1:], w.columns[idx:])
	w.columns[idx] = columnState{tiles: []tileState{tile}}
	w.revealFocused()
	return true
}

// ConsumeOrExpelWindowRight consumes a lone window into the adjacent column,
// or expels the selected tile into a new column to the right.
func (w *Workspace) ConsumeOrExpelWindowRight(id *uint64) bool {
	return w.consumeOrExpelWindow(id, 1)
}

// ConsumeOrExpelWindowLeft consumes a lone window into the adjacent column,
// or expels the selected tile into a new column to the left.
func (w *Workspace) ConsumeOrExpelWindowLeft(id *uint64) bool {
	return w.consumeOrExpelWindow(id, -1)
}

func (w *Workspace) consumeOrExpelWindow(id *uint64, direction int) bool {
	if id == nil && w.focusedFloat != 0 {
		return false
	}
	var windowID uint64
	if id == nil {
		var ok bool
		windowID, ok = w.FocusedWindow()
		if !ok {
			return false
		}
	} else {
		windowID = *id
	}
	columnIndex, tileIndex, ok := w.locationOf(windowID)
	if !ok {
		return false
	}
	column := &w.columns[columnIndex]
	wasActive := w.focused == columnIndex && w.focusedFloat == 0 && column.active == tileIndex
	tile := column.tiles[tileIndex]
	width := column.width

	if len(column.tiles) == 1 {
		if direction < 0 && columnIndex == 0 || direction > 0 && columnIndex+1 == len(w.columns) {
			return false
		}
		w.RemoveWindow(windowID)
		targetIndex := columnIndex
		if direction < 0 {
			targetIndex--
		}
		target := &w.columns[targetIndex]
		target.tiles = append(target.tiles, tile)
		if wasActive {
			target.active = len(target.tiles) - 1
			w.focused = targetIndex
			w.focusedFloat = 0
		}
		w.revealFocused()
		return true
	}

	w.RemoveWindow(windowID)
	insertAt := columnIndex
	if direction > 0 {
		insertAt++
	}
	if w.focused >= insertAt {
		w.focused++
	}
	w.columns = append(w.columns, columnState{})
	copy(w.columns[insertAt+1:], w.columns[insertAt:])
	w.columns[insertAt] = columnState{tiles: []tileState{tile}, width: width}
	if wasActive {
		w.focused = insertAt
		w.focusedFloat = 0
	}
	w.revealFocused()
	return true
}

// ToggleColumnTabbedDisplay toggles the focused column's display mode.
func (w *Workspace) ToggleColumnTabbedDisplay() bool {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	column := &w.columns[w.focused]
	if column.display == ColumnDisplayTabbed {
		column.display = ColumnDisplayNormal
	} else {
		column.display = ColumnDisplayTabbed
	}
	return true
}

// SetColumnDisplay sets the focused column's display mode.
func (w *Workspace) SetColumnDisplay(display ColumnDisplay) bool {
	if w.focused < 0 || w.focused >= len(w.columns) || display > ColumnDisplayTabbed {
		return false
	}
	w.columns[w.focused].display = display
	return true
}

// WindowOrder returns a copy of the strip's windows, flattened by column and tile.
func (w *Workspace) WindowOrder() []uint64 {
	order := make([]uint64, 0, len(w.floating))
	for _, column := range w.columns {
		for _, tile := range column.tiles {
			order = append(order, tile.id)
		}
	}
	for _, floating := range w.floating {
		order = append(order, floating.tile.id)
	}
	return order
}

// FocusedWindow returns the active column's window, if one exists.
func (w *Workspace) FocusedWindow() (uint64, bool) {
	if w.focusedFloat != 0 && w.floatingIndexOf(w.focusedFloat) >= 0 {
		return w.focusedFloat, true
	}
	if w.focused < 0 || w.focused >= len(w.columns) {
		return 0, false
	}
	column := w.columns[w.focused]
	if column.active < 0 || column.active >= len(column.tiles) {
		return 0, false
	}
	return column.tiles[column.active].id, true
}

// FocusWindow focuses the column and tile containing id.
func (w *Workspace) FocusWindow(id uint64) bool {
	if w.floatingIndexOf(id) >= 0 {
		w.focusedFloat = id
		w.activeFloat = id
		return true
	}
	columnIndex, tileIndex, ok := w.locationOf(id)
	if !ok {
		return false
	}
	w.focused = columnIndex
	w.focusedFloat = 0
	w.columns[columnIndex].active = tileIndex
	w.revealFocused()
	return true
}

// FocusFloating selects the last active floating window, or the topmost float.
func (w *Workspace) FocusFloating() bool {
	if len(w.floating) == 0 {
		return false
	}
	if w.floatingIndexOf(w.activeFloat) < 0 {
		w.activeFloat = w.floating[len(w.floating)-1].tile.id
	}
	w.focusedFloat = w.activeFloat
	return true
}

// FocusTiling restores the active tiled window when one exists.
func (w *Workspace) FocusTiling() bool {
	if w.focusedFloat == 0 || w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	w.focusedFloat = 0
	return true
}

// SwitchFocusFloatingAndTiling toggles focus between the two workspace layers.
func (w *Workspace) SwitchFocusFloatingAndTiling() bool {
	if w.focusedFloat != 0 {
		return w.FocusTiling()
	}
	return w.FocusFloating()
}

// FocusWindowInColumn selects a one-based tile index in the focused column.
func (w *Workspace) FocusWindowInColumn(index uint8) bool {
	if w.focused < 0 || w.focused >= len(w.columns) || index == 0 {
		return false
	}
	column := &w.columns[w.focused]
	idx := int(index) - 1
	if idx >= len(column.tiles) || idx == column.active {
		return false
	}
	column.active = idx
	w.focusedFloat = 0
	w.revealFocused()
	return true
}

// FocusWindowDown activates the nearest lower floating window, or the next
// tile in the focused column, matching Niri's vertical focus action.
func (w *Workspace) FocusWindowDown() bool { return w.focusWindowVertical(1) }

// FocusWindowUp activates the nearest upper floating window, or the previous
// tile in the focused column, matching Niri's vertical focus action.
func (w *Workspace) FocusWindowUp() bool { return w.focusWindowVertical(-1) }

func (w *Workspace) focusWindowVertical(direction int) bool {
	if w.focusedFloat != 0 {
		active := w.floatingIndexOf(w.focusedFloat)
		if active < 0 {
			return false
		}
		center := w.floating[active].bounds.Y + w.floating[active].bounds.Height/2
		nearest, distance := -1, 0
		for i, floating := range w.floating {
			if i == active {
				continue
			}
			otherCenter := floating.bounds.Y + floating.bounds.Height/2
			delta := direction * (otherCenter - center)
			if delta > 0 && (nearest < 0 || delta < distance) {
				nearest, distance = i, delta
			}
		}
		if nearest < 0 {
			return false
		}
		id := w.floating[nearest].tile.id
		w.focusedFloat, w.activeFloat = id, id
		return true
	}
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	column := &w.columns[w.focused]
	next := column.active + direction
	if next < 0 || next >= len(column.tiles) {
		return false
	}
	column.active = next
	return true
}

// MoveWindowDown moves a floating window by Niri's 50 logical pixels, or
// swaps the focused tiled window with the next tile in its column.
func (w *Workspace) MoveWindowDown() bool { return w.moveWindowVertical(1) }

// MoveWindowUp moves a floating window by Niri's 50 logical pixels, or swaps
// the focused tiled window with the previous tile in its column.
func (w *Workspace) MoveWindowUp() bool { return w.moveWindowVertical(-1) }

func (w *Workspace) moveWindowVertical(direction int) bool {
	if w.focusedFloat != 0 {
		index := w.floatingIndexOf(w.focusedFloat)
		if index < 0 {
			return false
		}
		bounds := w.floating[index].bounds
		bounds.Y += direction * directionalMovePixels
		return w.SetFloatingGeometry(w.focusedFloat, bounds)
	}
	if w.focused < 0 || w.focused >= len(w.columns) {
		return false
	}
	column := &w.columns[w.focused]
	next := column.active + direction
	if next < 0 || next >= len(column.tiles) {
		return false
	}
	column.tiles[column.active], column.tiles[next] = column.tiles[next], column.tiles[column.active]
	column.active = next
	return true
}

// SetWindowSizeHints records the xdg-toplevel size bounds for one window.
func (w *Workspace) SetWindowSizeHints(id uint64, minWidth, maxWidth, minHeight, maxHeight int) bool {
	columnIndex, tileIndex, ok := w.locationOf(id)
	if !ok {
		index := w.floatingIndexOf(id)
		if index < 0 {
			return false
		}
		tile := &w.floating[index].tile
		minWidth, maxWidth = max(0, minWidth), max(0, maxWidth)
		minHeight, maxHeight = max(0, minHeight), max(0, maxHeight)
		if tile.minWidth == minWidth && tile.maxWidth == maxWidth &&
			tile.minHeight == minHeight && tile.maxHeight == maxHeight {
			return false
		}
		tile.minWidth, tile.maxWidth = minWidth, maxWidth
		tile.minHeight, tile.maxHeight = minHeight, maxHeight
		bounds := w.floating[index].bounds
		bounds.Width = clampSizeHint(bounds.Width, minWidth, maxWidth)
		bounds.Height = clampSizeHint(bounds.Height, minHeight, maxHeight)
		w.floating[index].bounds = bounds
		tile.floatingBounds = bounds
		return true
	}
	tile := &w.columns[columnIndex].tiles[tileIndex]
	minWidth, maxWidth = max(0, minWidth), max(0, maxWidth)
	minHeight, maxHeight = max(0, minHeight), max(0, maxHeight)
	if tile.minWidth == minWidth && tile.maxWidth == maxWidth &&
		tile.minHeight == minHeight && tile.maxHeight == maxHeight {
		return false
	}
	tile.minWidth, tile.maxWidth = minWidth, maxWidth
	tile.minHeight, tile.maxHeight = minHeight, maxHeight
	w.revealFocused()
	return true
}

// ResizeWindow updates the selected dimensions at the grabbed position,
// keeping the far edge fixed when a left or top resize is clamped.
func (w *Workspace) ResizeWindow(id uint64, x, y, width, height int, options ResizeOptions) bool {
	if index := w.floatingIndexOf(id); index >= 0 {
		if (!options.ResizeWidth && !options.ResizeHeight) ||
			(options.ResizeWidth && width < 1) || (options.ResizeHeight && height < 1) {
			return false
		}
		floating := w.floating[index]
		bounds := floating.bounds
		if options.ResizeWidth {
			resized := clampSizeHint(width, floating.tile.minWidth, floating.tile.maxWidth)
			if options.AnchorRight {
				x += width - resized
			}
			bounds.X, bounds.Width = x, resized
		}
		if options.ResizeHeight {
			resized := clampSizeHint(height, floating.tile.minHeight, floating.tile.maxHeight)
			if options.AnchorBottom {
				y += height - resized
			}
			bounds.Y, bounds.Height = y, resized
		}
		return w.SetFloatingGeometry(id, bounds)
	}
	columnIndex, tileIndex, ok := w.locationOf(id)
	if !ok || (!options.ResizeWidth && !options.ResizeHeight) ||
		(options.ResizeWidth && width < 1) || (options.ResizeHeight && height < 1) {
		return false
	}
	column := &w.columns[columnIndex]
	tile := &column.tiles[tileIndex]
	if options.ResizeWidth && options.AnchorRight {
		x += width - clampSizeHint(width, columnMinWidth(column), columnMaxWidth(column))
	}
	if options.ResizeHeight && options.AnchorBottom {
		y += height - clampSizeHint(height, tile.minHeight, tile.maxHeight)
	}
	if options.ResizeWidth {
		column.width = width
		if width == w.columnWidth {
			column.width = 0
		}
		w.viewOffset = w.bounds.X + w.columnStart(columnIndex) - x
	}
	if options.ResizeHeight {
		defaultHeight := w.defaultTileHeight(columnIndex, tileIndex)
		tile.height = height
		if height == defaultHeight {
			tile.height = 0
		}
		tile.yOffset = y - w.tileBaseY(columnIndex, tileIndex)
	}
	w.revealFocused()
	return true
}

// Placement returns a window's current tile in output layout coordinates.
func (w *Workspace) Placement(id uint64) (Placement, bool) {
	if index := w.floatingIndexOf(id); index >= 0 {
		floating := w.floating[index]
		return Placement{
			ID: id, Active: w.focusedFloat == id, Visible: true, Floating: true,
			X: floating.bounds.X, Y: floating.bounds.Y,
			Width: floating.bounds.Width, Height: floating.bounds.Height,
		}, true
	}
	columnIndex, tileIndex, ok := w.locationOf(id)
	if !ok {
		return Placement{}, false
	}
	return w.placementAt(columnIndex, tileIndex), true
}

// Placements returns every tile in column and tile order.
func (w *Workspace) Placements() []Placement {
	placements := make([]Placement, 0)
	for columnIndex, column := range w.columns {
		for tileIndex := range column.tiles {
			placements = append(placements, w.placementAt(columnIndex, tileIndex))
		}
	}
	for _, floating := range w.floating {
		placements = append(placements, Placement{
			ID: floating.tile.id, Active: w.focusedFloat == floating.tile.id,
			Visible: true, Floating: true, X: floating.bounds.X, Y: floating.bounds.Y,
			Width: floating.bounds.Width, Height: floating.bounds.Height,
		})
	}
	return placements
}

func (w *Workspace) placementAt(columnIndex, tileIndex int) Placement {
	column := w.columns[columnIndex]
	tile := column.tiles[tileIndex]
	return Placement{
		ID: tile.id, Column: uint32(columnIndex), Tile: uint32(tileIndex),
		Active:  tileIndex == column.active,
		Visible: column.display != ColumnDisplayTabbed || tileIndex == column.active,
		Display: column.display,
		X:       w.bounds.X + w.columnStart(columnIndex) - w.viewOffset,
		Y:       w.tileBaseY(columnIndex, tileIndex) + tile.yOffset,
		Width:   w.columnWidthAt(columnIndex),
		Height:  w.windowHeightAt(columnIndex, tileIndex),
	}
}

// ponytail: keep order as the source of truth with O(n) ID lookups; add an
// ID-to-index map only if measured workspace sizes make mutation scans costly.
func (w *Workspace) locationOf(id uint64) (column, tile int, ok bool) {
	for columnIndex, state := range w.columns {
		for tileIndex, candidate := range state.tiles {
			if candidate.id == id {
				return columnIndex, tileIndex, true
			}
		}
	}
	return 0, 0, false
}

func (w *Workspace) floatingIndexOf(id uint64) int {
	for index, floating := range w.floating {
		if floating.tile.id == id {
			return index
		}
	}
	return -1
}

func (w *Workspace) indexOf(id uint64) int {
	column, _, ok := w.locationOf(id)
	if !ok {
		return -1
	}
	return column
}

func (w *Workspace) revealFocused() {
	if w.focused < 0 || w.focused >= len(w.columns) || w.bounds.Width == 0 {
		w.viewOffset = 0
		return
	}
	left := w.columnStart(w.focused)
	width := w.columnWidthAt(w.focused)
	right := left + width
	if width >= w.bounds.Width {
		w.viewOffset = left
		return
	}
	padding := min(w.gap, (w.bounds.Width-width)/2)
	if left-w.viewOffset < padding {
		w.viewOffset = left - padding
	} else if right-w.viewOffset > w.bounds.Width-padding {
		w.viewOffset = right - (w.bounds.Width - padding)
	}
}

func (w *Workspace) columnStart(index int) int {
	start := 0
	for i := 0; i < index; i++ {
		start += w.columnWidthAt(i) + w.gap
	}
	return start
}

func (w *Workspace) columnWidthAt(index int) int {
	column := w.columns[index]
	width := w.columnWidth
	if column.width > 0 {
		width = column.width
	}
	return clampSizeHint(width, columnMinWidth(&column), columnMaxWidth(&column))
}

func columnMinWidth(column *columnState) int {
	minimum := 0
	for _, tile := range column.tiles {
		minimum = max(minimum, tile.minWidth)
	}
	return minimum
}

func columnMaxWidth(column *columnState) int {
	maximum := 0
	for _, tile := range column.tiles {
		if tile.maxWidth > 0 && (maximum == 0 || tile.maxWidth < maximum) {
			maximum = tile.maxWidth
		}
	}
	return maximum
}

func (w *Workspace) verticalInset() int {
	return min(w.gap, max(0, (w.bounds.Height-1)/2))
}

func (w *Workspace) defaultWindowHeight() int {
	inset := w.verticalInset()
	return max(0, w.bounds.Height-2*inset)
}

func (w *Workspace) defaultTileHeight(columnIndex, tileIndex int) int {
	column := w.columns[columnIndex]
	if column.tiles[tileIndex].height > 0 {
		return column.tiles[tileIndex].height
	}
	if column.display == ColumnDisplayTabbed || len(column.tiles) == 1 {
		return w.defaultWindowHeight()
	}
	autoCount, fixedHeight := 0, 0
	for _, tile := range column.tiles {
		if tile.height > 0 {
			fixedHeight += tile.height
		} else {
			autoCount++
		}
	}
	if autoCount == 0 {
		return 0
	}
	remaining := w.defaultWindowHeight() - w.gap*(len(column.tiles)-1) - fixedHeight
	return max(1, remaining/autoCount)
}

func (w *Workspace) windowHeightAt(columnIndex, tileIndex int) int {
	tile := w.columns[columnIndex].tiles[tileIndex]
	return clampSizeHint(w.defaultTileHeight(columnIndex, tileIndex), tile.minHeight, tile.maxHeight)
}

func (w *Workspace) tileBaseY(columnIndex, tileIndex int) int {
	base := w.bounds.Y + w.verticalInset()
	column := w.columns[columnIndex]
	if column.display == ColumnDisplayTabbed {
		return base
	}
	for previous := 0; previous < tileIndex; previous++ {
		base += w.windowHeightAt(columnIndex, previous) + w.gap
	}
	return base
}

func clampSizeHint(size, minSize, maxSize int) int {
	if minSize <= 0 && maxSize <= 0 {
		return size
	}
	minSize = max(1, minSize)
	if maxSize > 0 && maxSize < minSize {
		maxSize = minSize
	}
	size = max(size, minSize)
	if maxSize > 0 {
		size = min(size, maxSize)
	}
	return size
}
