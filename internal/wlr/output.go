package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdbool.h>
#include <stdint.h>

#include <wlr/backend.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>

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

import "unsafe"

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
	c.listeners = append(c.listeners, frame)
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
