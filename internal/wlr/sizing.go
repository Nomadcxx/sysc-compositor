package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// The column width and window height actions of Niri, reachable over the
// Niri-compatible socket as well as from a key binding.

func (c *Compositor) SetColumnWidth(change layout.SizeChange) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SetColumnWidth(change)
	})
}

func (c *Compositor) SwitchPresetColumnWidth(backwards bool) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SwitchPresetColumnWidth(backwards)
	})
}

func (c *Compositor) ToggleMaximizeColumn() bool {
	return c.applyWorkspaceAction((*wm.Model).ToggleMaximizeColumn)
}

func (c *Compositor) MaximizeWindowToEdges(id *uint64) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.ToggleMaximizeWindowToEdgesFor(windowActionID(id))
	})
}

func (c *Compositor) CenterColumn() bool {
	return c.applyWorkspaceAction((*wm.Model).CenterColumn)
}

func (c *Compositor) CenterVisibleColumns() bool {
	return c.applyWorkspaceAction((*wm.Model).CenterVisibleColumns)
}

func (c *Compositor) ExpandColumnToAvailableWidth() bool {
	return c.applyWorkspaceAction((*wm.Model).ExpandColumnToAvailableWidth)
}

func (c *Compositor) SetWindowHeight(id *uint64, change layout.SizeChange) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SetWindowHeightFor(windowActionID(id), change)
	})
}

func (c *Compositor) ResetWindowHeight(id *uint64) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.ResetWindowHeightFor(windowActionID(id))
	})
}

func (c *Compositor) SwitchPresetWindowHeight(id *uint64, backwards bool) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SwitchPresetWindowHeightFor(windowActionID(id), backwards)
	})
}

// windowActionID turns an optional window id into the id the model uses, where
// zero means the focused window.
func windowActionID(id *uint64) uint64 {
	if id == nil {
		return 0
	}
	return *id
}
