package wm

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

// A workspace may set its own gaps and column width. The override applies to
// that workspace only; the other workspaces of the output keep the layout of
// the output.
func TestWorkspaceLayoutOverridesTheOutputLayout(t *testing.T) {
	gaps := 0
	proportion := 1.0
	m := New(WorkspaceConfig{
		Name:   "wide",
		Layout: &WorkspaceLayout{Gaps: &gaps, ColumnWidthProportion: &proportion},
	})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)

	wide, ok := m.WorkspaceIDByName("wide")
	if !ok {
		t.Fatal("the named workspace is missing")
	}
	snapshot := m.Snapshot()
	var plain uint64
	for _, workspace := range snapshot.Workspaces {
		if workspace.Name == nil {
			plain = workspace.ID
		}
	}
	if plain == 0 {
		t.Fatalf("workspaces = %+v, want an unnamed workspace", snapshot.Workspaces)
	}
	if !m.MapWindowToWorkspace(1, wide) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}
	if !m.MapWindowToWorkspace(2, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}

	snapshot = m.Snapshot()
	widths := map[uint64]int{}
	heights := map[uint64]int{}
	for _, window := range snapshot.Windows {
		widths[window.WorkspaceID] = window.Tile.Width
		heights[window.WorkspaceID] = window.Tile.Height
	}
	if got := widths[wide]; got != 1000 {
		t.Errorf("the column width of the named workspace = %d, want 1000", got)
	}
	if got := heights[wide]; got != 800 {
		t.Errorf("the window height of the named workspace = %d, want 800", got)
	}
	if got := widths[plain]; got != 500 {
		t.Errorf("the column width of the other workspace = %d, want the output's 500", got)
	}
	if got := heights[plain]; got != 768 {
		t.Errorf("the window height of the other workspace = %d, want the output's 768", got)
	}
}

// A workspace with a fixed column width keeps it, whatever the output says.
func TestWorkspaceLayoutFixedColumnWidth(t *testing.T) {
	fixed := 320
	m := New(WorkspaceConfig{
		Name:   "narrow",
		Layout: &WorkspaceLayout{FixedColumnWidth: &fixed},
	})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)

	narrow, ok := m.WorkspaceIDByName("narrow")
	if !ok {
		t.Fatal("the named workspace is missing")
	}
	if !m.MapWindowToWorkspace(1, narrow) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}
	snapshot := m.Snapshot()
	if got := snapshot.Windows[0].Tile.Width; got != 320 {
		t.Fatalf("the column width = %d, want 320", got)
	}
}

// The preset sizes come from the config, and a workspace may set its own.
func TestWorkspacePresetsFollowTheConfig(t *testing.T) {
	m := New(WorkspaceConfig{
		Name:   "wide",
		Layout: &WorkspaceLayout{PresetColumnWidths: []layout.PresetSize{{Fixed: true, Value: 700}}},
	})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.SetPresetSizes([]layout.PresetSize{{Value: 0.25}}, nil)

	wide, ok := m.WorkspaceIDByName("wide")
	if !ok {
		t.Fatal("the named workspace is missing")
	}
	snapshot := m.Snapshot()
	var plain uint64
	for _, workspace := range snapshot.Workspaces {
		if workspace.Name == nil {
			plain = workspace.ID
		}
	}
	if plain == 0 {
		t.Fatalf("workspaces = %+v, want an unnamed workspace", snapshot.Workspaces)
	}
	if !m.MapWindowToWorkspace(1, wide) || !m.MapWindowToWorkspace(2, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}

	// The workspaces without their own list cycle the config's presets, the
	// named workspace cycles its own.
	m.FocusWorkspaceID(plain)
	if !m.SwitchPresetColumnWidth(false) {
		t.Fatal("SwitchPresetColumnWidth = false, want true")
	}
	m.FocusWorkspaceID(wide)
	if !m.SwitchPresetColumnWidth(false) {
		t.Fatal("SwitchPresetColumnWidth = false, want true")
	}

	widths := map[uint64]int{}
	for _, window := range m.Snapshot().Windows {
		widths[window.WorkspaceID] = window.Tile.Width
	}
	if got := widths[plain]; got != 230 {
		t.Errorf("the preset width of the plain workspace = %d, want 230", got)
	}
	if got := widths[wide]; got != 700 {
		t.Errorf("the preset width of the named workspace = %d, want 700", got)
	}
}

// The default column display comes from the config, and a workspace may set
// its own.
func TestWorkspaceDefaultColumnDisplayFollowsTheConfig(t *testing.T) {
	tabbed := layout.ColumnDisplayTabbed
	normal := layout.ColumnDisplayNormal
	m := New(WorkspaceConfig{
		Name:   "wide",
		Layout: &WorkspaceLayout{DefaultColumnDisplay: &normal},
	})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.SetDefaultColumnDisplay(tabbed)

	wide, ok := m.WorkspaceIDByName("wide")
	if !ok {
		t.Fatal("the named workspace is missing")
	}
	snapshot := m.Snapshot()
	var plain uint64
	for _, workspace := range snapshot.Workspaces {
		if workspace.Name == nil {
			plain = workspace.ID
		}
	}
	if plain == 0 {
		t.Fatalf("workspaces = %+v, want an unnamed workspace", snapshot.Workspaces)
	}
	if !m.MapWindowToWorkspace(1, wide) || !m.MapWindowToWorkspace(2, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}

	displays := map[uint64]layout.ColumnDisplay{}
	for _, window := range m.Snapshot().Windows {
		displays[window.WorkspaceID] = window.Tile.Display
	}
	if got := displays[plain]; got != tabbed {
		t.Errorf("the display of the plain workspace = %v, want tabbed", got)
	}
	if got := displays[wide]; got != normal {
		t.Errorf("the display of the named workspace = %v, want its normal", got)
	}
}
