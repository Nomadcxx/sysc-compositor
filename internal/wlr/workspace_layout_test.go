package wlr

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
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
