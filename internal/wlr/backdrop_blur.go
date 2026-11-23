package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 pixman-1
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_damage_ring.h>
#include <wlr/util/transform.h>

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

// Every enabled node counts, as SceneFX's scene_node_bounds does: a layer
// shadow reaches past the surface.
static void sysc_blur_bounds_add(struct wlr_scene_node *node, int x, int y, pixman_box32_t *bounds) {
	if (!node->enabled) {
		return;
	}
	x += node->x;
	y += node->y;
	int width = 0, height = 0;
	switch (node->type) {
	case WLR_SCENE_NODE_TREE:;
		struct wlr_scene_node *child;
		wl_list_for_each(child, &wlr_scene_tree_from_node(node)->children, link) {
			sysc_blur_bounds_add(child, x, y, bounds);
		}
		return;
	case WLR_SCENE_NODE_BUFFER:;
		struct wlr_scene_buffer *buffer = wlr_scene_buffer_from_node(node);
		if (buffer->dst_width > 0 && buffer->dst_height > 0) {
			width = buffer->dst_width;
			height = buffer->dst_height;
		} else {
			width = buffer->WLR_PRIVATE.buffer_width;
			height = buffer->WLR_PRIVATE.buffer_height;
			wlr_output_transform_coords(buffer->transform, &width, &height);
		}
		break;
	case WLR_SCENE_NODE_RECT:
		width = wlr_scene_rect_from_node(node)->width;
		height = wlr_scene_rect_from_node(node)->height;
		break;
	case WLR_SCENE_NODE_SHADOW:
		width = wlr_scene_shadow_from_node(node)->width;
		height = wlr_scene_shadow_from_node(node)->height;
		break;
	case WLR_SCENE_NODE_BLUR:
		width = wlr_scene_blur_from_node(node)->width;
		height = wlr_scene_blur_from_node(node)->height;
		break;
	case WLR_SCENE_NODE_OPTIMIZED_BLUR:
		width = wlr_scene_optimized_blur_from_node(node)->width;
		height = wlr_scene_optimized_blur_from_node(node)->height;
		break;
	}
	if (width <= 0 || height <= 0) {
		return;
	}
	if (bounds->x1 >= bounds->x2) {
		*bounds = (pixman_box32_t){x, y, x + width, y + height};
		return;
	}
	if (x < bounds->x1) bounds->x1 = x;
	if (y < bounds->y1) bounds->y1 = y;
	if (x + width > bounds->x2) bounds->x2 = x + width;
	if (y + height > bounds->y2) bounds->y2 = y + height;
}

static pixman_box32_t sysc_blur_tree_bounds(void *tree_ptr) {
	struct wlr_scene_tree *tree = tree_ptr;
	pixman_box32_t bounds = {0};
	int x, y;
	if (tree != NULL && wlr_scene_node_coords(&tree->node, &x, &y)) {
		sysc_blur_bounds_add(&tree->node, x - tree->node.x, y - tree->node.y, &bounds);
	}
	return bounds;
}
*/
import "C"

import (
	"image"
	"math"
	"unsafe"
)

// backdropBlur is the blurred background an output's xray effects sample.
// The node stays disabled between rebuilds: SceneFX lifts occlusion below an
// enabled one, and its framebuffer outlives the node's visibility.
type backdropBlur struct {
	node *C.struct_wlr_scene_optimized_blur
	// stale: the backdrop changed after the node last rendered. The next
	// repaint that shows an xray effect on the output rebuilds it.
	stale bool
}

// backdropBlurOn: SceneFX renders no blur for a zero radius, so a cache would
// never rebuild. Effects then take the live path, as with no cache.
func (c *Compositor) backdropBlurOn() bool {
	return c.blurBackdropTree != nil && !c.blurConfig.Off && math.Round(c.blurConfig.Offset) > 0
}

func (c *Compositor) syncBackdropBlur(output unsafe.Pointer, box image.Rectangle) *backdropBlur {
	if c.blurBackdropTree == nil || box.Empty() {
		return nil
	}
	x, y := C.int(box.Min.X), C.int(box.Min.Y)
	width, height := C.int(box.Dx()), C.int(box.Dy())
	cache := c.blurBackdrops[output]
	if cache == nil {
		node := C.wlr_scene_optimized_blur_create(c.blurBackdropTree, width, height)
		if node == nil {
			return nil
		}
		C.wlr_scene_node_set_enabled(&node.node, false)
		C.wlr_scene_node_set_position(&node.node, x, y)
		if c.blurBackdrops == nil {
			c.blurBackdrops = make(map[unsafe.Pointer]*backdropBlur)
		}
		cache = &backdropBlur{node: node, stale: true}
		c.blurBackdrops[output] = cache
		return cache
	}
	if cache.node.node.x != x || cache.node.node.y != y {
		C.wlr_scene_node_set_position(&cache.node.node, x, y)
		cache.stale = true
	}
	if cache.node.width != width || cache.node.height != height {
		C.wlr_scene_optimized_blur_set_size(cache.node, width, height)
		cache.stale = true
	}
	return cache
}

// invalidateBackdropBlur leaves the scene alone: an output without a visible
// xray effect keeps its damage and direct scanout.
func (c *Compositor) invalidateBackdropBlur(output unsafe.Pointer) {
	cache := c.blurBackdrops[output]
	if cache == nil || cache.stale {
		return
	}
	cache.stale = true
	// A change under an opaque window damages nothing, but blur samples
	// past the window's edge. The repaint commits only if it rebuilds.
	if _, adopted := c.sceneOutputs[output]; adopted {
		C.wlr_output_schedule_frame((*C.struct_wlr_output)(output))
	}
}

func (c *Compositor) invalidateBackdropBlurIn(area image.Rectangle) {
	for output := range c.blurBackdrops {
		if c.outputLayoutBox(output).Overlaps(area) {
			c.invalidateBackdropBlur(output)
		}
	}
}

func (c *Compositor) dropBackdropBlur(output unsafe.Pointer) {
	if cache := c.blurBackdrops[output]; cache != nil {
		delete(c.blurBackdrops, output)
		C.wlr_scene_node_destroy(&cache.node.node)
	}
}

func (c *Compositor) blurBackdropContains(node *C.struct_wlr_scene_node) bool {
	tree := c.layerTrees[layerBackground]
	return tree != nil && bool(C.sysc_below_tree(node, tree))
}

// The background tree is below the cache it feeds. Effects within it
// must sample the current scene rather than the result they help produce.
func (c *Compositor) effectUsesBackdrop(effect *backgroundEffect, parent *C.struct_wlr_scene_tree, xray *bool) bool {
	if !c.backdropBlurOn() || c.blurBackdropContains(&parent.node) {
		return false
	}
	if xray != nil {
		return *xray
	}
	return !isXDGPopup(effect.root)
}

// prepareBackdropBlur runs before an output renders. It rebuilds the cache
// only for an output with a visible consumer. A fullscreen opaque window can
// then keep the compositor's usual direct scanout path.
func (c *Compositor) prepareBackdropBlur(output, scene unsafe.Pointer) (*backdropBlur, func()) {
	c.flushBackdropLayers()
	if !c.backdropBlurOn() {
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
		if cache := c.blurBackdrops[output]; cache != nil && cache.node.node.enabled {
			// A rebuild that did not complete leaves the node dirty.
			cache.stale = cache.stale || bool(cache.node.dirty)
			C.wlr_scene_node_set_enabled(&cache.node.node, false)
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
	if !cache.stale && !bool(cache.node.dirty) {
		return cache, nil
	}
	cache.stale = false
	// Enabling and dirtying the node lifts the occlusion below it and
	// damages the output through the scene, so the commit carries it.
	if !cache.node.node.enabled {
		C.wlr_scene_node_set_enabled(&cache.node.node, true)
	}
	if !cache.node.dirty {
		C.wlr_scene_optimized_blur_mark_dirty(cache.node)
	}

	// The final opaque-region pass would still cull the black clear below
	// opaque windows, and undamaged pixels hold the last composed frame.
	// Repaint the whole output with culling off so the cache samples only
	// the backdrop. Clean frames keep the scene's normal visibility rules.
	C.sysc_blur_damage_whole(scene)
	sceneRoot := unsafe.Pointer(c.scene)
	saved := C.sysc_blur_set_visibility(sceneRoot, false)
	return cache, func() {
		C.sysc_blur_set_visibility(sceneRoot, saved)
		if !cache.node.dirty {
			// The framebuffer persists. Disabling the node restores the
			// occlusion SceneFX lifted below it for the rebuild.
			C.wlr_scene_node_set_enabled(&cache.node.node, false)
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
// desynchronized subsurface commit, an unmap and a destroy.
func (c *Compositor) backdropSurfaceChanged(surface *C.struct_wlr_surface) {
	if l := c.layerSurfaceForSurface(surface); l != nil {
		c.backdropLayerChanged(l)
	}
}

// backdropLayerChanged queues a layer surface whose tree may have changed
// the backdrop. The next repaint measures it after the scene has applied the
// change. A popup, shadow or oversized buffer can reach another output, and
// what it covered before still needs a rebuild.
func (c *Compositor) backdropLayerChanged(l *layerSurface) {
	if c.blurBackdropTree == nil || l.layer != layerBackground && l.backdropBox.Empty() {
		return
	}
	if c.blurBackdropLayers == nil {
		c.blurBackdropLayers = make(map[*layerSurface]struct{})
	}
	c.blurBackdropLayers[l] = struct{}{}
}

func (c *Compositor) flushBackdropLayers() {
	for l := range c.blurBackdropLayers {
		var box image.Rectangle
		if l.layer == layerBackground {
			box = backdropTreeBounds(unsafe.Pointer(l.tree))
		}
		c.invalidateBackdropBlurIn(box.Union(l.backdropBox))
		l.backdropBox = box
	}
	clear(c.blurBackdropLayers)
}

// backdropLayerDestroyed reads no scene state, which may already be gone.
// Every rebuild flushes first, so the last box covers what any cache holds.
func (c *Compositor) backdropLayerDestroyed(l *layerSurface) {
	delete(c.blurBackdropLayers, l)
	c.invalidateBackdropBlurIn(l.backdropBox)
}

// backdropTreeBounds is where an enabled tree draws, in layout coordinates.
func backdropTreeBounds(tree unsafe.Pointer) image.Rectangle {
	b := C.sysc_blur_tree_bounds(tree)
	return image.Rect(int(b.x1), int(b.y1), int(b.x2), int(b.y2))
}

func newBackdropBlurTestCompositor(scene unsafe.Pointer) *Compositor {
	return &Compositor{blurBackdropTree: (*C.struct_wlr_scene_tree)(scene)}
}
