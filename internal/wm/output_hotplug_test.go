package wm

import (
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"testing"
)

func TestOutputHotplugPreservesAndRestoresWholeWorkspaces(t *testing.T) {
	m := New(WorkspaceConfig{Name: "side", OpenOnOutput: "DP-1"})
	m.AddOutput("eDP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	m.AddOutput("DP-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	id, _ := m.WorkspaceIDByName("side")
	m.MapWindowToWorkspace(1, id)
	m.MapWindowToWorkspace(2, id)
	m.RemoveOutput("DP-1")
	for _, window := range m.Snapshot().Windows {
		if window.Output != "eDP-1" || window.WorkspaceID != id {
			t.Fatalf("workspace split on unplug: %+v", window)
		}
	}
	m.AddOutput("DP-1", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 500, 16)
	for _, window := range m.Snapshot().Windows {
		if window.Output != "DP-1" || window.WorkspaceID != id {
			t.Fatalf("workspace not restored: %+v", window)
		}
	}
	m.RemoveOutput("DP-1")
	m.RemoveOutput("eDP-1")
	if snapshot := m.Snapshot(); len(snapshot.Windows) != 2 || snapshot.Windows[0].Output != "" || snapshot.Windows[0].WorkspaceID != id {
		t.Fatalf("detached workspace disappeared: %+v", snapshot)
	}
	m.DestroyWindow(2)
	m.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	windows := m.Snapshot().Windows
	if len(windows) != 1 || windows[0].ID != 1 || windows[0].WorkspaceID != id {
		t.Fatalf("all outputs unplugged lost state or resurrected a closed window: %+v", windows)
	}
}
