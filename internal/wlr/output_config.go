package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdbool.h>
#include <stdint.h>

#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>

// sysc_output_config is one output's whole configured state. mode_index picks
// a listed mode, custom_width a custom one; neither leaves the mode alone.
// adaptive_sync is -1 to leave it alone.
struct sysc_output_config {
	int mode_index;
	int32_t custom_width, custom_height, custom_refresh;
	float scale;
	uint32_t transform;
	int adaptive_sync;
};

// sysc_output_try_config tests the state, and commits it unless test_only.
static bool sysc_output_try_config(struct wlr_output *output,
		const struct sysc_output_config *config, bool test_only) {
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_enabled(&state, true);
	if (config->mode_index >= 0) {
		struct wlr_output_mode *mode;
		int i = 0;
		wl_list_for_each(mode, &output->modes, link) {
			if (i++ == config->mode_index) {
				wlr_output_state_set_mode(&state, mode);
				break;
			}
		}
	} else if (config->custom_width > 0) {
		wlr_output_state_set_custom_mode(&state, config->custom_width,
			config->custom_height, config->custom_refresh);
	}
	wlr_output_state_set_scale(&state, config->scale);
	wlr_output_state_set_transform(&state, config->transform);
	if (config->adaptive_sync >= 0) {
		wlr_output_state_set_adaptive_sync_enabled(&state, config->adaptive_sync);
	}
	bool ok = wlr_output_test_state(output, &state);
	if (ok && !test_only) {
		ok = wlr_output_commit_state(output, &state);
	}
	wlr_output_state_finish(&state);
	return ok;
}

static bool sysc_output_disable(struct wlr_output *output) {
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_enabled(&state, false);
	bool ok = wlr_output_commit_state(output, &state);
	wlr_output_state_finish(&state);
	return ok;
}

// sysc_output_place moves an output in the layout and its scene output with it.
static void sysc_output_place(struct wlr_output_layout *layout,
		struct wlr_scene_output *scene_output, struct wlr_output *output, int x, int y) {
	wlr_output_layout_add(layout, output, x, y);
	wlr_scene_output_set_position(scene_output, x, y);
}

// sysc_output_release takes an output out of the layout, which also stops
// exposing its wl_output, and destroys its scene output.
static void sysc_output_release(struct wlr_output_layout *layout,
		struct wlr_scene_output *scene_output, struct wlr_output *output) {
	wlr_scene_output_destroy(scene_output);
	wlr_output_layout_remove(layout, output);
}
*/
import "C"

import (
	"fmt"
	"image"
	"os"
	"slices"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// outputConfig returns the config section that matches the output, or nil.
func (c *Compositor) outputConfig(pointer unsafe.Pointer) *config.OutputConfig {
	identity := outputIdentity(pointer)
	for i := range c.outputConfigs {
		if identity.Matches(c.outputConfigs[i].Name) {
			return &c.outputConfigs[i]
		}
	}
	return nil
}

// applyOutputConfigs is Niri's reload_output_config: it brings every output
// to its section's state, then places them all. Every path that changes the
// output config calls it.
func (c *Compositor) applyOutputConfigs() {
	outputs := make([]unsafe.Pointer, 0, len(c.backendOutputs))
	for pointer := range c.backendOutputs {
		outputs = append(outputs, pointer)
	}
	slices.SortFunc(outputs, func(a, b unsafe.Pointer) int {
		return outputIdentity(a).Compare(outputIdentity(b))
	})
	for _, pointer := range outputs {
		c.applyOutputConfig(pointer)
	}
	c.placeOutputs()
}

// applyOutputConfig brings one output to its section's state. A change the
// output refuses falls back as Niri's does: without VRR, then to the preferred
// mode; when even that fails the output keeps its working state.
func (c *Compositor) applyOutputConfig(pointer unsafe.Pointer) {
	output := (*C.struct_wlr_output)(pointer)
	section := c.outputConfig(pointer)
	if section != nil && section.Off {
		c.turnOutputOff(pointer)
		return
	}
	// Power is not config: an output that PowerOffMonitors or a power
	// management client turned off stays off, and setOutputPower applies the
	// config when it turns the output back on.
	if _, adopted := c.sceneOutputs[pointer]; adopted && !c.outputIsEnabled(pointer) {
		return
	}
	var target *config.OutputMode
	var transform config.Transform
	var scale *float64
	adaptiveSync := 0
	if section != nil {
		target, transform, scale = section.Mode, section.Transform, section.Scale
		// On demand waits for a window that asks for VRR (slice 5), so it
		// starts off, as Niri's does with no such window shown.
		if section.VRR != nil && !section.VRR.OnDemand {
			adaptiveSync = 1
		}
	}
	modes := outputModeChoices(pointer)
	want := C.struct_sysc_output_config{mode_index: -1, transform: C.uint32_t(transform), adaptive_sync: C.int(adaptiveSync)}
	width, height := int(output.width), int(output.height)
	fallback := false
	if target != nil && target.Custom {
		want.custom_width, want.custom_height = C.int32_t(target.Width), C.int32_t(target.Height)
		want.custom_refresh = C.int32_t(target.Refresh * 1000)
		width, height = target.Width, target.Height
	} else if index, missed := pickOutputMode(modes, target); index >= 0 {
		want.mode_index = C.int(index)
		width, height = modes[index].width, modes[index].height
		fallback = missed
	}
	want.scale = C.float(outputScale(scale, int(output.phys_width), int(output.phys_height), width, height))
	name := C.GoString(output.name)
	if fallback {
		fmt.Fprintf(os.Stderr, "sysc-compositor: output %s: configured mode %dx%d@%g not found, using the preferred mode\n",
			name, target.Width, target.Height, target.Refresh)
	}

	attempts := []C.struct_sysc_output_config{want}
	if adaptiveSync == 1 {
		withoutVRR := want
		withoutVRR.adaptive_sync = 0
		attempts = append(attempts, withoutVRR)
	}
	if target != nil {
		preferred := want
		preferred.custom_width, preferred.adaptive_sync = 0, 0
		preferred.mode_index = -1
		if index, _ := pickOutputMode(modes, nil); index >= 0 {
			preferred.mode_index = C.int(index)
		}
		attempts = append(attempts, preferred)
	}
	applied := false
	for i := range attempts {
		if bool(C.sysc_output_try_config(output, &attempts[i], false)) {
			if i > 0 {
				fmt.Fprintf(os.Stderr, "sysc-compositor: output %s refused its configured state; fallback %d applied\n", name, i)
			}
			applied = true
			break
		}
	}
	if !applied {
		fmt.Fprintf(os.Stderr, "sysc-compositor: output %s refused its configured state; it keeps its current one\n", name)
		if !c.outputIsEnabled(pointer) {
			return
		}
	}
	if _, adopted := c.sceneOutputs[pointer]; !adopted {
		c.adoptOutput(pointer)
	}
}

// turnOutputOff disables an output and takes it out of the layout, as an
// unplugged output leaves it; the output stays known so a later config can
// turn it back on.
func (c *Compositor) turnOutputOff(pointer unsafe.Pointer) {
	if sceneOutput, adopted := c.sceneOutputs[pointer]; adopted {
		output := (*C.struct_wlr_output)(pointer)
		c.retireOutput(pointer)
		C.sysc_output_release(c.layout, (*C.struct_wlr_scene_output)(unsafe.Pointer(sceneOutput)), output)
	}
	if c.outputIsEnabled(pointer) && !bool(C.sysc_output_disable((*C.struct_wlr_output)(pointer))) {
		fmt.Fprintf(os.Stderr, "sysc-compositor: output %s: turning it off failed\n", outputName(pointer))
	}
}

// placeOutputs puts every output in the layout where Niri's
// reposition_outputs would, and lets what lives on an output follow it.
func (c *Compositor) placeOutputs() {
	pointers := make([]unsafe.Pointer, 0, len(c.sceneOutputs))
	placements := make([]outputPlacement, 0, len(c.sceneOutputs))
	for pointer := range c.sceneOutputs {
		var width, height C.int
		C.wlr_output_effective_resolution((*C.struct_wlr_output)(pointer), &width, &height)
		placement := outputPlacement{name: outputIdentity(pointer), size: image.Pt(int(width), int(height))}
		if section := c.outputConfig(pointer); section != nil {
			placement.configured = section.Position
		}
		pointers = append(pointers, pointer)
		placements = append(placements, placement)
	}
	// Every output is placed explicitly, even where it already is: wlroots
	// moves an output it placed itself (wlr_output_layout_add_auto) whenever
	// the layout changes.
	for i, position := range placeOutputs(placements) {
		pointer := pointers[i]
		C.sysc_output_place(c.layout, (*C.struct_wlr_scene_output)(unsafe.Pointer(c.sceneOutputs[pointer])),
			(*C.struct_wlr_output)(pointer), C.int(position.X), C.int(position.Y))
		c.outputGeometryChanged(pointer)
	}
}

// focusDefaultOutput is Niri's focus_default_monitor, run once the backend
// has announced its outputs: the first output in placement order, or the
// first section with focus-at-startup that names a connected output.
func (c *Compositor) focusDefaultOutput() {
	var target unsafe.Pointer
	for pointer := range c.sceneOutputs {
		if target == nil || c.placementBefore(pointer, target) {
			target = pointer
		}
	}
sections:
	for i := range c.outputConfigs {
		if !c.outputConfigs[i].FocusAtStartup {
			continue
		}
		for pointer := range c.sceneOutputs {
			if outputIdentity(pointer).Matches(c.outputConfigs[i].Name) {
				target = pointer
				break sections
			}
		}
	}
	if target != nil {
		c.FocusMonitor(outputName(target))
	}
}

// placementBefore orders outputs as placeOutputs does: configured positions
// first, then by name.
func (c *Compositor) placementBefore(a, b unsafe.Pointer) bool {
	configured := func(pointer unsafe.Pointer) bool {
		section := c.outputConfig(pointer)
		return section != nil && section.Position != nil
	}
	if configured(a) != configured(b) {
		return configured(a)
	}
	return outputIdentity(a).Compare(outputIdentity(b)) < 0
}
