package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_cursor_shape_v1.h>
#include <wlr/types/wlr_seat.h>

// A shape request counts only from the client the user seat's pointer is
// over, for its pointer: an injected seat's clients and tablet tools do not
// steer the drawn pointer.
static bool sysc_cursor_shape_request_valid(struct wlr_seat *seat,
		struct wlr_cursor_shape_manager_v1_request_set_shape_event *event) {
	return event->device_type == WLR_CURSOR_SHAPE_MANAGER_V1_DEVICE_TYPE_POINTER &&
		event->seat_client == seat->pointer_state.focused_client;
}
*/
import "C"

import "unsafe"

// cursorShapeVersion is the wp_cursor_shape_manager_v1 version advertised.
const cursorShapeVersion = 1

func (c *Compositor) createCursorShape() bool {
	manager := C.wlr_cursor_shape_manager_v1_create(c.display, cursorShapeVersion)
	if manager == nil {
		return false
	}
	c.keep(AddListener(unsafe.Pointer(&manager.events.request_set_shape), c.handleRequestSetShape))
	return true
}

// handleRequestSetShape draws the named theme image a focused client asked
// for, in place of any cursor surface it had set.
func (c *Compositor) handleRequestSetShape(data unsafe.Pointer) {
	event := (*C.struct_wlr_cursor_shape_manager_v1_request_set_shape_event)(data)
	if !bool(C.sysc_cursor_shape_request_valid(c.seat, event)) {
		return
	}
	if c.cursorSurfaceDestroy != nil {
		c.drop(c.cursorSurfaceDestroy)
		c.cursorSurfaceDestroy = nil
	}
	c.cursorSurface, c.cursorClientSet = nil, false
	c.cursorShape = C.GoString(C.wlr_cursor_shape_v1_name(event.shape))
	if c.pointerGrab == nil {
		c.showCursorImage(c.cursorShape)
	}
}
