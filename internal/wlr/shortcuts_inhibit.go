package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_keyboard_shortcuts_inhibit_v1.h>
#include <wlr/types/wlr_seat.h>
*/
import "C"

import "unsafe"

// Keyboard shortcuts inhibit (keyboard-shortcuts-inhibit-unstable-v1). As in
// Niri, an inhibitor is active while its surface has the user seat's
// keyboard focus, and then compositor binds pass to the client unless they
// are marked allow-inhibiting=false. ToggleKeyboardShortcutsInhibit turns the
// focused surface's inhibitor off and on. Inhibitors on injected seats never
// activate: they would hide the user's binds.

type shortcutsInhibitor struct {
	inhibitor *C.struct_wlr_keyboard_shortcuts_inhibitor_v1
	destroy   *Listener
	// toggledOff is set by ToggleKeyboardShortcutsInhibit and kept until the
	// toggle turns it back on or the inhibitor goes.
	toggledOff bool
}

func (c *Compositor) createShortcutsInhibit() bool {
	manager := C.wlr_keyboard_shortcuts_inhibit_v1_create(c.display)
	if manager == nil {
		return false
	}
	c.shortcutsInhibitors = map[*C.struct_wlr_keyboard_shortcuts_inhibitor_v1]*shortcutsInhibitor{}
	c.keep(AddListener(unsafe.Pointer(&manager.events.new_inhibitor), c.handleNewShortcutsInhibitor))
	return true
}

func (c *Compositor) handleNewShortcutsInhibitor(data unsafe.Pointer) {
	inhibitor := (*C.struct_wlr_keyboard_shortcuts_inhibitor_v1)(data)
	s := &shortcutsInhibitor{inhibitor: inhibitor}
	s.destroy = AddListener(unsafe.Pointer(&inhibitor.events.destroy), func(unsafe.Pointer) {
		c.drop(s.destroy)
		delete(c.shortcutsInhibitors, inhibitor)
	})
	c.keep(s.destroy)
	c.shortcutsInhibitors[inhibitor] = s
	c.updateShortcutsInhibitors()
}

// updateShortcutsInhibitors runs after every user keyboard focus change.
func (c *Compositor) updateShortcutsInhibitors() {
	focused := c.seat.keyboard_state.focused_surface
	for inhibitor, s := range c.shortcutsInhibitors {
		active := inhibitor.seat == c.seat && inhibitor.surface == focused && !s.toggledOff
		if active == bool(inhibitor.active) {
			continue
		}
		if active {
			C.wlr_keyboard_shortcuts_inhibitor_v1_activate(inhibitor)
		} else {
			C.wlr_keyboard_shortcuts_inhibitor_v1_deactivate(inhibitor)
		}
	}
}

// shortcutsInhibited reports whether the focused surface inhibits binds.
func (c *Compositor) shortcutsInhibited() bool {
	for inhibitor := range c.shortcutsInhibitors {
		if bool(inhibitor.active) {
			return true
		}
	}
	return false
}

// ToggleKeyboardShortcutsInhibit turns the focused surface's inhibitor off,
// or back on. With no inhibitor on the focused surface it does nothing.
func (c *Compositor) ToggleKeyboardShortcutsInhibit() {
	focused := c.seat.keyboard_state.focused_surface
	for inhibitor, s := range c.shortcutsInhibitors {
		if inhibitor.seat == c.seat && inhibitor.surface == focused {
			s.toggledOff = !s.toggledOff
		}
	}
	c.updateShortcutsInhibitors()
}
