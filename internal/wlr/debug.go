package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5 glesv2
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#cgo LDFLAGS: -lm
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <GLES2/gl2.h>
#include <wlr/backend/drm.h>
#include <wlr/backend/session.h>
#include <wlr/render/pass.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layer.h>
#include <wlr/util/region.h>
#include <wlr/util/transform.h>
#include <wlr/types/wlr_damage_ring.h>
#include <time.h>
#include <scenefx/types/wlr_scene.h>

static void sysc_debug_scene(struct wlr_scene *scene, bool scanout, bool damage) {
	scene->WLR_PRIVATE.direct_scanout = scanout;
	scene->WLR_PRIVATE.debug_damage_option =
			damage ? WLR_SCENE_DEBUG_DAMAGE_HIGHLIGHT : WLR_SCENE_DEBUG_DAMAGE_NONE;
}
static void sysc_debug_identity(struct wlr_output *output, const char *make, const char *model,
		const char *serial) {
	free(output->make);
	free(output->model);
	free(output->serial);
	output->make = make ? strdup(make) : NULL;
	output->model = model ? strdup(model) : NULL;
	output->serial = serial ? strdup(serial) : NULL;
}
static void sysc_debug_zero_presentation(struct wlr_output_event_present *event) {
	event->when = (struct timespec){0};
	event->flags &= ~WLR_OUTPUT_PRESENT_HW_CLOCK;
}
static bool sysc_debug_reset_output(struct wlr_output *output) {
	struct wlr_output_state old, off;
	wlr_output_state_init(&old);
	wlr_output_state_init(&off);
	wlr_output_state_set_enabled(&old, output->enabled);
	if (output->current_mode) {
		wlr_output_state_set_mode(&old, output->current_mode);
	} else {
		wlr_output_state_set_custom_mode(
				&old, output->width, output->height, output->refresh);
	}
	wlr_output_state_set_enabled(&off, false);
	bool ok = wlr_output_commit_state(output, &off);
	bool restored = wlr_output_commit_state(output, &old);
	wlr_output_state_finish(&off);
	wlr_output_state_finish(&old);
	return ok && restored;
}
struct sysc_debug_leaf {
	struct wlr_scene_buffer *buffer;
	int x, y;
};
// Only the highest intersecting scene leaf can become an overlay. Any effect,
// rectangle or clipped/transparent surface above it is a composition barrier.
static bool sysc_debug_top_leaf(struct wlr_scene_node *node, int x, int y,
		const struct wlr_box *output_box, struct sysc_debug_leaf *found) {
	if (!node->enabled) {
		return false;
	}
	x += node->x;
	y += node->y;
	if (node->type == WLR_SCENE_NODE_TREE) {
		struct wlr_scene_node *child;
		wl_list_for_each_reverse(child, &wlr_scene_tree_from_node(node)->children,
				link) if (sysc_debug_top_leaf(child, x, y, output_box,
							  found)) return true;
		return false;
	}
	if (!pixman_region32_not_empty(&node->WLR_PRIVATE.visible)) {
		return false;
	}
	if (pixman_region32_contains_rectangle(&node->WLR_PRIVATE.visible,
			    &(pixman_box32_t){output_box->x, output_box->y,
					    output_box->x + output_box->width,
					    output_box->y + output_box->height}) ==
			PIXMAN_REGION_OUT) {
		return false;
	}
	if (node->type == WLR_SCENE_NODE_BUFFER) {
		found->buffer = wlr_scene_buffer_from_node(node);
		found->x = x;
		found->y = y;
	}
	return true;
}
static struct sysc_debug_leaf sysc_debug_candidate(struct wlr_scene_output *so) {
	struct sysc_debug_leaf found = {0};
	int width, height;
	wlr_output_effective_resolution(so->output, &width, &height);
	struct wlr_box box = {so->x, so->y, width, height};
	sysc_debug_top_leaf(&so->scene->tree.node, 0, 0, &box, &found);
	return found;
}
static bool sysc_debug_overlay_state(
		struct wlr_scene_output *so, struct wlr_output_layer_state *layer) {
	struct sysc_debug_leaf leaf = sysc_debug_candidate(so);
	struct wlr_scene_buffer *buffer = leaf.buffer;
	if (!buffer || !buffer->buffer || buffer->opacity != 1 ||
			buffer->transform != WL_OUTPUT_TRANSFORM_NORMAL ||
			so->output->transform != WL_OUTPUT_TRANSFORM_NORMAL ||
			buffer->corners.top_left || buffer->corners.top_right ||
			buffer->corners.bottom_left || buffer->corners.bottom_right) {
		return false;
	}
	int width = buffer->dst_width ? buffer->dst_width : buffer->buffer->width;
	int height = buffer->dst_height ? buffer->dst_height : buffer->buffer->height;
	pixman_box32_t opaque = {0, 0, width, height};
	if (!buffer->WLR_PRIVATE.buffer_is_opaque &&
			pixman_region32_contains_rectangle(&buffer->opaque_region, &opaque) !=
					PIXMAN_REGION_IN) {
		return false;
	}
	struct wlr_dmabuf_attributes attributes;
	if (!wlr_buffer_get_dmabuf(buffer->buffer, &attributes)) {
		return false;
	}
	float scale = so->output->scale;
	layer->buffer = buffer->buffer;
	layer->src_box = buffer->src_box;
	layer->dst_box = (struct wlr_box){lround((leaf.x - so->x) * scale),
			lround((leaf.y - so->y) * scale), lround(width * scale),
			lround(height * scale)};
	return layer->dst_box.x >= 0 && layer->dst_box.y >= 0 &&
			layer->dst_box.x + layer->dst_box.width <= so->output->width &&
			layer->dst_box.y + layer->dst_box.height <= so->output->height;
}
static bool sysc_debug_format_matches(struct wlr_scene_output *so) {
	struct sysc_debug_leaf leaf = sysc_debug_candidate(so);
	struct wlr_dmabuf_attributes attributes;
	return !leaf.buffer || !leaf.buffer->buffer ||
			!wlr_buffer_get_dmabuf(leaf.buffer->buffer, &attributes) ||
			attributes.format == so->output->render_format;
}
struct sysc_debug_paint {
	struct wlr_render_pass *pass;
	struct wlr_scene_output *so;
};
static void sysc_debug_region_to_buffer(pixman_region32_t *region, struct wlr_scene_output *so) {
	pixman_region32_translate(region, -so->x, -so->y);
	wlr_region_scale(region, region, so->output->scale);
	int width, height;
	wlr_output_transformed_resolution(so->output, &width, &height);
	wlr_region_transform(region, region, wlr_output_transform_invert(so->output->transform),
			width, height);
}
static void sysc_debug_paint_region(struct sysc_debug_paint *paint, pixman_region32_t *region,
		struct wlr_render_color color) {
	sysc_debug_region_to_buffer(region, paint->so);
	struct wlr_box whole = {0, 0, paint->so->output->width, paint->so->output->height};
	wlr_render_pass_add_rect(paint->pass,
			&(struct wlr_render_rect_options){
					.box = whole, .color = color, .clip = region});
}
static void sysc_debug_opaque_buffer(struct wlr_scene_buffer *buffer, int x, int y, void *data) {
	struct sysc_debug_paint *paint = data;
	pixman_region32_t visible, opaque;
	pixman_region32_init(&visible);
	pixman_region32_init(&opaque);
	pixman_region32_copy(&visible, &buffer->node.WLR_PRIVATE.visible);
	int width = buffer->dst_width ? buffer->dst_width : buffer->WLR_PRIVATE.buffer_width;
	int height = buffer->dst_height ? buffer->dst_height : buffer->WLR_PRIVATE.buffer_height;
	if (buffer->opacity == 1) {
		if (buffer->WLR_PRIVATE.buffer_is_opaque) {
			pixman_region32_union_rect(&opaque, &opaque, x, y, width, height);
		} else {
			pixman_region32_copy(&opaque, &buffer->opaque_region);
			pixman_region32_intersect_rect(&opaque, &opaque, 0, 0, width, height);
			pixman_region32_translate(&opaque, x, y);
		}
		struct fx_corner_radii r = buffer->corners;
		struct wlr_box corners[4] = {{x, y, r.top_left, r.top_left},
				{x + width - r.top_right, y, r.top_right, r.top_right},
				{x + width - r.bottom_right, y + height - r.bottom_right,
						r.bottom_right, r.bottom_right},
				{x, y + height - r.bottom_left, r.bottom_left, r.bottom_left}};
		for (size_t i = 0; i < 4; i++) {
			if (corners[i].width) {
				pixman_region32_t cut;
				pixman_region32_init_rect(&cut, corners[i].x, corners[i].y,
						corners[i].width, corners[i].height);
				pixman_region32_subtract(&opaque, &opaque, &cut);
				pixman_region32_fini(&cut);
			}
		}
	}
	pixman_region32_intersect(&opaque, &opaque, &visible);
	pixman_region32_subtract(&visible, &visible, &opaque);
	sysc_debug_paint_region(paint, &visible, (struct wlr_render_color){.r = .25, .a = .25});
	sysc_debug_paint_region(paint, &opaque, (struct wlr_render_color){.b = .25, .a = .25});
	pixman_region32_fini(&opaque);
	pixman_region32_fini(&visible);
}
static bool sysc_debug_repaint(struct wlr_scene_output *so, struct wlr_output_layer *overlay,
		bool overlays, bool tint, bool opaque, bool finish, bool restrict_format) {
	if (!wlr_scene_output_needs_frame(so)) {
		return true;
	}
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	bool saved_scanout = so->scene->WLR_PRIVATE.direct_scanout;
	if (opaque || (restrict_format && !sysc_debug_format_matches(so))) {
		so->scene->WLR_PRIVATE.direct_scanout = false;
	}
	bool ok = wlr_scene_output_build_state(so, &state, NULL);
	so->scene->WLR_PRIVATE.direct_scanout = saved_scanout;
	if (!ok) {
		wlr_output_state_finish(&state);
		return false;
	}
	bool scanout = so->WLR_PRIVATE.prev_scanout;
	struct wlr_output_layer_state layer = {.layer = overlay};
	if (overlay) {
		if (overlays && !scanout && !tint && !opaque) {
			sysc_debug_overlay_state(so, &layer);
		}
		wlr_output_state_set_layers(&state, &layer, 1);
		if (layer.buffer &&
				(!wlr_output_test_state(so->output, &state) || !layer.accepted)) {
			layer.buffer = NULL;
			wlr_output_state_set_layers(&state, &layer, 1);
		}
	}
	if (!scanout && state.buffer && (tint || opaque || finish)) {
		struct wlr_render_pass *pass = wlr_renderer_begin_buffer_pass(
				so->output->renderer, state.buffer, NULL);
		if (!pass) {
			wlr_output_state_finish(&state);
			return false;
		}
		if (tint) {
			wlr_render_pass_add_rect(pass,
					&(struct wlr_render_rect_options){
							.box = {0, 0, so->output->width,
									so->output->height},
							.color = {.g = .2, .a = .2}});
		}
		if (opaque) {
			struct sysc_debug_paint paint = {pass, so};
			wlr_scene_output_for_each_buffer(so, sysc_debug_opaque_buffer, &paint);
		}
		if (finish) {
			glFinish();
		}
		if (!wlr_render_pass_submit(pass)) {
			wlr_output_state_finish(&state);
			return false;
		}
	}
	ok = wlr_output_commit_state(so->output, &state);
	wlr_output_state_finish(&state);
	return ok;
}
static void sysc_debug_frame_done(struct wlr_scene_output *so) {
	struct timespec now;
	clock_gettime(CLOCK_MONOTONIC, &now);
	wlr_scene_output_send_frame_done(so, &now);
}
*/
import "C"

import (
	"fmt"
	"os"
	"reflect"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

type debugOutput struct {
	identity         config.OutputName
	softwareCursor   bool
	content          uint64
	present, destroy *Listener
	overlay          *C.struct_wlr_output_layer
}
type debugRuntime struct {
	outputs              map[unsafe.Pointer]*debugOutput
	content              uint64
	tint, opaque, damage bool
	filteredHotplug      bool
	active, hotplug      *Listener
	activationLogger     *C.struct_wl_protocol_logger
	activationToken      uintptr
	activation           map[unsafe.Pointer]*debugActivationToken
	pendingActivation    map[unsafe.Pointer]*debugPendingActivation
}

func (c *Compositor) debugContentChanged() { c.debugRuntime.content++ }
func (c *Compositor) debugFullDamage() {
	c.debugContentChanged()
	for _, output := range c.sceneOutputs {
		C.wlr_damage_ring_add_whole(&output.damage_ring)
		C.wlr_output_schedule_frame(output.output)
	}
}
func (c *Compositor) ToggleDebugTint() {
	c.debugRuntime.tint = !c.debugRuntime.tint
	c.debugFullDamage()
}
func (c *Compositor) DebugToggleOpaqueRegions() {
	c.debugRuntime.opaque = !c.debugRuntime.opaque
	c.debugFullDamage()
}
func (c *Compositor) DebugToggleDamage() {
	c.debugRuntime.damage = !c.debugRuntime.damage
	c.applyDebugScene()
	c.debugFullDamage()
}
func (c *Compositor) applyDebugScene() {
	if c.scene != nil {
		C.sysc_debug_scene(c.scene, C.bool(!c.debugConfig.DisableDirectScanout), C.bool(c.debugRuntime.damage))
	}
}

func (c *Compositor) initializeDebug() error {
	if c.debugRuntime.outputs == nil {
		c.debugRuntime.outputs = map[unsafe.Pointer]*debugOutput{}
	}
	c.applyDebugScene()
	if err := c.initializeDebugActivation(); err != nil {
		return err
	}
	if c.session != nil {
		c.debugRuntime.active = AddListener(unsafe.Pointer(&c.session.events.active), func(unsafe.Pointer) {
			if !bool(c.session.active) || !c.debugConfig.ForceDisableConnectorsOnResume {
				return
			}
			_ = c.Post(func() {
				for pointer := range c.backendOutputs {
					output := (*C.struct_wlr_output)(pointer)
					if bool(output.enabled) && !bool(C.sysc_debug_reset_output(output)) {
						fmt.Fprintf(os.Stderr, "sysc-compositor: output %s: forced resume reset failed\n", outputName(pointer))
					}
				}
			})
		})
		c.keep(c.debugRuntime.active)
		if c.debugRuntime.filteredHotplug {
			c.debugRuntime.hotplug = AddListener(unsafe.Pointer(&c.session.events.add_drm_card), func(data unsafe.Pointer) {
				event := (*C.struct_wlr_session_add_event)(data)
				path := C.GoString(event.path)
				_ = c.Post(func() { c.addDebugDRMDevice(path) })
			})
			c.keep(c.debugRuntime.hotplug)
		}
	}
	return nil
}
func (c *Compositor) admitDebugOutput(pointer unsafe.Pointer) bool {
	output := (*C.struct_wlr_output)(pointer)
	if c.debugBackendIgnored(output.backend) {
		return false
	}
	if c.debugRuntime.outputs == nil {
		c.debugRuntime.outputs = map[unsafe.Pointer]*debugOutput{}
	}
	state := &debugOutput{identity: outputIdentity(pointer), content: ^uint64(0)}
	c.debugRuntime.outputs[pointer] = state
	state.present = AddListener(unsafe.Pointer(&output.events.present), func(data unsafe.Pointer) {
		if c.debugConfig.EmulateZeroPresentationTime {
			C.sysc_debug_zero_presentation((*C.struct_wlr_output_event_present)(data))
		}
	})
	state.destroy = AddListener(unsafe.Pointer(&output.events.destroy), func(unsafe.Pointer) {
		if state.overlay != nil {
			C.wlr_output_layer_destroy(state.overlay)
			state.overlay = nil
		}
		delete(c.debugRuntime.outputs, pointer)
		c.drop(state.present)
		c.drop(state.destroy)
	})
	c.keep(state.present, state.destroy)
	c.configureDebugOutput(pointer, state)
	return true
}
func (c *Compositor) configureDebugOutput(pointer unsafe.Pointer, state *debugOutput) {
	output := (*C.struct_wlr_output)(pointer)
	if state.softwareCursor != c.debugConfig.DisableCursorPlane {
		C.wlr_output_lock_software_cursors(output, C.bool(c.debugConfig.DisableCursorPlane))
		state.softwareCursor = c.debugConfig.DisableCursorPlane
	}
	current := outputIdentity(pointer)
	desired := state.identity
	if c.debugConfig.DisableMonitorNames {
		desired.Make = ""
		desired.Model = ""
		desired.Serial = ""
	}
	if current != desired {
		make, model, serial := C.CString(desired.Make), C.CString(desired.Model), C.CString(desired.Serial)
		C.sysc_debug_identity(output, make, model, serial)
		C.free(unsafe.Pointer(make))
		C.free(unsafe.Pointer(model))
		C.free(unsafe.Pointer(serial))
	}
	if c.debugConfig.EnableOverlayPlanes && state.overlay == nil {
		state.overlay = C.wlr_output_layer_create(output)
	}
}
func (c *Compositor) applyDebugSettings() {
	c.applyDebugScene()
	for pointer, state := range c.debugRuntime.outputs {
		c.configureDebugOutput(pointer, state)
		if c.debugBackendIgnored((*C.struct_wlr_output)(pointer).backend) {
			_ = c.turnOutputOff(pointer)
		}
	}
	c.debugFullDamage()
}
func (c *Compositor) repaintOutput(pointer unsafe.Pointer, scene *C.struct_wlr_scene_output) bool {
	defer C.sysc_debug_frame_done(scene)
	state := c.debugRuntime.outputs[pointer]
	if state == nil {
		return bool(C.wlr_scene_output_commit(scene, nil))
	}
	output := (*C.struct_wlr_output)(pointer)
	if c.debugConfig.SkipCursorOnlyUpdatesDuringVRR && output.adaptive_sync_status == C.WLR_OUTPUT_ADAPTIVE_SYNC_ENABLED && state.content == c.debugRuntime.content {
		return true
	}
	var restore func()
	if target := c.debugConfig.PreviewRender; target != nil {
		var err error
		restore, err = c.beginCapturePrivacy(*target)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sysc-compositor: debug render preview:", err)
			return false
		}
		defer restore()
	}
	ok := bool(C.sysc_debug_repaint(scene, state.overlay, C.bool(c.debugConfig.EnableOverlayPlanes), C.bool(c.debugRuntime.tint), C.bool(c.debugRuntime.opaque), C.bool(c.debugConfig.WaitForFrameCompletionBeforeQueueing), C.bool(c.debugConfig.RestrictPrimaryScanoutToMatchingFormat)))
	if ok {
		state.content = c.debugRuntime.content
	}
	return ok
}
func (c *Compositor) finishDebug() {
	c.finishDebugActivation()
	for pointer, state := range c.debugRuntime.outputs {
		if state.overlay != nil {
			C.wlr_output_layer_destroy(state.overlay)
		}
		if state.softwareCursor {
			C.wlr_output_lock_software_cursors((*C.struct_wlr_output)(pointer), false)
		}
		c.drop(state.present)
		c.drop(state.destroy)
	}
	c.drop(c.debugRuntime.active)
	c.drop(c.debugRuntime.hotplug)
	c.debugRuntime.outputs = nil
}

// Creation-time tuning is compared by value rather than by pointer identity.
func debugRendererChanged(before, after config.Debug) bool {
	return !reflect.DeepEqual(before.RenderDRMDevice, after.RenderDRMDevice)
}
