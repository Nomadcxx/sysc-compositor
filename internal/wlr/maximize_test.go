package wlr

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestNativeAndForeignMaximizeShareRestorePolicy(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	runTouchClients(t, c, 2, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		if err := clients[0].SetIdentity("maximize-target", "org.sysc.MaximizeTest"); err != nil {
			t.Fatal(err)
		}
		observer, err := testclient.StartForeignClient(c.SocketName())
		if err != nil {
			t.Fatal(err)
		}
		defer observer.Close()
		index := -1
		for i, window := range observer.Windows(true) {
			if window.Title == "maximize-target" {
				index = i
			}
		}
		if index < 0 {
			t.Fatal("foreign target was not advertised")
		}
		target := views[0].id
		onCompositor(t, c, func() { c.model.FocusWindow(target); c.model.ConsumeWindowIntoColumn(); c.applyWorkspaceModel() })
		check := func(maximized bool) {
			t.Helper()
			onCompositor(t, c, func() {
				if c.model.IsMaximized(target) != maximized {
					t.Errorf("model maximized=%t want=%t", c.model.IsMaximized(target), maximized)
				}
				if maximized {
					first, _ := findWindowSnapshot(c.WorkspaceSnapshot().Windows, target)
					second, _ := findWindowSnapshot(c.WorkspaceSnapshot().Windows, views[1].id)
					area := c.workspaceBounds(first.Output)
					if first.Tile.Column == second.Tile.Column || first.Tile.Width != area.Width || first.Tile.Height != area.Height || second.Tile.Maximized {
						t.Errorf("maximize did not extract to usable output: %+v %+v area=%+v", first, second, area)
					}
				}
			})
			if err := clients[0].SyncInput(); err != nil {
				t.Fatal(err)
			}
			_, _, native := clients[0].ConfiguredState()
			if native != maximized {
				t.Errorf("native configured maximized=%t want=%t", native, maximized)
			}
			if err := observer.Sync(); err != nil {
				t.Fatal(err)
			}
			foreign := observer.Windows(true)[index].State&1 != 0
			if foreign != maximized {
				t.Errorf("foreign maximized=%t want=%t", foreign, maximized)
			}
		}
		if err := clients[0].SetMaximized(true); err != nil {
			t.Fatal(err)
		}
		check(true)
		if err := clients[0].SetMaximized(true); err != nil {
			t.Fatal(err)
		}
		check(true)
		if err := observer.Request(index, 1); err != nil {
			t.Fatal(err)
		}
		check(false)
		if err := observer.Request(index, 1); err != nil {
			t.Fatal(err)
		}
		check(false)

		var beforeX, beforeY, beforeW, beforeH int
		onCompositor(t, c, func() {
			c.SetWindowFloating(target, true)
			before, _ := findWindowSnapshot(c.WorkspaceSnapshot().Windows, target)
			beforeX, beforeY, beforeW, beforeH = before.Tile.X, before.Tile.Y, before.Tile.Width, before.Tile.Height
		})
		if err := observer.Request(index, 0); err != nil {
			t.Fatal(err)
		}
		check(true)
		if err := clients[0].SetMaximized(false); err != nil {
			t.Fatal(err)
		}
		check(false)
		onCompositor(t, c, func() {
			after, _ := findWindowSnapshot(c.WorkspaceSnapshot().Windows, target)
			if !after.Tile.Floating || after.Tile.X != beforeX || after.Tile.Y != beforeY || after.Tile.Width != beforeW || after.Tile.Height != beforeH {
				t.Errorf("unmaximize did not restore floating geometry: %+v", after)
			}
			c.sessionLocked = true
		})
		if err := observer.Request(index, 0); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			if c.model.IsMaximized(target) {
				t.Error("foreign maximize changed a locked session")
			}
			c.sessionLocked = false
		})
	})
}
