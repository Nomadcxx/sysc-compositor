package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdint.h>
#include <stdlib.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_foreign_toplevel_management_v1.h>
#include <wlr/types/wlr_xdg_shell.h>

struct wl_global *sysc_foreign_list_create(struct wl_display *, uintptr_t);
struct wl_resource *sysc_foreign_handle_create(struct wl_resource *, uintptr_t);
void sysc_foreign_list_finished(struct wl_resource *);
void sysc_foreign_handle_closed(struct wl_resource *);
void sysc_foreign_handle_done(struct wl_resource *);
void sysc_foreign_handle_title(struct wl_resource *, const char *);
void sysc_foreign_handle_app_id(struct wl_resource *, const char *);
void sysc_foreign_handle_identifier(struct wl_resource *, const char *);
void sysc_foreign_set_output(struct wlr_foreign_toplevel_handle_v1 *, struct wlr_output *);
void sysc_foreign_manager_no_memory(struct wlr_foreign_toplevel_manager_v1 *);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"strconv"
	"unsafe"
)

type foreignToplevelProtocol struct {
	handle  cgo.Handle
	manager *C.struct_wlr_foreign_toplevel_manager_v1
	lists   map[*C.struct_wl_resource]bool
	handles map[*C.struct_wl_resource]*foreignToplevel
}

// A mapping owns its native handle and listeners. Closed ext resources keep
// this object alive with view=nil until the client destroys them; a later map
// gets a new object, so an old resource cannot resolve to the remapped window.
type foreignToplevel struct {
	view               *toplevelView
	native             *C.struct_wlr_foreign_toplevel_handle_v1
	resources          map[*C.struct_wl_resource]bool
	listeners          []*Listener
	title, appID       string
	hasTitle, hasAppID bool
}

func (c *Compositor) createForeignToplevels() error {
	p := &foreignToplevelProtocol{lists: map[*C.struct_wl_resource]bool{}, handles: map[*C.struct_wl_resource]*foreignToplevel{}}
	p.handle = cgo.NewHandle(c)
	c.foreign = p
	p.manager = C.wlr_foreign_toplevel_manager_v1_create(c.display)
	if p.manager == nil || C.sysc_foreign_list_create(c.display, C.uintptr_t(p.handle)) == nil {
		return errors.New("foreign-toplevel manager create failed")
	}
	return nil
}

func (c *Compositor) bindForeignList(resource *C.struct_wl_resource) {
	c.foreign.lists[resource] = true
	for _, view := range c.views {
		if view.foreign != nil {
			c.addForeignHandle(view.foreign, resource)
		}
	}
}

func (c *Compositor) addForeignHandle(f *foreignToplevel, list *C.struct_wl_resource) {
	resource := C.sysc_foreign_handle_create(list, C.uintptr_t(c.foreign.handle))
	if resource == nil {
		return
	}
	c.foreign.handles[resource] = f
	f.resources[resource] = true
	id := C.CString(strconv.FormatUint(f.view.id, 10))
	C.sysc_foreign_handle_identifier(resource, id)
	C.free(unsafe.Pointer(id))
	if f.hasTitle {
		title := C.CString(f.title)
		C.sysc_foreign_handle_title(resource, title)
		C.free(unsafe.Pointer(title))
	}
	if f.hasAppID {
		app := C.CString(f.appID)
		C.sysc_foreign_handle_app_id(resource, app)
		C.free(unsafe.Pointer(app))
	}
	C.sysc_foreign_handle_done(resource)
}

func (c *Compositor) syncForeignToplevels() {
	if c.foreign == nil {
		return
	}
	for _, view := range c.views {
		if !view.mapped || view.toplevel == nil || !bool(view.toplevel.base.surface.mapped) {
			continue
		}
		f := view.foreign
		created := f == nil
		if created {
			handle := C.wlr_foreign_toplevel_handle_v1_create(c.foreign.manager)
			if handle == nil {
				for list := range c.foreign.lists {
					C.wl_resource_post_no_memory(list)
				}
				C.sysc_foreign_manager_no_memory(c.foreign.manager)
				continue
			}
			f = &foreignToplevel{view: view, native: handle, resources: map[*C.struct_wl_resource]bool{}}
			view.foreign = f
			c.watchForeignRequests(f)
		}
		title, app := view.toplevel.title, view.toplevel.app_id
		titleChanged := title != nil && (!f.hasTitle || f.title != C.GoString(title))
		appChanged := app != nil && (!f.hasAppID || f.appID != C.GoString(app))
		if titleChanged {
			f.title, f.hasTitle = C.GoString(title), true
			C.wlr_foreign_toplevel_handle_v1_set_title(f.native, title)
		}
		if appChanged {
			f.appID, f.hasAppID = C.GoString(app), true
			C.wlr_foreign_toplevel_handle_v1_set_app_id(f.native, app)
		}
		if created {
			for list := range c.foreign.lists {
				c.addForeignHandle(f, list)
			}
		} else if titleChanged || appChanged {
			for resource := range f.resources {
				if titleChanged {
					C.sysc_foreign_handle_title(resource, title)
				}
				if appChanged {
					C.sysc_foreign_handle_app_id(resource, app)
				}
				C.sysc_foreign_handle_done(resource)
			}
		}
		C.wlr_foreign_toplevel_handle_v1_set_activated(f.native, C.bool(c.keyboardFocusedView() == view))
		fullscreen := view.fullscreen || (c.model != nil && c.model.IsWindowedFullscreen(view.id))
		C.wlr_foreign_toplevel_handle_v1_set_fullscreen(f.native, C.bool(fullscreen))
		var output *C.struct_wlr_output
		name := view.outputName
		if name == "" {
			name = c.outputNameForView(view)
		}
		for pointer := range c.sceneOutputs {
			candidate := (*C.struct_wlr_output)(pointer)
			if C.GoString(candidate.name) == name {
				output = candidate
				break
			}
		}
		C.sysc_foreign_set_output(f.native, output)
	}
}

func (c *Compositor) watchForeignRequests(f *foreignToplevel) {
	add := func(signal unsafe.Pointer, fn func(unsafe.Pointer)) {
		listener := AddListener(signal, fn)
		f.listeners = append(f.listeners, listener)
		c.keep(listener)
	}
	allowed := func() bool { return f.view != nil && f.view.mapped && !c.sessionLocked }
	add(unsafe.Pointer(&f.native.events.request_activate), func(data unsafe.Pointer) {
		event := (*C.struct_wlr_foreign_toplevel_handle_v1_activated_event)(data)
		if allowed() && event.seat == c.seat {
			c.FocusWindow(f.view.id)
		}
	})
	add(unsafe.Pointer(&f.native.events.request_close), func(unsafe.Pointer) {
		if allowed() {
			c.CloseWindow(&f.view.id)
		}
	})
	add(unsafe.Pointer(&f.native.events.request_fullscreen), func(data unsafe.Pointer) {
		if !allowed() {
			return
		}
		event := (*C.struct_wlr_foreign_toplevel_handle_v1_fullscreen_event)(data)
		if event.output != nil && c.sceneOutputs[unsafe.Pointer(event.output)] != nil {
			c.MoveWindowToMonitor(&f.view.id, C.GoString(event.output.name))
		}
		c.SetFullscreen(f.view.id, bool(event.fullscreen))
	})
	// Niri ignores minimize and rectangle requests. ponytail: maximize waits
	// for M15's column sizing policy; keep this protocol's parity row partial.
}

func (c *Compositor) closeForeignToplevel(view *toplevelView) {
	f := view.foreign
	if f == nil {
		return
	}
	view.foreign = nil
	f.view = nil
	for resource := range f.resources {
		C.sysc_foreign_handle_closed(resource)
	}
	for _, listener := range f.listeners {
		c.drop(listener)
	}
	f.listeners = nil
	C.wlr_foreign_toplevel_handle_v1_destroy(f.native)
	f.native = nil
}

//export sysc_foreign_list_event
func sysc_foreign_list_event(handle C.uintptr_t, resource *C.struct_wl_resource, stop C.int) {
	c := cgo.Handle(handle).Value().(*Compositor)
	if stop == 0 {
		c.bindForeignList(resource)
	} else if c.foreign.lists[resource] {
		delete(c.foreign.lists, resource)
		C.sysc_foreign_list_finished(resource)
	}
}

//export sysc_foreign_resource_destroyed
func sysc_foreign_resource_destroyed(handle C.uintptr_t, resource *C.struct_wl_resource) {
	c := cgo.Handle(handle).Value().(*Compositor)
	delete(c.foreign.lists, resource)
	if f := c.foreign.handles[resource]; f != nil {
		delete(f.resources, resource)
		delete(c.foreign.handles, resource)
	}
}
