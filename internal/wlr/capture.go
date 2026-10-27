package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/backend.h>
#include <wlr/types/wlr_alpha_modifier_v1.h>
#include <wlr/types/wlr_data_control_v1.h>
#include <wlr/types/wlr_ext_data_control_v1.h>
#include <wlr/types/wlr_ext_image_copy_capture_v1.h>
#include <wlr/types/wlr_presentation_time.h>
#include <wlr/types/wlr_single_pixel_buffer_v1.h>

// wlroots owns clipboard/presentation globals and the image-copy frame engine.
// Capture source factories and legacy screencopy use the compositor's privacy
// rendering path in window_capture.go.
static bool sysc_capture_globals_create(struct wl_display *display, struct wlr_backend *backend) {
	return wlr_ext_data_control_manager_v1_create(display, 1) != NULL &&
		wlr_data_control_manager_v1_create(display) != NULL &&
		wlr_presentation_create(display, backend, 2) != NULL &&
		wlr_single_pixel_buffer_manager_v1_create(display) != NULL &&
		wlr_alpha_modifier_v1_create(display) != NULL;
}
*/
import "C"

func (c *Compositor) createCaptureGlobals() bool {
	if !bool(C.sysc_capture_globals_create(c.display, c.backend)) {
		return false
	}
	manager := C.wlr_ext_image_copy_capture_manager_v1_create(c.display, 1)
	return manager != nil && c.initCaptureProtocol(manager)
}
