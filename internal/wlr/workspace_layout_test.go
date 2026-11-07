package wlr

import (
	"slices"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// TestWorkspaceLayoutOverridesTheOutputLayout: a workspace may set its own
// gaps and column width. The window that opens on that workspace uses them.
func TestWorkspaceLayoutOverridesTheOutputLayout(t *testing.T) {
	c := newNested(t)
	gaps := 0
	proportion := 1.0
	workspace := "wide"
	if err := c.Reconfigure(Options{
		Animations:  config.Animations{Off: true},
		WindowRules: []config.WindowRule{{OpenOnWorkspace: &workspace}},
		Workspaces: []wm.WorkspaceConfig{{
			Name:   workspace,
			Layout: &wm.WorkspaceLayout{Gaps: &gaps, ColumnWidthProportion: &proportion},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		if window.Tile.Width != bounds.w {
			t.Errorf("the tile width = %d, want the whole working width %d", window.Tile.Width, bounds.w)
		}
		if window.Tile.Height != bounds.h {
			t.Errorf("the tile height = %d, want the whole working height %d", window.Tile.Height, bounds.h)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWorkspacePresetsReachTheLayout: the config-wide preset sizes and the
// default column display reach the layout through the compositor options. The
// window that opens uses the display, and the preset cycle steps through the
// configured sizes.
func TestWorkspacePresetsReachTheLayout(t *testing.T) {
	c := newNested(t)
	workspace := "wide"
	if err := c.Reconfigure(Options{
		Animations: config.Animations{Off: true},
		Layout: &LayoutOptions{
			Gaps: 16, ColumnWidthProportion: 0.5,
			PresetColumnWidths:   []layout.PresetSize{{Fixed: true, Value: 300}},
			DefaultColumnDisplay: layout.ColumnDisplayTabbed,
		},
		WindowRules: []config.WindowRule{{OpenOnWorkspace: &workspace}},
	}); err != nil {
		t.Fatal(err)
	}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		if window.Tile.Display != layout.ColumnDisplayTabbed {
			t.Errorf("the display of the new column = %v, want tabbed", window.Tile.Display)
		}
		if !c.model.SwitchPresetColumnWidth(false) {
			t.Errorf("SwitchPresetColumnWidth = false, want true")
			c.Terminate()
			return
		}
		if window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id); ok {
			if window.Tile.Width != 300 {
				t.Errorf("the preset column width = %d, want the configured 300", window.Tile.Width)
			}
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestCenterFocusedColumnReachesTheLayout: the centering policy reaches the
// layout through the compositor options. The mapped window's column ends up
// in the middle of the working area, computed from the layout box so the
// assertion does not depend on the nested output size.
func TestCenterFocusedColumnReachesTheLayout(t *testing.T) {
	c := newNested(t)
	if err := c.Reconfigure(Options{
		Animations: config.Animations{Off: true},
		Layout: &LayoutOptions{
			Gaps: 16, FixedColumnWidth: 300,
			CenterFocusedColumn: layout.CenterFocusedColumnAlways,
		},
	}); err != nil {
		t.Fatal(err)
	}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		want := bounds.x + (bounds.w-300)/2
		if window.Tile.X != want {
			t.Errorf("the tile X = %d, want %d: the focused column centered in %+v", window.Tile.X, want, bounds)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWorkspaceStrutsReachTheLayout: the struts the config reserves shrink
// the layout area of the workspaces on the output. The mapped column sits
// behind the reserved edge, computed from the layout box so the assertion
// does not depend on the nested output size.
func TestWorkspaceStrutsReachTheLayout(t *testing.T) {
	c := newNested(t)
	if err := c.Reconfigure(Options{
		Animations: config.Animations{Off: true},
		Layout: &LayoutOptions{
			Gaps: 16, FixedColumnWidth: 300,
			Struts: layout.Struts{Left: 100},
		},
	}); err != nil {
		t.Fatal(err)
	}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		// A lone 300-wide column in the strutted area still gets the fit
		// padding, capped at the gaps: min(16, (w-100-300)/2).
		pad := min(16, (bounds.w-400)/2)
		want := bounds.x + 100 + pad
		if window.Tile.X != want {
			t.Errorf("the tile X = %d, want %d: the column behind the reserved edge in %+v", window.Tile.X, want, bounds)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestLayoutFlagsReachTheModel: always-center-single-column centers the
// first window of a workspace, and empty-workspace-above-first leaves an
// empty workspace above the one the window opens on.
func TestLayoutFlagsReachTheModel(t *testing.T) {
	c := newNested(t)
	if err := c.Reconfigure(Options{
		Animations: config.Animations{Off: true},
		Layout: &LayoutOptions{
			Gaps: 16, FixedColumnWidth: 300,
			AlwaysCenterSingleColumn: true, EmptyWorkspaceAboveFirst: true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	c.viewMapped = func(view *toplevelView) {
		defer c.Terminate()
		snapshot := c.WorkspaceSnapshot()
		window, ok := findWindowSnapshot(snapshot.Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			return
		}
		bounds := c.layoutBox()
		if want := bounds.x + (bounds.w-300)/2; window.Tile.X != want {
			t.Errorf("the tile X = %d, want %d: the lone column centered in %+v", window.Tile.X, want, bounds)
		}
		var counts []int
		for _, workspace := range snapshot.Workspaces {
			if workspace.Output == window.Output {
				counts = append(counts, len(workspace.WindowIDs))
			}
		}
		if !slices.Equal(counts, []int{0, 1, 0}) {
			t.Errorf("windows per workspace = %v, want an empty workspace above and below the window's", counts)
		}
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}
