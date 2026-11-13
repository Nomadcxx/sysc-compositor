package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_primary_selection.h>
#include <wlr/types/wlr_seat.h>

static void sysc_cancel_native_drag(struct wlr_seat *seat) {
	if (seat != NULL && seat->drag != NULL) {
		struct wlr_seat_keyboard_grab *grab = &seat->drag->keyboard_grab;
		grab->interface->cancel(grab);
	}
}
*/
import "C"

import "unsafe"

func (c *Compositor) cancelNativeDrag() { C.sysc_cancel_native_drag(c.seat) }

// wlroots routes every selection request, from wl_data_device, primary
// selection and data control alike, through the seat's request events and
// leaves the decision to the compositor. As in Niri and tinywl, the user
// seat takes every request it is given (sysc-1645).
func (c *Compositor) watchSelectionRequests() {
	c.keep(
		AddListener(unsafe.Pointer(&c.seat.events.request_start_drag), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_seat_request_start_drag_event)(data)
			var point *C.struct_wlr_touch_point
			switch {
			case !c.sessionLocked && bool(C.wlr_seat_validate_pointer_grab_serial(c.seat, event.origin, event.serial)):
				C.wlr_seat_start_pointer_drag(c.seat, event.drag, event.serial)
			case !c.sessionLocked && bool(C.wlr_seat_validate_touch_grab_serial(c.seat, event.origin, event.serial, &point)):
				C.wlr_seat_start_touch_drag(c.seat, event.drag, event.serial, point)
			default:
				if event.drag.source != nil {
					C.wlr_data_source_destroy(event.drag.source)
				}
			}
		}),
		AddListener(unsafe.Pointer(&c.seat.events.request_set_selection), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_seat_request_set_selection_event)(data)
			C.wlr_seat_set_selection(c.seat, event.source, event.serial)
		}),
		AddListener(unsafe.Pointer(&c.seat.events.request_set_primary_selection), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_seat_request_set_primary_selection_event)(data)
			C.wlr_seat_set_primary_selection(c.seat, event.source, event.serial)
		}),
	)
}
