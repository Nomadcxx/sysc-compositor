package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <sys/types.h>
#include <time.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_xdg_shell.h>

static pid_t sysc_surface_pid(struct wlr_surface *surface) {
	pid_t pid = 0;
	wl_client_get_credentials(wl_resource_get_client(surface->resource), &pid, NULL, NULL);
	return pid;
}

static uint64_t sysc_monotonic_time_ns(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (uint64_t)ts.tv_sec * 1000000000ULL + (uint64_t)ts.tv_nsec;
}
*/
import "C"

import (
	"cmp"
	"reflect"
	"slices"
	"time"
)

// WindowInfo is an immutable snapshot of one mapped toplevel. ID is stable for
// the window's lifetime and never reused within a run.
type WindowInfo struct {
	ID                uint64
	AppID, Title      *string
	PID               int
	X, Y              int // layout coordinates of the window geometry
	Width, Height     int
	Output            string
	IsFocused         bool
	FocusTimestamp    time.Duration
	HasFocusTimestamp bool
}

// Windows lists toplevels, most recently focused first by either seat, then
// newest first. Call it on the compositor thread.
func (c *Compositor) Windows() []WindowInfo {
	views := make([]*toplevelView, 0, len(c.views))
	for _, v := range c.views {
		if v.toplevel != nil && v.tree != nil && v.mapped && bool(v.toplevel.base.surface.mapped) {
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
			ID: v.id, AppID: cStringPointer(v.toplevel.app_id), Title: cStringPointer(v.toplevel.title),
			PID: int(C.sysc_surface_pid(v.toplevel.base.surface)),
			X:   x, Y: y, Width: int(g.width), Height: int(g.height),
			Output: c.outputNameForView(v), IsFocused: c.keyboardFocusedView() == v,
			FocusTimestamp: v.focusTimestamp, HasFocusTimestamp: v.hasFocusTimestamp,
		})
	}
	return out
}

func cStringPointer(value *C.char) *string {
	if value == nil {
		return nil
	}
	text := C.GoString(value)
	return &text
}

func (c *Compositor) notifyWindowStateChanged() {
	windows := c.Windows()
	if reflect.DeepEqual(windows, c.lastWindowState) {
		return
	}
	c.lastWindowState = slices.Clone(windows)
	c.notifyStateChanged()
}

func (c *Compositor) keyboardFocusedView() *toplevelView {
	if c.seat == nil {
		return nil
	}
	return c.viewForSurface(c.seat.keyboard_state.focused_surface)
}

func (c *Compositor) recordWindowFocus(view *toplevelView) {
	stamp := time.Duration(C.sysc_monotonic_time_ns())
	if c.pendingFocus != nil {
		c.cancelPendingWindowFocus(c.pendingFocus)
	}
	if !view.hasFocusTimestamp {
		view.focusTimestamp = stamp
		view.hasFocusTimestamp = true
		return
	}
	if c.focusTimer != nil {
		c.focusTimer.Stop()
	}
	c.pendingFocus = view
	c.pendingFocusStamp = stamp
	c.pendingFocusGeneration++
	generation := c.pendingFocusGeneration
	c.focusTimer = time.AfterFunc(750*time.Millisecond, func() {
		_ = c.Post(func() {
			c.commitPendingWindowFocus(view, generation)
		})
	})
}

func (c *Compositor) commitPendingWindowFocus(view *toplevelView, generation uint64) {
	if c.pendingFocus != view || c.pendingFocusGeneration != generation {
		return
	}
	c.pendingFocus = nil
	c.focusTimer = nil
	if !view.mapped {
		return
	}
	view.focusTimestamp = c.pendingFocusStamp
	view.hasFocusTimestamp = true
	c.notifyWindowStateChanged()
}

func (c *Compositor) cancelPendingWindowFocus(view *toplevelView) {
	if c.pendingFocus != view {
		return
	}
	if c.focusTimer != nil {
		c.focusTimer.Stop()
	}
	c.focusTimer = nil
	c.pendingFocus = nil
	c.pendingFocusGeneration++
}

// FocusWindow focuses the mapped toplevel with id. Unknown or unmapped IDs are
// harmless no-ops, as in Niri's window actions.
func (c *Compositor) FocusWindow(id uint64) bool {
	for _, view := range c.views {
		if view.id == id && view.mapped && bool(view.toplevel.base.surface.mapped) {
			c.focusView(view)
			return true
		}
	}
	return false
}

// CloseWindow sends the xdg-toplevel close request for id, or for the Niri
// layout-focused window when id is nil. A layer surface may own keyboard focus.
func (c *Compositor) CloseWindow(id *uint64) bool {
	var target *toplevelView
	if id == nil {
		target = c.focusedView
	} else {
		for _, view := range c.views {
			if view.id == *id {
				target = view
				break
			}
		}
	}
	if target == nil || !target.mapped || !bool(target.toplevel.base.surface.mapped) {
		return false
	}
	C.wlr_xdg_toplevel_send_close(target.toplevel)
	return true
}

// FocusWorkspaceOutput focuses the current workspace hosted by name. M4 has
// one active workspace per output, so this selects that output's most recently
// focused mapped window when one exists.
func (c *Compositor) FocusWorkspaceOutput(name string) bool {
	if name == "" {
		return false
	}
	connected := false
	for pointer := range c.sceneOutputs {
		if C.GoString((*C.struct_wlr_output)(pointer).name) == name {
			connected = true
			break
		}
	}
	if !connected {
		return false
	}
	var recent *toplevelView
	for _, view := range c.views {
		if !view.mapped || view.tree == nil || !bool(view.toplevel.base.surface.mapped) {
			continue
		}
		if c.outputNameForView(view) == name && (recent == nil || view.touched > recent.touched) {
			recent = view
		}
	}
	if recent != nil {
		c.focusView(recent)
		return true
	}
	c.setActiveOutputName(name)
	c.updateKeyboardFocus()
	return true
}

// touch records that a seat focused the view, for most-recently-focused order.
func (c *Compositor) touch(v *toplevelView) {
	if v != nil {
		c.touchCount++
		v.touched = c.touchCount
	}
}
