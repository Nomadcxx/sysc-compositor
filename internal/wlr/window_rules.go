package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// ruleState reads the state a window rule is matched against. The window's
// own state comes from the model, the focus from the seat, and the startup
// flag from the compositor's clock.
func (c *Compositor) ruleState(view *toplevelView, snapshot wm.Snapshot) config.WindowState {
	state := config.WindowState{IsActiveInColumn: true, AtStartup: c.atStartup}
	if view == nil {
		return state
	}
	for _, window := range snapshot.Windows {
		if window.ID != view.id {
			continue
		}
		state.IsUrgent = window.IsUrgent
		state.IsFloating = window.Tile.Floating
		state.IsActiveInColumn = window.Tile.Active
		break
	}
	// Niri's is-active is the window that has the active border: every
	// workspace on the focused monitor has one.
	for _, workspace := range snapshot.Workspaces {
		if workspace.Output != snapshot.FocusedOutput || workspace.ActiveWindowID == nil {
			continue
		}
		if *workspace.ActiveWindowID == view.id {
			state.IsActive = true
			break
		}
	}
	// Niri's is-window-cast-target is the window that an ongoing window
	// screencast shows. The compositor has no window screencast protocol, so
	// no window is ever the target of one.
	if focused := c.keyboardFocusedView(); focused != nil && focused.id == view.id {
		state.IsFocused = true
	}
	return state
}
