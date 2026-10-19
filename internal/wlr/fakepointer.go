package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/interfaces/wlr_pointer.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_pointer.h>

static const struct wlr_pointer_impl sysc_fake_pointer_impl = {
	.name = "sysc-fake-pointer",
};

static struct wlr_pointer *sysc_fake_pointer_create(void) {
	struct wlr_pointer *pointer = calloc(1, sizeof(*pointer));
	if (pointer != NULL) {
		wlr_pointer_init(pointer, &sysc_fake_pointer_impl, "sysc-fake-pointer");
	}
	return pointer;
}

static void sysc_fake_pointer_destroy(struct wlr_pointer *pointer) {
	wlr_pointer_finish(pointer);
	free(pointer);
}

// An absolute position in [0, 1] over the whole layout, as a tablet or a
// nested backend reports one, then the frame that ends the event group.
static void sysc_fake_pointer_warp(struct wlr_pointer *pointer, uint32_t time, double x, double y) {
	struct wlr_pointer_motion_absolute_event event = {
		.pointer = pointer,
		.time_msec = time,
		.x = x,
		.y = y,
	};
	wl_signal_emit_mutable(&pointer->events.motion_absolute, &event);
	wl_signal_emit_mutable(&pointer->events.frame, pointer);
}

static void sysc_fake_pointer_button(struct wlr_pointer *pointer, uint32_t time,
		uint32_t button, bool pressed) {
	struct wlr_pointer_button_event event = {
		.pointer = pointer,
		.time_msec = time,
		.button = button,
		.state = pressed ? WL_POINTER_BUTTON_STATE_PRESSED : WL_POINTER_BUTTON_STATE_RELEASED,
	};
	wlr_pointer_notify_button(pointer, &event);
	wl_signal_emit_mutable(&pointer->events.frame, pointer);
}
*/
import "C"

import "unsafe"

// fakePointer is an in-process pointer attached to the user seat the way a
// backend pointer is, for tests: the nested backend's one pointer is the
// host's, and a test cannot move it.
type fakePointer struct {
	c       *Compositor
	pointer *C.struct_wlr_pointer
}

func (c *Compositor) addFakePointer() *fakePointer {
	// The nested Wayland backend forwards host pointer motion. Let this fake
	// device own cursor position during scripted input tests.
	for device := range c.pointerDevices {
		C.wlr_cursor_detach_input_device(c.cursor, (*C.struct_wlr_input_device)(device))
		delete(c.pointerDevices, device)
	}
	pointer := C.sysc_fake_pointer_create()
	if pointer == nil {
		return nil
	}
	c.handleNewInput(unsafe.Pointer(&pointer.base))
	return &fakePointer{c: c, pointer: pointer}
}

// moveTo moves the drawn pointer to layout coordinates.
func (f *fakePointer) moveTo(x, y float64) {
	box := f.c.layoutBox()
	C.sysc_fake_pointer_warp(f.pointer, nowMsec(),
		C.double((x-float64(box.x))/float64(box.w)), C.double((y-float64(box.y))/float64(box.h)))
}

func (f *fakePointer) button(button uint32, pressed bool) {
	C.sysc_fake_pointer_button(f.pointer, nowMsec(), C.uint32_t(button), C.bool(pressed))
}

func (f *fakePointer) remove() {
	if f.pointer != nil {
		C.sysc_fake_pointer_destroy(f.pointer)
		f.pointer = nil
	}
}

// userPointerFocus is the surface the user seat's pointer is on, for tests.
func (c *Compositor) userPointerFocus() unsafe.Pointer {
	return unsafe.Pointer(c.seat.pointer_state.focused_surface)
}
