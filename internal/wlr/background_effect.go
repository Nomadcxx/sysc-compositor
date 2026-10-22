package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 wayland-server pixman-1
#cgo CFLAGS: -DWLR_USE_UNSTABLE -I${SRCDIR}/protocols

#include <stdbool.h>
#include <stdint.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_scene.h>
#include "ext-background-effect-v1-protocol.h"

struct wl_global *sysc_bg_global_create(struct wl_display *, uintptr_t);
struct wl_resource *sysc_bg_surface_create(struct wl_client *, uint32_t, uint32_t, uintptr_t);
void sysc_bg_post_duplicate(struct wl_resource *);
void sysc_bg_post_surface_destroyed(struct wl_resource *);
const pixman_region32_t *sysc_bg_region(struct wl_resource *);
int sysc_bg_region_count(const pixman_region32_t *);
bool sysc_bg_region_rect(const pixman_region32_t *, int, int *, int *, int *, int *);
bool sysc_bg_find_scene_surface(struct wlr_scene *, struct wlr_surface *,
		struct wlr_scene_node **, struct wlr_scene_tree **, int *, int *);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"
)

type backgroundEffect struct {
	c              *Compositor
	surface        *C.struct_wlr_surface
	root           *C.struct_wlr_surface
	resource       *C.struct_wl_resource
	resourceHandle uintptr
	pending        []rect
	committed      []rect
	pendingSet     bool
	blurNodes      []unsafe.Pointer
	blurParent     unsafe.Pointer
	commit         *Listener
	rootCommit     *Listener
	surfaceDestroy *Listener
}

func (c *Compositor) createBackgroundEffectProtocol() error {
	handle := cgo.NewHandle(c)
	if C.sysc_bg_global_create(c.display, C.uintptr_t(handle)) == nil {
		handle.Delete()
		return errors.New("ext_background_effect_manager_v1 global create failed")
	}
	c.backgroundEffectHandle = uintptr(handle)
	return nil
}

func (c *Compositor) destroyBackgroundEffectProtocol() {
	if c.backgroundEffectHandle != 0 {
		cgo.Handle(c.backgroundEffectHandle).Delete()
		c.backgroundEffectHandle = 0
	}
}

func (c *Compositor) getBackgroundEffect(manager *C.struct_wl_resource, id uint32,
	surfaceResource *C.struct_wl_resource) {
	surface := C.wlr_surface_from_resource(surfaceResource)
	if surface == nil {
		return
	}
	key := unsafe.Pointer(surface)
	effect := c.backgroundEffects[key]
	if effect != nil && effect.resource != nil {
		C.sysc_bg_post_duplicate(manager)
		return
	}
	if effect == nil {
		effect = &backgroundEffect{c: c, surface: surface, root: C.wlr_surface_get_root_surface(surface)}
		c.backgroundEffects[key] = effect
		effect.commit = AddListener(unsafe.Pointer(&surface.events.commit), func(unsafe.Pointer) {
			if effect.pendingSet {
				effect.committed = append(effect.committed[:0], effect.pending...)
				effect.pending = nil
				effect.pendingSet = false
			}
			c.syncBackgroundEffect(effect, true)
		})
		effect.surfaceDestroy = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
			effect.surface = nil
			c.clearBackgroundEffectNodes(effect)
			c.drop(effect.commit)
			effect.commit = nil
			c.drop(effect.rootCommit)
			effect.rootCommit = nil
			c.drop(effect.surfaceDestroy)
			effect.surfaceDestroy = nil
			delete(c.backgroundEffects, key)
		})
		if effect.root != nil && effect.root != surface {
			effect.rootCommit = AddListener(unsafe.Pointer(&effect.root.events.commit), func(unsafe.Pointer) {
				c.syncBackgroundEffect(effect, false)
			})
		}
		c.keep(effect.commit, effect.surfaceDestroy, effect.rootCommit)
	}
	resourceHandle := cgo.NewHandle(effect)
	resource := C.sysc_bg_surface_create(C.wl_resource_get_client(manager),
		C.uint32_t(C.wl_resource_get_version(manager)), C.uint32_t(id), C.uintptr_t(resourceHandle))
	if resource == nil {
		resourceHandle.Delete()
		return
	}
	effect.resource = resource
	effect.resourceHandle = uintptr(resourceHandle)
}

func (e *backgroundEffect) setRegion(resource, region *C.struct_wl_resource) {
	if e.surface == nil {
		C.sysc_bg_post_surface_destroyed(resource)
		return
	}
	e.pending = e.pending[:0]
	if region != nil {
		pixmanRegion := C.sysc_bg_region(region)
		count := int(C.sysc_bg_region_count(pixmanRegion))
		for i := 0; i < count; i++ {
			var x, y, width, height C.int
			if C.sysc_bg_region_rect(pixmanRegion, C.int(i), &x, &y, &width, &height) {
				e.pending = append(e.pending, rect{int(x), int(y), int(width), int(height)})
			}
		}
	}
	e.pendingSet = true
}

func (e *backgroundEffect) resourceDestroyed(handle uintptr) {
	e.resource = nil
	e.pending = nil
	e.pendingSet = true
	e.resourceHandle = 0
	cgo.Handle(handle).Delete()
}

func (c *Compositor) syncBackgroundEffect(effect *backgroundEffect, rebuild bool) {
	if effect.surface == nil {
		c.clearBackgroundEffectNodes(effect)
		return
	}
	var surfaceNode *C.struct_wlr_scene_node
	var parent *C.struct_wlr_scene_tree
	var x, y C.int
	if !bool(C.sysc_bg_find_scene_surface(c.scene, effect.surface, &surfaceNode, &parent, &x, &y)) {
		c.clearBackgroundEffectNodes(effect)
		return
	}
	clipped := clipSurfaceRegion(effect.committed, int(effect.surface.current.width), int(effect.surface.current.height))
	parentPointer := unsafe.Pointer(parent)
	if rebuild || effect.blurParent != parentPointer || len(effect.blurNodes) != len(clipped) {
		c.clearBackgroundEffectNodes(effect)
		for _, area := range clipped {
			node := sceneFXCreateBlurNode(parentPointer, area.w, area.h)
			if node == nil {
				continue
			}
			sceneFXNodeSetPosition(node, int(x)+area.x, int(y)+area.y)
			sceneFXPlaceBelow(node, unsafe.Pointer(surfaceNode))
			effect.blurNodes = append(effect.blurNodes, node)
		}
		effect.blurParent = parentPointer
		return
	}
	for i, area := range clipped {
		node := effect.blurNodes[i]
		sceneFXNodeSetPosition(node, int(x)+area.x, int(y)+area.y)
		sceneFXPlaceBelow(node, unsafe.Pointer(surfaceNode))
	}
}

func clipSurfaceRegion(region []rect, width, height int) []rect {
	if width <= 0 || height <= 0 {
		return nil
	}
	clipped := make([]rect, 0, len(region))
	for _, item := range region {
		left, top := max(int64(0), int64(item.x)), max(int64(0), int64(item.y))
		right := min(int64(width), int64(item.x)+int64(item.w))
		bottom := min(int64(height), int64(item.y)+int64(item.h))
		if right > left && bottom > top {
			clipped = append(clipped, rect{int(left), int(top), int(right - left), int(bottom - top)})
		}
	}
	return clipped
}

func (c *Compositor) clearBackgroundEffectNodes(effect *backgroundEffect) {
	for _, node := range effect.blurNodes {
		sceneFXNodeDestroy(node)
	}
	effect.blurNodes = nil
	effect.blurParent = nil
}

func (c *Compositor) destroyBackgroundEffects() {
	for key, effect := range c.backgroundEffects {
		c.clearBackgroundEffectNodes(effect)
		c.drop(effect.commit)
		c.drop(effect.rootCommit)
		c.drop(effect.surfaceDestroy)
		delete(c.backgroundEffects, key)
	}
	c.backgroundEffects = nil
}
