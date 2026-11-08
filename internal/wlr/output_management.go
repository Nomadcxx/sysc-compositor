package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_output_management_v1.h>
#include <wlr/types/wlr_output_power_management_v1.h>
#include <wlr/types/wlr_output_layout.h>

static struct wlr_output_configuration_head_v1 *sysc_output_config_first(
		struct wlr_output_configuration_v1 *config) {
	if (wl_list_empty(&config->heads)) return NULL;
	return wl_container_of(config->heads.next, (struct wlr_output_configuration_head_v1 *)NULL, link);
}
static struct wlr_output_configuration_head_v1 *sysc_output_config_next(
		struct wlr_output_configuration_head_v1 *head) {
	if (head->link.next == &head->config->heads) return NULL;
	return wl_container_of(head->link.next, (struct wlr_output_configuration_head_v1 *)NULL, link);
}
static void sysc_output_publish_head(struct wlr_output_configuration_v1 *config,
		struct wlr_output_layout *layout, struct wlr_output *output, bool enabled) {
	struct wlr_output_configuration_head_v1 *head = wlr_output_configuration_head_v1_create(config, output);
	if (head == NULL) return;
	head->state.enabled = enabled;
	struct wlr_box box;
	wlr_output_layout_get_box(layout, output, &box);
	head->state.x = box.x;
	head->state.y = box.y;
}
*/
import "C"

import (
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func (c *Compositor) createOutputManagement() bool {
	c.outputManager = C.wlr_output_manager_v1_create(c.display)
	power := C.wlr_output_power_manager_v1_create(c.display)
	if c.outputManager == nil || power == nil {
		return false
	}
	c.keep(
		AddListener(unsafe.Pointer(&c.outputManager.events.test), func(data unsafe.Pointer) { c.handleOutputConfiguration(data, true) }),
		AddListener(unsafe.Pointer(&c.outputManager.events.apply), func(data unsafe.Pointer) { c.handleOutputConfiguration(data, false) }),
		AddListener(unsafe.Pointer(&power.events.set_mode), c.handleOutputPowerSetMode),
	)
	return c.startOutputProtocolEdits()
}

func (c *Compositor) publishOutputConfiguration() {
	if c.outputManager == nil || c.applyingOutputConfigs {
		return
	}
	state := C.wlr_output_configuration_v1_create()
	if state == nil {
		return
	}
	for pointer := range c.backendOutputs {
		_, enabled := c.sceneOutputs[pointer]
		C.sysc_output_publish_head(state, c.layout, (*C.struct_wlr_output)(pointer), C.bool(enabled))
	}
	C.wlr_output_manager_v1_set_configuration(c.outputManager, state)
}

func (c *Compositor) handleOutputConfiguration(data unsafe.Pointer, testOnly bool) {
	requested := (*C.struct_wlr_output_configuration_v1)(data)
	edits := c.outputProtocolEdits[requested]
	defer delete(c.outputProtocolEdits, requested)
	defer C.wlr_output_configuration_v1_destroy(requested)
	var changes []config.OutputConfig
	for head := C.sysc_output_config_first(requested); head != nil; head = C.sysc_output_config_next(head) {
		state := head.state
		pointer := unsafe.Pointer(state.output)
		if _, known := c.backendOutputs[pointer]; !known {
			C.wlr_output_configuration_v1_send_failed(requested)
			return
		}
		section := config.OutputConfig{Name: outputName(pointer)}
		if current := c.outputConfig(pointer); current != nil {
			section = *current
		}
		section.Off = !bool(state.enabled)
		if !section.Off {
			scale := float64(state.scale)
			if scale <= 0 || scale > 10 || state.transform > 7 {
				C.wlr_output_configuration_v1_send_failed(requested)
				return
			}
			section.Scale, section.Transform = &scale, config.Transform(state.transform)
			output := (*C.struct_wlr_output)(pointer)
			if state.mode != output.current_mode || (state.mode == nil && (state.custom_mode.width != output.width || state.custom_mode.height != output.height || state.custom_mode.refresh != output.refresh)) {
				section.Modeline = nil
			}
			section.Position = &config.OutputPosition{X: int(state.x), Y: int(state.y)}
			if state.mode != nil {
				section.Mode = &config.OutputMode{Width: int(state.mode.width), Height: int(state.mode.height), Refresh: float64(state.mode.refresh) / 1000}
			} else {
				if state.custom_mode.width <= 0 || state.custom_mode.height <= 0 || state.custom_mode.width > 65535 || state.custom_mode.height > 65535 || state.custom_mode.refresh < 0 {
					C.wlr_output_configuration_v1_send_failed(requested)
					return
				}
				section.Mode = &config.OutputMode{Width: int(state.custom_mode.width), Height: int(state.custom_mode.height), Refresh: float64(state.custom_mode.refresh) / 1000, Custom: true}
			}
			if edits != nil {
				if enabled, specified := edits.adaptive[unsafe.Pointer(head)]; specified {
					section.VRR = nil
					if enabled {
						section.VRR = &config.VRR{}
					}
				}
			}
		}
		changes = append(changes, section)
	}
	if err := c.ConfigureOutputs(changes, testOnly); err != nil {
		C.wlr_output_configuration_v1_send_failed(requested)
	} else {
		C.wlr_output_configuration_v1_send_succeeded(requested)
	}
}
