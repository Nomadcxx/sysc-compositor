package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server

#include <stdint.h>
#include <wayland-server-core.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);

// The startup timer ends the compositor's first minute. It fires once.
static int sysc_startup_timer(void *data) {
	sysc_listener_fire((uintptr_t)data, NULL);
	return 0;
}

static struct wl_event_source *sysc_startup_source(struct wl_event_loop *loop,
		uintptr_t token) {
	return wl_event_loop_add_timer(loop, sysc_startup_timer, (void *)token);
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// startupWindowMS is how long the compositor's startup lasts. Niri's
// at-startup window rules match for the first sixty seconds. It is a
// variable so a test can shorten it.
var startupWindowMS = 60000

// armStartupTimer starts the clock on the compositor's startup. When it runs
// out, the rules that matched at startup stop matching.
func (c *Compositor) armStartupTimer() {
	token := Register(func(unsafe.Pointer) { c.startupElapsed() })
	source := C.sysc_startup_source(c.loop, C.uintptr_t(token))
	if source == nil {
		Unregister(token)
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not create the startup timer")
		return
	}
	c.startupTimerSource = source
	c.startupTimerToken = token
	if C.wl_event_source_timer_update(source, C.int(startupWindowMS)) < 0 {
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not start the startup timer")
	}
}

// startupElapsed ends the startup window and reads the window rules again, so
// that a rule with at-startup stops matching.
func (c *Compositor) startupElapsed() {
	c.atStartup = false
	c.reapplyWindowRules()
}

func (c *Compositor) stopStartupTimer() {
	if c.startupTimerSource != nil {
		C.wl_event_source_remove(c.startupTimerSource)
		c.startupTimerSource = nil
	}
	if c.startupTimerToken != 0 {
		Unregister(c.startupTimerToken)
		c.startupTimerToken = 0
	}
}
