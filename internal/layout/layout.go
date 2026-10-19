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

// Workspace is one horizontal strip of single-window columns.
type Workspace struct {
	bounds      Bounds
	columnWidth int
	gap         int
	columns     []uint64
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
	if columnWidth > bounds.Width && bounds.Width > 0 {
		columnWidth = bounds.Width
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
	w.columns = append(w.columns, 0)
	copy(w.columns[idx+1:], w.columns[idx:])
	w.columns[idx] = id
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

// WindowOrder returns a copy of the strip's window order.
func (w *Workspace) WindowOrder() []uint64 { return append([]uint64(nil), w.columns...) }

// FocusedWindow returns the active column's window, if one exists.
func (w *Workspace) FocusedWindow() (uint64, bool) {
	if w.focused < 0 || w.focused >= len(w.columns) {
		return 0, false
	}
	return w.columns[w.focused], true
}

// Placement returns a window's current tile in output layout coordinates.
func (w *Workspace) Placement(id uint64) (Placement, bool) {
	idx := w.indexOf(id)
	if idx < 0 {
		return Placement{}, false
	}
	x := idx * (w.columnWidth + w.gap)
	return Placement{
		ID: id, X: w.bounds.X + x - w.viewOffset, Y: w.bounds.Y,
		Width: w.columnWidth, Height: w.bounds.Height,
	}, true
}

// Placements returns tiles in column order.
func (w *Workspace) Placements() []Placement {
	placements := make([]Placement, 0, len(w.columns))
	for _, id := range w.columns {
		if p, ok := w.Placement(id); ok {
			placements = append(placements, p)
		}
	}
	return placements
}

func (w *Workspace) indexOf(id uint64) int {
	for i, candidate := range w.columns {
		if candidate == id {
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
	left := w.focused * (w.columnWidth + w.gap)
	right := left + w.columnWidth
	if left < w.viewOffset {
		w.viewOffset = left
	} else if right > w.viewOffset+w.bounds.Width {
		w.viewOffset = right - w.bounds.Width
	}
	if w.viewOffset < 0 {
		w.viewOffset = 0
	}
}
