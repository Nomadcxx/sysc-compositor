package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_primary_selection.h>
#include <wlr/types/wlr_seat.h>
*/
import "C"

import "unsafe"

// wlroots routes every selection request, from wl_data_device, primary
// selection and data control alike, through the seat's request events and
// leaves the decision to the compositor. As in Niri and tinywl, the user
// seat takes every request it is given (sysc-1645).
func (c *Compositor) watchSelectionRequests() {
	c.keep(
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
