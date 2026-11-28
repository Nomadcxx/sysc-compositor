package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#cgo LDFLAGS: -lm

#include <math.h>
#include <stdint.h>
#include <stdlib.h>
#include <time.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/util/box.h>
#include <wlr/util/transform.h>

struct sysc_overview_project_context {
	struct wlr_scene_tree *tree;
	int source_x, source_y;
	int target_x, target_y;
	double zoom;
	// clip crops every copy to a box in layout coordinates, as
	// sysc_scene_clip_tree does with square corners.
	bool clip;
	int clip_x, clip_y, clip_width, clip_height;
	// reuse is the next copy already in tree, in stacking order. The walk
	// visits the source's buffers in the order it created them.
	struct wlr_scene_node *reuse;
	int count, created;
	uintptr_t first_buffer, last_buffer;
};

// sysc_overview_next_copy returns the copy after node in tree, or the first
// one when node is NULL. Copies are the tree's only buffers.
static struct wlr_scene_node *sysc_overview_next_copy(struct wlr_scene_tree *tree,
		struct wlr_scene_node *node) {
	struct wl_list *link = node == NULL ? tree->children.next : node->link.next;
	for (; link != &tree->children; link = link->next) {
		struct wlr_scene_node *next = wl_container_of(link, next, link);
		if (next->type == WLR_SCENE_NODE_BUFFER) {
			return next;
		}
	}
	return NULL;
}

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
	struct wlr_scene_buffer *copy;
	if (context->reuse != NULL) {
		copy = wlr_scene_buffer_from_node(context->reuse);
		context->reuse = sysc_overview_next_copy(context->tree, context->reuse);
		// Setting the same buffer again would drop its texture and damage it.
		if (copy->buffer != source->buffer) {
			wlr_scene_buffer_set_buffer(copy, source->buffer);
		}
	} else {
		copy = wlr_scene_buffer_create(context->tree, source->buffer);
		if (copy == NULL) {
			return;
		}
		context->created++;
	}
	int target_x = context->target_x + (int)lround((x - context->source_x) * context->zoom);
	int target_y = context->target_y + (int)lround((y - context->source_y) * context->zoom);
	struct wlr_fbox src = source->src_box;
	bool has_src = src.width > 0 && src.height > 0;
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
	bool sized = width > 0 && height > 0;
	if (sized) {
		width = (int)lround(width * context->zoom);
		height = (int)lround(height * context->zoom);
		width = width > 0 ? width : 1;
		height = height > 0 ? height : 1;
	}
	bool enabled = true;
	if (context->clip && sized) {
		int left = target_x > context->clip_x ? target_x : context->clip_x;
		int top = target_y > context->clip_y ? target_y : context->clip_y;
		int clip_right = context->clip_x + context->clip_width;
		int clip_bottom = context->clip_y + context->clip_height;
		int right = target_x + width < clip_right ? target_x + width : clip_right;
		int bottom = target_y + height < clip_bottom ? target_y + height : clip_bottom;
		if (right <= left || bottom <= top) {
			enabled = false;
		} else if (left != target_x || top != target_y ||
				right != target_x + width || bottom != target_y + height) {
			if (!has_src) {
				src = (struct wlr_fbox){ .width = source->buffer->width, .height = source->buffer->height };
				has_src = true;
			}
			struct wlr_fbox uv = {
				.x = (double)(left - target_x) / width, .y = (double)(top - target_y) / height,
				.width = (double)(right - left) / width, .height = (double)(bottom - top) / height,
			};
			wlr_fbox_transform(&uv, &uv, wlr_output_transform_invert(source->transform), 1, 1);
			src = (struct wlr_fbox){
				.x = src.x + uv.x * src.width, .y = src.y + uv.y * src.height,
				.width = uv.width * src.width, .height = uv.height * src.height,
			};
			target_x = left;
			target_y = top;
			width = right - left;
			height = bottom - top;
		}
	}
	// Each setter returns early when nothing changed, so a copy that is
	// reused unchanged takes no damage.
	wlr_scene_node_set_enabled(&copy->node, enabled);
	wlr_scene_node_set_position(&copy->node, target_x, target_y);
	wlr_scene_buffer_set_transform(copy, source->transform);
	wlr_scene_buffer_set_opacity(copy, source->opacity);
	wlr_scene_buffer_set_filter_mode(copy, source->filter_mode);
	wlr_scene_buffer_set_source_box(copy, has_src ? &src : NULL);
	if (sized) {
		wlr_scene_buffer_set_dest_size(copy, width, height);
	}
	context->count++;
}

// sysc_overview_frame_done answers the frame callbacks of every surface in a
// projected source tree. It walks disabled subtrees too: the windows of an
// inactive workspace sit in a disabled tree.
static void sysc_overview_frame_done(struct wlr_scene_node *node, const struct timespec *now) {
	if (node->type == WLR_SCENE_NODE_TREE) {
		struct wlr_scene_node *child;
		wl_list_for_each(child, &wlr_scene_tree_from_node(node)->children, link) {
			sysc_overview_frame_done(child, now);
		}
		return;
	}
	if (node->type != WLR_SCENE_NODE_BUFFER) {
		return;
	}
	struct wlr_scene_surface *scene_surface =
		wlr_scene_surface_try_from_buffer(wlr_scene_buffer_from_node(node));
	if (scene_surface != NULL && scene_surface->surface->mapped) {
		wlr_surface_send_frame_done(scene_surface->surface, now);
	}
}

static void sysc_overview_send_frame_done(struct wlr_scene_tree **sources, size_t count) {
	struct timespec now;
	clock_gettime(CLOCK_MONOTONIC, &now);
	for (size_t i = 0; i < count; i++) {
		sysc_overview_frame_done(&sources[i]->node, &now);
	}
}

// sysc_overview_project_buffers brings tree's copies of source's buffers up
// to date. It reuses the copies tree already holds, in order, and creates or
// destroys only the difference, so a commit that changes buffers alone
// swaps them in place. It returns the number of copies; *created counts the
// new ones.
static int sysc_overview_project_buffers(struct wlr_scene_tree *tree,
		struct wlr_scene_tree *source, int source_x, int source_y,
		int target_x, int target_y, double zoom, bool clip,
		int clip_x, int clip_y, int clip_width, int clip_height,
		int *created, uintptr_t *first_buffer, uintptr_t *last_buffer) {
	struct sysc_overview_project_context context = {
		.tree = tree, .source_x = source_x, .source_y = source_y,
		.target_x = target_x, .target_y = target_y, .zoom = zoom,
		.clip = clip, .clip_x = clip_x, .clip_y = clip_y,
		.clip_width = clip_width, .clip_height = clip_height,
	};
	context.reuse = sysc_overview_next_copy(tree, NULL);
	wlr_scene_node_for_each_buffer(&source->node, sysc_overview_project_buffer, &context);
	while (context.reuse != NULL) {
		struct wlr_scene_node *stale = context.reuse;
		context.reuse = sysc_overview_next_copy(tree, stale);
		wlr_scene_node_destroy(stale);
	}
	*created = context.created;
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
	// source is the window or layer surface tree the copy was taken from.
	source *C.struct_wlr_scene_tree
	// layer is the background or bottom layer surface this copy projects;
	// nil for a window.
	layer *layerSurface
	// view is the window this copy projects; nil for a layer surface.
	view *toplevelView
	// place maps the source onto the overview; a commit reprojects with it.
	place overviewPlacement
}

// overviewPlacement maps a source's layout origin (sourceX, sourceY) to
// (targetX, targetY), scaled by zoom, cropped to clip unless it is empty.
type overviewPlacement struct {
	sourceX, sourceY, targetX, targetY int
	zoom                               float64
	clip                               rect
}

// project brings tree's copies of source's buffers up to date, reusing the
// copies it holds. It returns the number of copies and of new ones.
func (p overviewPlacement) project(tree, source *C.struct_wlr_scene_tree) (count, created int, first, last uintptr) {
	var sourceFirst, sourceLast C.uintptr_t
	var made C.int
	count = int(C.sysc_overview_project_buffers(tree, source, C.int(p.sourceX), C.int(p.sourceY),
		C.int(p.targetX), C.int(p.targetY), C.double(p.zoom), C.bool(p.clip.w > 0 && p.clip.h > 0),
		C.int(p.clip.x), C.int(p.clip.y), C.int(p.clip.w), C.int(p.clip.h),
		&made, &sourceFirst, &sourceLast))
	return count, int(made), uintptr(sourceFirst), uintptr(sourceLast)
}

// refreshOverviewSource reprojects the copies of source, a window or layer
// surface tree, after a commit that changed its buffers but not the layout.
// It reports false when a full rebuild is needed instead.
func (c *Compositor) refreshOverviewSource(source *C.struct_wlr_scene_tree) bool {
	refreshed := false
	for i := range c.overviewProjections {
		projection := &c.overviewProjections[i]
		if projection.source != source {
			continue
		}
		count, created, first, last := projection.place.project(projection.tree, source)
		if count == 0 {
			return false
		}
		projection.BufferCount, projection.SourceFirst, projection.SourceLast = count, first, last
		if created > 0 && projection.view != nil {
			c.rememberCaptureTree(unsafe.Pointer(projection.tree), projection.view.captureIdentity)
		}
		refreshed = true
	}
	if refreshed && bool(c.overviewTree.node.enabled) {
		c.damageCaptureSources()
	}
	return true
}

// sendOverviewFrameDone answers, after output repaints, the frame callbacks
// of the windows and layer surfaces the overview or a workspace switch
// projects onto it. The copies cover their originals, so the scene gives
// those no output and they would fall back to the once-a-second timer; Niri
// renders them live.
func (c *Compositor) sendOverviewFrameDone(output string) {
	var sources []*C.struct_wlr_scene_tree
	for _, projection := range c.overviewProjections {
		if projection.Output == output && projection.source != nil && !slices.Contains(sources, projection.source) {
			sources = append(sources, projection.source)
		}
	}
	if len(sources) == 0 {
		return
	}
	C.sysc_overview_send_frame_done(&sources[0], C.size_t(len(sources)))
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

// overviewNodeKey names a workspace's background colour or shadow in the
// overview.
type overviewNodeKey struct {
	output    string
	workspace uint64
	shadow    bool
}

// overviewProjectionKey names a projection: the same source on the same
// workspace is the same projection from one rebuild to the next.
type overviewProjectionKey struct {
	source    *C.struct_wlr_scene_tree
	output    string
	workspace uint64
}

func (p overviewProjection) key() overviewProjectionKey {
	return overviewProjectionKey{source: p.source, output: p.Output, workspace: p.WorkspaceID}
}

// overviewRebuild hands the previous overview scene to syncOverviewScene.
// Each node the new scene still needs is taken from it and updated in place,
// so an animation tick that only moves and zooms the scene creates nothing
// and its copies keep their textures. finish destroys what is left.
type overviewRebuild struct {
	projections map[overviewProjectionKey]overviewProjection
	backdrops   map[string]*C.struct_wlr_scene_rect
	nodes       map[overviewNodeKey]unsafe.Pointer
	// order lists the overview tree's children in drawing order, bottom first.
	order []*C.struct_wlr_scene_node
}

func (c *Compositor) takeOverviewScene() *overviewRebuild {
	r := &overviewRebuild{
		projections: make(map[overviewProjectionKey]overviewProjection, len(c.overviewProjections)),
		backdrops:   make(map[string]*C.struct_wlr_scene_rect, len(c.overviewBackdrops)),
		nodes:       c.overviewWorkspaceNodes,
	}
	for _, projection := range c.overviewProjections {
		r.projections[projection.key()] = projection
	}
	for _, backdrop := range c.overviewBackdrops {
		r.backdrops[backdrop.output] = backdrop.node
	}
	c.overviewProjections, c.overviewBackdrops, c.overviewWorkspaceNodes = nil, nil, nil
	return r
}

// finish destroys the nodes the new scene did not take and stacks the rest
// in drawing order. A node already in place is not moved.
func (r *overviewRebuild) finish() {
	for _, projection := range r.projections {
		C.wlr_scene_node_destroy(&projection.tree.node)
	}
	for _, node := range r.backdrops {
		C.wlr_scene_node_destroy(&node.node)
	}
	for _, node := range r.nodes {
		sceneFXNodeDestroy(node)
	}
	for i, node := range r.order {
		if i == 0 {
			C.wlr_scene_node_lower_to_bottom(node)
		} else {
			C.wlr_scene_node_place_above(node, r.order[i-1])
		}
	}
}

// addOverviewProjection projects p.source into the overview with p.place,
// reusing the previous scene's projection of the same source if it had one.
func (c *Compositor) addOverviewProjection(r *overviewRebuild, p overviewProjection) {
	if previous, ok := r.projections[p.key()]; ok {
		delete(r.projections, p.key())
		p.tree = previous.tree
	} else if p.tree = C.wlr_scene_tree_create(c.overviewTree); p.tree == nil {
		return
	}
	count, _, first, last := p.place.project(p.tree, p.source)
	if count == 0 {
		C.wlr_scene_node_destroy(&p.tree.node)
		return
	}
	if p.view != nil {
		c.rememberCaptureTree(unsafe.Pointer(p.tree), p.view.captureIdentity)
	}
	p.BufferCount, p.SourceFirst, p.SourceLast = count, first, last
	c.overviewProjections = append(c.overviewProjections, p)
	r.order = append(r.order, &p.tree.node)
}

func (c *Compositor) syncOverviewScene() {
	if c.overviewTree == nil || c.model == nil {
		return
	}
	if bool(c.overviewTree.node.enabled) || c.model.OverviewProgress() > 0 {
		defer c.damageCaptureSources()
	}
	r := c.takeOverviewScene()
	defer r.finish()
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
		node := r.backdrops[name]
		delete(r.backdrops, name)
		if node == nil {
			node = C.wlr_scene_rect_create(c.overviewTree, C.int(bounds.Width), C.int(bounds.Height), &rgba[0])
			if node == nil {
				continue
			}
		} else {
			C.wlr_scene_rect_set_size(node, C.int(bounds.Width), C.int(bounds.Height))
			C.wlr_scene_rect_set_color(node, &rgba[0])
		}
		C.wlr_scene_node_set_position(&node.node, C.int(bounds.X), C.int(bounds.Y))
		r.order = append(r.order, &node.node)
		c.overviewBackdrops = append(c.overviewBackdrops, overviewBackdrop{
			output: name, Bounds: bounds, Color: color, node: node,
		})
		// Niri draws place-within-backdrop surfaces unzoomed over the backdrop.
		for _, layer := range c.layerOrder {
			if layer.outputName == name && layer.withinBackdrop() {
				c.projectOverviewLayer(r, layer, name, 0, layout.Bounds{}, 0, 0, 1)
			}
		}
	}
	C.wlr_scene_node_set_enabled(&c.overviewTree.node, C.bool(len(frames) > 0))
	if len(frames) > 0 {
		c.overviewWorkspaceNodes = make(map[overviewNodeKey]unsafe.Pointer)
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
				key := overviewNodeKey{output: current.name, workspace: workspace.ID, shadow: true}
				node := r.nodes[key]
				delete(r.nodes, key)
				if node == nil {
					node = sceneFXCreateShadow(unsafe.Pointer(c.overviewTree), width, height, style.radius, style.blurSigma, style.color)
				}
				if node != nil {
					sceneFXShadowUpdate(node, bounds.X-inset+x, bounds.Y-inset+y, width, height, style.radius, style.blurSigma, style.color)
					sceneFXShadowClip(node, inset-x, inset-y, bounds.Width, bounds.Height, [4]uint16{})
					c.overviewWorkspaceNodes[key] = node
					r.order = append(r.order, (*C.struct_wlr_scene_node)(node))
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
				key := overviewNodeKey{output: current.name, workspace: workspace.ID}
				node := r.nodes[key]
				delete(r.nodes, key)
				if node == nil {
					node = sceneFXCreateRect(unsafe.Pointer(c.overviewTree), right-left, bottom-top, premultipliedColor(color))
					sceneFXRectIgnoreInput(node)
				}
				if node != nil {
					sceneFXRectUpdateFill(node, left, top, right-left, bottom-top, [4]uint16{}, premultipliedColor(color))
					c.overviewWorkspaceNodes[key] = node
					r.order = append(r.order, (*C.struct_wlr_scene_node)(node))
				}
			}
			// Background and bottom layers move and zoom with each workspace,
			// below its windows.
			for _, kind := range []int{layerBackground, layerBottom} {
				for _, layer := range c.layerOrder {
					if layer.outputName == current.name && layer.layer == kind && !layer.withinBackdrop() {
						c.projectOverviewLayer(r, layer, current.name, workspace.ID, workspace.Bounds,
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
				c.rememberCaptureNodes(view)
				c.addOverviewProjection(r, overviewProjection{
					Output: current.name, WorkspaceID: workspace.ID,
					WindowID: window.ID, Bounds: window.Bounds, source: view.tree, view: view,
					place: overviewPlacement{sourceX: current.frame.Output.X, sourceY: current.frame.Output.Y,
						targetX: workspace.Bounds.X, targetY: workspace.Bounds.Y, zoom: current.frame.Zoom},
				})
			}
		}
	}
}

// projectOverviewLayer copies layer's mapped buffers into the overview,
// mapping the output origin (sourceX, sourceY) to the origin of bounds.
func (c *Compositor) projectOverviewLayer(r *overviewRebuild, layer *layerSurface, output string, workspace uint64,
	bounds layout.Bounds, sourceX, sourceY int, zoom float64) {
	if layer.tree == nil || !bool(layer.surface.surface.mapped) {
		return
	}
	place := overviewPlacement{sourceX: sourceX, sourceY: sourceY, targetX: bounds.X, targetY: bounds.Y, zoom: zoom}
	if workspace != 0 {
		if outputBounds, ok := c.outputLayoutBounds(output); ok {
			left, top := max(bounds.X, outputBounds.X), max(bounds.Y, outputBounds.Y)
			right := min(bounds.X+bounds.Width, outputBounds.X+outputBounds.Width)
			bottom := min(bounds.Y+bounds.Height, outputBounds.Y+outputBounds.Height)
			if right <= left || bottom <= top {
				return
			}
			place.clip = rect{left, top, right - left, bottom - top}
		}
	}
	c.addOverviewProjection(r, overviewProjection{
		Output: output, WorkspaceID: workspace, Bounds: bounds, source: layer.tree, layer: layer, place: place,
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
				if !c.refreshOverviewSource(view.tree) {
					c.syncOverviewScene()
				}
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
