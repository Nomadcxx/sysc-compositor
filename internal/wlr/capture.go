package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/backend.h>
#include <wlr/types/wlr_alpha_modifier_v1.h>
#include <wlr/types/wlr_data_control_v1.h>
#include <wlr/types/wlr_ext_data_control_v1.h>
#include <wlr/types/wlr_ext_image_capture_source_v1.h>
#include <wlr/types/wlr_ext_image_copy_capture_v1.h>
#include <wlr/types/wlr_presentation_time.h>
#include <wlr/types/wlr_screencopy_v1.h>
#include <wlr/types/wlr_single_pixel_buffer_v1.h>

// The M11 capture and clipboard globals (owner decision Q9). wlroots and the
// SceneFX scene implement each one behind its global: data control serves
// the seat's selection, screencopy and image-copy-capture read back output
// commits (wlroots refuses a buffer that is not exactly the frame's size, as
// Niri does), and the scene answers presentation feedback and applies
// alpha-modifier and single-pixel buffers.
static bool sysc_capture_globals_create(struct wl_display *display, struct wlr_backend *backend) {
	return wlr_ext_data_control_manager_v1_create(display, 1) != NULL &&
		wlr_data_control_manager_v1_create(display) != NULL &&
		wlr_screencopy_manager_v1_create(display) != NULL &&
		wlr_ext_image_copy_capture_manager_v1_create(display, 1) != NULL &&
		wlr_ext_output_image_capture_source_manager_v1_create(display, 1) != NULL &&
		wlr_presentation_create(display, backend, 2) != NULL &&
		wlr_single_pixel_buffer_manager_v1_create(display) != NULL &&
		wlr_alpha_modifier_v1_create(display) != NULL;
}
*/
import "C"

func (c *Compositor) createCaptureGlobals() bool {
	return bool(C.sysc_capture_globals_create(c.display, c.backend))
}
