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

// sysc_project_view_children projects a view's buffers relative to the
// view tree's origin, walking its children so that a view hidden on another
// workspace, whose own node is disabled, still shows.
static int sysc_project_view_children(struct wlr_scene_tree *tree,
		struct wlr_scene_tree *source, int target_x, int target_y, double zoom) {
	struct sysc_overview_project_context context = {
		.tree = tree, .target_x = target_x, .target_y = target_y, .zoom = zoom,
	};
	struct wlr_scene_node *child;
	wl_list_for_each(child, &source->children, link) {
		wlr_scene_node_for_each_buffer(child, sysc_overview_project_buffer, &context);
	}
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
	"image"
	"math"
	"slices"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/ui"
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
	// layer is the background or bottom layer surface this copy projects;
	// nil for a window.
	layer *layerSurface
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
	if c.model == nil {
		return false
	}
	from, velocity, _ := c.animationValue(animationKey{property: "overview"},
		c.model.OverviewProgress(), time.Now())
	if !c.model.ToggleOverview() {
		return false
	}
	if c.model.OverviewOpen() {
		c.endTouchesForOverview()
	}
	to := c.model.OverviewProgress()
	c.model.SetOverviewAnimationProgress(from)
	c.animateOverviewProgress(from, to, velocity)
	c.syncOverviewScene()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) OpenOverview() bool {
	if c.model == nil {
		return false
	}
	from, velocity, _ := c.animationValue(animationKey{property: "overview"},
		c.model.OverviewProgress(), time.Now())
	if !c.model.OpenOverview() {
		return false
	}
	c.endTouchesForOverview()
	to := c.model.OverviewProgress()
	c.model.SetOverviewAnimationProgress(from)
	c.animateOverviewProgress(from, to, velocity)
	c.syncOverviewScene()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) CloseOverview() bool {
	if c.model == nil {
		return false
	}
	from, velocity, _ := c.animationValue(animationKey{property: "overview"},
		c.model.OverviewProgress(), time.Now())
	if !c.model.CloseOverview() {
		return false
	}
	to := c.model.OverviewProgress()
	c.model.SetOverviewAnimationProgress(from)
	c.animateOverviewProgress(from, to, velocity)
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
	for node := range c.overviewWorkspaceNodes {
		sceneFXNodeDestroy(node)
	}
	c.overviewWorkspaceNodes = nil
	c.overviewProjections = nil
	c.overviewBackdrops = nil
}

func (c *Compositor) syncOverviewScene() {
	if c.overviewTree == nil || c.model == nil {
		return
	}
	if bool(c.overviewTree.node.enabled) || c.model.OverviewProgress() > 0 {
		defer c.damageCaptureSources()
	}
	c.clearOverviewScene()
	progress := c.model.OverviewProgress()
	if progress > 0 {
		C.wlr_scene_node_place_below(&c.overviewTree.node, &c.layerTrees[layerTop].node)
	} else {
		// The same layer copies form the switch backdrop beneath real windows
		// and their decorations. The overview also projects the windows.
		C.wlr_scene_node_place_below(&c.overviewTree.node, &c.windowShadowTree.node)
	}

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
		if progress <= 0 {
			if _, switching := c.model.WorkspaceRenderOffset(name); !switching {
				continue
			}
			bounds, exists := c.outputLayoutBounds(name)
			if !exists {
				continue
			}
			frame, ok = c.model.WorkspaceFrame(name, bounds, 1)
		}
		if !ok {
			continue
		}
		frames = append(frames, outputFrame{name: name, frame: frame})
		bounds, ok := c.outputLayoutBounds(name)
		if !ok || bounds.Width <= 0 || bounds.Height <= 0 {
			continue
		}
		color := c.overviewConfig.BackdropColor
		if section := c.outputConfigByName(name); section != nil && section.BackdropColor != nil {
			color = *section.BackdropColor
		}
		opacity := progress
		if progress <= 0 {
			// Cover the stationary originals, including a transparent backdrop's
			// black output clear colour, before drawing the moving copies.
			opacity = 1
			color.R, color.G, color.B, color.A = color.R*color.A, color.G*color.A, color.B*color.A, 1
		}
		var rgba [4]C.float
		rgba[0] = C.float(color.R * color.A * opacity)
		rgba[1] = C.float(color.G * color.A * opacity)
		rgba[2] = C.float(color.B * color.A * opacity)
		rgba[3] = C.float(color.A * opacity)
		node := C.wlr_scene_rect_create(c.overviewTree,
			C.int(bounds.Width), C.int(bounds.Height), &rgba[0])
		if node == nil {
			continue
		}
		C.wlr_scene_node_set_position(&node.node, C.int(bounds.X), C.int(bounds.Y))
		c.overviewBackdrops = append(c.overviewBackdrops, overviewBackdrop{
			output: name, Bounds: bounds, Color: color, node: node,
		})
		// Niri draws place-within-backdrop surfaces unzoomed over the backdrop.
		for _, layer := range c.layerOrder {
			if layer.outputName == name && layer.withinBackdrop() {
				c.projectOverviewLayer(layer, name, 0, layout.Bounds{}, 0, 0, 1)
			}
		}
	}
	C.wlr_scene_node_set_enabled(&c.overviewTree.node, C.bool(len(frames) > 0))
	if len(frames) > 0 {
		c.overviewWorkspaceNodes = make(map[unsafe.Pointer]string)
	}
	// Normalize shadow dimensions to a 1080-pixel output before zooming.
	if progress > 0 && c.overviewConfig.WorkspaceShadow.On {
		for _, current := range frames {
			output, ok := c.outputLayoutBounds(current.name)
			if !ok {
				continue
			}
			shadow := c.overviewConfig.WorkspaceShadow
			factor := float64(output.Height) / 1080 * current.frame.Zoom
			shadow.Softness *= factor
			shadow.Spread *= factor
			shadow.OffsetX *= factor
			shadow.OffsetY *= factor
			shadow.Color.A *= progress
			style := makeShadowRenderStyle(shadow, config.CornerRadius{}, true)
			inset := style.inset
			x, y := int(math.Round(shadow.OffsetX)), int(math.Round(shadow.OffsetY))
			for _, workspace := range current.frame.Workspaces {
				bounds := workspace.Bounds
				width, height := bounds.Width+2*inset, bounds.Height+2*inset
				if width <= 0 || height <= 0 {
					continue
				}
				node := sceneFXCreateShadow(unsafe.Pointer(c.overviewTree), width, height, style.radius, style.blurSigma, style.color)
				if node != nil {
					sceneFXNodeSetPosition(node, bounds.X-inset+x, bounds.Y-inset+y)
					sceneFXShadowClip(node, inset-x, inset-y, bounds.Width, bounds.Height, [4]uint16{})
					c.overviewWorkspaceNodes[node] = current.name
				}
			}
		}
	}

	views := make(map[uint64]*toplevelView, len(c.views))
	for _, view := range c.views {
		if view != nil && view.mapped && view.tree != nil && view.toplevel != nil &&
			bool(view.toplevel.base.surface.mapped) {
			views[view.id] = view
		}
	}
	workspaceNames := workspaceNamesByID(c.model.Snapshot().Workspaces)
	for _, current := range frames {
		for _, workspace := range current.frame.Workspaces {
			bounds := workspace.Bounds
			left, top := max(bounds.X, current.frame.Output.X), max(bounds.Y, current.frame.Output.Y)
			right := min(bounds.X+bounds.Width, current.frame.Output.X+current.frame.Output.Width)
			bottom := min(bounds.Y+bounds.Height, current.frame.Output.Y+current.frame.Output.Height)
			if right <= left || bottom <= top {
				continue
			}
			color := c.workspaceBackgroundColor(current.name, workspaceNames[workspace.ID])
			if color.A > 0 {
				node := sceneFXCreateRect(unsafe.Pointer(c.overviewTree), right-left, bottom-top, premultipliedColor(color))
				if node != nil {
					sceneFXRectIgnoreInput(node)
					sceneFXNodeSetPosition(node, left, top)
					c.overviewWorkspaceNodes[node] = current.name
				}
			}
			// Background and bottom layers move and zoom with each workspace,
			// below its windows.
			for _, kind := range []int{layerBackground, layerBottom} {
				for _, layer := range c.layerOrder {
					if layer.outputName == current.name && layer.layer == kind && !layer.withinBackdrop() {
						c.projectOverviewLayer(layer, current.name, workspace.ID, workspace.Bounds,
							current.frame.Output.X, current.frame.Output.Y, current.frame.Zoom)
					}
				}
			}
			if progress <= 0 {
				continue
			}
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
				c.rememberCaptureNodes(view)
				count := int(C.sysc_overview_project_buffers(tree, view.tree,
					C.int(current.frame.Output.X), C.int(current.frame.Output.Y),
					C.int(workspace.Bounds.X), C.int(workspace.Bounds.Y), C.double(current.frame.Zoom),
					&sourceFirst, &sourceLast))
				if count == 0 {
					C.wlr_scene_node_destroy(&tree.node)
					continue
				}
				c.rememberCaptureTree(unsafe.Pointer(tree), view.captureIdentity)
				c.overviewProjections = append(c.overviewProjections, overviewProjection{
					Output: current.name, WorkspaceID: workspace.ID,
					WindowID: window.ID, Bounds: window.Bounds, BufferCount: count,
					SourceFirst: uintptr(sourceFirst), SourceLast: uintptr(sourceLast), tree: tree,
				})
			}
		}
	}
}

// projectOverviewLayer copies layer's mapped buffers into the overview,
// mapping the output origin (sourceX, sourceY) to the origin of bounds.
func (c *Compositor) projectOverviewLayer(layer *layerSurface, output string, workspace uint64,
	bounds layout.Bounds, sourceX, sourceY int, zoom float64) {
	if layer.tree == nil || !bool(layer.surface.surface.mapped) {
		return
	}
	tree := C.wlr_scene_tree_create(c.overviewTree)
	if tree == nil {
		return
	}
	var sourceFirst, sourceLast C.uintptr_t
	count := int(C.sysc_overview_project_buffers(tree, layer.tree, C.int(sourceX), C.int(sourceY),
		C.int(bounds.X), C.int(bounds.Y), C.double(zoom), &sourceFirst, &sourceLast))
	if count == 0 {
		C.wlr_scene_node_destroy(&tree.node)
		return
	}
	if workspace != 0 {
		if outputBounds, ok := c.outputLayoutBounds(output); ok {
			left, top := max(bounds.X, outputBounds.X), max(bounds.Y, outputBounds.Y)
			right := min(bounds.X+bounds.Width, outputBounds.X+outputBounds.Width)
			bottom := min(bounds.Y+bounds.Height, outputBounds.Y+outputBounds.Height)
			if right <= left || bottom <= top {
				C.wlr_scene_node_destroy(&tree.node)
				return
			}
			sceneFXClipTree(unsafe.Pointer(tree), rect{left, top, right - left, bottom - top}, [4]uint16{})
		}
	}
	c.overviewProjections = append(c.overviewProjections, overviewProjection{
		Output: output, WorkspaceID: workspace, Bounds: bounds, BufferCount: count,
		SourceFirst: uintptr(sourceFirst), SourceLast: uintptr(sourceLast), tree: tree, layer: layer,
	})
}

// projectViewPreview draws view's buffers under parent with its window
// geometry's origin at (x, y), scaled by zoom. It returns nil when the view
// has no buffer.
func projectViewPreview(parent *C.struct_wlr_scene_tree, view *toplevelView, x, y int, zoom float64) *C.struct_wlr_scene_tree {
	tree := C.wlr_scene_tree_create(parent)
	if tree == nil {
		return nil
	}
	if C.sysc_project_view_children(tree, view.tree, C.int(x), C.int(y), C.double(zoom)) == 0 {
		C.wlr_scene_node_destroy(&tree.node)
		return nil
	}
	return tree
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
	if c.insertHintImage != nil {
		sceneFXNodeDestroy(c.insertHintImage)
		c.insertHintImage = nil
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
	if gradient := c.layoutOptions.InsertHint.Gradient; gradient != nil {
		box := image.Rect(bounds.X, bounds.Y, bounds.X+bounds.Width, bounds.Y+bounds.Height)
		area := box
		for _, workspace := range c.model.Snapshot().Workspaces {
			if workspace.ID != drag.Target.WorkspaceID {
				continue
			}
			if output, ok := c.outputLayoutBounds(workspace.Output); ok {
				view := image.Rect(output.X, output.Y, output.X+output.Width, output.Y+output.Height)
				box = box.Intersect(view)
				if gradient.RelativeTo == "workspace-view" {
					area = view
				}
			}
		}
		if _, err := captureAllocation(box.Dx(), box.Dy(), box.Dx()*4); err == nil {
			// ponytail: O(visible hint pixels) per layout update; cache by gradient
			// and bounds if drag profiling shows this rasterization is hot.
			node := sceneImage(c.insertHintTree, ui.GradientImage(box, area, *gradient))
			c.insertHintImage = unsafe.Pointer(node)
			if node != nil {
				sceneImagePlace(node, box, 1)
				sceneImageIgnoreInput(node)
			}
		}
		C.wlr_scene_node_set_enabled(&c.insertHintTree.node, C.bool(c.insertHintImage != nil))
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
				c.rememberCaptureNodes(view)
				c.damageCaptureSources(view)
				c.syncOverviewScene()
				c.syncSwitcher()
			})
			listeners.unmap = AddListener(unsafe.Pointer(&surface.events.unmap), func(unsafe.Pointer) {
				c.damageCaptureSources(view)
				c.syncOverviewScene()
				c.syncSwitcher()
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
