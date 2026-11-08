package wm

import (
	"slices"
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

// TestWorkspaceCenterFocusedColumnFollowsTheConfig: the config-wide centering
// policy reaches every workspace after a change, and a workspace of its own
// keeps its own policy.
func TestWorkspaceCenterFocusedColumnFollowsTheConfig(t *testing.T) {
	always := layout.CenterFocusedColumnAlways
	never := layout.CenterFocusedColumnNever
	m := New(WorkspaceConfig{Name: "wide", Layout: &WorkspaceLayout{CenterFocusedColumn: &never}})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.SetCenterFocusedColumn(always)
	wide, foundWide := m.WorkspaceIDByName("wide")
	var plain uint64
	foundPlain := false
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Name == nil {
			plain, foundPlain = workspace.ID, true
		}
	}
	if !foundWide || !foundPlain {
		t.Fatalf("workspaces found = %t %t, want the two", foundWide, foundPlain)
	}
	if !m.MapWindowToWorkspace(1, wide) || !m.MapWindowToWorkspace(2, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}
	xs := map[uint64]int{}
	for _, window := range m.Snapshot().Windows {
		xs[window.WorkspaceID] = window.Tile.X
	}
	if got := xs[plain]; got != 250 {
		t.Errorf("the X of the plain workspace = %d, want 250 (centered)", got)
	}
	if got := xs[wide]; got != 16 {
		t.Errorf("the X of the named workspace = %d, want 16 (its own never)", got)
	}
}

// TestWorkspaceStrutsFollowsTheConfig: the config-wide struts shrink the
// layout area of every workspace, and a workspace of its own keeps its own
// set. The strip only: the tile is placed inside the strutted area.
func TestWorkspaceStrutsFollowsTheConfig(t *testing.T) {
	wideStruts := layout.Struts{Left: 200}
	m := New(WorkspaceConfig{
		Name:   "wide",
		Layout: &WorkspaceLayout{Struts: &wideStruts},
	})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.SetStruts(layout.Struts{Left: 100})

	wide, foundWide := m.WorkspaceIDByName("wide")
	var plain uint64
	foundPlain := false
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Name == nil {
			plain, foundPlain = workspace.ID, true
		}
	}
	if !foundWide || !foundPlain {
		t.Fatalf("workspaces found = %t %t, want the two", foundWide, foundPlain)
	}
	if !m.MapWindowToWorkspace(1, wide) || !m.MapWindowToWorkspace(2, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}

	// A lone column of 500 in the strutted area still gets the fit padding
	// of 16, so the tile starts at the inset edge plus that padding.
	xs := map[uint64]int{}
	widths := map[uint64]int{}
	for _, window := range m.Snapshot().Windows {
		xs[window.WorkspaceID] = window.Tile.X
		widths[window.WorkspaceID] = window.Tile.Width
	}
	if got := xs[plain]; got != 116 {
		t.Errorf("the X of the plain workspace = %d, want 116 (strut 100 + padding 16)", got)
	}
	if got := xs[wide]; got != 216 {
		t.Errorf("the X of the named workspace = %d, want 216 (its strut 200 + padding 16)", got)
	}
	if got := widths[plain]; got != 500 {
		t.Errorf("the column width of the plain workspace = %d, want the output's 500", got)
	}
}

// TestConfigWideLayoutReachesLaterWorkspaces: the config-wide setters may
// run before an output exists, so a workspace created afterwards must start
// from the config's values, not from the layout's own defaults.
func TestConfigWideLayoutReachesLaterWorkspaces(t *testing.T) {
	m := New()
	m.SetPresetSizes([]layout.PresetSize{{Fixed: true, Value: 300}}, nil)
	m.SetDefaultColumnDisplay(layout.ColumnDisplayTabbed)
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)

	var plain uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Name == nil {
			plain = workspace.ID
		}
	}
	if plain == 0 {
		t.Fatalf("workspaces = %+v, want an unnamed workspace", m.Snapshot().Workspaces)
	}
	if !m.MapWindowToWorkspace(1, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}
	m.FocusWorkspaceID(plain)
	if !m.SwitchPresetColumnWidth(false) {
		t.Fatal("SwitchPresetColumnWidth = false, want true")
	}
	window := m.Snapshot().Windows[0]
	if got := window.Tile.Width; got != 300 {
		t.Errorf("the preset width = %d, want the config's fixed 300", got)
	}
	if got := window.Tile.Display; got != layout.ColumnDisplayTabbed {
		t.Errorf("the display = %v, want the config's tabbed", got)
	}
}

// TestWorkspaceAlwaysCenterSingleColumnFollowsTheConfig: the config-wide
// always-center-single-column centers a lone column on every workspace, and a
// workspace that turns it off keeps the fit reveal.
func TestWorkspaceAlwaysCenterSingleColumnFollowsTheConfig(t *testing.T) {
	off := false
	m := New(WorkspaceConfig{Name: "fit", Layout: &WorkspaceLayout{AlwaysCenterSingleColumn: &off}})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 400, 16)
	m.SetAlwaysCenterSingleColumn(true)
	fit, _ := m.WorkspaceIDByName("fit")
	var plain uint64
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.Name == nil {
			plain = workspace.ID
		}
	}
	if fit == 0 || plain == 0 {
		t.Fatalf("workspaces = %+v, want a named and a plain one", m.Snapshot().Workspaces)
	}
	if !m.MapWindowToWorkspace(1, fit) || !m.MapWindowToWorkspace(2, plain) {
		t.Fatal("MapWindowToWorkspace() = false, want true")
	}
	xs := map[uint64]int{}
	for _, window := range m.Snapshot().Windows {
		xs[window.WorkspaceID] = window.Tile.X
	}
	if got := xs[plain]; got != 300 {
		t.Errorf("the X of the plain workspace = %d, want 300 (centered)", got)
	}
	if got := xs[fit]; got != 16 {
		t.Errorf("the X of the named workspace = %d, want 16 (its own false)", got)
	}
}

// TestEmptyWorkspaceAboveFirstKeepsAnEmptyWorkspaceOnTop follows Niri's
// Monitor: with empty-workspace-above-first an output keeps an empty
// workspace above its first one as soon as it has more than one, and two
// empty workspaces collapse into one.
func TestEmptyWorkspaceAboveFirstKeepsAnEmptyWorkspaceOnTop(t *testing.T) {
	m := New()
	m.SetEmptyWorkspaceAboveFirst(true)
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if got := len(workspaceIDs(m, "eDP-1")); got != 1 {
		t.Fatalf("an empty output has %d workspaces, want 1", got)
	}
	if !m.MapWindow(1, "eDP-1") {
		t.Fatal("MapWindow() = false")
	}
	assertWorkspaceShape := func(step string, want []int) {
		t.Helper()
		var got []int
		for _, workspace := range m.Snapshot().Workspaces {
			got = append(got, len(workspace.WindowIDs))
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s: windows per workspace = %v, want %v", step, got, want)
		}
	}
	assertWorkspaceShape("after the first window", []int{0, 1, 0})
	if active := activeWorkspaceIndex(m); active != 2 {
		t.Fatalf("the active workspace index = %d, want 2, the window's", active)
	}

	m.SetEmptyWorkspaceAboveFirst(false)
	assertWorkspaceShape("with the flag off", []int{1, 0})
	m.SetEmptyWorkspaceAboveFirst(true)
	assertWorkspaceShape("with the flag on again", []int{0, 1, 0})

	if !m.MoveWorkspaceUp() {
		t.Fatal("MoveWorkspaceUp() = false")
	}
	assertWorkspaceShape("after moving the workspace up", []int{0, 1, 0})

	m.RemoveWindow(1)
	if !m.FocusWorkspace("eDP-1", 1) {
		t.Fatal("FocusWorkspace() = false")
	}
	assertWorkspaceShape("after the last window closed", []int{0})
}

// TestEmptyWorkspaceAboveFirstPrecedesNamedWorkspaces: a named workspace is
// never the first one when the flag is set, from the moment the output
// connects.
func TestEmptyWorkspaceAboveFirstPrecedesNamedWorkspaces(t *testing.T) {
	m := New(WorkspaceConfig{Name: "mail"})
	m.SetEmptyWorkspaceAboveFirst(true)
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	workspaces := m.Snapshot().Workspaces
	if len(workspaces) != 3 || workspaces[0].Name != nil || workspaces[1].Name == nil || workspaces[2].Name != nil {
		t.Fatalf("workspaces = %+v, want empty, mail, empty", workspaces)
	}
}

func activeWorkspaceIndex(m *Model) int {
	for _, workspace := range m.Snapshot().Workspaces {
		if workspace.IsActive {
			return int(workspace.Index)
		}
	}
	return 0
}
