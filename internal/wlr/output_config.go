package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <drm_mode.h>
#include <xf86drmMode.h>

#include <wlr/types/wlr_output.h>
#include <wlr/backend/drm.h>
#include <wlr/backend/session.h>
#include <wlr/backend.h>
#include <wlr/util/transform.h>
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
	const drmModeModeInfo *modeline;
};

// sysc_output_try_config tests the state, and commits it unless test_only.
static bool sysc_output_try_config(struct wlr_output *output,
		const struct sysc_output_config *config, bool test_only, struct wlr_output_state *prepared) {
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_enabled(&state, true);
	if (config->modeline != NULL && wlr_output_is_drm(output)) {
		struct wlr_output_mode *mode = NULL, *candidate;
		wl_list_for_each(candidate, &output->modes, link) {
			const drmModeModeInfo *info = wlr_drm_mode_get_info(candidate);
			if (info->clock == config->modeline->clock && info->flags == config->modeline->flags &&
				memcmp(&info->hdisplay, &config->modeline->hdisplay, 10*sizeof(uint16_t)) == 0) { mode = candidate; break; }
		}
		if (mode == NULL) mode = wlr_drm_connector_add_mode(output, config->modeline);
		if (mode == NULL) { wlr_output_state_finish(&state); return false; }
		wlr_output_state_set_mode(&state, mode);
	} else if (config->mode_index >= 0) {
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
	if (ok && prepared != NULL) ok = wlr_output_state_copy(prepared,&state);
	wlr_output_state_finish(&state);
	return ok;
}

static uint32_t sysc_output_configured_transform(struct wlr_output *output, uint32_t configured) {
	if (!wlr_output_is_drm(output)) return configured;
	return wlr_output_transform_compose(wlr_drm_connector_get_panel_orientation(output),configured);
}
static uint32_t sysc_output_relative_transform(struct wlr_output *output, uint32_t effective) {
	if (!wlr_output_is_drm(output)) return effective;
	return wlr_output_transform_compose(wlr_output_transform_invert(wlr_drm_connector_get_panel_orientation(output)),effective);
}
static void sysc_modeline_name(drmModeModeInfo *mode) {
	mode->type = DRM_MODE_TYPE_USERDEF;
	snprintf(mode->name,sizeof(mode->name),"%dx%d",mode->hdisplay,mode->vdisplay);
}

static bool sysc_output_disable(struct wlr_output *output, bool test_only, struct wlr_output_state *prepared) {
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_enabled(&state, false);
	bool ok = wlr_output_test_state(output, &state);
	if (ok && !test_only) ok = wlr_output_commit_state(output, &state);
	if (ok && prepared != NULL) ok = wlr_output_state_copy(prepared,&state);
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

// Retain the actual working state, including a fallback mode or DPMS blanking,
// rather than trying to reconstruct it from the user's requested config.
static struct wlr_output_state *sysc_output_save(struct wlr_output *output) {
	struct wlr_output_state *state = calloc(1, sizeof(*state));
	if (state == NULL) return NULL;
	wlr_output_state_init(state);
	wlr_output_state_set_enabled(state, output->enabled);
	if (output->current_mode != NULL) wlr_output_state_set_mode(state, output->current_mode);
	else if (output->width > 0 && output->height > 0)
		wlr_output_state_set_custom_mode(state, output->width, output->height, output->refresh);
	wlr_output_state_set_scale(state, output->scale);
	wlr_output_state_set_transform(state, output->transform);
	wlr_output_state_set_adaptive_sync_enabled(state,
		output->adaptive_sync_status == WLR_OUTPUT_ADAPTIVE_SYNC_ENABLED);
	return state;
}
static void sysc_output_saved_free(struct wlr_output_state *state) {
	wlr_output_state_finish(state);
	free(state);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"math"
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

func (c *Compositor) outputRelativeTransform(pointer unsafe.Pointer, effective config.Transform) config.Transform {
	return config.Transform(C.sysc_output_relative_transform((*C.struct_wlr_output)(pointer), C.uint32_t(effective)))
}

func (c *Compositor) outputConfigByName(name string) *config.OutputConfig {
	for pointer := range c.backendOutputs {
		if outputName(pointer) == name {
			return c.outputConfig(pointer)
		}
	}
	return nil
}

func (c *Compositor) outputDecorations(name string) config.Decorations {
	if section := c.outputConfigByName(name); section != nil && section.Layout != nil {
		return section.Layout.Decorations
	}
	return config.Decorations{}
}

// applyOutputConfigs is Niri's reload_output_config: it brings every output
// to its section's state, then places them all. Every path that changes the
// output config calls it. An inactive session cannot commit, so, as Niri's
// on_output_config_changed, it waits for watchSessionResume instead.
func (c *Compositor) applyOutputConfigs() error {
	if c.session != nil && !bool(c.session.active) {
		c.outputConfigOnResume = true
		return nil
	}
	c.outputConfigOnResume = false
	err := c.applyOutputConfigsMode(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: output config: %v\n", err)
	}
	return err
}

// watchSessionResume applies output config that changed while the session
// was inactive, after the backend has restored its own state on resume.
func (c *Compositor) watchSessionResume() {
	if c.session == nil {
		return
	}
	c.keep(AddListener(unsafe.Pointer(&c.session.events.active), func(unsafe.Pointer) {
		if bool(c.session.active) && c.outputConfigOnResume {
			_ = c.Post(func() {
				if c.outputConfigOnResume {
					c.applyOutputConfigs()
				}
			})
		}
	}))
}

func (c *Compositor) applyOutputConfigsMode(changed map[unsafe.Pointer]bool) error {
	c.applyingOutputConfigs = true
	defer func() {
		c.applyingOutputConfigs = false
		c.updateInjectedInputGate()
		c.publishOutputConfiguration()
		c.refreshState()
	}()
	c.vrrRequested = nil
	outputs := make([]unsafe.Pointer, 0, len(c.backendOutputs))
	for pointer := range c.backendOutputs {
		if changed == nil || changed[pointer] {
			outputs = append(outputs, pointer)
		}
	}
	slices.SortFunc(outputs, func(a, b unsafe.Pointer) int { return outputIdentity(a).Compare(outputIdentity(b)) })
	if changed != nil {
		prepared, err := c.prepareOutputConfigs(changed)
		defer freeOutputStates(prepared)
		if err != nil {
			return err
		}
		saved := map[unsafe.Pointer]*C.struct_wlr_output_state{}
		defer freeOutputStates(saved)
		for _, pointer := range outputs {
			state := C.sysc_output_save((*C.struct_wlr_output)(pointer))
			if state == nil {
				return fmt.Errorf("saving output state failed")
			}
			saved[pointer] = state
		}
		if err = c.applyOutputStates(prepared, false); err != nil {
			return errors.Join(err, c.applyOutputStates(saved, false))
		}
		for _, pointer := range outputs {
			if c.outputOff(pointer) {
				if err = c.turnOutputOff(pointer); err != nil {
					return err
				}
			} else if _, adopted := c.sceneOutputs[pointer]; !adopted {
				c.adoptOutput(pointer)
			}
		}
	} else {
		var failures []error
		for _, pointer := range outputs {
			if err := c.applyOutputConfig(pointer, false, false, nil); err != nil {
				failures = append(failures, err)
			}
		}
		if len(failures) > 0 {
			c.placeOutputs()
			return errors.Join(failures...)
		}
	}
	c.placeOutputs()
	return nil
}

func freeOutputStates(states map[unsafe.Pointer]*C.struct_wlr_output_state) {
	for _, state := range states {
		C.sysc_output_saved_free(state)
	}
}

// Test/commit each native backend as a group, so shared bandwidth and CRTC
// constraints are checked together. Failed commits restore exact saved states
// before any output is removed from the workspace model.
func (c *Compositor) applyOutputStates(states map[unsafe.Pointer]*C.struct_wlr_output_state, testOnly bool) error {
	groups := map[*C.struct_wlr_backend][]C.struct_wlr_backend_output_state{}
	for pointer, state := range states {
		output := (*C.struct_wlr_output)(pointer)
		groups[output.backend] = append(groups[output.backend], C.struct_wlr_backend_output_state{output: output, base: *state})
	}
	for backend, group := range groups {
		if !bool(C.wlr_backend_test(backend, &group[0], C.size_t(len(group)))) {
			return fmt.Errorf("backend refused the output configuration")
		}
	}
	if !testOnly {
		for backend, group := range groups {
			if !bool(C.wlr_backend_commit(backend, &group[0], C.size_t(len(group)))) {
				return fmt.Errorf("committing the output configuration failed")
			}
		}
	}
	return nil
}

// applyOutputConfig brings one output to its section's state. A change the
// output refuses falls back as Niri's does: without VRR, then to the preferred
// mode; when even that fails the output keeps its working state.
func (c *Compositor) applyOutputConfig(pointer unsafe.Pointer, testOnly, strict bool, prepared *C.struct_wlr_output_state) error {
	output := (*C.struct_wlr_output)(pointer)
	section := c.outputConfig(pointer)
	if c.outputOff(pointer) {
		if testOnly || strict {
			if !bool(C.sysc_output_disable(output, C.bool(testOnly), prepared)) {
				return fmt.Errorf("output %s refused off", outputName(pointer))
			}
			return nil
		}
		return c.turnOutputOff(pointer)
	}
	// Power is not config: an output that PowerOffMonitors or a power
	// management client turned off stays off, and setOutputPower applies the
	// config when it turns the output back on.
	if _, adopted := c.sceneOutputs[pointer]; adopted && !c.outputIsEnabled(pointer) {
		return nil
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
	want := C.struct_sysc_output_config{mode_index: -1, transform: C.sysc_output_configured_transform(output, C.uint32_t(transform)), adaptive_sync: C.int(adaptiveSync)}
	width, height := int(output.width), int(output.height)
	fallback := false
	if target != nil && target.Custom {
		want.custom_width, want.custom_height = C.int32_t(target.Width), C.int32_t(target.Height)
		want.custom_refresh = C.int32_t(math.Round(target.Refresh * 1000))
		width, height = target.Width, target.Height
	} else {
		index, missed := pickOutputMode(modes, target)
		fallback = missed
		if index >= 0 {
			want.mode_index = C.int(index)
			width, height = modes[index].width, modes[index].height
		}
	}
	if section != nil && section.Modeline != nil {
		width, height = section.Modeline.HDisplay, section.Modeline.VDisplay
	}
	want.scale = C.float(outputScale(scale, int(output.phys_width), int(output.phys_height), width, height))
	var modeline C.drmModeModeInfo
	if section != nil && section.Modeline != nil {
		m := section.Modeline
		modeline.clock = C.uint32_t(math.Round(m.Clock * 1000))
		modeline.hdisplay, modeline.hsync_start, modeline.hsync_end, modeline.htotal = C.uint16_t(m.HDisplay), C.uint16_t(m.HSyncStart), C.uint16_t(m.HSyncEnd), C.uint16_t(m.HTotal)
		modeline.vdisplay, modeline.vsync_start, modeline.vsync_end, modeline.vtotal = C.uint16_t(m.VDisplay), C.uint16_t(m.VSyncStart), C.uint16_t(m.VSyncEnd), C.uint16_t(m.VTotal)
		C.sysc_modeline_name(&modeline)
		if m.HSyncPositive {
			modeline.flags |= C.DRM_MODE_FLAG_PHSYNC
		} else {
			modeline.flags |= C.DRM_MODE_FLAG_NHSYNC
		}
		if m.VSyncPositive {
			modeline.flags |= C.DRM_MODE_FLAG_PVSYNC
		} else {
			modeline.flags |= C.DRM_MODE_FLAG_NVSYNC
		}
		if bool(C.wlr_output_is_drm(output)) {
			want.modeline = (*C.drmModeModeInfo)(C.malloc(C.size_t(unsafe.Sizeof(modeline))))
			if want.modeline == nil {
				return fmt.Errorf("allocating output modeline failed")
			}
			defer C.free(unsafe.Pointer(want.modeline))
			*want.modeline = modeline
		} else {
			want.mode_index = -1
			want.custom_width, want.custom_height = C.int32_t(m.HDisplay), C.int32_t(m.VDisplay)
			want.custom_refresh = C.int32_t(math.Round(m.Clock * 1e9 / float64(m.HTotal*m.VTotal)))
		}
	}
	name := C.GoString(output.name)
	if fallback && (testOnly || strict) {
		return fmt.Errorf("output %s does not list the requested mode", name)
	}
	if testOnly {
		if !bool(C.sysc_output_try_config(output, &want, true, prepared)) {
			return fmt.Errorf("output %s refused the requested state", name)
		}
		return nil
	}
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
	if target != nil || want.modeline != nil {
		preferred := want
		preferred.custom_width, preferred.adaptive_sync = 0, 0
		preferred.modeline = nil
		preferred.mode_index = -1
		if index, _ := pickOutputMode(modes, nil); index >= 0 {
			preferred.mode_index = C.int(index)
		}
		attempts = append(attempts, preferred)
	}
	if strict {
		attempts = attempts[:1]
	}
	applied := false
	for i := range attempts {
		if bool(C.sysc_output_try_config(output, &attempts[i], false, nil)) {
			if i > 0 {
				fmt.Fprintf(os.Stderr, "sysc-compositor: output %s refused its configured state; fallback %d applied\n", name, i)
			}
			applied = true
			break
		}
	}
	if !applied {
		fmt.Fprintf(os.Stderr, "sysc-compositor: output %s refused its configured state; it keeps its current one\n", name)
		return fmt.Errorf("output %s refused its configured state", name)
	}
	if _, adopted := c.sceneOutputs[pointer]; !adopted && !strict {
		c.adoptOutput(pointer)
	}
	return nil
}

// outputOff reports whether an output belongs off: its section says so, or
// the lid policy disables it. The policy is derived on every apply and never
// written into the config, so reload and requests keep the user's sections.
func (c *Compositor) outputOff(pointer unsafe.Pointer) bool {
	if section := c.outputConfig(pointer); section != nil && section.Off {
		return true
	}
	return isLaptopPanel(outputName(pointer)) && c.laptopPanelsDisabled()
}

// laptopPanelsDisabled applies Niri's lid policy to the connected outputs,
// including those the config turned off, as Niri counts every connector.
func (c *Compositor) laptopPanelsDisabled() bool {
	connectors := make([]string, 0, len(c.backendOutputs))
	for pointer := range c.backendOutputs {
		connectors = append(connectors, outputName(pointer))
	}
	return shouldDisableLaptopPanels(c.lidClosed, c.debugConfig.KeepLaptopPanelOnWhenLidIsClosed, connectors)
}

// SetLidClosed records the lid from either source, the input switch or
// login1, and reapplies the output config when it changes, as Niri's
// set_lid_closed does. It runs on the compositor thread.
func (c *Compositor) SetLidClosed(closed bool) {
	if c.lidClosed == closed {
		return
	}
	c.lidClosed = closed
	c.applyOutputConfigs()
}

// turnOutputOff disables an output and takes it out of the layout, as an
// unplugged output leaves it; the output stays known so a later config can
// turn it back on.
func (c *Compositor) turnOutputOff(pointer unsafe.Pointer) error {
	if c.outputIsEnabled(pointer) && !bool(C.sysc_output_disable((*C.struct_wlr_output)(pointer), false, nil)) {
		return fmt.Errorf("output %s: turning it off failed", outputName(pointer))
	}
	if sceneOutput, adopted := c.sceneOutputs[pointer]; adopted {
		c.retireOutput(pointer)
		C.sysc_output_release(c.layout, (*C.struct_wlr_scene_output)(unsafe.Pointer(sceneOutput)), (*C.struct_wlr_output)(pointer))
	}
	return nil
}

func (c *Compositor) prepareOutputConfigs(changed map[unsafe.Pointer]bool) (map[unsafe.Pointer]*C.struct_wlr_output_state, error) {
	states := map[unsafe.Pointer]*C.struct_wlr_output_state{}
	for pointer := range changed {
		state := C.sysc_output_save((*C.struct_wlr_output)(pointer))
		if state == nil {
			return states, fmt.Errorf("allocating an output state failed")
		}
		states[pointer] = state
		if err := c.applyOutputConfig(pointer, true, true, state); err != nil {
			return states, err
		}
	}
	return states, c.applyOutputStates(states, true)
}

func (c *Compositor) testOutputConfigs(changed map[unsafe.Pointer]bool) error {
	states, err := c.prepareOutputConfigs(changed)
	freeOutputStates(states)
	return err
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
