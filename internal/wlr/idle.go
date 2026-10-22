package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_idle_inhibit_v1.h>
#include <wlr/types/wlr_idle_notify_v1.h>

// An inhibitor counts while its surface is on screen: mapped and shown on
// at least one output, which the scene reports through wl_surface.enter.
static bool sysc_idle_inhibitor_visible(struct wlr_idle_inhibitor_v1 *inhibitor) {
	struct wlr_surface *surface = inhibitor->surface;
	return surface->mapped && !wl_list_empty(&surface->current_outputs);
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// Idle (ext-idle-notify-v1 and idle-inhibit-v1). Only the user seat's real
// input counts as activity: injected seats never reset idle. Idle is
// inhibited while an inhibitor's surface is visible or a ScreenSaver D-Bus
// caller holds a cookie. Policy (when to blank or lock) stays with the shell.

func (c *Compositor) createIdle() bool {
	if c.idleNotifier = C.wlr_idle_notifier_v1_create(c.display); c.idleNotifier == nil {
		return false
	}
	manager := C.wlr_idle_inhibit_v1_create(c.display)
	if manager == nil {
		return false
	}
	c.idleInhibitors = map[*C.struct_wlr_idle_inhibitor_v1]*Listener{}
	c.keep(AddListener(unsafe.Pointer(&manager.events.new_inhibitor), c.handleNewIdleInhibitor))
	return true
}

func (c *Compositor) handleNewIdleInhibitor(data unsafe.Pointer) {
	inhibitor := (*C.struct_wlr_idle_inhibitor_v1)(data)
	var destroy *Listener
	destroy = AddListener(unsafe.Pointer(&inhibitor.events.destroy), func(unsafe.Pointer) {
		c.drop(destroy)
		delete(c.idleInhibitors, inhibitor)
		c.updateIdleInhibit()
	})
	c.keep(destroy)
	c.idleInhibitors[inhibitor] = destroy
	c.updateIdleInhibit()
}

// SetIdleInhibitedByDBus records whether the ScreenSaver service holds any
// cookie. It runs on the compositor thread.
func (c *Compositor) SetIdleInhibitedByDBus(inhibited bool) {
	c.dbusIdleInhibited = inhibited
	c.updateIdleInhibit()
}

// updateIdleInhibit runs when an inhibitor comes or goes and after every
// output repaint, which is when a surface's visibility can change.
func (c *Compositor) updateIdleInhibit() {
	if c.idleNotifier == nil {
		return
	}
	inhibited := c.dbusIdleInhibited
	for inhibitor := range c.idleInhibitors {
		if inhibited {
			break
		}
		inhibited = bool(C.sysc_idle_inhibitor_visible(inhibitor))
	}
	if inhibited == c.idleInhibited {
		return
	}
	c.idleInhibited = inhibited
	C.wlr_idle_notifier_v1_set_inhibited(c.idleNotifier, C.bool(inhibited))
}

// userActivity runs on every physical input event of the user seat. It resets
// idle timers and, as Niri does, wakes outputs PowerOffMonitors turned off.
func (c *Compositor) userActivity() {
	if c.idleNotifier != nil {
		C.wlr_idle_notifier_v1_notify_activity(c.idleNotifier, c.seat)
	}
	if c.powerOffActive {
		if err := c.PowerOnMonitors(); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: waking monitors on input: %v\n", err)
		}
	}
}
