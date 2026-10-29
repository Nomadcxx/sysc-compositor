package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/interfaces/wlr_tablet_pad.h>
#include <wlr/interfaces/wlr_tablet_tool.h>

static const struct wlr_tablet_impl sysc_fake_tablet_impl = { .name = "sysc-fake-tablet" };
static const struct wlr_tablet_pad_impl sysc_fake_pad_impl = { .name = "sysc-fake-pad" };

static struct wlr_tablet *sysc_fake_tablet_create(double width_mm, double height_mm) {
	struct wlr_tablet *tablet = calloc(1, sizeof(*tablet));
	if (tablet != NULL) {
		wlr_tablet_init(tablet, &sysc_fake_tablet_impl, "sysc-fake-tablet");
		tablet->width_mm = width_mm;
		tablet->height_mm = height_mm;
	}
	return tablet;
}

static void sysc_fake_tablet_destroy(struct wlr_tablet *tablet) {
	wlr_tablet_finish(tablet);
	free(tablet);
}

static struct wlr_tablet_tool *sysc_fake_tool_create(void) {
	struct wlr_tablet_tool *tool = calloc(1, sizeof(*tool));
	if (tool != NULL) {
		tool->type = WLR_TABLET_TOOL_TYPE_PEN;
		tool->pressure = true;
		wl_signal_init(&tool->events.destroy);
	}
	return tool;
}

static void sysc_fake_tool_destroy(struct wlr_tablet_tool *tool) {
	wl_signal_emit_mutable(&tool->events.destroy, tool);
	free(tool);
}

static void sysc_fake_tool_proximity(struct wlr_tablet *tablet, struct wlr_tablet_tool *tool,
		uint32_t time, double x, double y, bool in) {
	struct wlr_tablet_tool_proximity_event event = {
		.tablet = tablet, .tool = tool, .time_msec = time, .x = x, .y = y,
		.state = in ? WLR_TABLET_TOOL_PROXIMITY_IN : WLR_TABLET_TOOL_PROXIMITY_OUT,
	};
	wl_signal_emit_mutable(&tablet->events.proximity, &event);
}

static void sysc_fake_tool_axis(struct wlr_tablet *tablet, struct wlr_tablet_tool *tool,
		uint32_t time, double x, double y, double pressure) {
	struct wlr_tablet_tool_axis_event event = {
		.tablet = tablet, .tool = tool, .time_msec = time, .x = x, .y = y,
		.pressure = pressure,
		.updated_axes = WLR_TABLET_TOOL_AXIS_X | WLR_TABLET_TOOL_AXIS_Y |
			WLR_TABLET_TOOL_AXIS_PRESSURE,
	};
	wl_signal_emit_mutable(&tablet->events.axis, &event);
}

static void sysc_fake_tool_tip(struct wlr_tablet *tablet, struct wlr_tablet_tool *tool,
		uint32_t time, double x, double y, bool down) {
	struct wlr_tablet_tool_tip_event event = {
		.tablet = tablet, .tool = tool, .time_msec = time, .x = x, .y = y,
		.state = down ? WLR_TABLET_TOOL_TIP_DOWN : WLR_TABLET_TOOL_TIP_UP,
	};
	wl_signal_emit_mutable(&tablet->events.tip, &event);
}

static void sysc_fake_tool_button(struct wlr_tablet *tablet, struct wlr_tablet_tool *tool,
		uint32_t time, uint32_t button, bool pressed) {
	struct wlr_tablet_tool_button_event event = {
		.tablet = tablet, .tool = tool, .time_msec = time, .button = button,
		.state = pressed ? WLR_BUTTON_PRESSED : WLR_BUTTON_RELEASED,
	};
	wl_signal_emit_mutable(&tablet->events.button, &event);
}

static struct wlr_tablet_pad *sysc_fake_pad_create(size_t buttons) {
	struct wlr_tablet_pad *pad = calloc(1, sizeof(*pad));
	if (pad != NULL) {
		wlr_tablet_pad_init(pad, &sysc_fake_pad_impl, "sysc-fake-pad");
		pad->button_count = buttons;
	}
	return pad;
}

static void sysc_fake_pad_destroy(struct wlr_tablet_pad *pad) {
	wlr_tablet_pad_finish(pad);
	free(pad);
}

// A pad with no button groups: the group index is past its end, which the
// compositor must not pass on to wlroots' mode lookup.
static void sysc_fake_pad_button(struct wlr_tablet_pad *pad, uint32_t time,
		uint32_t button, bool pressed) {
	struct wlr_tablet_pad_button_event event = {
		.time_msec = time, .button = button,
		.state = pressed ? WLR_BUTTON_PRESSED : WLR_BUTTON_RELEASED,
	};
	wl_signal_emit_mutable(&pad->events.button, &event);
}
*/
import "C"

import "unsafe"

// fakeTablet is a tablet with one pen, mapped over the whole layout unless
// the tablet config names an output.
type fakeTablet struct {
	c      *Compositor
	tablet *C.struct_wlr_tablet
	tool   *C.struct_wlr_tablet_tool
}

func (c *Compositor) addFakeTablet(widthMM, heightMM float64) *fakeTablet {
	tablet := C.sysc_fake_tablet_create(C.double(widthMM), C.double(heightMM))
	tool := C.sysc_fake_tool_create()
	if tablet == nil || tool == nil {
		return nil
	}
	c.handleNewInput(unsafe.Pointer(&tablet.base))
	return &fakeTablet{c: c, tablet: tablet, tool: tool}
}

// device is a layout point as the unmapped tablet reports it.
func (f *fakeTablet) device(x, y float64) (C.double, C.double) {
	box := f.c.layoutBox()
	return C.double((x - float64(box.x)) / float64(box.w)), C.double((y - float64(box.y)) / float64(box.h))
}

func (f *fakeTablet) proximity(x, y float64, in bool) {
	dx, dy := f.device(x, y)
	C.sysc_fake_tool_proximity(f.tablet, f.tool, nowMsec(), dx, dy, C.bool(in))
}

func (f *fakeTablet) moveTo(x, y float64) {
	dx, dy := f.device(x, y)
	C.sysc_fake_tool_axis(f.tablet, f.tool, nowMsec(), dx, dy, 0.5)
}

func (f *fakeTablet) tip(down bool) {
	x, y := float64(f.c.cursor.x), float64(f.c.cursor.y)
	dx, dy := f.device(x, y)
	C.sysc_fake_tool_tip(f.tablet, f.tool, nowMsec(), dx, dy, C.bool(down))
}

func (f *fakeTablet) button(button uint32, pressed bool) {
	C.sysc_fake_tool_button(f.tablet, f.tool, nowMsec(), C.uint32_t(button), C.bool(pressed))
}

// remove unplugs the tablet, then retires its pen.
func (f *fakeTablet) remove() {
	if f.tablet != nil {
		C.sysc_fake_tablet_destroy(f.tablet)
		f.tablet = nil
	}
	if f.tool != nil {
		C.sysc_fake_tool_destroy(f.tool)
		f.tool = nil
	}
}

type fakePad struct{ pad *C.struct_wlr_tablet_pad }

func (c *Compositor) addFakePad(buttons int) *fakePad {
	pad := C.sysc_fake_pad_create(C.size_t(buttons))
	if pad == nil {
		return nil
	}
	c.handleNewInput(unsafe.Pointer(&pad.base))
	return &fakePad{pad: pad}
}

func (f *fakePad) button(button uint32, pressed bool) {
	C.sysc_fake_pad_button(f.pad, nowMsec(), C.uint32_t(button), C.bool(pressed))
}

func (f *fakePad) remove() {
	if f.pad != nil {
		C.sysc_fake_pad_destroy(f.pad)
		f.pad = nil
	}
}
