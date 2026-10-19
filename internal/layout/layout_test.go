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
	if !ws.ResizeWindow(1, 16, 16, 560, 420) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}
	if ws.ResizeWindow(1, 0, 0, 0, 0) {
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
