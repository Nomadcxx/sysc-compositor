package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <sys/types.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_xdg_shell.h>

static pid_t sysc_surface_pid(struct wlr_surface *surface) {
	pid_t pid = 0;
	wl_client_get_credentials(wl_resource_get_client(surface->resource), &pid, NULL, NULL);
	return pid;
}
*/
import "C"

import (
	"cmp"
	"slices"
)

// WindowInfo describes one toplevel for the control socket. ID is stable for
// the window's lifetime and never reused within a run.
type WindowInfo struct {
	ID            uint64
	AppID, Title  string
	PID           int
	X, Y          int // layout coordinates of the window geometry
	Width, Height int
}

// Windows lists toplevels, most recently focused first by either seat, then
// newest first. Call it on the compositor thread.
func (c *Compositor) Windows() []WindowInfo {
	views := make([]*toplevelView, 0, len(c.views))
	for _, v := range c.views {
		if v.toplevel != nil && v.tree != nil {
			views = append(views, v)
		}
	}
	slices.SortFunc(views, func(a, b *toplevelView) int {
		return cmp.Or(cmp.Compare(b.touched, a.touched), cmp.Compare(b.id, a.id))
	})
	out := make([]WindowInfo, 0, len(views))
	for _, v := range views {
		x, y := v.treePosition()
		g := v.toplevel.base.geometry
		out = append(out, WindowInfo{
			ID: v.id, AppID: C.GoString(v.toplevel.app_id), Title: C.GoString(v.toplevel.title),
			PID: int(C.sysc_surface_pid(v.toplevel.base.surface)),
			X:   x + int(g.x), Y: y + int(g.y), Width: int(g.width), Height: int(g.height),
		})
	}
	return out
}

// touch records that a seat focused the view, for most-recently-focused order.
func (c *Compositor) touch(v *toplevelView) {
	if v != nil {
		c.touchCount++
		v.touched = c.touchCount
	}
}
