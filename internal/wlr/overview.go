package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#cgo LDFLAGS: -lm

#include <math.h>
#include <stdint.h>
#include <stdlib.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_scene.h>

struct sysc_overview_project_context {
	struct wlr_scene_tree *tree;
	int source_x, source_y;
	int target_x, target_y;
	double zoom;
	int count;
	uintptr_t first_buffer, last_buffer;
};

static void sysc_overview_project_buffer(struct wlr_scene_buffer *source,
		int x, int y, void *data) {
	struct sysc_overview_project_context *context = data;
	if (source->buffer == NULL) {
		return;
	}
	if (context->first_buffer == 0) {
		context->first_buffer = (uintptr_t)source->buffer;
	}
	context->last_buffer = (uintptr_t)source->buffer;
	struct wlr_scene_buffer *copy = wlr_scene_buffer_create(context->tree, source->buffer);
	if (copy == NULL) {
		return;
	}
	int target_x = context->target_x + (int)lround((x - context->source_x) * context->zoom);
	int target_y = context->target_y + (int)lround((y - context->source_y) * context->zoom);
	wlr_scene_node_set_position(&copy->node, target_x, target_y);
	wlr_scene_buffer_set_transform(copy, source->transform);
	wlr_scene_buffer_set_opacity(copy, source->opacity);
	wlr_scene_buffer_set_filter_mode(copy, source->filter_mode);
	if (source->src_box.width > 0 && source->src_box.height > 0) {
		wlr_scene_buffer_set_source_box(copy, &source->src_box);
	}
	int width = source->dst_width > 0 ? source->dst_width : source->buffer->width;
	int height = source->dst_height > 0 ? source->dst_height : source->buffer->height;
	if (source->dst_width == 0 && source->dst_height == 0 &&
			(source->transform == WL_OUTPUT_TRANSFORM_90 ||
			 source->transform == WL_OUTPUT_TRANSFORM_270 ||
			 source->transform == WL_OUTPUT_TRANSFORM_FLIPPED_90 ||
			 source->transform == WL_OUTPUT_TRANSFORM_FLIPPED_270)) {
		int swap = width;
		width = height;
		height = swap;
	}
	if (width > 0 && height > 0) {
		width = (int)lround(width * context->zoom);
		height = (int)lround(height * context->zoom);
		wlr_scene_buffer_set_dest_size(copy, width > 0 ? width : 1, height > 0 ? height : 1);
	}
	context->count++;
}

static int sysc_overview_project_buffers(struct wlr_scene_tree *tree,
		struct wlr_scene_tree *source, int source_x, int source_y,
		int target_x, int target_y, double zoom, uintptr_t *first_buffer,
		uintptr_t *last_buffer) {
	struct sysc_overview_project_context context = {
		.tree = tree, .source_x = source_x, .source_y = source_y,
		.target_x = target_x, .target_y = target_y, .zoom = zoom,
	};
	wlr_scene_node_for_each_buffer(&source->node, sysc_overview_project_buffer, &context);
	*first_buffer = context.first_buffer;
	*last_buffer = context.last_buffer;
	return context.count;
}

struct sysc_overview_surface_list {
	struct wlr_surface **surfaces;
	size_t count;
	size_t capacity;
};

static void sysc_overview_collect_surface(struct wlr_surface *surface,
		int sx, int sy, void *data) {
	struct sysc_overview_surface_list *list = data;
	for (size_t i = 0; i < list->count; i++) {
		if (list->surfaces[i] == surface) {
			return;
		}
	}
	if (list->count == list->capacity) {
		size_t capacity = list->capacity == 0 ? 4 : list->capacity * 2;
		void *items = realloc(list->surfaces, capacity * sizeof(*list->surfaces));
		if (items == NULL) {
			return;
		}
		list->surfaces = items;
		list->capacity = capacity;
	}
	list->surfaces[list->count++] = surface;
}

static struct sysc_overview_surface_list *sysc_overview_surfaces(struct wlr_surface *root) {
	struct sysc_overview_surface_list *list = calloc(1, sizeof(*list));
	if (list != NULL) {
		wlr_surface_for_each_surface(root, sysc_overview_collect_surface, list);
	}
	return list;
}

static size_t sysc_overview_surface_count(struct sysc_overview_surface_list *list) {
	return list == NULL ? 0 : list->count;
}

static struct wlr_surface *sysc_overview_surface_at(struct sysc_overview_surface_list *list, size_t index) {
	return list == NULL || index >= list->count ? NULL : list->surfaces[index];
}

static void sysc_overview_surface_list_destroy(struct sysc_overview_surface_list *list) {
	if (list != NULL) {
		free(list->surfaces);
		free(list);
	}
}
*/
import "C"

import (
	"slices"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

type overviewBackdrop struct {
	output string
	Bounds layout.Bounds
	Color  config.Color
	node   *C.struct_wlr_scene_rect
}

type overviewProjection struct {
	Output      string
	WorkspaceID uint64
	WindowID    uint64
	Bounds      layout.Bounds
	BufferCount int
	SourceFirst uintptr
	SourceLast  uintptr
	tree        *C.struct_wlr_scene_tree
}

type overviewSurfaceListeners struct {
	commit, unmap, newSubsurface, destroy *Listener
}

func (c *Compositor) createOverviewTree() bool {
	c.overviewTree = C.wlr_scene_tree_create(&c.scene.tree)
	if c.overviewTree == nil {
		return false
	}
	C.wlr_scene_node_place_below(&c.overviewTree.node, &c.layerTrees[layerTop].node)
	C.wlr_scene_node_set_enabled(&c.overviewTree.node, false)
	c.insertHintTree = C.wlr_scene_tree_create(&c.scene.tree)
	if c.insertHintTree == nil {
		return false
	}
	C.wlr_scene_node_place_below(&c.insertHintTree.node, &c.layerTrees[layerTop].node)
	C.wlr_scene_node_set_enabled(&c.insertHintTree.node, false)
	return true
}

func (c *Compositor) ToggleOverview() bool {
	if c.model == nil || !c.model.ToggleOverview() {
		return false
	}
	c.syncOverviewScene()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) OpenOverview() bool {
	if c.model == nil || !c.model.OpenOverview() {
		return false
	}
	c.syncOverviewScene()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) CloseOverview() bool {
	if c.model == nil || !c.model.CloseOverview() {
		return false
	}
	c.syncOverviewScene()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) clearOverviewScene() {
	for _, projection := range c.overviewProjections {
		C.wlr_scene_node_destroy(&projection.tree.node)
	}
	for _, backdrop := range c.overviewBackdrops {
		C.wlr_scene_node_destroy(&backdrop.node.node)
	}
	c.overviewProjections = nil
	c.overviewBackdrops = nil
}

func (c *Compositor) syncOverviewScene() {
	if c.overviewTree == nil || c.model == nil {
		return
	}
	c.clearOverviewScene()
	progress := c.model.OverviewProgress()
	if progress <= 0 {
		C.wlr_scene_node_set_enabled(&c.overviewTree.node, false)
		return
	}
	C.wlr_scene_node_set_enabled(&c.overviewTree.node, true)

	type outputFrame struct {
		name  string
		frame wm.OverviewFrame
	}
	names := make([]string, 0, len(c.sceneOutputs))
	for pointer := range c.sceneOutputs {
		names = append(names, C.GoString((*C.struct_wlr_output)(pointer).name))
	}
	slices.Sort(names)
	frames := make([]outputFrame, 0, len(names))
	zoom := c.model.OverviewZoom(c.overviewConfig.Zoom)
	for _, name := range names {
		frame, ok := c.model.OverviewFrame(name, zoom)
		if !ok {
			continue
		}
		frames = append(frames, outputFrame{name: name, frame: frame})
		bounds, ok := c.outputLayoutBounds(name)
		if !ok || bounds.Width <= 0 || bounds.Height <= 0 {
			continue
		}
		color := c.overviewConfig.BackdropColor
		var rgba [4]C.float
		rgba[0] = C.float(color.R * color.A * progress)
		rgba[1] = C.float(color.G * color.A * progress)
		rgba[2] = C.float(color.B * color.A * progress)
		rgba[3] = C.float(color.A * progress)
		node := C.wlr_scene_rect_create(c.overviewTree,
			C.int(bounds.Width), C.int(bounds.Height), &rgba[0])
		if node == nil {
			continue
		}
		C.wlr_scene_node_set_position(&node.node, C.int(bounds.X), C.int(bounds.Y))
		c.overviewBackdrops = append(c.overviewBackdrops, overviewBackdrop{
			output: name, Bounds: bounds, Color: color, node: node,
		})
	}

	views := make(map[uint64]*toplevelView, len(c.views))
	for _, view := range c.views {
		if view != nil && view.mapped && view.tree != nil && view.toplevel != nil &&
			bool(view.toplevel.base.surface.mapped) {
			views[view.id] = view
		}
	}
	for _, current := range frames {
		for _, workspace := range current.frame.Workspaces {
			for _, window := range workspace.Windows {
				if !window.Visible {
					continue
				}
				view := views[window.ID]
				if view == nil {
					continue
				}
				tree := C.wlr_scene_tree_create(c.overviewTree)
				if tree == nil {
					continue
				}
				var sourceFirst, sourceLast C.uintptr_t
				count := int(C.sysc_overview_project_buffers(tree, view.tree,
					C.int(current.frame.Output.X), C.int(current.frame.Output.Y),
					C.int(workspace.Bounds.X), C.int(workspace.Bounds.Y), C.double(current.frame.Zoom),
					&sourceFirst, &sourceLast))
				if count == 0 {
					C.wlr_scene_node_destroy(&tree.node)
					continue
				}
				c.overviewProjections = append(c.overviewProjections, overviewProjection{
					Output: current.name, WorkspaceID: workspace.ID,
					WindowID: window.ID, Bounds: window.Bounds, BufferCount: count,
					SourceFirst: uintptr(sourceFirst), SourceLast: uintptr(sourceLast), tree: tree,
				})
			}
		}
	}
}

func (c *Compositor) overviewSelectAt(x, y float64) bool {
	if c.model == nil {
		return false
	}
	output := c.outputNameAt(x, y)
	if output == "" || !c.model.SelectOverviewAt(output, x, y, c.model.OverviewZoom(c.overviewConfig.Zoom)) {
		return false
	}
	c.syncOverviewScene()
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	return true
}

func (c *Compositor) syncInsertHint() {
	if c.insertHintTree == nil {
		return
	}
	if c.insertHintNode != nil {
		C.wlr_scene_node_destroy(&c.insertHintNode.node)
		c.insertHintNode = nil
	}
	if c.model == nil || c.layoutOptions.InsertHint.Off {
		C.wlr_scene_node_set_enabled(&c.insertHintTree.node, false)
		return
	}
	drag, ok := c.model.WindowDragState()
	if !ok || !drag.Started || drag.Target == nil {
		C.wlr_scene_node_set_enabled(&c.insertHintTree.node, false)
		return
	}
	bounds := drag.Target.Hint
	if bounds.Width <= 0 || bounds.Height <= 0 {
		C.wlr_scene_node_set_enabled(&c.insertHintTree.node, false)
		return
	}
	color := c.layoutOptions.InsertHint.Color
	rgba := [4]C.float{
		C.float(color.R * color.A), C.float(color.G * color.A),
		C.float(color.B * color.A), C.float(color.A),
	}
	c.insertHintNode = C.wlr_scene_rect_create(c.insertHintTree,
		C.int(bounds.Width), C.int(bounds.Height), &rgba[0])
	if c.insertHintNode == nil {
		C.wlr_scene_node_set_enabled(&c.insertHintTree.node, false)
		return
	}
	C.wlr_scene_node_set_position(&c.insertHintNode.node, C.int(bounds.X), C.int(bounds.Y))
	C.wlr_scene_node_set_enabled(&c.insertHintTree.node, true)
}

func (c *Compositor) syncOverviewSurfaceListeners(view *toplevelView) {
	if view == nil || view.toplevel == nil || view.toplevel.base.surface == nil {
		return
	}
	if view.overviewSurfaces == nil {
		view.overviewSurfaces = make(map[unsafe.Pointer]*overviewSurfaceListeners)
	}
	root := view.toplevel.base.surface
	list := C.sysc_overview_surfaces(root)
	if list == nil {
		return
	}
	defer C.sysc_overview_surface_list_destroy(list)
	for index, count := C.size_t(0), C.sysc_overview_surface_count(list); index < count; index++ {
		surface := C.sysc_overview_surface_at(list, index)
		if surface == nil {
			continue
		}
		key := unsafe.Pointer(surface)
		if view.overviewSurfaces[key] != nil {
			continue
		}
		listeners := &overviewSurfaceListeners{}
		if surface != root {
			listeners.commit = AddListener(unsafe.Pointer(&surface.events.commit), func(unsafe.Pointer) {
				c.syncOverviewScene()
			})
			listeners.unmap = AddListener(unsafe.Pointer(&surface.events.unmap), func(unsafe.Pointer) {
				c.syncOverviewScene()
			})
		}
		listeners.newSubsurface = AddListener(unsafe.Pointer(&surface.events.new_subsurface), func(unsafe.Pointer) {
			c.syncOverviewSurfaceListeners(view)
			c.syncOverviewScene()
		})
		listeners.destroy = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
			c.dropOverviewSurfaceListeners(view, key)
			c.syncOverviewScene()
		})
		view.overviewSurfaces[key] = listeners
		c.keep(listeners.commit, listeners.unmap, listeners.newSubsurface, listeners.destroy)
	}
}

func (c *Compositor) dropOverviewSurfaceListeners(view *toplevelView, only unsafe.Pointer) {
	if view == nil {
		return
	}
	for surface, listeners := range view.overviewSurfaces {
		if only != nil && surface != only {
			continue
		}
		if listeners.commit != nil {
			c.drop(listeners.commit)
		}
		if listeners.unmap != nil {
			c.drop(listeners.unmap)
		}
		if listeners.newSubsurface != nil {
			c.drop(listeners.newSubsurface)
		}
		if listeners.destroy != nil {
			c.drop(listeners.destroy)
		}
		delete(view.overviewSurfaces, surface)
	}
}
