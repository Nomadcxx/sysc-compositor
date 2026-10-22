package wlr

/*
#cgo pkg-config: scenefx-0.5
#cgo LDFLAGS: -lm
#include <scenefx/render/fx_renderer/fx_renderer.h>
#include <scenefx/types/wlr_scene.h>
#include <math.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_xdg_shell.h>

struct sysc_corner_data {
	struct wlr_surface *root;
	struct fx_corner_radii radii;
	int root_x, root_y, root_width, root_height;
};

static void sysc_set_surface_buffer_corners(struct wlr_scene_buffer *buffer,
		int sx, int sy, void *data) {
	struct sysc_corner_data *corners = data;
	struct wlr_scene_surface *scene_surface = wlr_scene_surface_try_from_buffer(buffer);
	if (scene_surface == NULL || wlr_surface_get_root_surface(scene_surface->surface) != corners->root) {
		return;
	}
	int x, y;
	wlr_scene_node_coords(&buffer->node, &x, &y);
	int right = x + scene_surface->surface->current.width;
	int bottom = y + scene_surface->surface->current.height;
	int root_right = corners->root_x + corners->root_width;
	int root_bottom = corners->root_y + corners->root_height;
	struct fx_corner_radii radii = corner_radii_new(
		x == corners->root_x && y == corners->root_y ? corners->radii.top_left : 0,
		right == root_right && y == corners->root_y ? corners->radii.top_right : 0,
		right == root_right && bottom == root_bottom ? corners->radii.bottom_right : 0,
		x == corners->root_x && bottom == root_bottom ? corners->radii.bottom_left : 0);
	wlr_scene_buffer_set_corner_radii(buffer, radii);
}

static void sysc_find_window_root_buffer(struct wlr_scene_buffer *buffer,
		int sx, int sy, void *data) {
	struct sysc_corner_data *corners = data;
	struct wlr_scene_surface *scene_surface = wlr_scene_surface_try_from_buffer(buffer);
	if (scene_surface == NULL || scene_surface->surface != corners->root) {
		return;
	}
	wlr_scene_node_coords(&buffer->node, &corners->root_x, &corners->root_y);
	corners->root_width = scene_surface->surface->current.width;
	corners->root_height = scene_surface->surface->current.height;
}

static void sysc_scene_set_window_corners(struct wlr_scene_node *node,
		struct wlr_surface *root, int tl, int tr, int br, int bl) {
	struct sysc_corner_data data = {
		.root = root,
		.radii = corner_radii_new(tl, tr, br, bl),
	};
	wlr_scene_node_for_each_buffer(node, sysc_find_window_root_buffer, &data);
	if (data.root_width <= 0 || data.root_height <= 0) {
		return;
	}
	wlr_scene_node_for_each_buffer(node, sysc_set_surface_buffer_corners, &data);
}

static void sysc_scene_set_subsurface_clip(struct wlr_scene_node *node,
		bool enabled, int x, int y, int width, int height) {
	if (!enabled) {
		wlr_scene_subsurface_tree_set_clip(node, NULL);
		return;
	}
	struct wlr_box clip = { .x = x, .y = y, .width = width, .height = height };
	wlr_scene_subsurface_tree_set_clip(node, &clip);
}

static struct wlr_scene_node *sysc_scene_rect_create_node(
		struct wlr_scene_tree *parent, int width, int height, const float color[static 4]) {
	struct wlr_scene_rect *rect = wlr_scene_rect_create(parent, width, height, color);
	return rect == NULL ? NULL : &rect->node;
}

static void sysc_scene_rect_set_corner_radius_node(struct wlr_scene_node *node, int radius) {
	wlr_scene_rect_set_corner_radius(wlr_scene_rect_from_node(node), radius);
}

static void sysc_scene_rect_size_node(struct wlr_scene_node *node, int *width, int *height) {
	struct wlr_scene_rect *rect = wlr_scene_rect_from_node(node);
	*width = rect->width;
	*height = rect->height;
}

static void sysc_scene_shadow_size_node(struct wlr_scene_node *node, int *width, int *height) {
	struct wlr_scene_shadow *shadow = wlr_scene_shadow_from_node(node);
	*width = shadow->width;
	*height = shadow->height;
}

static void sysc_scene_rect_update_ring_node(struct wlr_scene_node *node,
		int x, int y, int width, int height, int border,
		int outer_tl, int outer_tr, int outer_br, int outer_bl,
		int inner_tl, int inner_tr, int inner_br, int inner_bl,
		const float color[static 4]) {
	struct wlr_scene_rect *rect = wlr_scene_rect_from_node(node);
	wlr_scene_node_set_position(node, x, y);
	wlr_scene_rect_set_size(rect, width, height);
	wlr_scene_rect_set_color(rect, color);
	wlr_scene_rect_set_corner_radii(rect, corner_radii_new(outer_tl, outer_tr, outer_br, outer_bl));
	struct clipped_region clip = {
		.area = { .x = border, .y = border,
			.width = width - border * 2, .height = height - border * 2 },
		.corners = corner_radii_new(inner_tl, inner_tr, inner_br, inner_bl),
	};
	wlr_scene_rect_set_clipped_region(rect, clip);
}

struct sysc_scene_clone_context {
	struct wlr_scene_tree *tree;
	int origin_x, origin_y;
	int count;
};

static void sysc_clone_scene_buffer(struct wlr_scene_buffer *source,
		int x, int y, void *data) {
	struct sysc_scene_clone_context *context = data;
	if (source->buffer == NULL) {
		return;
	}
	struct wlr_scene_buffer *copy = wlr_scene_buffer_create(context->tree, source->buffer);
	if (copy == NULL) {
		return;
	}
	wlr_scene_node_set_position(&copy->node, x - context->origin_x, y - context->origin_y);
	wlr_scene_buffer_set_transform(copy, source->transform);
	wlr_scene_buffer_set_opacity(copy, source->opacity);
	wlr_scene_buffer_set_filter_mode(copy, source->filter_mode);
	wlr_scene_buffer_set_corner_radii(copy, source->corners);
	if (source->src_box.width > 0 && source->src_box.height > 0) {
		wlr_scene_buffer_set_source_box(copy, &source->src_box);
	}
	if (source->dst_width > 0 && source->dst_height > 0) {
		wlr_scene_buffer_set_dest_size(copy, source->dst_width, source->dst_height);
	}
	context->count++;
}

static struct wlr_scene_tree *sysc_scene_clone_view(struct wlr_scene_tree *parent,
		struct wlr_scene_tree *source, int *count) {
	int origin_x = 0, origin_y = 0;
	if (!wlr_scene_node_coords(&source->node, &origin_x, &origin_y)) {
		*count = 0;
		return NULL;
	}
	struct wlr_scene_tree *copy = wlr_scene_tree_create(parent);
	if (copy == NULL) {
		*count = 0;
		return NULL;
	}
	wlr_scene_node_set_position(&copy->node, origin_x, origin_y);
	struct sysc_scene_clone_context context = {
		.tree = copy, .origin_x = origin_x, .origin_y = origin_y,
	};
	wlr_scene_node_for_each_buffer(&source->node, sysc_clone_scene_buffer, &context);
	*count = context.count;
	if (context.count == 0) {
		wlr_scene_node_destroy(&copy->node);
		return NULL;
	}
	return copy;
}

struct sysc_scene_scale_context {
	int root_x, root_y;
	double ratio_x, ratio_y;
};

static void sysc_scale_scene_buffer(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	struct sysc_scene_scale_context *context = data;
	int width = buffer->dst_width, height = buffer->dst_height;
	if (width <= 0 || height <= 0) {
		width = buffer->buffer->width;
		height = buffer->buffer->height;
		if (buffer->transform == WL_OUTPUT_TRANSFORM_90 ||
				buffer->transform == WL_OUTPUT_TRANSFORM_270 ||
				buffer->transform == WL_OUTPUT_TRANSFORM_FLIPPED_90 ||
				buffer->transform == WL_OUTPUT_TRANSFORM_FLIPPED_270) {
			int swap = width;
			width = height;
			height = swap;
		}
	}
	int scaled_x = (int)lround((x - context->root_x) * context->ratio_x);
	int scaled_y = (int)lround((y - context->root_y) * context->ratio_y);
	int scaled_width = (int)lround(width * context->ratio_x);
	int scaled_height = (int)lround(height * context->ratio_y);
	if (scaled_width < 1) scaled_width = 1;
	if (scaled_height < 1) scaled_height = 1;
	wlr_scene_node_set_position(&buffer->node, scaled_x, scaled_y);
	wlr_scene_buffer_set_dest_size(buffer, scaled_width, scaled_height);
}

static void sysc_scene_scale_tree(struct wlr_scene_tree *tree,
		double ratio_x, double ratio_y) {
	int root_x = 0, root_y = 0;
	wlr_scene_node_coords(&tree->node, &root_x, &root_y);
	struct sysc_scene_scale_context context = {
		.root_x = root_x, .root_y = root_y,
		.ratio_x = ratio_x, .ratio_y = ratio_y,
	};
	wlr_scene_node_for_each_buffer(&tree->node, sysc_scale_scene_buffer, &context);
}

static void sysc_scene_set_buffer_opacity(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	(void)x; (void)y;
	wlr_scene_buffer_set_opacity(buffer, *(float *)data);
}

static void sysc_scene_tree_set_opacity(struct wlr_scene_tree *tree, float opacity) {
	wlr_scene_node_for_each_buffer(&tree->node, sysc_scene_set_buffer_opacity, &opacity);
}

struct sysc_scene_opacity_ratio { float ratio; };

static void sysc_scene_multiply_buffer_opacity(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	(void)x; (void)y;
	struct sysc_scene_opacity_ratio *context = data;
	wlr_scene_buffer_set_opacity(buffer, buffer->opacity * context->ratio);
}

static void sysc_scene_tree_scale_opacity(struct wlr_scene_tree *tree, float ratio) {
	struct sysc_scene_opacity_ratio context = { .ratio = ratio };
	wlr_scene_node_for_each_buffer(&tree->node, sysc_scene_multiply_buffer_opacity, &context);
}

static void sysc_scene_tree_enable(struct wlr_scene_tree *tree, bool enabled) {
	wlr_scene_node_set_enabled(&tree->node, enabled);
}

static void sysc_scene_tree_set_position_node(struct wlr_scene_tree *tree, int x, int y) {
	wlr_scene_node_set_position(&tree->node, x, y);
}

struct sysc_scene_buffer_probe {
	int width, height;
	int origin_x, origin_y;
	float opacity;
	uint16_t corner_radii[4];
	bool corner_radii_found;
	struct wlr_surface *surface;
	int surface_x, surface_y, surface_width, surface_height;
	uint16_t surface_corner_radii[4];
	bool surface_found;
	bool found;
};

struct sysc_scene_bounds_probe {
	int origin_x, origin_y;
	int left, top, right, bottom;
	bool found;
};

static void sysc_scene_accumulate_buffer_bounds(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	if (buffer->buffer == NULL) {
		return;
	}
	struct sysc_scene_bounds_probe *probe = data;
	int width = buffer->dst_width, height = buffer->dst_height;
	if (width <= 0 || height <= 0) {
		width = buffer->buffer->width;
		height = buffer->buffer->height;
		if (buffer->transform == WL_OUTPUT_TRANSFORM_90 ||
				buffer->transform == WL_OUTPUT_TRANSFORM_270 ||
				buffer->transform == WL_OUTPUT_TRANSFORM_FLIPPED_90 ||
				buffer->transform == WL_OUTPUT_TRANSFORM_FLIPPED_270) {
			int swap = width;
			width = height;
			height = swap;
		}
	}
	x -= probe->origin_x;
	y -= probe->origin_y;
	if (!probe->found) {
		probe->left = x;
		probe->top = y;
		probe->right = x + width;
		probe->bottom = y + height;
		probe->found = true;
		return;
	}
	if (x < probe->left) probe->left = x;
	if (y < probe->top) probe->top = y;
	if (x + width > probe->right) probe->right = x + width;
	if (y + height > probe->bottom) probe->bottom = y + height;
}

static void sysc_scene_tree_bounds(struct wlr_scene_tree *tree,
		struct sysc_scene_bounds_probe *probe) {
	wlr_scene_node_coords(&tree->node, &probe->origin_x, &probe->origin_y);
	wlr_scene_node_for_each_buffer(&tree->node, sysc_scene_accumulate_buffer_bounds, probe);
}

static void sysc_scene_probe_buffer(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	(void)x; (void)y;
	struct sysc_scene_buffer_probe *probe = data;
	if (buffer->buffer == NULL) {
		return;
	}
	if (!probe->found) {
		probe->width = buffer->dst_width > 0 ? buffer->dst_width : buffer->buffer->width;
		probe->height = buffer->dst_height > 0 ? buffer->dst_height : buffer->buffer->height;
		probe->opacity = buffer->opacity;
		probe->found = true;
	}
	if (!probe->corner_radii_found &&
			(buffer->corners.top_left || buffer->corners.top_right || buffer->corners.bottom_right || buffer->corners.bottom_left)) {
		probe->corner_radii[0] = buffer->corners.top_left;
		probe->corner_radii[1] = buffer->corners.top_right;
		probe->corner_radii[2] = buffer->corners.bottom_right;
		probe->corner_radii[3] = buffer->corners.bottom_left;
		probe->corner_radii_found = true;
	}
	struct wlr_scene_surface *scene_surface = wlr_scene_surface_try_from_buffer(buffer);
	if (probe->surface != NULL && scene_surface != NULL && scene_surface->surface == probe->surface) {
		probe->surface_x = x - probe->origin_x;
		probe->surface_y = y - probe->origin_y;
		probe->surface_width = buffer->dst_width > 0 ? buffer->dst_width : buffer->buffer->width;
		probe->surface_height = buffer->dst_height > 0 ? buffer->dst_height : buffer->buffer->height;
		probe->surface_corner_radii[0] = buffer->corners.top_left;
		probe->surface_corner_radii[1] = buffer->corners.top_right;
		probe->surface_corner_radii[2] = buffer->corners.bottom_right;
		probe->surface_corner_radii[3] = buffer->corners.bottom_left;
		probe->surface_found = true;
	}
}

static void sysc_scene_tree_probe(struct wlr_scene_tree *tree,
		struct sysc_scene_buffer_probe *probe) {
	wlr_scene_node_coords(&tree->node, &probe->origin_x, &probe->origin_y);
	wlr_scene_node_for_each_buffer(&tree->node, sysc_scene_probe_buffer, probe);
}
*/
import "C"

import (
	"unsafe"
)

func createSceneFXRenderer(backend unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(C.fx_renderer_create((*C.struct_wlr_backend)(backend)))
}

func (c *Compositor) createAppearanceTrees() bool {
	c.windowShadowTree = C.wlr_scene_tree_create(&c.scene.tree)
	c.windowAnimationTree = C.wlr_scene_tree_create(&c.scene.tree)
	c.windowFocusTree = C.wlr_scene_tree_create(&c.scene.tree)
	if c.windowShadowTree == nil || c.windowAnimationTree == nil || c.windowFocusTree == nil {
		return false
	}
	C.wlr_scene_node_place_below(&c.windowShadowTree.node, &c.layerTrees[layerWindows].node)
	C.wlr_scene_node_place_below(&c.windowAnimationTree.node, &c.layerTrees[layerWindows].node)
	C.wlr_scene_node_place_below(&c.windowFocusTree.node, &c.layerTrees[layerTop].node)
	return true
}

func (c *Compositor) usesSceneFXRenderer() bool {
	return c.renderer != nil && C.fx_get_renderer(c.renderer) != nil
}

func sceneFXCreateShadow(parent unsafe.Pointer, width, height, radius int, softness float64, color [4]float32) unsafe.Pointer {
	rgba := [4]C.float{C.float(color[0]), C.float(color[1]), C.float(color[2]), C.float(color[3])}
	shadow := C.wlr_scene_shadow_create((*C.struct_wlr_scene_tree)(parent),
		C.int(width), C.int(height), C.int(radius), C.float(softness), &rgba[0])
	return unsafe.Pointer(shadow)
}

func sceneFXCreateRoundedRect(parent unsafe.Pointer, width, height, radius int, color [4]float32) unsafe.Pointer {
	rgba := [4]C.float{C.float(color[0]), C.float(color[1]), C.float(color[2]), C.float(color[3])}
	node := C.sysc_scene_rect_create_node((*C.struct_wlr_scene_tree)(parent),
		C.int(width), C.int(height), &rgba[0])
	if node == nil {
		return nil
	}
	C.sysc_scene_rect_set_corner_radius_node(node, C.int(radius))
	return unsafe.Pointer(node)
}

func sceneFXCreateRect(parent unsafe.Pointer, width, height int, color [4]float32) unsafe.Pointer {
	rgba := [4]C.float{C.float(color[0]), C.float(color[1]), C.float(color[2]), C.float(color[3])}
	return unsafe.Pointer(C.sysc_scene_rect_create_node((*C.struct_wlr_scene_tree)(parent), C.int(width), C.int(height), &rgba[0]))
}

func sceneFXCreateBlurNode(parent unsafe.Pointer, width, height int) unsafe.Pointer {
	blur := C.wlr_scene_blur_create((*C.struct_wlr_scene_tree)(parent), C.int(width), C.int(height))
	if blur == nil {
		return nil
	}
	return unsafe.Pointer(&blur.node)
}

func sceneFXPlaceBelow(node, sibling unsafe.Pointer) {
	C.wlr_scene_node_place_below((*C.struct_wlr_scene_node)(node), (*C.struct_wlr_scene_node)(sibling))
}

func sceneFXRectUpdateRing(node unsafe.Pointer, x, y, width, height, border int,
	outer, inner [4]uint16, color [4]float32) {
	rgba := [4]C.float{C.float(color[0]), C.float(color[1]), C.float(color[2]), C.float(color[3])}
	C.sysc_scene_rect_update_ring_node((*C.struct_wlr_scene_node)(node),
		C.int(x), C.int(y), C.int(width), C.int(height), C.int(border),
		C.int(outer[0]), C.int(outer[1]), C.int(outer[2]), C.int(outer[3]),
		C.int(inner[0]), C.int(inner[1]), C.int(inner[2]), C.int(inner[3]), &rgba[0])
}

func sceneFXShadowUpdate(node unsafe.Pointer, x, y, width, height, radius int, softness float64, color [4]float32) {
	shadow := (*C.struct_wlr_scene_shadow)(node)
	rgba := [4]C.float{C.float(color[0]), C.float(color[1]), C.float(color[2]), C.float(color[3])}
	C.wlr_scene_node_set_position(&shadow.node, C.int(x), C.int(y))
	C.wlr_scene_shadow_set_size(shadow, C.int(width), C.int(height))
	C.wlr_scene_shadow_set_corner_radius(shadow, C.int(radius))
	C.wlr_scene_shadow_set_blur_sigma(shadow, C.float(softness))
	C.wlr_scene_shadow_set_color(shadow, &rgba[0])
}

func sceneFXSetNodeEnabled(node unsafe.Pointer, enabled bool) {
	if node != nil {
		C.wlr_scene_node_set_enabled((*C.struct_wlr_scene_node)(node), C.bool(enabled))
	}
}

func sceneFXNodeEnabled(node unsafe.Pointer) bool {
	if node == nil {
		return false
	}
	var x, y C.int
	return bool(C.wlr_scene_node_coords((*C.struct_wlr_scene_node)(node), &x, &y))
}

func sceneFXNodeDestroy(node unsafe.Pointer) {
	if node != nil {
		C.wlr_scene_node_destroy((*C.struct_wlr_scene_node)(node))
	}
}

func sceneFXSetWindowCorners(node, surface unsafe.Pointer, radii [4]uint16) {
	C.sysc_scene_set_window_corners((*C.struct_wlr_scene_node)(node), (*C.struct_wlr_surface)(surface),
		C.int(radii[0]), C.int(radii[1]), C.int(radii[2]), C.int(radii[3]))
}

func sceneFXRectSize(node unsafe.Pointer) (int, int) {
	if node == nil {
		return 0, 0
	}
	var width, height C.int
	C.sysc_scene_rect_size_node((*C.struct_wlr_scene_node)(node), &width, &height)
	return int(width), int(height)
}

func sceneFXShadowSize(node unsafe.Pointer) (int, int) {
	if node == nil {
		return 0, 0
	}
	var width, height C.int
	C.sysc_scene_shadow_size_node((*C.struct_wlr_scene_node)(node), &width, &height)
	return int(width), int(height)
}

func sceneFXSetSubsurfaceClip(node unsafe.Pointer, enabled bool, x, y, width, height int) {
	C.sysc_scene_set_subsurface_clip((*C.struct_wlr_scene_node)(node), C.bool(enabled),
		C.int(x), C.int(y), C.int(width), C.int(height))
}

func sceneFXNodeSetPosition(node unsafe.Pointer, x, y int) {
	C.wlr_scene_node_set_position((*C.struct_wlr_scene_node)(node), C.int(x), C.int(y))
}

func sceneFXCloneView(parent, source unsafe.Pointer) (unsafe.Pointer, int) {
	var count C.int
	tree := C.sysc_scene_clone_view((*C.struct_wlr_scene_tree)(parent),
		(*C.struct_wlr_scene_tree)(source), &count)
	return unsafe.Pointer(tree), int(count)
}

func sceneFXScaleTree(tree unsafe.Pointer, previousX, previousY, x, y float64) {
	if tree == nil || previousX <= 0 || previousY <= 0 || x <= 0 || y <= 0 {
		return
	}
	C.sysc_scene_scale_tree((*C.struct_wlr_scene_tree)(tree),
		C.double(x/previousX), C.double(y/previousY))
}

func sceneFXSetTreeOpacity(tree unsafe.Pointer, opacity float64) {
	if tree != nil {
		C.sysc_scene_tree_set_opacity((*C.struct_wlr_scene_tree)(tree), C.float(opacity))
	}
}

func sceneFXScaleTreeOpacity(tree unsafe.Pointer, ratio float64) {
	if tree != nil {
		C.sysc_scene_tree_scale_opacity((*C.struct_wlr_scene_tree)(tree), C.float(ratio))
	}
}

func sceneFXTreeSetEnabled(tree unsafe.Pointer, enabled bool) {
	if tree != nil {
		C.sysc_scene_tree_enable((*C.struct_wlr_scene_tree)(tree), C.bool(enabled))
	}
}

func sceneFXTreeSetPosition(tree unsafe.Pointer, x, y int) {
	if tree != nil {
		C.sysc_scene_tree_set_position_node((*C.struct_wlr_scene_tree)(tree), C.int(x), C.int(y))
	}
}

func sceneFXTreeFirstBuffer(tree unsafe.Pointer) (width, height int, opacity float64, found bool) {
	if tree == nil {
		return 0, 0, 0, false
	}
	var probe C.struct_sysc_scene_buffer_probe
	C.sysc_scene_tree_probe((*C.struct_wlr_scene_tree)(tree), &probe)
	return int(probe.width), int(probe.height), float64(probe.opacity), bool(probe.found)
}

func sceneFXTreeBounds(tree unsafe.Pointer) (bounds rect, found bool) {
	if tree == nil {
		return rect{}, false
	}
	var probe C.struct_sysc_scene_bounds_probe
	C.sysc_scene_tree_bounds((*C.struct_wlr_scene_tree)(tree), &probe)
	return rect{
		x: int(probe.left), y: int(probe.top),
		w: int(probe.right - probe.left), h: int(probe.bottom - probe.top),
	}, bool(probe.found)
}

func sceneFXTreeRoundedBufferCornerRadii(tree unsafe.Pointer) [4]uint16 {
	var probe C.struct_sysc_scene_buffer_probe
	if tree != nil {
		C.sysc_scene_tree_probe((*C.struct_wlr_scene_tree)(tree), &probe)
	}
	return [4]uint16{
		uint16(probe.corner_radii[0]), uint16(probe.corner_radii[1]),
		uint16(probe.corner_radii[2]), uint16(probe.corner_radii[3]),
	}
}

func sceneFXSurfaceCornerRadii(tree, surface unsafe.Pointer) ([4]uint16, bool) {
	if tree == nil || surface == nil {
		return [4]uint16{}, false
	}
	var probe C.struct_sysc_scene_buffer_probe
	probe.surface = (*C.struct_wlr_surface)(surface)
	C.sysc_scene_tree_probe((*C.struct_wlr_scene_tree)(tree), &probe)
	return [4]uint16{
		uint16(probe.surface_corner_radii[0]), uint16(probe.surface_corner_radii[1]),
		uint16(probe.surface_corner_radii[2]), uint16(probe.surface_corner_radii[3]),
	}, bool(probe.surface_found)
}

func sceneFXSurfaceBounds(tree, surface unsafe.Pointer) (rect, bool) {
	if tree == nil || surface == nil {
		return rect{}, false
	}
	var probe C.struct_sysc_scene_buffer_probe
	probe.surface = (*C.struct_wlr_surface)(surface)
	C.sysc_scene_tree_probe((*C.struct_wlr_scene_tree)(tree), &probe)
	return rect{
		x: int(probe.surface_x), y: int(probe.surface_y),
		w: int(probe.surface_width), h: int(probe.surface_height),
	}, bool(probe.surface_found)
}
