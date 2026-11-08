package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_scene.h>
struct sysc_vrr_visible { struct wlr_output *output; bool visible; };
static void sysc_vrr_buffer(struct wlr_scene_buffer *buffer, int x, int y, void *data) {
	struct sysc_vrr_visible *check = data;
	if (wlr_scene_surface_try_from_buffer(buffer) != NULL && buffer->primary_output != NULL && buffer->primary_output->output == check->output)
		check->visible = true;
}
static bool sysc_vrr_surface_visible(struct wlr_scene_tree *tree, struct wlr_output *output) {
	struct sysc_vrr_visible check = { .output = output };
	wlr_scene_node_for_each_buffer(&tree->node, sysc_vrr_buffer, &check);
	return check.visible;
}
static bool sysc_output_vrr(struct wlr_output *output, bool enabled) {
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	wlr_output_state_set_adaptive_sync_enabled(&state, enabled);
	bool ok = wlr_output_test_state(output,&state) && wlr_output_commit_state(output,&state);
	wlr_output_state_finish(&state);
	return ok;
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// outputWantsVRR follows Niri's refresh_on_demand_vrr: a rule must ask for
// VRR and one of that window's surfaces must have this primary scanout output.
func (c *Compositor) outputWantsVRR(pointer unsafe.Pointer) bool {
	section := c.outputConfig(pointer)
	if section == nil || section.VRR == nil {
		return false
	}
	if !section.VRR.OnDemand {
		return true
	}
	for _, view := range c.views {
		if view.mapped && view.tree != nil && view.appearance.VariableRefreshRate != nil && *view.appearance.VariableRefreshRate && bool(C.sysc_vrr_surface_visible(view.tree, (*C.struct_wlr_output)(pointer))) {
			return true
		}
	}
	return false
}

func (c *Compositor) syncOutputVRR(pointer unsafe.Pointer) {
	output := (*C.struct_wlr_output)(pointer)
	if !bool(output.enabled) || !bool(output.adaptive_sync_supported) {
		return
	}
	want := c.outputWantsVRR(pointer)
	if previous, ok := c.vrrRequested[pointer]; ok && previous == want {
		return
	}
	if c.vrrRequested == nil {
		c.vrrRequested = map[unsafe.Pointer]bool{}
	}
	c.vrrRequested[pointer] = want
	if want == (output.adaptive_sync_status == C.WLR_OUTPUT_ADAPTIVE_SYNC_ENABLED) {
		return
	}
	if !bool(C.sysc_output_vrr(output, C.bool(want))) {
		fmt.Fprintf(os.Stderr, "sysc-compositor: output %s refused VRR=%t\n", outputName(pointer), want)
		return
	}
	c.publishOutputConfiguration()
	c.refreshState()
}
