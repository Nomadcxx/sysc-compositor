package wlr

import (
	"os"
	"os/exec"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// TestWindowRuleOpenMaximizedFillsTheWorkingWidth: a window that opens
// maximized takes the whole working width, the way Niri's open-maximized does.
func TestWindowRuleOpenMaximizedFillsTheWorkingWidth(t *testing.T) {
	c := newNested(t)
	openMaximized := true
	c.windowRules = []config.WindowRule{{OpenMaximized: &openMaximized}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		want := bounds.w - 2*16
		if window.Tile.Width != want {
			t.Errorf("maximized tile width = %d, want %d", window.Tile.Width, want)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleOpenMaximizedToEdgesFillsTheColumn: a window that opens
// maximized to the edges fills the height of its column.
func TestWindowRuleOpenMaximizedToEdgesFillsTheColumn(t *testing.T) {
	c := newNested(t)
	openMaximizedToEdges := true
	c.windowRules = []config.WindowRule{{OpenMaximizedToEdges: &openMaximizedToEdges}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		bounds := c.layoutBox()
		if window.Tile.Height != bounds.h-2*16 {
			t.Errorf("maximized tile height = %d, want %d", window.Tile.Height, bounds.h-2*16)
		}
		if window.Tile.Width != bounds.w-2*16 {
			t.Errorf("maximized tile width = %d, want %d", window.Tile.Width, bounds.w-2*16)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

// TestWindowRuleOpenFocusedFalseKeepsTheFocus: a window that opens without
// taking the focus leaves the focus where it was.
func TestWindowRuleOpenFocusedFalseKeepsTheFocus(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var first, second *toplevelView
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			// The window that maps next opens without taking the focus.
			openFocused := false
			c.windowRules = []config.WindowRule{{OpenFocused: &openFocused}}
			return
		}
		second = view
		if focused := c.keyboardFocusedView(); focused != first {
			if focused == nil {
				t.Error("the keyboard focus was lost when the second window mapped")
			} else {
				t.Errorf("keyboard focus after the second window mapped = window %d, want window %d",
					focused.id, first.id)
			}
		}
		c.Terminate()
	}

	clients := make([]*exec.Cmd, 0, 2)
	defer func() {
		for _, client := range clients {
			_ = client.Process.Kill()
		}
		for _, client := range clients {
			_ = client.Wait()
		}
	}()
	for range 2 {
		client := exec.Command(terminal)
		client.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
		if err := client.Start(); err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if first == nil || second == nil {
		t.Fatal("both windows did not map")
	}
}

// TestWindowRuleMinWidthAmendsTheSizeHints: the window rule's min-width
// raises the minimum the window gets, the way Niri's min-width does.
func TestWindowRuleMinWidthAmendsTheSizeHints(t *testing.T) {
	c := newNested(t)
	minWidth := c.layoutBox().w - 100
	c.windowRules = []config.WindowRule{{MinWidth: &minWidth}}
	c.viewMapped = func(view *toplevelView) {
		window, ok := findWindowSnapshot(c.WorkspaceSnapshot().Windows, view.id)
		if !ok {
			t.Errorf("mapped window %d missing from the workspace snapshot", view.id)
			c.Terminate()
			return
		}
		if window.Tile.Width != minWidth {
			t.Errorf("tile width = %d, want the rule's min-width %d", window.Tile.Width, minWidth)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}
