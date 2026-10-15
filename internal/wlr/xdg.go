package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/types/wlr_scene.h>

// The scene graph does the surface tracking, the buffer plumbing and the
// subsurface tree. Doing it by hand is the rest of tinywl; for M0 the scene
// graph is the same behaviour in fewer lines.
// The scene's root tree is embedded as its first member, so the scene itself
// is addressable as the parent tree. wlroots 0.20 no longer takes the layout
// here; the scene owns output placement.
static struct wlr_scene_tree *sysc_scene_xdg_create(struct wlr_scene *scene,
		struct wlr_xdg_surface *xdg_surface) {
	return wlr_scene_xdg_surface_create(&scene->tree, xdg_surface);
}
*/
import "C"

import "unsafe"

// handleNewSurface maps a client's first xdg_surface into the scene graph.
// The client commits its own buffer afterwards; nothing else is needed for a
// window to appear.
func (c *Compositor) handleNewSurface(data unsafe.Pointer) {
	xdgSurface := (*C.struct_wlr_xdg_surface)(data)
	if C.sysc_scene_xdg_create(c.scene, xdgSurface) == nil {
		return
	}
	c.mapped[unsafe.Pointer(xdgSurface)] = true
}

// Mapped reports whether an xdg_surface reached the scene graph. It exists so a
// test can assert a client mapped without reading the pixels back.
func (c *Compositor) Mapped() int { return len(c.mapped) }

// newSurfaceSignal is the xdg_wm_base new_surface signal, used by the handle
// ownership test to attach a listener to a real signal.
func (c *Compositor) newSurfaceSignal() Signal {
	return unsafe.Pointer(&(*C.struct_wlr_xdg_shell)(c.xdgShell).events.new_surface)
}
