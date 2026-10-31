package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdbool.h>
#include <wlr/types/wlr_layer_shell_v1.h>
#include <wlr/types/wlr_seat.h>

static bool sysc_keyboard_on_layer(struct wlr_seat *seat) {
	struct wlr_surface *surface = seat->keyboard_state.focused_surface;
	return surface != NULL && wlr_layer_surface_v1_try_from_wlr_surface(surface) != NULL;
}
*/
import "C"

import (
	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/Nomadcxx/sysc-compositor/internal/ui"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// Announcer speaks compositor UI to a screen reader: the session's
// *a11y.Announcer. Its methods must not block.
type Announcer interface {
	Announce(text string)
	Focus(node a11y.Node)
	SetRecentWindows(name string, button bool)
	SetDescription(node a11y.Node, text string)
}

// a11yState is what the screen reader was last told about the window
// switcher, as Niri's A11y keeps mru_selection and mru_scope.
type a11yState struct {
	announcer    Announcer
	hasSelection bool
	selection    uint64
	hasScope     bool
	scope        wm.MRUScope
}

// SetAnnouncer connects compositor UI to a screen reader; nil disconnects it.
// Call it on the compositor thread.
func (c *Compositor) SetAnnouncer(announcer Announcer) {
	c.a11y = a11yState{announcer: announcer}
	if announcer != nil {
		announcer.SetDescription(a11y.ExitDialog, ui.ExitDialogText())
		c.refreshA11y()
	}
}

// a11yFocus is the compositor UI holding the keyboard, in the order of
// Niri's update_keyboard_focus.
func (c *Compositor) a11yFocus() a11y.Node {
	switch {
	case c.exitDialog.open:
		return a11y.ExitDialog
	case c.sessionLocked:
		return a11y.NoFocus
	case c.shotUI != nil:
		return a11y.ScreenshotUI
	case c.switcher.isOpen():
		return a11y.RecentWindows
	case c.model != nil && c.model.OverviewOpen() && !bool(C.sysc_keyboard_on_layer(c.seat)):
		return a11y.Overview
	}
	return a11y.NoFocus
}

// refreshA11y tells the screen reader what changed since it was last told:
// Niri's refresh_a11y without the workspace announcement, which the server
// makes from its IPC state.
func (c *Compositor) refreshA11y() {
	s := &c.a11y
	if s.announcer == nil {
		return
	}
	focus := c.a11yFocus()
	s.announcer.Focus(focus)
	if focus != a11y.RecentWindows {
		s.hasSelection, s.selection, s.hasScope = false, 0, false
		return
	}
	mru := c.switcher.mru
	id, ok := mru.Current()
	selectionChanged := ok != s.hasSelection || (ok && id != s.selection)
	s.hasSelection, s.selection = ok, id
	scope := mru.Scope()
	switch {
	case selectionChanged && ok:
		title := "Unknown"
		for _, v := range c.views {
			if v.id == id && v.toplevel != nil {
				if t := cStringPointer(v.toplevel.title); t != nil {
					title = *t
				}
			}
		}
		s.announcer.SetRecentWindows(title, true)
	case selectionChanged:
		s.announcer.SetRecentWindows("Recent windows empty, "+scopeText(scope), false)
	case !s.hasScope || s.scope != scope:
		s.announcer.Announce(scopeText(scope))
	}
	s.hasScope, s.scope = true, scope
}

// scopeText is Niri's a11y_scope_text.
func scopeText(scope wm.MRUScope) string {
	switch scope {
	case wm.MRUScopeOutput:
		return "Scope output"
	case wm.MRUScopeWorkspace:
		return "Scope workspace"
	}
	return "Scope all"
}

// announceHotkeyOverlay reads the overlay out when its action opens it,
// as Niri's a11y_announce_hotkey_overlay.
func (c *Compositor) announceHotkeyOverlay() {
	if c.a11y.announcer != nil {
		o := &c.hotkeyOverlay
		c.a11y.announcer.Announce(ui.HotkeyOverlayText(o.binds, o.hideNotBound, c.modKey, keyName))
	}
}
