package layout

import (
	"slices"
	"testing"
)

func TestNewWindowGoesRightOfFocusedColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusLeft() {
		t.Fatal("FocusLeft() = false, want true")
	}
	ws.AddWindow(3)

	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 3 {
		t.Fatalf("FocusedWindow() = %d, %t, want 3, true", got, ok)
	}
}

func TestMoveColumnKeepsWindowFocused(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)
	if !ws.FocusLeft() || !ws.MoveRight() {
		t.Fatal("expected to focus and move the middle column right")
	}

	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t, want moved window 2", got, ok)
	}
}

func TestFocusWindowSelectsItsColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.FocusLeft()

	if !ws.FocusWindow(2) {
		t.Fatal("FocusWindow(2) = false, want true")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t, want 2, true", got, ok)
	}
}

func TestFocusedColumnStaysInsideViewport(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 600, 20)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)
	ws.FocusRight()
	ws.FocusRight()

	placement, ok := ws.Placement(3)
	if !ok {
		t.Fatal("Placement(3) missing")
	}
	if placement.X < 0 || placement.X+placement.Width > 1000 {
		t.Fatalf("focused column %+v is outside the viewport", placement)
	}
}

func TestFocusedColumnKeepsConfiguredGapsAtOutputEdges(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 700, 20)
	ws.AddWindow(1)

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 120 || placement.Y != 70 || placement.Width != 700 || placement.Height != 760 {
		t.Fatalf("placement = %+v, want 20-pixel outer gaps around the tile", placement)
	}
}

func TestOversizedColumnRemainsScrollable(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 1200, 16)
	ws.AddWindow(1)

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 0 || placement.Width != 1200 {
		t.Fatalf("placement = %+v, want an oversized column left-aligned in the viewport", placement)
	}
}

func TestRemovingWindowRemovesItsColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.RemoveWindow(2)

	if got, want := ws.WindowOrder(), []uint64{1}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Fatalf("FocusedWindow() = %d, %t, want 1, true", got, ok)
	}
}

func TestFocusFirstAndLastColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 250, Height: 100}, 100, 10)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusFirst() {
		t.Fatal("FocusFirst() = false, want true")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Fatalf("FocusedWindow() = %d, %t after FocusFirst(), want 1, true", got, ok)
	}
	if !ws.FocusLast() {
		t.Fatal("FocusLast() = false, want true")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 3 {
		t.Fatalf("FocusedWindow() = %d, %t after FocusLast(), want 3, true", got, ok)
	}
}

func TestMoveFocusedColumnToFirstAndLast(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 250, Height: 100}, 100, 10)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	ws.FocusWindow(2)
	if !ws.MoveFirst() {
		t.Fatal("MoveFirst() = false, want true")
	}
	if got, want := ws.WindowOrder(), []uint64{2, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() after MoveFirst() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t after MoveFirst(), want 2, true", got, ok)
	}
	if !ws.MoveLast() {
		t.Fatal("MoveLast() = false, want true")
	}
	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() after MoveLast() = %v, want %v", got, want)
	}
}

func TestResizeRetainsColumnWidthAndWindowHeightAcrossLayoutUpdates(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.ResizeWindow(1, 16, 16, 560, 420, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}
	if ws.ResizeWindow(1, 0, 0, 0, 0, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow with an empty size = true, want false")
	}
	if !ws.FocusWindow(2) || !ws.FocusWindow(1) {
		t.Fatal("could not move focus away and back to the resized column")
	}
	ws.SetGeometry(Bounds{Width: 1200, Height: 900}, 600, 16)

	first, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if first.Width != 560 || first.Height != 420 {
		t.Fatalf("resized placement = %+v, want retained size 560x420", first)
	}
	second, ok := ws.Placement(2)
	if !ok {
		t.Fatal("Placement(2) missing")
	}
	if second.X != first.X+first.Width+16 {
		t.Fatalf("second column x = %d, want after resized first column at %d", second.X, first.X+first.Width+16)
	}
}

func TestVerticalResizePreservesConstrainedColumnWidth(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	if !ws.ResizeWindow(1, 16, 16, 900, 500, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("initial ResizeWindow() failed")
	}
	ws.SetWindowSizeHints(1, 0, 500, 0, 0)

	placement, ok := ws.Placement(1)
	if !ok || placement.Width != 500 {
		t.Fatalf("constrained placement = %+v, %t, want width 500", placement, ok)
	}
	if !ws.ResizeWindow(1, placement.X, placement.Y, placement.Width, 420, ResizeOptions{ResizeHeight: true}) {
		t.Fatal("vertical ResizeWindow() failed")
	}
	ws.SetWindowSizeHints(1, 0, 0, 0, 0)

	placement, _ = ws.Placement(1)
	if placement.Width != 900 || placement.Height != 420 {
		t.Fatalf("placement after vertical resize and hint removal = %+v, want 900x420", placement)
	}
}

func TestResizeClampsAndKeepsLeftAndTopAnchorsFixed(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowSizeHints(1, 500, 0, 400, 0)

	// The pointer requested a 300x300 tile at (316, 166). Its right and bottom
	// edges were anchored at (616, 466), so size-hint clamping must move the
	// tile origin back as the tile grows to 500x400.
	if !ws.ResizeWindow(1, 316, 166, 300, 300, ResizeOptions{
		ResizeWidth: true, ResizeHeight: true, AnchorRight: true, AnchorBottom: true,
	}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 116 || placement.Y != 66 || placement.Width != 500 || placement.Height != 400 {
		t.Fatalf("clamped placement = %+v, want origin (116, 66), size 500x400", placement)
	}
	if right, bottom := placement.X+placement.Width, placement.Y+placement.Height; right != 616 || bottom != 466 {
		t.Fatalf("anchored edges = (%d, %d), want (616, 466)", right, bottom)
	}
}

func TestResizeClampsRightAndBottomEdgesWithoutMovingOrigin(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowSizeHints(1, 500, 0, 400, 0)

	if !ws.ResizeWindow(1, 200, 100, 300, 300, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 200 || placement.Y != 100 || placement.Width != 500 || placement.Height != 400 {
		t.Fatalf("clamped placement = %+v, want origin (200, 100), size 500x400", placement)
	}
}

func TestResizeClampsAtMaximumAndKeepsLeftAndTopAnchorsFixed(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowSizeHints(1, 0, 500, 0, 400)

	// The requested 600x500 tile exceeds the client maximum. The grabbed
	// right and bottom edges still need to stay at (616, 466) after clamping.
	if !ws.ResizeWindow(1, 16, -34, 600, 500, ResizeOptions{
		ResizeWidth: true, ResizeHeight: true, AnchorRight: true, AnchorBottom: true,
	}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 116 || placement.Y != 66 || placement.Width != 500 || placement.Height != 400 {
		t.Fatalf("clamped placement = %+v, want origin (116, 66), size 500x400", placement)
	}
	if right, bottom := placement.X+placement.Width, placement.Y+placement.Height; right != 616 || bottom != 466 {
		t.Fatalf("anchored edges = (%d, %d), want (616, 466)", right, bottom)
	}
}
