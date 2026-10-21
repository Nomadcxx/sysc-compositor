package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/interfaces/wlr_pointer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_subcompositor.h>

static bool sysc_scene_node_before(struct wlr_scene_node *first,
		struct wlr_scene_node *second) {
	if (first == NULL || second == NULL || first->parent == NULL ||
			first->parent != second->parent) {
		return false;
	}
	struct wlr_scene_node *node;
	wl_list_for_each(node, &first->parent->children, link) {
		if (node == first) {
			return true;
		}
		if (node == second) {
			return false;
		}
	}
	return false;
}

struct sysc_scene_buffer_size {
	int width, height;
	bool found;
};

static void sysc_scene_buffer_size_one(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	struct sysc_scene_buffer_size *size = data;
	if (size->found || buffer->buffer == NULL) {
		return;
	}
	int width = buffer->dst_width > 0 ? buffer->dst_width : buffer->buffer->width;
	int height = buffer->dst_height > 0 ? buffer->dst_height : buffer->buffer->height;
	if (buffer->dst_width == 0 && buffer->dst_height == 0 &&
			(buffer->transform == WL_OUTPUT_TRANSFORM_90 ||
			 buffer->transform == WL_OUTPUT_TRANSFORM_270 ||
			 buffer->transform == WL_OUTPUT_TRANSFORM_FLIPPED_90 ||
			 buffer->transform == WL_OUTPUT_TRANSFORM_FLIPPED_270)) {
		int swap = width;
		width = height;
		height = swap;
	}
	size->width = width;
	size->height = height;
	size->found = true;
}

static bool sysc_scene_tree_first_buffer_size(struct wlr_scene_tree *tree,
		int *width, int *height) {
	struct sysc_scene_buffer_size size = {0};
	if (tree == NULL) {
		return false;
	}
	wlr_scene_node_for_each_buffer(&tree->node, sysc_scene_buffer_size_one, &size);
	*width = size.width;
	*height = size.height;
	return size.found;
}

static const struct wlr_pointer_impl sysc_fake_pointer_impl = {
	.name = "sysc-fake-pointer",
};

static struct wlr_pointer *sysc_fake_pointer_create(void) {
	struct wlr_pointer *pointer = calloc(1, sizeof(*pointer));
	if (pointer != NULL) {
		wlr_pointer_init(pointer, &sysc_fake_pointer_impl, "sysc-fake-pointer");
	}
	return pointer;
}

static void sysc_fake_pointer_destroy(struct wlr_pointer *pointer) {
	wlr_pointer_finish(pointer);
	free(pointer);
}

// An absolute position in [0, 1] over the whole layout, as a tablet or a
// nested backend reports one, then the frame that ends the event group.
static void sysc_fake_pointer_warp(struct wlr_pointer *pointer, uint32_t time, double x, double y) {
	struct wlr_pointer_motion_absolute_event event = {
		.pointer = pointer,
		.time_msec = time,
		.x = x,
		.y = y,
	};
	wl_signal_emit_mutable(&pointer->events.motion_absolute, &event);
	wl_signal_emit_mutable(&pointer->events.frame, pointer);
}

static void sysc_fake_pointer_button(struct wlr_pointer *pointer, uint32_t time,
		uint32_t button, bool pressed) {
	struct wlr_pointer_button_event event = {
		.pointer = pointer,
		.time_msec = time,
		.button = button,
		.state = pressed ? WL_POINTER_BUTTON_STATE_PRESSED : WL_POINTER_BUTTON_STATE_RELEASED,
	};
	wlr_pointer_notify_button(pointer, &event);
	wl_signal_emit_mutable(&pointer->events.frame, pointer);
}

static void sysc_fake_pointer_swipe_begin(struct wlr_pointer *pointer, uint32_t time,
		uint32_t fingers) {
	struct wlr_pointer_swipe_begin_event event = {
		.pointer = pointer, .time_msec = time, .fingers = fingers,
	};
	wl_signal_emit_mutable(&pointer->events.swipe_begin, &event);
}

static void sysc_fake_pointer_swipe_update(struct wlr_pointer *pointer, uint32_t time,
		uint32_t fingers, double dx, double dy) {
	struct wlr_pointer_swipe_update_event event = {
		.pointer = pointer, .time_msec = time, .fingers = fingers, .dx = dx, .dy = dy,
	};
	wl_signal_emit_mutable(&pointer->events.swipe_update, &event);
}

static void sysc_fake_pointer_swipe_end(struct wlr_pointer *pointer, uint32_t time,
		bool cancelled) {
	struct wlr_pointer_swipe_end_event event = {
		.pointer = pointer, .time_msec = time, .cancelled = cancelled,
	};
	wl_signal_emit_mutable(&pointer->events.swipe_end, &event);
}

static struct wlr_drag *sysc_fake_drag_create(void) {
	struct wlr_drag *drag = calloc(1, sizeof(*drag));
	if (drag == NULL) {
		return NULL;
	}
	wl_signal_init(&drag->events.focus);
	wl_signal_init(&drag->events.motion);
	wl_signal_init(&drag->events.drop);
	wl_signal_init(&drag->events.destroy);
	return drag;
}

static void sysc_fake_drag_start(struct wlr_seat *seat, struct wlr_drag *drag) {
	wl_signal_emit_mutable(&seat->events.start_drag, drag);
}

static void sysc_fake_drag_drop(struct wlr_drag *drag) {
	wl_signal_emit_mutable(&drag->events.drop, drag);
}

static void sysc_fake_drag_motion(struct wlr_drag *drag) {
	wl_signal_emit_mutable(&drag->events.motion, drag);
}

static void sysc_fake_drag_destroy(struct wlr_drag *drag) {
	wl_signal_emit_mutable(&drag->events.destroy, drag);
	free(drag);
}

struct sysc_projection_probe {
	struct wlr_scene_tree *parent;
	struct wlr_scene_tree *tree;
	struct wlr_buffer *first;
	struct wlr_buffer *last;
	int count;
	int lock_delta;
	bool destroyed;
};

static void sysc_projection_probe_buffer(struct wlr_scene_buffer *source,
		int x, int y, void *data) {
	struct sysc_projection_probe *probe = data;
	if (source->buffer == NULL) {
		return;
	}
	if (probe->first == NULL) {
		probe->first = source->buffer;
	}
	probe->last = source->buffer;
	int locks = source->buffer->n_locks;
	struct wlr_scene_buffer *copy = wlr_scene_buffer_create(probe->tree, source->buffer);
	if (copy == NULL) {
		return;
	}
	wlr_scene_node_set_position(&copy->node, x, y);
	wlr_scene_buffer_set_transform(copy, source->transform);
	wlr_scene_buffer_set_opacity(copy, source->opacity);
	if (source->src_box.width > 0 && source->src_box.height > 0) {
		wlr_scene_buffer_set_source_box(copy, &source->src_box);
	}
	if (source->dst_width > 0 && source->dst_height > 0) {
		wlr_scene_buffer_set_dest_size(copy, source->dst_width, source->dst_height);
	}
	probe->lock_delta += (int)source->buffer->n_locks - locks;
	probe->count++;
}

static void sysc_projection_probe_refresh(struct sysc_projection_probe *probe,
		struct wlr_scene_tree *source) {
	if (probe->tree != NULL) {
		wlr_scene_node_destroy(&probe->tree->node);
	}
	probe->first = NULL;
	probe->last = NULL;
	probe->count = 0;
	probe->lock_delta = 0;
	probe->tree = wlr_scene_tree_create(probe->parent);
	if (probe->tree == NULL) {
		return;
	}
	wlr_scene_node_set_enabled(&probe->tree->node, false);
	wlr_scene_node_for_each_buffer(&source->node, sysc_projection_probe_buffer, probe);
}

static struct sysc_projection_probe *sysc_projection_probe_create(
		struct wlr_scene_tree *parent, struct wlr_scene_tree *source) {
	struct sysc_projection_probe *probe = calloc(1, sizeof(*probe));
	if (probe == NULL) {
		return NULL;
	}
	probe->parent = parent;
	sysc_projection_probe_refresh(probe, source);
	return probe;
}

static void sysc_projection_probe_destroy(struct sysc_projection_probe *probe) {
	if (probe == NULL) {
		return;
	}
	if (probe->tree != NULL) {
		wlr_scene_node_destroy(&probe->tree->node);
		probe->tree = NULL;
	}
	probe->first = NULL;
	probe->last = NULL;
	probe->count = 0;
	probe->destroyed = true;
}

static void sysc_projection_probe_free(struct sysc_projection_probe *probe) {
	free(probe);
}

static struct wl_signal *sysc_surface_commit_signal(struct wlr_surface *surface) {
	return &surface->events.commit;
}

static struct wl_signal *sysc_surface_destroy_signal(struct wlr_surface *surface) {
	return &surface->events.destroy;
}

static struct wl_signal *sysc_surface_new_subsurface_signal(struct wlr_surface *surface) {
	return &surface->events.new_subsurface;
}

static struct wl_signal *sysc_subsurface_destroy_signal(struct wlr_subsurface *subsurface) {
	return &subsurface->events.destroy;
}

static uintptr_t sysc_surface_current_buffer(struct wlr_surface *surface) {
	return (uintptr_t)surface->current.buffer;
}

static int sysc_surface_subsurface_count(struct wlr_surface *surface) {
	int count = 0;
	struct wl_list *subsurface;
	for (subsurface = surface->current.subsurfaces_below.next;
			subsurface != &surface->current.subsurfaces_below; subsurface = subsurface->next) {
		count++;
	}
	for (subsurface = surface->current.subsurfaces_above.next;
			subsurface != &surface->current.subsurfaces_above; subsurface = subsurface->next) {
		count++;
	}
	return count;
}
*/
import "C"

import "unsafe"

// fakePointer is an in-process pointer attached to the user seat the way a
// backend pointer is, for tests: the nested backend's one pointer is the
// host's, and a test cannot move it.
type fakePointer struct {
	c       *Compositor
	pointer *C.struct_wlr_pointer
}

func sceneNodeBefore(first, second *C.struct_wlr_scene_node) bool {
	return bool(C.sysc_scene_node_before(first, second))
}

func sceneRectColor(rect *C.struct_wlr_scene_rect) [4]float32 {
	return [4]float32{
		float32(rect.color[0]), float32(rect.color[1]),
		float32(rect.color[2]), float32(rect.color[3]),
	}
}

func sceneTreeBufferSize(tree *C.struct_wlr_scene_tree) (int, int, bool) {
	var width, height C.int
	if !bool(C.sysc_scene_tree_first_buffer_size(tree, &width, &height)) {
		return 0, 0, false
	}
	return int(width), int(height), true
}

func (c *Compositor) addFakePointer() *fakePointer {
	// The nested Wayland backend forwards host pointer motion. Let this fake
	// device own cursor position during scripted input tests.
	for device := range c.pointerDevices {
		C.wlr_cursor_detach_input_device(c.cursor, (*C.struct_wlr_input_device)(device))
		delete(c.pointerDevices, device)
	}
	pointer := C.sysc_fake_pointer_create()
	if pointer == nil {
		return nil
	}
	c.handleNewInput(unsafe.Pointer(&pointer.base))
	return &fakePointer{c: c, pointer: pointer}
}

// moveTo moves the drawn pointer to layout coordinates.
func (f *fakePointer) moveTo(x, y float64) {
	box := f.c.layoutBox()
	C.sysc_fake_pointer_warp(f.pointer, nowMsec(),
		C.double((x-float64(box.x))/float64(box.w)), C.double((y-float64(box.y))/float64(box.h)))
}

func (f *fakePointer) button(button uint32, pressed bool) {
	C.sysc_fake_pointer_button(f.pointer, nowMsec(), C.uint32_t(button), C.bool(pressed))
}

func (f *fakePointer) swipeBegin(fingers uint32) {
	f.swipeBeginAt(uint32(nowMsec()), fingers)
}

func (f *fakePointer) swipeUpdate(fingers uint32, dx, dy float64) {
	f.swipeUpdateAt(uint32(nowMsec()), fingers, dx, dy)
}

func (f *fakePointer) swipeEnd(cancelled bool) {
	f.swipeEndAt(uint32(nowMsec()), cancelled)
}

func (f *fakePointer) swipeBeginAt(timeMsec, fingers uint32) {
	C.sysc_fake_pointer_swipe_begin(f.pointer, C.uint32_t(timeMsec), C.uint32_t(fingers))
}

func (f *fakePointer) swipeUpdateAt(timeMsec, fingers uint32, dx, dy float64) {
	C.sysc_fake_pointer_swipe_update(f.pointer, C.uint32_t(timeMsec), C.uint32_t(fingers), C.double(dx), C.double(dy))
}

func (f *fakePointer) swipeEndAt(timeMsec uint32, cancelled bool) {
	C.sysc_fake_pointer_swipe_end(f.pointer, C.uint32_t(timeMsec), C.bool(cancelled))
}

type fakeDrag struct {
	drag *C.struct_wlr_drag
}

func newFakeDrag(c *Compositor) *fakeDrag {
	drag := C.sysc_fake_drag_create()
	if drag == nil {
		return nil
	}
	C.sysc_fake_drag_start(c.seat, drag)
	return &fakeDrag{drag: drag}
}

func (f *fakeDrag) drop() {
	if f != nil && f.drag != nil {
		C.sysc_fake_drag_drop(f.drag)
	}
}

func (f *fakeDrag) motion() {
	if f != nil && f.drag != nil {
		C.sysc_fake_drag_motion(f.drag)
	}
}

func (f *fakeDrag) destroy() {
	if f != nil && f.drag != nil {
		C.sysc_fake_drag_destroy(f.drag)
		f.drag = nil
	}
}

func (f *fakePointer) remove() {
	if f.pointer != nil {
		C.sysc_fake_pointer_destroy(f.pointer)
		f.pointer = nil
	}
}

// projectionProbe exercises wlroots buffer sharing and source-tree traversal
// from nested tests without adding test-only C files to the repository.
type projectionProbe struct {
	probe  *C.struct_sysc_projection_probe
	source *C.struct_wlr_scene_tree
}

func newProjectionProbe(c *Compositor, view *toplevelView) *projectionProbe {
	return newProjectionProbeForTree(c, view.tree)
}

func (c *Compositor) surfaceCreatedSignal() unsafe.Pointer {
	return unsafe.Pointer(&c.comp.events.new_surface)
}

func viewSurface(view *toplevelView) unsafe.Pointer {
	if view == nil || view.toplevel == nil {
		return nil
	}
	return unsafe.Pointer(view.toplevel.base.surface)
}

func newProjectionProbeForTree(c *Compositor, source *C.struct_wlr_scene_tree) *projectionProbe {
	probe := C.sysc_projection_probe_create(&c.scene.tree, source)
	if probe == nil {
		return nil
	}
	return &projectionProbe{probe: probe, source: source}
}

func (p *projectionProbe) refreshSource() {
	if p != nil && p.probe != nil && p.source != nil {
		C.sysc_projection_probe_refresh(p.probe, p.source)
	}
}

func surfaceCommitSignal(surface unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(C.sysc_surface_commit_signal((*C.struct_wlr_surface)(surface)))
}

func surfaceDestroySignal(surface unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(C.sysc_surface_destroy_signal((*C.struct_wlr_surface)(surface)))
}

func surfaceNewSubsurfaceSignal(surface unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(C.sysc_surface_new_subsurface_signal((*C.struct_wlr_surface)(surface)))
}

func subsurfaceDestroySignal(subsurface unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(C.sysc_subsurface_destroy_signal((*C.struct_wlr_subsurface)(subsurface)))
}

func surfaceSubsurfaceState(surface unsafe.Pointer) (buffer uintptr, children int) {
	if surface == nil {
		return 0, 0
	}
	s := (*C.struct_wlr_surface)(surface)
	return uintptr(C.sysc_surface_current_buffer(s)), int(C.sysc_surface_subsurface_count(s))
}

func (p *projectionProbe) stats() (first, last uintptr, buffers, lockDelta int) {
	if p == nil || p.probe == nil {
		return 0, 0, 0, 0
	}
	return uintptr(unsafe.Pointer(p.probe.first)), uintptr(unsafe.Pointer(p.probe.last)),
		int(p.probe.count), int(p.probe.lock_delta)
}

func (p *projectionProbe) destroy() bool {
	if p == nil || p.probe == nil {
		return false
	}
	C.sysc_projection_probe_destroy(p.probe)
	return bool(p.probe.destroyed) && p.probe.tree == nil && p.probe.count == 0
}

func (p *projectionProbe) free() {
	if p != nil && p.probe != nil {
		C.sysc_projection_probe_free(p.probe)
		p.probe = nil
	}
}

// userPointerFocus is the surface the user seat's pointer is on, for tests.
func (c *Compositor) userPointerFocus() unsafe.Pointer {
	return unsafe.Pointer(c.seat.pointer_state.focused_surface)
}
