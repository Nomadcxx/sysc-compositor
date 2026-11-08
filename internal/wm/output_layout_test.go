package wm

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestOutputLayoutMergesBeforeWorkspaceLayout(t *testing.T) {
	gaps, wide, narrow := 30, 600, 300
	m := New(WorkspaceConfig{Name: "own", Layout: &WorkspaceLayout{FixedColumnWidth: &narrow}})
	m.AddOutput("DP-1", layout.Bounds{Width: 1920, Height: 1080}, 400, 10)
	m.SetOutputLayout("DP-1", &WorkspaceLayout{Gaps: &gaps, FixedColumnWidth: &wide})
	own, _ := m.WorkspaceIDByName("own")
	m.MapWindowToWorkspace(1, own)
	for _, w := range m.Snapshot().Windows {
		if w.Tile.Width != narrow || w.Tile.X != gaps {
			t.Fatalf("workspace layout = %+v", w.Tile)
		}
	}
	m.FocusWorkspace("DP-1", 2)
	m.MapWindow(2, "DP-1")
	for _, w := range m.Snapshot().Windows {
		if w.ID == 2 && (w.Tile.Width != wide || w.Tile.X != gaps) {
			t.Fatalf("output layout = %+v", w.Tile)
		}
	}
	m.SetOutputLayout("DP-1", nil)
	for _, w := range m.Snapshot().Windows {
		if w.ID == 2 && w.Tile.Width != 400 {
			t.Fatalf("global layout not restored: %+v", w.Tile)
		}
	}
}

func TestWorkspaceTabSpaceReloadIsNotLost(t *testing.T) {
	first := &WorkspaceLayout{TabSpace: &layout.TabSpace{Size: 4}}
	m := New(WorkspaceConfig{Name: "own", Layout: first})
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	if !m.SetWorkspaceConfig([]WorkspaceConfig{{Name: "own", Layout: &WorkspaceLayout{TabSpace: &layout.TabSpace{Size: 10}}}}) {
		t.Fatal("tab space change was mistaken for an unchanged layout")
	}
}

func TestOutputTabSpaceMergesWorkspaceFields(t *testing.T) {
	base := config.TabIndicator{Width: 8, Gap: 1, PlaceWithinColumn: true, Position: config.TabIndicatorBottom}
	global := config.TabIndicator{Width: 4, Gap: 1, PlaceWithinColumn: true, Position: config.TabIndicatorLeft}
	gap := 2.0
	merged := (&WorkspaceLayout{ConfigTabIndicator: &base}).merged(&WorkspaceLayout{ConfigTabIndicator: &global, TabIndicatorPart: config.TabIndicatorPart{Gap: &gap}})
	if merged.TabSpace == nil || merged.TabSpace.Size != 10 || merged.TabSpace.Side != layout.TabBottom {
		t.Fatalf("merged tab space = %+v", merged.TabSpace)
	}
}
