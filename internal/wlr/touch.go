package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <stdint.h>
#include <wayland-server-core.h>
#include <wayland-util.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_touch.h>

// The first output the layout holds, in the order the outputs were added.
// Niri's output_for_touch falls back to the same output.
static struct wlr_output *sysc_layout_first_output(struct wlr_output_layout *layout) {
	struct wlr_output_layout_output *l_output;
	wl_list_for_each(l_output, &layout->outputs, link) {
		return l_output->output;
	}
	return NULL;
}

// The output's logical box after its transform, and the transform itself.
// The box is the output's size with the sides swapped for a quarter turn,
// which is the size a touch position is scaled to.
static void sysc_touch_output_box(struct wlr_output_layout *layout, struct wlr_output *output,
		int32_t *x, int32_t *y, int32_t *width, int32_t *height, uint32_t *transform) {
	struct wlr_box box;
	wlr_output_layout_get_box(layout, output, &box);
	*x = box.x;
	*y = box.y;
	*width = box.width;
	*height = box.height;
	*transform = output->transform;
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// touchDevice is one touch screen on the user seat.
//
// wlroots 0.20 delivers touch on the device's own signals, with the position
// normalized to [0, 1] over the glass. sysc reads them there and does the
// mapping itself, so the output transform is applied once and in one place.
type touchDevice struct {
	device    *C.struct_wlr_input_device
	wlr       *C.struct_wlr_touch
	listeners []*Listener
}

// touchPoint is one finger on the glass. The offsets keep the finger on the
// surface it went down on, so the stroke stays with it (the implicit grab)
// wherever it moves.
type touchPoint struct {
	id      int32
	surface *C.struct_wlr_surface
	xOffset C.double
	yOffset C.double
	gone    *Listener
}

// addTouch puts a touch screen on the user seat.
func (c *Compositor) addTouch(device *C.struct_wlr_input_device) bool {
	touch := C.wlr_touch_from_input_device(device)
	if touch == nil {
		return false
	}
	t := &touchDevice{device: device, wlr: touch}
	t.listeners = []*Listener{
		AddListener(unsafe.Pointer(&touch.events.down), c.handleTouchDown),
		AddListener(unsafe.Pointer(&touch.events.motion), c.handleTouchMotion),
		AddListener(unsafe.Pointer(&touch.events.up), c.handleTouchUp),
		AddListener(unsafe.Pointer(&touch.events.cancel), c.handleTouchCancel),
		AddListener(unsafe.Pointer(&touch.events.frame), c.handleTouchFrame),
	}
	c.keep(t.listeners...)
	c.touchDevices[unsafe.Pointer(device)] = t
	fmt.Fprintf(os.Stderr, "sysc-compositor: touch device added: %s\n", touchDeviceName(device))
	return true
}

// touchDeviceName is the name the device reports, for the log.
func touchDeviceName(device *C.struct_wlr_input_device) string {
	if device == nil || device.name == nil {
		return "unknown"
	}
	return C.GoString(device.name)
}

// removeTouch takes a touch screen off the seat. Every finger on the glass
// ends with it.
func (c *Compositor) removeTouch(device unsafe.Pointer) {
	touch := c.touchDevices[device]
	if touch == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "sysc-compositor: touch device removed: %s\n", touchDeviceName(touch.device))
	c.endTouchPoints(nowMsec())
	for _, listener := range touch.listeners {
		c.drop(listener)
	}
	touch.listeners = nil
	delete(c.touchDevices, device)
}

// touchOutput is the output a touch belongs to: the output the device names,
// else the configured output, else the first output the layout holds. That is
// Niri's order, where the device's own output comes first and
// output_for_touch is the fallback.
func (c *Compositor) touchOutput(device *C.struct_wlr_touch) *C.struct_wlr_output {
	if device != nil && device.output_name != nil {
		name := C.GoString(device.output_name)
		for pointer := range c.sceneOutputs {
			output := (*C.struct_wlr_output)(pointer)
			if c.outputIsEnabled(pointer) && outputNameMatches(name,
				C.GoString(output.name), cString(output.make), cString(output.model), cString(output.serial)) {
				return output
			}
		}
	}
	if c.touchConfig.MapToOutput != "" {
		for pointer := range c.sceneOutputs {
			output := (*C.struct_wlr_output)(pointer)
			if c.outputIsEnabled(pointer) && outputNameMatches(c.touchConfig.MapToOutput,
				C.GoString(output.name), cString(output.make), cString(output.model), cString(output.serial)) {
				return output
			}
		}
	}
	output := C.sysc_layout_first_output(c.layout)
	if output == nil || !bool(output.enabled) {
		return nil
	}
	return output
}

// touchLayoutPoint maps the device position, normalized to [0, 1] over the
// glass, into layout coordinates: the output's logical box after its
// transform, with the position turned by that transform.
func (c *Compositor) touchLayoutPoint(device *C.struct_wlr_touch, x, y float64) (float64, float64) {
	output := c.touchOutput(device)
	if output == nil {
		return 0, 0
	}
	var bx, by, bw, bh C.int32_t
	var transform C.uint32_t
	C.sysc_touch_output_box(c.layout, output, &bx, &by, &bw, &bh, &transform)
	tx, ty := touchTransformPoint(x, y, int(transform))
	return float64(bx) + float64(bw)*tx, float64(by) + float64(bh)*ty
}

// touchOutputBox is the box a touch position is scaled to, with the output's
// transform.
func (c *Compositor) touchOutputBox() (rect, int) {
	output := c.touchOutput(nil)
	if output == nil {
		return c.layoutBox(), 0
	}
	var bx, by, bw, bh C.int32_t
	var transform C.uint32_t
	C.sysc_touch_output_box(c.layout, output, &bx, &by, &bw, &bh, &transform)
	return rect{int(bx), int(by), int(bw), int(bh)}, int(transform)
}

// handleTouchDown starts a finger on the surface under it.
func (c *Compositor) handleTouchDown(data unsafe.Pointer) {
	event := (*C.struct_wlr_touch_down_event)(data)
	touch := event.touch
	if c.touchDevices[unsafe.Pointer(&touch.base)] == nil {
		return
	}
	if c.touchConfig.Off {
		fmt.Fprintf(os.Stderr, "sysc-compositor: touch down dropped: input.touch is off\n")
		return
	}
	if c.sessionLocked {
		fmt.Fprintf(os.Stderr, "sysc-compositor: touch down dropped: the session is locked\n")
		return
	}
	if c.modalUIOpen() {
		fmt.Fprintf(os.Stderr, "sysc-compositor: touch down dropped: a modal surface is open\n")
		return
	}
	lx, ly := c.touchLayoutPoint(touch, float64(event.x), float64(event.y))
	surface, sx, sy := c.surfaceAt(lx, ly)
	if surface == nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: touch down dropped: no surface under the finger\n")
		return
	}
	if !bool(C.wlr_surface_accepts_touch(surface, c.seat)) {
		// The client did not ask for touch. Niri would deliver it anyway;
		// sysc drops it rather than emulating a mouse.
		fmt.Fprintf(os.Stderr, "sysc-compositor: touch down dropped: the client did not ask for touch\n")
		return
	}
	point := &touchPoint{
		id:      int32(event.touch_id),
		surface: surface,
		xOffset: C.double(lx) - sx,
		yOffset: C.double(ly) - sy,
	}
	point.gone = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
		c.touchSurfaceGone(point)
	})
	c.keep(point.gone)
	c.touchPoints[int32(event.touch_id)] = point
	// The touch takes the pointer's focus, as Niri and Mango do.
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	C.wlr_seat_touch_notify_down(c.seat, surface, event.time_msec, event.touch_id, sx, sy)
	fmt.Fprintf(os.Stderr, "sysc-compositor: touch down id=%d delivered to the client\n", int32(event.touch_id))
	// Touching a window focuses it, as Niri does.
	if view := c.viewForSurface(surface); view != nil {
		c.focusView(view)
	}
}

// handleTouchMotion moves a finger. The touch stays with the surface it went
// down on, so the coordinates are relative to that surface's origin.
func (c *Compositor) handleTouchMotion(data unsafe.Pointer) {
	event := (*C.struct_wlr_touch_motion_event)(data)
	touch := event.touch
	if c.touchDevices[unsafe.Pointer(&touch.base)] == nil {
		return
	}
	point := c.touchPoints[int32(event.touch_id)]
	if point == nil || point.surface == nil {
		return
	}
	lx, ly := c.touchLayoutPoint(touch, float64(event.x), float64(event.y))
	C.wlr_seat_touch_notify_motion(c.seat, event.time_msec, event.touch_id,
		C.double(lx)-point.xOffset, C.double(ly)-point.yOffset)
}

// handleTouchUp ends a finger.
func (c *Compositor) handleTouchUp(data unsafe.Pointer) {
	event := (*C.struct_wlr_touch_up_event)(data)
	point := c.touchPoints[int32(event.touch_id)]
	if point == nil {
		return
	}
	C.wlr_seat_touch_notify_up(c.seat, event.time_msec, event.touch_id)
	fmt.Fprintf(os.Stderr, "sysc-compositor: touch up id=%d\n", int32(event.touch_id))
	c.dropTouchPoint(int32(event.touch_id))
}

// handleTouchCancel ends every finger, as a lost grab or a device going away
// does.
func (c *Compositor) handleTouchCancel(data unsafe.Pointer) {
	event := (*C.struct_wlr_touch_cancel_event)(data)
	c.endTouchPoints(event.time_msec)
}

// handleTouchFrame ends a frame of touch events.
func (c *Compositor) handleTouchFrame(unsafe.Pointer) {
	C.wlr_seat_touch_notify_frame(c.seat)
}

// touchSurfaceGone forgets the surface a finger went down on, so a later
// motion never reports to a dead surface.
func (c *Compositor) touchSurfaceGone(point *touchPoint) {
	if point == nil {
		return
	}
	point.surface = nil
	if current, ok := c.touchPoints[point.id]; ok && current == point {
		delete(c.touchPoints, point.id)
	}
	if point.gone != nil {
		c.drop(point.gone)
		point.gone = nil
	}
}

// dropTouchPoint forgets a finger.
func (c *Compositor) dropTouchPoint(id int32) {
	point := c.touchPoints[id]
	if point == nil {
		return
	}
	delete(c.touchPoints, id)
	if point.gone != nil {
		c.drop(point.gone)
		point.gone = nil
	}
}

// releaseTouchPoints forgets every finger on the glass.
func (c *Compositor) releaseTouchPoints() {
	for id := range c.touchPoints {
		c.dropTouchPoint(id)
	}
}

// endTouchPoints lifts every finger off the glass. wlroots 0.20's
// wlr_seat_touch_notify_cancel wants a seat client and faults on a nil one,
// and it leaves the points in place anyway, so each finger is ended on its
// own.
func (c *Compositor) endTouchPoints(timeMsec C.uint32_t) {
	for id := range c.touchPoints {
		C.wlr_seat_touch_notify_up(c.seat, timeMsec, C.int32_t(id))
	}
	c.releaseTouchPoints()
}

// setTouchConfig applies input.touch. Turning touch off ends every finger.
func (c *Compositor) setTouchConfig(cfg config.Touch) {
	c.touchConfig = cfg
	if cfg.Off {
		c.endTouchPoints(nowMsec())
	}
	c.refreshCapabilities()
}
