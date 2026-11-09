package wm

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestWindowSizingTargetsTheWorkspaceAndRetainsFloatingGeometry(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1200, Height: 800}, 400, 16)
	m.AddOutput("DP-2", layout.Bounds{X: 1200, Width: 1200, Height: 800}, 400, 16)
	m.MapWindow(1, "DP-1")
	m.MapWindow(2, "DP-2")
	if !m.SetWindowWidthFor(1, layout.SizeChange{Fixed: 500}) {
		t.Fatal("width action ignored remote workspace")
	}
	if !m.SwitchPresetWindowWidthFor(1, false) {
		t.Fatal("preset action ignored remote workspace")
	}
	if snapshot := m.Snapshot(); snapshot.FocusedOutput != "DP-2" || snapshot.Windows[0].Tile.Width != 576 {
		t.Fatalf("targeted sizing = %+v", snapshot)
	}
	m.SetWindowFloating(2, true, layout.Bounds{})
	m.SetWindowWidthFor(2, layout.SizeChange{Fixed: 600})
	m.SetWindowHeightFor(2, layout.SizeChange{Fixed: 300})
	m.CenterWindowFor(2)
	before := m.floatingWindows[2]
	if before.Width != 600 || before.Height != 300 || before.X != 1500 || before.Y != 250 {
		t.Fatalf("saved floating geometry = %+v", before)
	}
	m.SetWindowFloating(2, false, layout.Bounds{})
	m.SetWindowFloating(2, true, layout.Bounds{})
	if got := m.floatingWindows[2]; got != before {
		t.Fatalf("float restore = %+v, want %+v", got, before)
	}
}

func TestMaximizedStateSurvivesFullscreenAndRestoresFloating(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1200, Height: 800}, 400, 16)
	m.MapWindow(1, "DP-1")
	m.SetWindowFloating(1, true, layout.Bounds{X: 100, Y: 120, Width: 300, Height: 200})
	m.SetFullscreen(1, true)
	if !m.SetMaximized(1, true) {
		t.Fatal("maximize during fullscreen was ignored")
	}
	m.SetFullscreen(1, false)
	window := m.Snapshot().Windows[0]
	if window.Tile.Floating || !window.Tile.Maximized || window.Fullscreen {
		t.Fatalf("fullscreen exit lost underlying maximize: %+v", window)
	}
	m.SetMaximized(1, false)
	window = m.Snapshot().Windows[0]
	if !window.Tile.Floating || window.Tile.X != 100 || window.Tile.Y != 120 || window.Tile.Width != 300 || window.Tile.Height != 200 {
		t.Fatalf("unmaximize lost floating restore: %+v", window)
	}
	m.SetMaximized(1, true)
	m.SetFullscreen(1, true)
	m.SetMaximized(1, false)
	if window = m.Snapshot().Windows[0]; !window.Fullscreen {
		t.Fatalf("unmaximize exited fullscreen: %+v", window)
	}
	m.SetFullscreen(1, false)
	if window = m.Snapshot().Windows[0]; !window.Tile.Floating || window.Tile.Width != 300 {
		t.Fatalf("fullscreen lost restored floating size: %+v", window)
	}
}

func TestUnmaximizeDefersFloatingRestoreUntilFullscreenExit(t *testing.T) {
	m := New()
	m.AddOutput("DP-1", layout.Bounds{Width: 1200, Height: 800}, 400, 16)
	m.MapWindow(1, "DP-1")
	saved := layout.Bounds{X: 100, Y: 120, Width: 300, Height: 200}
	m.SetWindowFloating(1, true, saved)
	m.SetMaximized(1, true)
	m.SetFullscreen(1, true)
	m.SetMaximized(1, false)
	window := m.Snapshot().Windows[0]
	if !window.Fullscreen || window.Tile.Floating || window.Tile.Maximized {
		t.Fatalf("fullscreen unmaximize = %+v", window)
	}
	m.SetFullscreen(1, false)
	window = m.Snapshot().Windows[0]
	if !window.Tile.Floating || window.Tile.X != saved.X || window.Tile.Y != saved.Y || window.Tile.Width != saved.Width || window.Tile.Height != saved.Height {
		t.Fatalf("fullscreen exit lost deferred floating restore: %+v", window)
	}
}
