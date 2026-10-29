package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <linux/input-event-codes.h>
#include <wayland-server-core.h>
#include <wayland-server-protocol.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_tablet_pad.h>
#include <wlr/types/wlr_tablet_tool.h>
#include <wlr/types/wlr_tablet_v2.h>

// Forwards an axis event's changed axes other than position, as Niri does
// before the motion.
static void sysc_tablet_tool_axes(struct wlr_tablet_v2_tablet_tool *tool,
		struct wlr_tablet_tool_axis_event *event) {
	uint32_t axes = event->updated_axes;
	if (axes & WLR_TABLET_TOOL_AXIS_PRESSURE) {
		wlr_tablet_v2_tablet_tool_notify_pressure(tool, event->pressure);
	}
	if (axes & WLR_TABLET_TOOL_AXIS_DISTANCE) {
		wlr_tablet_v2_tablet_tool_notify_distance(tool, event->distance);
	}
	if (axes & (WLR_TABLET_TOOL_AXIS_TILT_X | WLR_TABLET_TOOL_AXIS_TILT_Y)) {
		wlr_tablet_v2_tablet_tool_notify_tilt(tool, event->tilt_x, event->tilt_y);
	}
	if (axes & WLR_TABLET_TOOL_AXIS_ROTATION) {
		wlr_tablet_v2_tablet_tool_notify_rotation(tool, event->rotation);
	}
	if (axes & WLR_TABLET_TOOL_AXIS_SLIDER) {
		wlr_tablet_v2_tablet_tool_notify_slider(tool, event->slider);
	}
	if (axes & WLR_TABLET_TOOL_AXIS_WHEEL) {
		wlr_tablet_v2_tablet_tool_notify_wheel(tool, event->wheel_delta, 0);
	}
}

// Reports a pad button's mode, then the button. wlroots indexes the pad's
// groups by the event's group unchecked, so a group the pad lacks is
// dropped rather than read past the end.
static void sysc_tablet_pad_button(struct wlr_tablet_v2_tablet_pad *pad,
		struct wlr_tablet_pad *wlr_pad, struct wlr_tablet_pad_button_event *event) {
	if (event->group < (unsigned int)wl_list_length(&wlr_pad->groups)) {
		wlr_tablet_v2_tablet_pad_notify_mode(pad, event->group, event->mode, event->time_msec);
	}
	wlr_tablet_v2_tablet_pad_notify_button(pad, event->button, event->time_msec,
		(enum zwp_tablet_pad_v2_button_state)event->state);
}

// A tablet-aware client may draw the pen's image only while its tool is
// over that client's surface, as the pointer's focused client may.
static bool sysc_tablet_set_cursor(struct wlr_cursor *cursor,
		struct wlr_tablet_v2_tablet_tool *tool, struct wlr_tablet_v2_event_cursor *event) {
	if (tool->focused_surface == NULL || event->seat_client == NULL ||
			event->seat_client->client != wl_resource_get_client(tool->focused_surface->resource)) {
		return false;
	}
	wlr_cursor_set_surface(cursor, event->surface, event->hotspot_x, event->hotspot_y);
	return true;
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// tablet is one tablet device on the user seat. Injected seats never get
// one: wlroots' tablet-v2 objects are created for c.seat only.
type tablet struct {
	device    *C.struct_wlr_input_device
	wlr       *C.struct_wlr_tablet
	v2        *C.struct_wlr_tablet_v2_tablet
	listeners []*Listener
}

// tabletTool is one physical tool (a pen, eraser, ...). wlroots' v2 tool
// tracks the tablet-aware surface it is over and clears it when that surface
// goes away.
type tabletTool struct {
	wlr    *C.struct_wlr_tablet_tool
	v2     *C.struct_wlr_tablet_v2_tablet_tool
	tablet *tablet
	// x and y are the last device position in [0, 1].
	x, y float64
	// originX and originY place the surface the tip went down on, so the
	// stroke stays with it (the implicit grab) wherever the pen moves.
	originX, originY float64
	// emulating: the tip went down through the pointer click path.
	emulating bool
	listeners []*Listener
}

// tabletPad is one pad (express keys, rings, strips). wlroots' libinput
// backend never says which tablet a pad belongs to, and Sway and Mango ask
// libinput's device groups, so a pad follows the most recently active
// tablet's tool instead: exact with one tablet.
type tabletPad struct {
	device    *C.struct_wlr_input_device
	wlr       *C.struct_wlr_tablet_pad
	v2        *C.struct_wlr_tablet_v2_tablet_pad
	tablet    *tablet
	surface   *C.struct_wlr_surface
	gone      *Listener
	listeners []*Listener
}

func (c *Compositor) createTabletManager() error {
	if c.tabletManager = C.wlr_tablet_v2_create(c.display); c.tabletManager == nil {
		return errors.New("wlr_tablet_v2_create failed")
	}
	return nil
}

// wlroots frees its tablet-v2 objects from the same destroy signals sysc
// cleans up on, and a signal runs its listeners in the order they were
// added. sysc's destroy listeners are therefore added before the v2 object
// is created, so its cleanup still finds that object alive.

func (c *Compositor) addTablet(device *C.struct_wlr_input_device) bool {
	t := &tablet{device: device, wlr: C.wlr_tablet_from_input_device(device)}
	gone := AddListener(unsafe.Pointer(&device.events.destroy), func(unsafe.Pointer) {
		c.removeTablet(unsafe.Pointer(device))
	})
	if t.v2 = C.wlr_tablet_create(c.tabletManager, c.seat, device); t.v2 == nil {
		gone.Destroy()
		return false
	}
	t.listeners = []*Listener{
		gone,
		AddListener(unsafe.Pointer(&t.wlr.events.proximity), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_tablet_tool_proximity_event)(data)
			if tool := c.tabletToolFor(t, event.tool); tool != nil {
				c.tabletProximity(tool, event)
			}
		}),
		AddListener(unsafe.Pointer(&t.wlr.events.axis), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_tablet_tool_axis_event)(data)
			if tool := c.tabletToolFor(t, event.tool); tool != nil {
				c.tabletAxis(tool, event)
			}
		}),
		AddListener(unsafe.Pointer(&t.wlr.events.tip), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_tablet_tool_tip_event)(data)
			if tool := c.tabletToolFor(t, event.tool); tool != nil {
				c.tabletTip(tool, event.time_msec, event.state == C.WLR_TABLET_TOOL_TIP_DOWN)
			}
		}),
		AddListener(unsafe.Pointer(&t.wlr.events.button), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_tablet_tool_button_event)(data)
			if tool := c.tabletToolFor(t, event.tool); tool != nil && tool.v2.focused_surface != nil {
				C.wlr_tablet_v2_tablet_tool_notify_button(tool.v2, event.button,
					C.enum_zwp_tablet_pad_v2_button_state(event.state))
			}
		}),
	}
	c.keep(t.listeners...)
	c.tablets[unsafe.Pointer(device)] = t
	return true
}

func (c *Compositor) removeTablet(device unsafe.Pointer) {
	t := c.tablets[device]
	if t == nil {
		return
	}
	for _, tool := range c.tabletTools {
		if tool.tablet == t {
			c.tabletRelease(tool, nowMsec())
			tool.tablet = nil
		}
	}
	for _, pad := range c.tabletPads {
		if pad.tablet == t {
			c.padFocus(pad, nil, nil)
		}
	}
	if c.activeTablet == t {
		c.activeTablet = nil
	}
	c.releasePointerButtons(device)
	for _, l := range t.listeners {
		c.drop(l)
	}
	delete(c.tablets, device)
}

// tabletToolFor is the tool's state, created the first time the tool comes
// near any tablet; nil while the tablet config is off.
func (c *Compositor) tabletToolFor(t *tablet, wlrTool *C.struct_wlr_tablet_tool) *tabletTool {
	if c.tabletConfig.Off {
		return nil
	}
	tool := c.tabletTools[wlrTool]
	if tool == nil {
		tool = &tabletTool{wlr: wlrTool}
		gone := AddListener(unsafe.Pointer(&wlrTool.events.destroy), func(unsafe.Pointer) {
			c.removeTabletTool(tool)
		})
		if tool.v2 = C.wlr_tablet_tool_create(c.tabletManager, c.seat, wlrTool); tool.v2 == nil {
			gone.Destroy()
			return nil
		}
		tool.listeners = []*Listener{
			gone,
			AddListener(unsafe.Pointer(&tool.v2.events.set_cursor), func(data unsafe.Pointer) {
				C.sysc_tablet_set_cursor(c.cursor, tool.v2, (*C.struct_wlr_tablet_v2_event_cursor)(data))
			}),
		}
		c.keep(tool.listeners...)
		c.tabletTools[wlrTool] = tool
	}
	tool.tablet = t
	if c.activeTablet != t {
		c.activeTablet = t
		for _, pad := range c.tabletPads {
			c.padFocus(pad, nil, nil)
		}
	}
	return tool
}

func (c *Compositor) removeTabletTool(tool *tabletTool) {
	c.tabletRelease(tool, nowMsec())
	for _, l := range tool.listeners {
		c.drop(l)
	}
	delete(c.tabletTools, tool.wlr)
}

func (c *Compositor) tabletProximity(tool *tabletTool, event *C.struct_wlr_tablet_tool_proximity_event) {
	tool.x, tool.y = float64(event.x), float64(event.y)
	if event.state == C.WLR_TABLET_TOOL_PROXIMITY_IN {
		c.tabletMove(tool, event.time_msec)
		return
	}
	c.tabletRelease(tool, event.time_msec)
	// Niri moves the pointer to the pen, so pointer focus resumes where the
	// pen left rather than where the mouse last was.
	c.tabletPointerMotion(event.time_msec)
}

// tabletPointerMotion is pointer motion to the cursor the pen moved. A pen
// has no pointer frame event of its own, so the frame is sent here.
func (c *Compositor) tabletPointerMotion(timeMsec C.uint32_t) {
	c.processCursorMotion(timeMsec)
	C.wlr_seat_pointer_notify_frame(c.seat)
}

func (c *Compositor) tabletAxis(tool *tabletTool, event *C.struct_wlr_tablet_tool_axis_event) {
	if event.updated_axes&C.WLR_TABLET_TOOL_AXIS_X != 0 {
		tool.x = float64(event.x)
	}
	if event.updated_axes&C.WLR_TABLET_TOOL_AXIS_Y != 0 {
		tool.y = float64(event.y)
	}
	c.tabletMove(tool, event.time_msec)
	if tool.v2.focused_surface != nil {
		C.sysc_tablet_tool_axes(tool.v2, event)
	}
}

// tabletMove takes the pen to its mapped point. A tablet-aware surface with
// no compositor UI over it gets the tool; anything else gets pointer motion.
func (c *Compositor) tabletMove(tool *tabletTool, timeMsec C.uint32_t) {
	c.userActivity()
	x, y := c.tabletLayoutPoint(tool)
	C.wlr_cursor_warp_closest(c.cursor, nil, C.double(x), C.double(y))
	c.damageCapturePointers()
	if tool.v2.is_down {
		C.wlr_tablet_v2_tablet_tool_notify_motion(tool.v2, C.double(x-tool.originX), C.double(y-tool.originY))
		return
	}
	if c.shotUIMotion() || c.switcherMotion() {
		c.tabletFocus(tool, nil)
		return
	}
	surface, sx, sy := c.surfaceAt(x, y)
	if c.tabletModalUI() || surface == nil || !bool(C.wlr_surface_accepts_tablet_v2(surface, tool.tablet.v2)) {
		c.tabletFocus(tool, nil)
		c.tabletPointerMotion(timeMsec)
		return
	}
	c.tabletFocus(tool, surface)
	C.wlr_tablet_v2_tablet_tool_notify_motion(tool.v2, sx, sy)
}

// tabletFocus moves the tool, and the pads that follow it, to surface.
func (c *Compositor) tabletFocus(tool *tabletTool, surface *C.struct_wlr_surface) {
	if tool.v2.focused_surface != surface {
		if tool.v2.focused_surface != nil {
			C.wlr_tablet_v2_tablet_tool_notify_proximity_out(tool.v2)
		}
		if surface != nil {
			C.wlr_tablet_v2_tablet_tool_notify_proximity_in(tool.v2, tool.tablet.v2, surface)
			c.showDefaultImage()
		}
	}
	if tool.tablet == c.activeTablet {
		for _, pad := range c.tabletPads {
			c.padFocus(pad, tool.tablet, surface)
		}
	}
}

// tabletRelease ends whatever the tool was doing: a stroke, an emulated
// click, its proximity.
func (c *Compositor) tabletRelease(tool *tabletTool, timeMsec C.uint32_t) {
	if tool.emulating && tool.tablet != nil {
		tool.emulating = false
		c.deliverPointerButton(unsafe.Pointer(tool.tablet.device), timeMsec, C.BTN_LEFT,
			C.WL_POINTER_BUTTON_STATE_RELEASED)
	}
	if tool.v2.is_down {
		C.wlr_tablet_v2_tablet_tool_notify_up(tool.v2)
	}
	if tool.tablet != nil {
		c.tabletFocus(tool, nil)
	} else if tool.v2.focused_surface != nil {
		C.wlr_tablet_v2_tablet_tool_notify_proximity_out(tool.v2)
	}
}

// tabletTip presses or lifts the pen. On a tablet-aware surface the tip
// focuses it, as Niri's does, and the stroke stays with it; elsewhere, and
// over any compositor UI, the tip is a left click. A tip lifts the way it
// went down.
func (c *Compositor) tabletTip(tool *tabletTool, timeMsec C.uint32_t, down bool) {
	source := unsafe.Pointer(tool.tablet.device)
	if !down {
		if tool.emulating {
			tool.emulating = false
			c.deliverPointerButton(source, timeMsec, C.BTN_LEFT, C.WL_POINTER_BUTTON_STATE_RELEASED)
		} else if tool.v2.is_down {
			C.wlr_tablet_v2_tablet_tool_notify_up(tool.v2)
			c.tabletMove(tool, timeMsec)
		}
		return
	}
	if tool.v2.focused_surface == nil || c.tabletModalUI() {
		tool.emulating = true
		c.deliverPointerButton(source, timeMsec, C.BTN_LEFT, C.WL_POINTER_BUTTON_STATE_PRESSED)
		return
	}
	c.focusSurfaceAtCursor()
	x, y := c.tabletLayoutPoint(tool)
	if surface, sx, sy := c.surfaceAt(x, y); surface == tool.v2.focused_surface {
		tool.originX, tool.originY = x-float64(sx), y-float64(sy)
	}
	C.wlr_tablet_v2_tablet_tool_notify_down(tool.v2)
	C.wlr_tablet_tool_v2_start_implicit_grab(tool.v2)
}

// tabletModalUI reports a compositor UI that takes the pen as a pointer.
func (c *Compositor) tabletModalUI() bool {
	return c.pickColor != nil || c.shotUI != nil || c.switcher.isOpen() || c.exitDialog.open ||
		(c.model != nil && (c.model.OverviewProgress() > 0 || c.model.OverviewOpen()))
}

// tabletLayoutPoint maps the tool's device position as Niri does: onto the
// focused or the named output, or over every output.
func (c *Compositor) tabletLayoutPoint(tool *tabletTool) (float64, float64) {
	aspect := 0.0
	if t := tool.tablet; t != nil && t.wlr.width_mm > 0 && t.wlr.height_mm > 0 {
		aspect = float64(t.wlr.width_mm) / float64(t.wlr.height_mm)
	}
	if output := c.tabletOutput(); output != nil {
		var box C.struct_wlr_box
		C.wlr_output_layout_get_box(c.layout, output, &box)
		return tabletPoint(tool.x, tool.y, rect{int(box.x), int(box.y), int(box.width), int(box.height)},
			int(output.transform), true, aspect, 1/float64(output.scale))
	}
	px := 1.0
	for pointer := range c.sceneOutputs {
		px = 1 / float64((*C.struct_wlr_output)(pointer).scale)
		break
	}
	return tabletPoint(tool.x, tool.y, c.layoutBox(), 0, false, aspect, px)
}

// tabletOutput is Niri's output_for_tablet.
func (c *Compositor) tabletOutput() *C.struct_wlr_output {
	if c.tabletConfig.MapToFocusedOutput {
		return (*C.struct_wlr_output)(c.outputByName(c.FocusedOutputName()))
	}
	if c.tabletConfig.MapToOutput == "" {
		return nil
	}
	for pointer := range c.sceneOutputs {
		output := (*C.struct_wlr_output)(pointer)
		if c.outputIsEnabled(pointer) && outputNameMatches(c.tabletConfig.MapToOutput,
			C.GoString(output.name), cString(output.make), cString(output.model), cString(output.serial)) {
			return output
		}
	}
	return nil
}

// outputNameMatches is Niri's output name match: the connector, or "make
// model serial" with Unknown for a missing part, ignoring ASCII case.
func outputNameMatches(target, connector, make, model, serial string) bool {
	if asciiEqualFold(target, connector) {
		return true
	}
	if make == "" && model == "" && serial == "" {
		return false
	}
	unknown := func(s string) string {
		if s == "" {
			return "Unknown"
		}
		return s
	}
	return asciiEqualFold(target, unknown(make)+" "+unknown(model)+" "+unknown(serial))
}

func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	lower := func(c byte) byte {
		if 'A' <= c && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	for i := 0; i < len(a); i++ {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func (c *Compositor) addTabletPad(device *C.struct_wlr_input_device) bool {
	pad := &tabletPad{device: device, wlr: C.wlr_tablet_pad_from_input_device(device)}
	gone := AddListener(unsafe.Pointer(&device.events.destroy), func(unsafe.Pointer) {
		c.removeTabletPad(unsafe.Pointer(device))
	})
	if pad.v2 = C.wlr_tablet_pad_create(c.tabletManager, c.seat, device); pad.v2 == nil {
		gone.Destroy()
		return false
	}
	pad.listeners = []*Listener{
		gone,
		AddListener(unsafe.Pointer(&pad.wlr.events.button), func(data unsafe.Pointer) {
			if pad.surface != nil {
				C.sysc_tablet_pad_button(pad.v2, pad.wlr, (*C.struct_wlr_tablet_pad_button_event)(data))
			}
		}),
		AddListener(unsafe.Pointer(&pad.wlr.events.ring), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_tablet_pad_ring_event)(data)
			if pad.surface != nil {
				C.wlr_tablet_v2_tablet_pad_notify_ring(pad.v2, event.ring, event.position,
					event.source == C.WLR_TABLET_PAD_RING_SOURCE_FINGER, event.time_msec)
			}
		}),
		AddListener(unsafe.Pointer(&pad.wlr.events.strip), func(data unsafe.Pointer) {
			event := (*C.struct_wlr_tablet_pad_strip_event)(data)
			if pad.surface != nil {
				C.wlr_tablet_v2_tablet_pad_notify_strip(pad.v2, event.strip, event.position,
					event.source == C.WLR_TABLET_PAD_STRIP_SOURCE_FINGER, event.time_msec)
			}
		}),
	}
	c.keep(pad.listeners...)
	c.tabletPads[unsafe.Pointer(device)] = pad
	// A pad that arrives while a tool is over a tablet-aware surface joins it.
	for _, tool := range c.tabletTools {
		if tool.tablet != nil && tool.tablet == c.activeTablet && tool.v2.focused_surface != nil {
			c.padFocus(pad, tool.tablet, tool.v2.focused_surface)
		}
	}
	return true
}

func (c *Compositor) removeTabletPad(device unsafe.Pointer) {
	pad := c.tabletPads[device]
	if pad == nil {
		return
	}
	c.padFocus(pad, nil, nil)
	for _, l := range pad.listeners {
		c.drop(l)
	}
	delete(c.tabletPads, device)
}

// padFocus enters the pad on surface for tablet t, leaving any surface it
// was on. A destroyed surface takes the pad's focus with it.
func (c *Compositor) padFocus(pad *tabletPad, t *tablet, surface *C.struct_wlr_surface) {
	if pad.surface == surface && pad.tablet == t {
		return
	}
	if pad.surface != nil {
		C.wlr_tablet_v2_tablet_pad_notify_leave(pad.v2, pad.surface)
		c.drop(pad.gone)
		pad.gone, pad.surface = nil, nil
	}
	pad.tablet = t
	if surface == nil || t == nil {
		return
	}
	C.wlr_tablet_v2_tablet_pad_notify_enter(pad.v2, t.v2, surface)
	pad.surface = surface
	pad.gone = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
		c.drop(pad.gone)
		pad.gone, pad.surface = nil, nil
	})
	c.keep(pad.gone)
}

// setTabletConfig applies input.tablet. Turning tablets off ends every
// tool's stroke and proximity, as libinput's disabled send-events mode does.
func (c *Compositor) setTabletConfig(cfg config.Tablet) {
	c.tabletConfig = cfg
	if !cfg.Off {
		return
	}
	for _, tool := range c.tabletTools {
		c.tabletRelease(tool, nowMsec())
	}
}
