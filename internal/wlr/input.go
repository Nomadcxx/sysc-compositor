package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/backend.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xcursor_manager.h>

static struct wlr_xcursor_manager *sysc_xcursor_manager(void) {
	return wlr_xcursor_manager_create(NULL, 24);
}

// The field is named `type`, which is a Go keyword, so cgo cannot expose
// struct_wlr_input_device.type directly. One C accessor is the whole fix.
static int sysc_device_type(struct wlr_input_device *device) {
	return (int)device->type;
}

// A cursor with no image is an invisible cursor, so the theme is loaded once
// and the left_ptr image is what the seat draws.
static bool sysc_load_cursor_image(struct wlr_cursor *cursor,
		struct wlr_xcursor_manager *manager) {
	if (!wlr_xcursor_manager_load(manager, 1.0f)) {
		return false;
	}
	// 0.20 takes the manager and a name, not a resolved xcursor, and returns
	// nothing; the load above is the only fallible step.
	wlr_cursor_set_xcursor(cursor, manager, "left_ptr");
	return true;
}
*/
import "C"

import "unsafe"

// handleNewInput assigns a backend input device to the single M0 seat.
//
// wlroots 0.20 does not wire devices to a seat for you: the compositor reads
// new_input and assigns by device type.
//
// The two device kinds are NOT symmetric in 0.20, and that is worth stating
// because it is the first hard evidence for the Tier 3 viability note. A
// keyboard has an explicit setter, wlr_seat_set_keyboard. A pointer has none:
// there is no wlr_seat_set_pointer, and wlr_scene exposes no routing either, so
// the cursor is attached to the layout and the compositor feeds the seat by
// calling the wlr_seat_pointer_notify_* functions itself. wlroots 0.20 has one
// interesting seat and leaves everything else to the compositor.
func (c *Compositor) handleNewInput(data unsafe.Pointer) {
	device := (*C.struct_wlr_input_device)(data)
	switch C.sysc_device_type(device) {
	case C.WLR_INPUT_DEVICE_KEYBOARD:
		kb := C.wlr_keyboard_from_input_device(device)
		if kb == nil {
			return
		}
		C.wlr_seat_set_keyboard(c.seat, kb)
		c.keyboardDevices[unsafe.Pointer(device)] = true
	case C.WLR_INPUT_DEVICE_POINTER:
		if c.xcursorManager == nil {
			c.xcursorManager = C.sysc_xcursor_manager()
			if c.xcursorManager == nil {
				return
			}
			C.sysc_load_cursor_image(c.cursor, c.xcursorManager)
		}
		C.wlr_cursor_attach_input_device(c.cursor, device)
		c.pointerDevices[unsafe.Pointer(device)] = true
	default:
		return
	}
	c.refreshCapabilities()
}

// refreshCapabilities advertises exactly what is attached. Advertising a
// capability no device backs is how a seat ends up promising input it cannot
// deliver.
func (c *Compositor) refreshCapabilities() {
	var caps C.uint32_t
	if len(c.pointerDevices) > 0 {
		caps |= C.WL_SEAT_CAPABILITY_POINTER
	}
	if len(c.keyboardDevices) > 0 {
		caps |= C.WL_SEAT_CAPABILITY_KEYBOARD
	}
	C.wlr_seat_set_capabilities(c.seat, caps)
	c.capabilities = int(caps)
}

// SeatCapabilities reports the capability bitmask the seat advertises. A
// zero-valued seat is a valid M0 state, not an error: a nested session with no
// forwarded input devices still has to serve clients.
func (c *Compositor) SeatCapabilities() int { return c.capabilities }
