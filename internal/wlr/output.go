package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdbool.h>
#include <stdlib.h>
#include <stdint.h>
#include <time.h>

#include <wlr/backend.h>
#include <wlr/backend/wayland.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>
#include <wayland-server-protocol.h>

static size_t sysc_output_mode_count(struct wlr_output *output) {
	size_t count = 0;
	struct wlr_output_mode *mode;
	wl_list_for_each(mode, &output->modes, link) {
		count++;
	}
	return count;
}

static bool sysc_output_mode_at(struct wlr_output *output, size_t index,
		int32_t *width, int32_t *height, int32_t *refresh,
		int *preferred, int *current) {
	struct wlr_output_mode *mode;
	size_t i = 0;
	wl_list_for_each(mode, &output->modes, link) {
		if (i++ != index) {
			continue;
		}
		*width = mode->width;
		*height = mode->height;
		*refresh = mode->refresh;
		*preferred = mode->preferred;
		*current = mode == output->current_mode;
		return true;
	}
	return false;
}

static void sysc_output_logical(struct wlr_output_layout *layout,
		struct wlr_output *output, int32_t *x, int32_t *y,
		int32_t *width, int32_t *height, float *scale, uint32_t *transform) {
	struct wlr_box box;
	wlr_output_layout_get_box(layout, output, &box);
	*x = box.x;
	*y = box.y;
	*width = box.width;
	*height = box.height;
	*scale = output->scale;
	*transform = output->transform;
}
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

static struct wlr_output *sysc_output_layout_at(struct wlr_output_layout *layout, double x, double y) {
	return wlr_output_layout_output_at(layout, x, y);
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

// sysc_repaint_scene_output commits the scene for one output, answers the
// frame callbacks of every surface on it, and asks for the next frame.
// wlr_scene_output takes a state-options struct rather than a timestamp, and
// M0 changes no options, so NULL is the honest argument.
//
// The scene never answers wl_surface.frame by itself. Without the done
// events a client that paces its drawing on them (kitty, weston-simple-shm)
// stops repainting after its first frames. tinywl sends them here too, even
// when the commit had nothing to draw.
static bool sysc_repaint_scene_output(struct wlr_scene_output *so) {
	bool committed = wlr_scene_output_commit(so, NULL);
	struct timespec now;
	clock_gettime(CLOCK_MONOTONIC, &now);
	wlr_scene_output_send_frame_done(so, &now);
	if (!committed) {
		return false;
	}
	wlr_output_schedule_frame(so->output);
	return true;
}
*/
import "C"

import (
	"errors"
	"sort"
	"unsafe"
)

type OutputModeSnapshot struct {
	Width       uint16
	Height      uint16
	RefreshRate uint32
	Preferred   bool
}

type LogicalOutputSnapshot struct {
	X, Y          int32
	Width, Height uint32
	Scale         float64
	Transform     uint32
}

type OutputSnapshot struct {
	Name         string
	Make         string
	Model        string
	Serial       *string
	PhysicalSize *[2]uint32
	Modes        []OutputModeSnapshot
	CurrentMode  *uint32
	IsCustomMode bool
	VRRSupported bool
	VRREnabled   bool
	Logical      *LogicalOutputSnapshot
}

func chooseActiveOutputName(current, focused string, outputs []string) string {
	names := append([]string(nil), outputs...)
	sort.Strings(names)
	for _, name := range names {
		if name == focused {
			return focused
		}
	}
	for _, name := range names {
		if name == current {
			return current
		}
	}
	if len(names) != 0 {
		return names[0]
	}
	return ""
}

func (c *Compositor) outputNameAt(x, y float64) string {
	output := C.sysc_output_layout_at(c.layout, C.double(x), C.double(y))
	if output == nil {
		return ""
	}
	if _, enabled := c.sceneOutputs[unsafe.Pointer(output)]; !enabled {
		return ""
	}
	return C.GoString(output.name)
}

func (c *Compositor) setActiveOutputName(focused string) {
	names := make([]string, 0, len(c.sceneOutputs))
	for pointer := range c.sceneOutputs {
		names = append(names, C.GoString((*C.struct_wlr_output)(pointer).name))
	}
	name := chooseActiveOutputName(c.activeOutputName, focused, names)
	if name == c.activeOutputName {
		return
	}
	c.activeOutputName = name
	c.notifyFocusedOutputChanged()
}

// OutputSnapshot copies the enabled outputs into plain Go values. Call it on
// the compositor thread; IPC readers consume the resulting immutable copies.
func (c *Compositor) OutputSnapshot() []OutputSnapshot {
	out := make([]OutputSnapshot, 0, len(c.sceneOutputs))
	for pointer := range c.sceneOutputs {
		output := (*C.struct_wlr_output)(pointer)
		vrrSupported := bool(output.adaptive_sync_supported)
		info := OutputSnapshot{
			Name: C.GoString(output.name), Make: cString(output.make), Model: cString(output.model),
			Modes:        make([]OutputModeSnapshot, 0),
			VRRSupported: vrrSupported,
			VRREnabled:   vrrSupported && output.adaptive_sync_status == C.WLR_OUTPUT_ADAPTIVE_SYNC_ENABLED,
		}
		if output.serial != nil && C.GoString(output.serial) != "" {
			serial := C.GoString(output.serial)
			info.Serial = &serial
		}
		if output.phys_width > 0 && output.phys_height > 0 {
			physical := [2]uint32{uint32(output.phys_width), uint32(output.phys_height)}
			info.PhysicalSize = &physical
		}
		count := int(C.sysc_output_mode_count(output))
		for i := 0; i < count; i++ {
			var width, height, refresh C.int32_t
			var preferred, current C.int
			if !bool(C.sysc_output_mode_at(output, C.size_t(i), &width, &height, &refresh, &preferred, &current)) {
				continue
			}
			info.Modes = append(info.Modes, OutputModeSnapshot{
				Width: clampModeSize(width), Height: clampModeSize(height),
				RefreshRate: uint32(max(0, int32(refresh))), Preferred: preferred != 0,
			})
			if current != 0 {
				index := uint32(len(info.Modes) - 1)
				info.CurrentMode = &index
			}
		}
		if info.CurrentMode == nil && output.width > 0 && output.height > 0 {
			width, height := clampModeSize(output.width), clampModeSize(output.height)
			refresh := uint32(max(0, int32(output.refresh)))
			if refresh == 0 {
				refresh = 60_000
			}
			info.Modes = append(info.Modes, OutputModeSnapshot{
				Width: width, Height: height, RefreshRate: refresh, Preferred: true,
			})
			index := uint32(len(info.Modes) - 1)
			info.CurrentMode = &index
			info.IsCustomMode = true
		}
		var x, y, width, height C.int32_t
		var scale C.float
		var transform C.uint32_t
		C.sysc_output_logical(c.layout, output, &x, &y, &width, &height, &scale, &transform)
		if width > 0 && height > 0 {
			info.Logical = &LogicalOutputSnapshot{
				X: int32(x), Y: int32(y), Width: uint32(width), Height: uint32(height),
				Scale: float64(scale), Transform: uint32(transform),
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FocusedOutputName reports the active output, falling back to the first
// connected output when no active output is set or its output disconnects.
func (c *Compositor) FocusedOutputName() string {
	names := make([]string, 0, len(c.sceneOutputs))
	for pointer := range c.sceneOutputs {
		names = append(names, C.GoString((*C.struct_wlr_output)(pointer).name))
	}
	c.activeOutputName = chooseActiveOutputName(c.activeOutputName, "", names)
	return c.activeOutputName
}

func cString(s *C.char) string {
	if s == nil {
		return ""
	}
	return C.GoString(s)
}

func clampModeSize(v C.int32_t) uint16 {
	if v <= 0 {
		return 0
	}
	if v > 1<<16-1 {
		return 1<<16 - 1
	}
	return uint16(v)
}

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

	// The scene output is captured here. M0 keyed a map by the listener's
	// own pointer, but callbacks receive the signal's payload, so no frame
	// ever found its scene output and nothing was drawn.
	frame := AddListener(unsafe.Pointer(&output.events.frame), func(unsafe.Pointer) {
		if C.sysc_repaint_scene_output(so) {
			c.drawn++
		}
	})
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
			c.refreshState()
		}
	})
	c.keep(request, commit)

	var destroy *Listener
	destroy = AddListener(unsafe.Pointer(&output.events.destroy), func(unsafe.Pointer) {
		c.closeOutputLayers(data)
		delete(c.sceneOutputs, data)
		delete(c.usable, data)
		c.refreshState()
		c.drop(frame)
		c.drop(request)
		c.drop(commit)
		c.drop(destroy)
		if bool(C.wlr_backend_is_wl(output.backend)) && len(c.sceneOutputs) == 0 {
			c.Terminate()
		}
	})
	c.keep(destroy)
	c.refreshState()
}

// destroyOutput asks the backend to destroy one output. The nested-session
// test uses it to simulate the host closing its Wayland output window.
func destroyOutput(output unsafe.Pointer) {
	C.wlr_output_destroy((*C.struct_wlr_output)(output))
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

// scenesDrawn is how many output frames ran the scene commit, for tests.
func (c *Compositor) scenesDrawn() int { return c.drawn }
