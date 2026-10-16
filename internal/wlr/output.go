package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdbool.h>
#include <stdlib.h>
#include <stdint.h>

#include <wlr/backend.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_xcursor_manager.h>

// sysc_apply_requested_state commits the state a backend asked for: the host
// compositor resizing the nested window, for example. It reports the size the
// request carried, or 0x0 when it carried no mode.
static bool sysc_apply_requested_state(struct wlr_output_event_request_state *event,
		int *width, int *height) {
	const struct wlr_output_state *state = event->state;
	*width = 0;
	*height = 0;
	if (state->committed & WLR_OUTPUT_STATE_MODE) {
		if (state->mode_type == WLR_OUTPUT_STATE_MODE_CUSTOM) {
			*width = state->custom_mode.width;
			*height = state->custom_mode.height;
		} else if (state->mode != NULL) {
			*width = state->mode->width;
			*height = state->mode->height;
		}
	}
	return wlr_output_commit_state(event->output, state);
}

// sysc_commit_changed_geometry reports whether a commit changed what an output
// covers in the layout.
static bool sysc_commit_changed_geometry(struct wlr_output_event_commit *event) {
	return (event->state->committed & (WLR_OUTPUT_STATE_MODE | WLR_OUTPUT_STATE_SCALE |
		WLR_OUTPUT_STATE_TRANSFORM)) != 0;
}

static bool sysc_output_set_scale(struct wlr_output *output, float scale) {
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_scale(&state, scale);
	bool ok = wlr_output_commit_state(output, &state);
	wlr_output_state_finish(&state);
	return ok;
}

// sysc_bring_output_up gives one backend output a renderer and turns it on at
// its preferred mode.
//
// The order is load-bearing. wlr_output_init_render must come first: the output
// layout attaches the cursor to every output it holds, and
// wlr_output_cursor_set_buffer asserts renderer != NULL. Enable it too late and
// the compositor dies inside wl_output_layout_add_auto with no Go stack.
//
// wlroots 0.20 also replaced wlr_output_enable/set_mode/commit with an output
// state object, so enable, mode and commit are one transaction. Getting that
// wrong is silent rather than fatal: the compositor advertises no wl_output,
// the client connects, then exits with "no monitors available".
// TestOutputsEnabled is the check that catches it.
static bool sysc_bring_output_up(struct wlr_output *output,
		struct wlr_allocator *allocator, struct wlr_renderer *renderer) {
	if (!wlr_output_init_render(output, allocator, renderer)) {
		return false;
	}
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_enabled(&state, true);
	struct wlr_output_mode *mode = wlr_output_preferred_mode(output);
	if (mode != NULL) {
		wlr_output_state_set_mode(&state, mode);
	}
	bool ok = wlr_output_commit_state(output, &state);
	wlr_output_state_finish(&state);
	return ok;
}

// sysc_place_in_layout puts the output in the layout and mirrors that placement
// into the scene graph. wlroots 0.20 attaches a scene output to a scene and a
// position, not to a separate layout object; the layout survives for cursor
// mapping. Layout coordinates are doubles in 0.20, the scene takes ints.
static struct wlr_scene_output *sysc_place_in_layout(
		struct wlr_output_layout *layout, struct wlr_scene *scene,
		struct wlr_output *output) {
	struct wlr_output_layout_output *lo = wlr_output_layout_add_auto(layout, output);
	if (lo == NULL) {
		return NULL;
	}
	struct wlr_scene_output *so = wlr_scene_output_create(scene, output);
	if (so == NULL) {
		return NULL;
	}
	double lx = 0, ly = 0;
	wlr_output_layout_output_coords(layout, output, &lx, &ly);
	wlr_scene_output_set_position(so, (int)lx, (int)ly);
	return so;
}

// sysc_repaint_scene_output commits the scene for one output and asks for the
// next frame. wlr_scene_output takes a state-options struct rather than a
// timestamp, and M0 changes no options, so NULL is the honest argument.
static bool sysc_repaint_scene_output(struct wlr_scene_output *so) {
	if (!wlr_scene_output_commit(so, NULL)) {
		return false;
	}
	wlr_output_schedule_frame(so->output);
	return true;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// handleNewOutput brings up an output the backend just announced and gives it a
// scene output so clients paint into the nested session rather than nowhere.
func (c *Compositor) handleNewOutput(data unsafe.Pointer) {
	output := (*C.struct_wlr_output)(data)
	if !bool(C.sysc_bring_output_up(output, c.alloc, c.renderer)) {
		return
	}
	so := C.sysc_place_in_layout(c.layout, c.scene, output)
	if so == nil {
		return
	}
	c.sceneOutputs[data] = so

	// The frame signal carries no payload, so the scene output cannot be
	// recovered from the callback argument. Key it by the listener instead.
	frame := AddListener(unsafe.Pointer(&output.events.frame), c.handleOutputFrame)
	c.frames[frame.Native()] = so
	c.keep(frame)
	c.arrangeLayers(data)

	// A nested output is sized by the host: Niri tiles the window and the
	// backend asks for that size. Ignoring the request leaves the output at
	// its first size and nothing after the first frames is drawn.
	request := AddListener(unsafe.Pointer(&output.events.request_state), func(event unsafe.Pointer) {
		var w, h C.int
		C.sysc_apply_requested_state((*C.struct_wlr_output_event_request_state)(event), &w, &h)
		if w > 0 {
			c.requested = rect{0, 0, int(w), int(h)}
		}
	})
	// The layout follows the output's commit first (its listener came
	// first); layer surfaces then follow the new box.
	commit := AddListener(unsafe.Pointer(&output.events.commit), func(event unsafe.Pointer) {
		if bool(C.sysc_commit_changed_geometry((*C.struct_wlr_output_event_commit)(event))) {
			c.arrangeLayers(data)
		}
	})
	c.keep(request, commit)

	var destroy *Listener
	destroy = AddListener(unsafe.Pointer(&output.events.destroy), func(unsafe.Pointer) {
		c.closeOutputLayers(data)
		delete(c.sceneOutputs, data)
		delete(c.usable, data)
		delete(c.frames, frame.Native())
		c.drop(frame)
		c.drop(request)
		c.drop(commit)
		c.drop(destroy)
	})
	c.keep(destroy)
}

// handleOutputFrame repaints an output and asks for the next one. wlroots
// drives the frame cadence; the compositor only has to commit.
func (c *Compositor) handleOutputFrame(listener unsafe.Pointer) {
	so := c.frames[listener]
	if so == nil {
		return
	}
	C.sysc_repaint_scene_output(so)
}

// EnabledOutputs reports how many backend outputs are up. It is the number a
// client means by "monitors".
func (c *Compositor) EnabledOutputs() int { return len(c.sceneOutputs) }

// setOutputScale sets every output's scale, for tests.
func (c *Compositor) setOutputScale(scale float64) error {
	for out := range c.sceneOutputs {
		if !bool(C.sysc_output_set_scale((*C.struct_wlr_output)(out), C.float(scale))) {
			return errors.New("output scale commit failed")
		}
	}
	return nil
}

// cursorLoadedAt reports whether the pointer theme is loaded at scale, for
// tests. wlr_cursor loads the theme at each output's scale itself whenever the
// image is a theme name, including after an output's scale changes.
func (c *Compositor) cursorLoadedAt(scale float64) bool {
	if c.xcursorManager == nil {
		return false
	}
	name := C.CString("left_ptr")
	defer C.free(unsafe.Pointer(name))
	return C.wlr_xcursor_manager_get_xcursor(c.xcursorManager, name, C.float(scale)) != nil
}

// requestedOutputSize is the size of the last state the host asked an output
// to take, for tests.
func (c *Compositor) requestedOutputSize() rect { return c.requested }
