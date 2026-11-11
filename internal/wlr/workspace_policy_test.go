package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
	"testing"
)

func TestExplicitWorkspaceFocusHonorsAutoBackAndForth(t *testing.T) {
	model := wm.New(wm.WorkspaceConfig{Name: "one"}, wm.WorkspaceConfig{Name: "two"})
	model.AddOutput("DP-1", layout.Bounds{Width: 1000, Height: 800}, 400, 16)
	one, _ := model.WorkspaceIDByName("one")
	two, _ := model.WorkspaceIDByName("two")
	model.FocusWorkspaceID(one)
	model.FocusWorkspaceID(two)
	if !focusExplicitWorkspace(model, two, true) {
		t.Fatal("active reference did not switch back")
	}
	active := func() uint64 {
		for _, w := range model.Snapshot().Workspaces {
			if w.IsFocused {
				return w.ID
			}
		}
		return 0
	}
	if got := active(); got != one {
		t.Fatalf("active=%d want %d", got, one)
	}
	focusExplicitWorkspace(model, one, false)
	if got := active(); got != one {
		t.Fatal("disabled setting still toggled")
	}
	model.AddOutput("DP-2", layout.Bounds{X: 1000, Width: 1000, Height: 800}, 400, 16)
	var remote uint64
	for _, w := range model.Snapshot().Workspaces {
		if w.Output == "DP-2" && w.IsActive {
			remote = w.ID
			break
		}
	}
	if !focusExplicitWorkspace(model, remote, true) || active() != remote {
		t.Fatal("cross-output focus toggled remote previous workspace")
	}
}
