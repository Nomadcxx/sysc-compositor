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
