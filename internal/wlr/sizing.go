package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// The column and window sizing actions of Niri, reachable over the
// Niri-compatible socket as well as from a key binding.

func (c *Compositor) SetColumnWidth(change layout.SizeChange) bool {
	return c.SetWindowWidth(nil, change)
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

func (c *Compositor) SetWindowWidth(id *uint64, change layout.SizeChange) bool {
	if id == nil && c.shotUI != nil {
		return c.setShotSize(change, true)
	}
	if change.Fixed != 0 && !change.Relative {
		if window, ok := c.windowActionSnapshot(id); ok && !window.Tile.Floating {
			for _, view := range c.views {
				if view.id == window.ID && !view.decorations.border.Off {
					change.Fixed += 2 * decorationWidth(view.decorations.border.Width)
					break
				}
			}
		}
	}
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SetWindowWidthFor(windowActionID(id), change)
	})
}

func (c *Compositor) SwitchPresetWindowWidth(id *uint64, backwards bool) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SwitchPresetWindowWidthFor(windowActionID(id), backwards)
	})
}

func (c *Compositor) CenterWindow(id *uint64) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.CenterWindowFor(windowActionID(id))
	})
}

func (c *Compositor) SetWindowHeight(id *uint64, change layout.SizeChange) bool {
	if id == nil && c.shotUI != nil {
		return c.setShotSize(change, false)
	}
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

// SetMaximized is shared by native xdg and foreign-toplevel requests. The
// layout owns extraction and restoration; fullscreen keeps its underlying state.
func (c *Compositor) SetMaximized(id uint64, maximized bool) bool {
	if c.model == nil || c.sessionLocked {
		return false
	}
	for _, view := range c.views {
		if view.id == id && view.mapped {
			return c.applyWorkspaceAction(func(model *wm.Model) bool {
				return model.SetMaximized(id, maximized)
			})
		}
	}
	return false
}

// windowActionID turns an optional window id into the id the model uses, where
// zero means the focused window.
func windowActionID(id *uint64) uint64 {
	if id == nil {
		return 0
	}
	return *id
}
