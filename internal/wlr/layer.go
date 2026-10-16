package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_scene.h>

static struct wlr_scene_tree *sysc_tree_create(struct wlr_scene_tree *parent) {
	return wlr_scene_tree_create(parent);
}
*/
import "C"

import "unsafe"

// Stacking order, bottom to top, as layer-shell defines it with the windows
// between the bottom and top layers.
const (
	layerBackground = iota
	layerBottom
	layerWindows
	layerTop
	layerOverlay
	layerCount
)

// createLayerTrees makes one scene tree per stacking layer under the scene
// root. Creation order is drawing order.
func (c *Compositor) createLayerTrees() bool {
	for i := range c.layerTrees {
		if c.layerTrees[i] = C.sysc_tree_create(&c.scene.tree); c.layerTrees[i] == nil {
			return false
		}
	}
	return true
}

// layerSurfaceTree is the scene tree of a mapped layer surface, or nil.
func (c *Compositor) layerSurfaceTree(surface *C.struct_wlr_surface) *C.struct_wlr_scene_tree {
	if l := c.layerSurfaces[unsafe.Pointer(surface)]; l != nil {
		return l.tree
	}
	return nil
}

// layerSurface is one layer-shell surface the compositor draws.
type layerSurface struct {
	tree *C.struct_wlr_scene_tree
}
