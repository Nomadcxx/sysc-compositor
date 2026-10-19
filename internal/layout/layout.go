// Package layout holds compositor-independent column order and viewport geometry.
package layout

// Bounds describes one workspace's visible output area in layout coordinates.
type Bounds struct {
	X, Y, Width, Height int
}

// Placement is a window's tile in the visible workspace view.
type Placement struct {
	ID                  uint64
	X, Y, Width, Height int
}

type columnState struct {
	id            uint64
	width, height int
	yOffset       int
}

// Workspace is one horizontal strip of single-window columns.
// ponytail: M5 keeps one window per column; add column tab groups with M6.
type Workspace struct {
	bounds      Bounds
	columnWidth int
	gap         int
	columns     []columnState
	focused     int
	viewOffset  int
}

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
	w.bounds = bounds
	w.columnWidth = columnWidth
	w.gap = gap
	w.revealFocused()
}

// AddWindow inserts a new column immediately to the right of the focused one.
func (w *Workspace) AddWindow(id uint64) {
	if id == 0 || w.indexOf(id) >= 0 {
		return
	}
	idx := 0
	if w.focused >= 0 {
		idx = w.focused + 1
	}
	w.columns = append(w.columns, columnState{})
	copy(w.columns[idx+1:], w.columns[idx:])
	w.columns[idx] = columnState{id: id}
	w.focused = idx
	w.revealFocused()
}

// RemoveWindow removes a mapped window and its column.
func (w *Workspace) RemoveWindow(id uint64) {
	idx := w.indexOf(id)
	if idx < 0 {
		return
	}
	w.columns = append(w.columns[:idx], w.columns[idx+1:]...)
	if len(w.columns) == 0 {
		w.focused = -1
		w.viewOffset = 0
		return
	}
	if idx < w.focused {
		w.focused--
	} else if idx == w.focused && w.focused == len(w.columns) {
		w.focused--
	}
	w.revealFocused()
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

// WindowOrder returns a copy of the strip's window order.
func (w *Workspace) WindowOrder() []uint64 {
	order := make([]uint64, len(w.columns))
	for index, column := range w.columns {
		order[index] = column.id
	}
	return order
}

// FocusedWindow returns the active column's window, if one exists.
func (w *Workspace) FocusedWindow() (uint64, bool) {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return 0, false
	}
	return w.columns[w.focused].id, true
}

// FocusWindow focuses the column containing id.
func (w *Workspace) FocusWindow(id uint64) bool {
	idx := w.indexOf(id)
	if idx < 0 {
		return false
	}
	w.focused = idx
	w.revealFocused()
	return true
}

// ResizeWindow retains a column's width and its window's height at the grabbed
// position, matching pointer resize grabs.
func (w *Workspace) ResizeWindow(id uint64, x, y, width, height int) bool {
	idx := w.indexOf(id)
	if idx < 0 || width < 1 || height < 1 {
		return false
	}
	column := &w.columns[idx]
	column.width = width
	if width == w.columnWidth {
		column.width = 0
	}
	column.height = height
	if height == w.defaultWindowHeight() {
		column.height = 0
	}
	column.yOffset = y - (w.bounds.Y + w.verticalInset())
	w.viewOffset = w.bounds.X + w.columnStart(idx) - x
	w.revealFocused()
	return true
}

// Placement returns a window's current tile in output layout coordinates.
func (w *Workspace) Placement(id uint64) (Placement, bool) {
	idx := w.indexOf(id)
	if idx < 0 {
		return Placement{}, false
	}
	column := w.columns[idx]
	inset := w.verticalInset()
	return Placement{
		ID: id, X: w.bounds.X + w.columnStart(idx) - w.viewOffset,
		Y:     w.bounds.Y + inset + column.yOffset,
		Width: w.columnWidthAt(idx), Height: w.windowHeightAt(idx),
	}, true
}

// Placements returns tiles in column order.
func (w *Workspace) Placements() []Placement {
	placements := make([]Placement, 0, len(w.columns))
	inset := w.verticalInset()
	for index, column := range w.columns {
		placements = append(placements, Placement{
			ID: column.id, X: w.bounds.X + w.columnStart(index) - w.viewOffset,
			Y:     w.bounds.Y + inset + column.yOffset,
			Width: w.columnWidthAt(index), Height: w.windowHeightAt(index),
		})
	}
	return placements
}

// ponytail: keep order as the source of truth with O(n) ID lookups; add an
// ID-to-index map only if measured workspace sizes make mutation scans costly.
func (w *Workspace) indexOf(id uint64) int {
	for i, candidate := range w.columns {
		if candidate.id == id {
			return i
		}
	}
	return -1
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
	if w.columns[index].width > 0 {
		return w.columns[index].width
	}
	return w.columnWidth
}

func (w *Workspace) verticalInset() int {
	return min(w.gap, max(0, (w.bounds.Height-1)/2))
}

func (w *Workspace) defaultWindowHeight() int {
	inset := w.verticalInset()
	return max(0, w.bounds.Height-2*inset)
}

func (w *Workspace) windowHeightAt(index int) int {
	if w.columns[index].height > 0 {
		return w.columns[index].height
	}
	return w.defaultWindowHeight()
}
