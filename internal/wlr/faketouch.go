package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <stdint.h>
#include <wlr/interfaces/wlr_touch.h>
#include <wlr/types/wlr_touch.h>

static const struct wlr_touch_impl sysc_fake_touch_impl = {
	.name = "sysc-fake-touch",
};

static struct wlr_touch *sysc_fake_touch_create(void) {
	struct wlr_touch *touch = calloc(1, sizeof(*touch));
	if (touch != NULL) {
		wlr_touch_init(touch, &sysc_fake_touch_impl, "sysc-fake-touch");
	}
	return touch;
}

static void sysc_fake_touch_destroy(struct wlr_touch *touch) {
	wlr_touch_finish(touch);
	free(touch);
}

// A finger going down, at a position normalized to [0, 1] over the glass.
static void sysc_fake_touch_down(struct wlr_touch *touch, uint32_t time, int32_t id,
		double x, double y) {
	struct wlr_touch_down_event event = {
		.touch = touch,
		.time_msec = time,
		.touch_id = id,
		.x = x,
		.y = y,
	};
	wl_signal_emit_mutable(&touch->events.down, &event);
}

static void sysc_fake_touch_motion(struct wlr_touch *touch, uint32_t time, int32_t id,
		double x, double y) {
	struct wlr_touch_motion_event event = {
		.touch = touch,
		.time_msec = time,
		.touch_id = id,
		.x = x,
		.y = y,
	};
	wl_signal_emit_mutable(&touch->events.motion, &event);
}

static void sysc_fake_touch_up(struct wlr_touch *touch, uint32_t time, int32_t id) {
	struct wlr_touch_up_event event = {
		.touch = touch,
		.time_msec = time,
		.touch_id = id,
	};
	wl_signal_emit_mutable(&touch->events.up, &event);
}

static void sysc_fake_touch_cancel(struct wlr_touch *touch, uint32_t time, int32_t id) {
	struct wlr_touch_cancel_event event = {
		.touch = touch,
		.time_msec = time,
		.touch_id = id,
	};
	wl_signal_emit_mutable(&touch->events.cancel, &event);
}

static void sysc_fake_touch_frame(struct wlr_touch *touch) {
	wl_signal_emit_mutable(&touch->events.frame, touch);
}
*/
import "C"

import "unsafe"

// fakeTouch is an in-process touch screen attached to the user seat the way a
// backend touch screen is, for tests: the nested backend forwards the host's
// devices, and a test cannot produce a touch with them.
type fakeTouch struct {
	c     *Compositor
	touch *C.struct_wlr_touch
}

func (c *Compositor) addFakeTouch() *fakeTouch {
	touch := C.sysc_fake_touch_create()
	if touch == nil {
		return nil
	}
	c.handleNewInput(unsafe.Pointer(&touch.base))
	return &fakeTouch{c: c, touch: touch}
}

// down puts a finger on the glass at a position normalized to [0, 1].
func (f *fakeTouch) down(id int32, x, y float64) {
	C.sysc_fake_touch_down(f.touch, nowMsec(), C.int32_t(id), C.double(x), C.double(y))
}

// motion moves a finger to a position normalized to [0, 1].
func (f *fakeTouch) motion(id int32, x, y float64) {
	C.sysc_fake_touch_motion(f.touch, nowMsec(), C.int32_t(id), C.double(x), C.double(y))
}

// up lifts a finger.
func (f *fakeTouch) up(id int32) {
	C.sysc_fake_touch_up(f.touch, nowMsec(), C.int32_t(id))
}

// cancel ends every finger.
func (f *fakeTouch) cancel(id int32) {
	C.sysc_fake_touch_cancel(f.touch, nowMsec(), C.int32_t(id))
}

// frame ends a frame of touch events.
func (f *fakeTouch) frame() {
	C.sysc_fake_touch_frame(f.touch)
}

func (f *fakeTouch) remove() {
	if f.touch != nil {
		C.sysc_fake_touch_destroy(f.touch)
		f.touch = nil
	}
}
