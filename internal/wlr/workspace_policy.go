package wlr

import "github.com/Nomadcxx/sysc-compositor/internal/wm"

// Only an explicit reference to the already-focused workspace toggles back.
func focusExplicitWorkspace(model *wm.Model, id uint64, auto bool) bool {
	if auto {
		for _, workspace := range model.Snapshot().Workspaces {
			if workspace.ID == id && workspace.IsFocused {
				return model.FocusWorkspacePrevious()
			}
		}
	}
	return model.FocusWorkspaceID(id)
}
