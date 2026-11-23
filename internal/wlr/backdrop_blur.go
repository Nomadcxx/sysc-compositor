package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 pixman-1
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/types/wlr_damage_ring.h>

static bool sysc_blur_visible_on(struct wlr_scene_node *node, int x, int y, int width, int height) {
	pixman_box32_t box = {x, y, x + width, y + height};
	return pixman_region32_contains_rectangle(&node->WLR_PRIVATE.visible, &box) != PIXMAN_REGION_OUT;
}

static bool sysc_blur_set_visibility(void *scene_ptr, bool enabled) {
	struct wlr_scene *scene = scene_ptr;
	bool previous = scene->WLR_PRIVATE.calculate_visibility;
	scene->WLR_PRIVATE.calculate_visibility = enabled;
	return previous;
}

static void sysc_blur_damage_whole(void *scene_output_ptr) {
	struct wlr_scene_output *scene_output = scene_output_ptr;
	wlr_damage_ring_add_whole(&scene_output->damage_ring);
}

static bool sysc_below_tree(struct wlr_scene_node *node, struct wlr_scene_tree *tree) {
	for (; node != NULL; node = node->parent == NULL ? NULL : &node->parent->node) {
		if (node == &tree->node) return true;
	}
	return false;
}
*/
import "C"

import (
	"image"
	"unsafe"
)

func (c *Compositor) syncBackdropBlur(output unsafe.Pointer, box image.Rectangle) *C.struct_wlr_scene_optimized_blur {
	if c.blurBackdropTree == nil || box.Empty() {
		return nil
	}
	cache := c.blurBackdrops[output]
	if cache == nil {
		cache = C.wlr_scene_optimized_blur_create(c.blurBackdropTree, C.int(box.Dx()), C.int(box.Dy()))
		if cache == nil {
			return nil
		}
		if c.blurBackdrops == nil {
			c.blurBackdrops = make(map[unsafe.Pointer]*C.struct_wlr_scene_optimized_blur)
		}
		c.blurBackdrops[output] = cache
		C.wlr_scene_node_set_position(&cache.node, C.int(box.Min.X), C.int(box.Min.Y))
		C.wlr_scene_optimized_blur_mark_dirty(cache)
		return cache
	}
	x, y := C.int(box.Min.X), C.int(box.Min.Y)
	moved := cache.node.x != x || cache.node.y != y
	resized := cache.width != C.int(box.Dx()) || cache.height != C.int(box.Dy())
	if moved {
		C.wlr_scene_node_set_position(&cache.node, x, y)
	}
	C.wlr_scene_optimized_blur_set_size(cache, C.int(box.Dx()), C.int(box.Dy()))
	if moved || resized {
		c.invalidateBackdropBlur(output)
	}
	return cache
}

func (c *Compositor) invalidateBackdropBlur(output unsafe.Pointer) {
	if cache := c.blurBackdrops[output]; cache != nil {
		if !cache.node.enabled {
			C.wlr_scene_node_set_enabled(&cache.node, true)
		}
		if !cache.dirty {
			C.wlr_scene_optimized_blur_mark_dirty(cache)
		}
	}
}

func (c *Compositor) dropBackdropBlur(output unsafe.Pointer) {
	if cache := c.blurBackdrops[output]; cache != nil {
		delete(c.blurBackdrops, output)
		C.wlr_scene_node_destroy(&cache.node)
	}
}

func (c *Compositor) blurBackdropContains(node *C.struct_wlr_scene_node) bool {
	tree := c.layerTrees[layerBackground]
	return tree != nil && bool(C.sysc_below_tree(node, tree))
}

// The background tree is below the cache it feeds. Effects within it
// must sample the current scene rather than the result they help produce.
func (c *Compositor) effectUsesBackdrop(effect *backgroundEffect, parent *C.struct_wlr_scene_tree, xray *bool) bool {
	if c.blurBackdropTree == nil || c.blurBackdropContains(&parent.node) {
		return false
	}
	if xray != nil {
		return *xray
	}
	return !isXDGPopup(effect.root)
}

// Only render a cache for an output with a visible consumer. A fullscreen
// opaque window can then retain the compositor's usual direct scanout path.
func (c *Compositor) prepareBackdropBlur(output, scene unsafe.Pointer) (*C.struct_wlr_scene_optimized_blur, func()) {
	if c.blurBackdropTree == nil || c.blurConfig.Off {
		return nil, nil
	}
	box := c.outputLayoutBox(output)
	if box.Empty() {
		return nil, nil
	}
	var visible bool
	for _, effect := range c.backgroundEffects {
		if !effect.cachedBlur {
			continue
		}
		for _, node := range effect.blurNodes {
			if bool(C.sysc_blur_visible_on((*C.struct_wlr_scene_node)(node),
				C.int(box.Min.X), C.int(box.Min.Y), C.int(box.Dx()), C.int(box.Dy()))) {
				visible = true
				break
			}
		}
		if visible {
			break
		}
	}
	if !visible {
		if cache := c.blurBackdrops[output]; cache != nil {
			C.wlr_scene_node_set_enabled(&cache.node, false)
		}
		return nil, nil
	}
	cache := c.syncBackdropBlur(output, box)
	if cache == nil {
		// A missing optimized buffer has no pixels. Use live blur until we
		// can allocate the backdrop node.
		c.setBackdropSampling(false)
		return nil, func() { c.setBackdropSampling(true) }
	}
	if !cache.dirty {
		return cache, nil
	}
	if !cache.node.enabled {
		C.wlr_scene_node_set_enabled(&cache.node, true)
	}

	// SceneFX exposes backdrop nodes below its dirty cache, but the
	// final opaque-region pass would still cull the black clear. Clear the
	// whole output during the rebuild so the cache cannot sample old pixels.
	// Clean frames keep the scene's normal visibility and damage rules.
	C.sysc_blur_damage_whole(scene)
	sceneRoot := unsafe.Pointer(c.scene)
	saved := C.sysc_blur_set_visibility(sceneRoot, false)
	return cache, func() {
		C.sysc_blur_set_visibility(sceneRoot, saved)
		if !cache.dirty {
			// The framebuffer persists. Removing the clean marker restores
			// the occlusion SceneFX lifted below it for the rebuild.
			C.wlr_scene_node_set_enabled(&cache.node, false)
		}
	}
}

func (c *Compositor) setBackdropSampling(cached bool) {
	for _, effect := range c.backgroundEffects {
		if effect.cachedBlur {
			for _, node := range effect.blurNodes {
				blur := C.wlr_scene_blur_from_node((*C.struct_wlr_scene_node)(node))
				C.wlr_scene_blur_set_should_only_blur_bottom_layer(blur, C.bool(cached))
			}
		}
	}
}

// Called from the existing surface observer after a commit, including a
// desynchronized subsurface commit. Follow popup parents back to their layer.
func (c *Compositor) backdropSurfaceChanged(surface *C.struct_wlr_surface) {
	if len(c.blurBackdrops) == 0 {
		return
	}
	for surface != nil {
		surface = C.wlr_surface_get_root_surface(surface)
		if surface == nil {
			return
		}
		if layer := c.layerSurfaces[unsafe.Pointer(surface)]; layer != nil {
			if layer.layer == layerBackground {
				c.invalidateBackdropBlur(layer.output)
			}
			return
		}
		popup := C.wlr_xdg_popup_try_from_wlr_surface(surface)
		if popup == nil {
			return
		}
		surface = popup.parent
	}
}

func newBackdropBlurTestCompositor(scene unsafe.Pointer) *Compositor {
	return &Compositor{blurBackdropTree: (*C.struct_wlr_scene_tree)(scene)}
}
