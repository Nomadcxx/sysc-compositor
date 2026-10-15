package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/backend.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/types/wlr_xcursor_manager.h>
#include <wlr/util/edges.h>
#include <linux/input-event-codes.h>

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

static struct wlr_surface *sysc_scene_surface_at(struct wlr_scene *scene,
		double x, double y, double *sx, double *sy) {
	struct wlr_scene_node *node = wlr_scene_node_at(&scene->tree.node, x, y, sx, sy);
	if (node == NULL || node->type != WLR_SCENE_NODE_BUFFER) {
		return NULL;
	}
	struct wlr_scene_surface *scene_surface =
		wlr_scene_surface_try_from_buffer(wlr_scene_buffer_from_node(node));
	return scene_surface == NULL ? NULL : scene_surface->surface;
}

static void sysc_scene_tree_position(struct wlr_scene_tree *tree, int *x, int *y) {
	wlr_scene_node_coords(&tree->node, x, y);
}

static void sysc_scene_tree_set_position(struct wlr_scene_tree *tree, int x, int y) {
	wlr_scene_node_set_position(&tree->node, x, y);
}

// Only the user seat's focused client may change the drawn image (rule
// D2.4.2); injected seats have no listener on this signal at all.
static void sysc_set_cursor_from_seat(struct wlr_cursor *cursor, struct wlr_seat *seat,
		struct wlr_seat_pointer_request_set_cursor_event *event) {
	if (event->seat_client != seat->pointer_state.focused_client) {
		return;
	}
	wlr_cursor_set_surface(cursor, event->surface, event->hotspot_x, event->hotspot_y);
}

static bool sysc_seat_has_set_cursor_listener(struct wlr_seat *seat) {
	return !wl_list_empty(&seat->events.request_set_cursor.listener_list);
}

static void sysc_seat_keyboard_focus(struct wlr_seat *seat,
		struct wlr_surface *surface, struct wlr_keyboard *keyboard) {
	wlr_seat_keyboard_notify_enter(seat, surface, keyboard->keycodes,
			keyboard->num_keycodes, &keyboard->modifiers);
}
*/
import "C"

import (
	"math"
	"unsafe"
)

type pointerGrabMode uint8

const (
	grabNone pointerGrabMode = iota
	grabMove
	grabResize
)

const (
	edgeTop uint32 = 1 << iota
	edgeBottom
	edgeLeft
	edgeRight
)

type pointerGrab struct {
	mode             pointerGrabMode
	cursorX, cursorY float64
	x, y             int
	width, height    int
	edges            uint32
}

type toplevelView struct {
	toplevel  *C.struct_wlr_xdg_toplevel
	tree      *C.struct_wlr_scene_tree
	listeners []*Listener
	announced bool
	id        uint64
	touched   uint64
}

type pointerGeometry struct {
	x, y          int
	width, height int
}

func (g pointerGrab) geometry(cursorX, cursorY float64) pointerGeometry {
	geometry := pointerGeometry{x: g.x, y: g.y, width: g.width, height: g.height}
	dx := int(math.Round(cursorX - g.cursorX))
	dy := int(math.Round(cursorY - g.cursorY))
	if g.mode == grabMove {
		geometry.x += dx
		geometry.y += dy
		return geometry
	}
	if g.mode != grabResize {
		return geometry
	}
	if g.edges&edgeLeft != 0 {
		geometry.width -= dx
	} else if g.edges&edgeRight != 0 {
		geometry.width += dx
	}
	if g.edges&edgeTop != 0 {
		geometry.height -= dy
	} else if g.edges&edgeBottom != 0 {
		geometry.height += dy
	}
	if geometry.width < 1 {
		geometry.width = 1
	}
	if geometry.height < 1 {
		geometry.height = 1
	}
	if g.edges&edgeLeft != 0 {
		geometry.x = g.x + g.width - geometry.width
	}
	if g.edges&edgeTop != 0 {
		geometry.y = g.y + g.height - geometry.height
	}
	return geometry
}

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
	deviceKey := unsafe.Pointer(device)
	switch C.sysc_device_type(device) {
	case C.WLR_INPUT_DEVICE_KEYBOARD:
		kb := C.wlr_keyboard_from_input_device(device)
		if kb == nil {
			return
		}
		C.wlr_seat_set_keyboard(c.seat, kb)
		c.keyboardDevices[deviceKey] = kb
		c.activeKeyboard = kb
		c.modifiers = uint32(C.wlr_keyboard_get_modifiers(kb))
		c.addKeyboardListeners(deviceKey, kb)
		if c.focusedView != nil {
			C.sysc_seat_keyboard_focus(c.seat,
				c.focusedView.toplevel.base.surface, kb)
		}
	case C.WLR_INPUT_DEVICE_POINTER:
		if c.xcursorManager == nil {
			c.xcursorManager = C.sysc_xcursor_manager()
			if c.xcursorManager != nil {
				C.sysc_load_cursor_image(c.cursor, c.xcursorManager)
			}
		}
		C.wlr_cursor_attach_input_device(c.cursor, device)
		c.pointerDevices[deviceKey] = true
	default:
		return
	}
	destroy := AddListener(unsafe.Pointer(&device.events.destroy), func(unsafe.Pointer) {
		c.handleInputDestroy(deviceKey)
	})
	c.inputDestroyListeners[deviceKey] = destroy
	c.keep(destroy)
	c.refreshCapabilities()
}

func (c *Compositor) addKeyboardListeners(device unsafe.Pointer, keyboard *C.struct_wlr_keyboard) {
	listeners := []*Listener{
		AddListener(unsafe.Pointer(&keyboard.events.key), func(data unsafe.Pointer) {
			c.handleKeyboardKey(keyboard, data)
		}),
		AddListener(unsafe.Pointer(&keyboard.events.modifiers), func(unsafe.Pointer) {
			c.handleKeyboardModifiers(keyboard)
		}),
	}
	c.keyboardListeners[device] = listeners
	c.keep(listeners...)
}

func (c *Compositor) handleInputDestroy(device unsafe.Pointer) {
	if listener := c.inputDestroyListeners[device]; listener != nil {
		c.drop(listener)
		delete(c.inputDestroyListeners, device)
	}
	if c.pointerDevices[device] {
		C.wlr_cursor_detach_input_device(c.cursor, (*C.struct_wlr_input_device)(device))
		delete(c.pointerDevices, device)
		c.clearPointerGrab()
		clear(c.pressedPointerButtons)
		clear(c.pointerSerialButtons)
		clear(c.consumedPointerButtons)
	}
	if keyboard := c.keyboardDevices[device]; keyboard != nil {
		for _, listener := range c.keyboardListeners[device] {
			c.drop(listener)
		}
		delete(c.keyboardListeners, device)
		delete(c.keyboardDevices, device)
		if c.activeKeyboard == keyboard {
			c.activeKeyboard = nil
			for _, remaining := range c.keyboardDevices {
				c.activeKeyboard = remaining
				break
			}
			C.wlr_seat_set_keyboard(c.seat, c.activeKeyboard)
			c.modifiers = 0
			if c.activeKeyboard != nil {
				c.modifiers = uint32(C.wlr_keyboard_get_modifiers(c.activeKeyboard))
				if c.focusedView != nil {
					C.sysc_seat_keyboard_focus(c.seat,
						c.focusedView.toplevel.base.surface, c.activeKeyboard)
				}
			} else {
				C.wlr_seat_keyboard_notify_clear_focus(c.seat)
			}
		}
	}
	c.refreshCapabilities()
}

func (c *Compositor) handleKeyboardKey(keyboard *C.struct_wlr_keyboard, data unsafe.Pointer) {
	if keyboard != c.activeKeyboard {
		return
	}
	event := (*C.struct_wlr_keyboard_key_event)(data)
	if event.state == C.WL_KEYBOARD_KEY_STATE_PRESSED && event.keycode == C.KEY_ESC {
		C.wl_display_terminate(c.display)
		return
	}
	C.wlr_seat_keyboard_notify_key(c.seat, event.time_msec, event.keycode,
		C.uint32_t(event.state))
}

func (c *Compositor) handleKeyboardModifiers(keyboard *C.struct_wlr_keyboard) {
	if keyboard != c.activeKeyboard {
		return
	}
	c.modifiers = uint32(C.wlr_keyboard_get_modifiers(keyboard))
	C.wlr_seat_keyboard_notify_modifiers(c.seat, &keyboard.modifiers)
}

func (c *Compositor) handleCursorMotion(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_motion_event)(data)
	device := (*C.struct_wlr_input_device)(unsafe.Pointer(event.pointer))
	C.wlr_cursor_move(c.cursor, device, event.delta_x, event.delta_y)
	c.processCursorMotion(event.time_msec)
}

func (c *Compositor) handleCursorMotionAbsolute(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_motion_absolute_event)(data)
	device := (*C.struct_wlr_input_device)(unsafe.Pointer(event.pointer))
	C.wlr_cursor_warp_absolute(c.cursor, device, event.x, event.y)
	c.processCursorMotion(event.time_msec)
}

func (c *Compositor) processCursorMotion(time C.uint32_t) {
	if c.pointerGrab != nil && c.grabView != nil && c.grabView.tree != nil {
		geometry := c.pointerGrab.geometry(float64(c.cursor.x), float64(c.cursor.y))
		C.sysc_scene_tree_set_position(c.grabView.tree,
			C.int(geometry.x), C.int(geometry.y))
		if c.pointerGrab.mode == grabResize {
			C.wlr_xdg_toplevel_set_size(c.grabView.toplevel,
				C.int(geometry.width), C.int(geometry.height))
		}
		return
	}
	var sx, sy C.double
	surface := C.sysc_scene_surface_at(c.scene, c.cursor.x, c.cursor.y, &sx, &sy)
	if surface == nil {
		C.wlr_seat_pointer_notify_clear_focus(c.seat)
		return
	}
	if !bool(C.wlr_seat_pointer_surface_has_focus(c.seat, surface)) {
		C.wlr_seat_pointer_notify_enter(c.seat, surface, sx, sy)
	}
	C.wlr_seat_pointer_notify_motion(c.seat, time, sx, sy)
}

func (c *Compositor) handleCursorButton(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_button_event)(data)
	button := uint32(event.button)
	pressed := event.state == C.WL_POINTER_BUTTON_STATE_PRESSED

	if pressed {
		c.pressedPointerButtons[button] = true
	} else {
		delete(c.pressedPointerButtons, button)
		for serial, pressedButton := range c.pointerSerialButtons {
			if pressedButton == button {
				delete(c.pointerSerialButtons, serial)
			}
		}
	}

	if !pressed {
		if c.consumedPointerButtons[button] {
			delete(c.consumedPointerButtons, button)
			if c.grabButton == button {
				c.clearPointerGrab()
			}
			C.wlr_seat_pointer_notify_frame(c.seat)
			return
		}
		if c.pointerGrab != nil && (c.grabButton == 0 || c.grabButton == button) {
			c.clearPointerGrab()
		}
	}

	if pressed && c.modifiers&uint32(C.WLR_MODIFIER_LOGO) != 0 {
		if view := c.viewAtCursor(); view != nil {
			mode := grabNone
			var edges uint32
			switch button {
			case uint32(C.BTN_LEFT):
				mode = grabMove
			case uint32(C.BTN_RIGHT):
				mode = grabResize
				edges = uint32(C.WLR_EDGE_RIGHT) | uint32(C.WLR_EDGE_BOTTOM)
			}
			if mode != grabNone && c.startPointerGrab(view, mode, edges, button) {
				c.focusView(view)
				c.consumedPointerButtons[button] = true
				return
			}
		}
	}

	if pressed {
		if view := c.viewAtCursor(); view != nil {
			c.focusView(view)
		}
	}
	serial := C.wlr_seat_pointer_notify_button(c.seat,
		event.time_msec, event.button, event.state)
	if pressed && serial != 0 {
		c.pointerSerialButtons[uint32(serial)] = button
	}
	C.wlr_seat_pointer_notify_frame(c.seat)
}

func (c *Compositor) handleCursorAxis(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_axis_event)(data)
	C.wlr_seat_pointer_notify_axis(c.seat, event.time_msec,
		event.orientation, event.delta, event.delta_discrete,
		event.source, event.relative_direction)
}

func (c *Compositor) handleCursorFrame(unsafe.Pointer) {
	C.wlr_seat_pointer_notify_frame(c.seat)
}

// surfaceAt is the client surface at layout coordinates, with the
// surface-local position, or nil over empty space.
func (c *Compositor) surfaceAt(x, y float64) (*C.struct_wlr_surface, C.double, C.double) {
	var sx, sy C.double
	surface := C.sysc_scene_surface_at(c.scene, C.double(x), C.double(y), &sx, &sy)
	return surface, sx, sy
}

func (c *Compositor) viewAtCursor() *toplevelView {
	var sx, sy C.double
	surface := C.sysc_scene_surface_at(c.scene, c.cursor.x, c.cursor.y, &sx, &sy)
	if surface == nil {
		return nil
	}
	toplevel := C.wlr_xdg_toplevel_try_from_wlr_surface(surface)
	if toplevel == nil {
		return nil
	}
	return c.views[unsafe.Pointer(toplevel)]
}

func (c *Compositor) focusView(view *toplevelView) {
	if view == nil || view.toplevel == nil {
		return
	}
	changed := c.focusedView != view
	if changed {
		if c.focusedView != nil && c.focusedView.toplevel != nil {
			C.wlr_xdg_toplevel_set_activated(c.focusedView.toplevel, false)
		}
		c.focusedView = view
		C.wlr_xdg_toplevel_set_activated(view.toplevel, true)
	}
	c.touch(view)
	if changed && c.activeKeyboard != nil {
		C.sysc_seat_keyboard_focus(c.seat,
			view.toplevel.base.surface, c.activeKeyboard)
	}
}

func (c *Compositor) startPointerGrab(view *toplevelView, mode pointerGrabMode,
	edges uint32, button uint32) bool {
	if view == nil || view.toplevel == nil || view.tree == nil {
		return false
	}
	var x, y C.int
	C.sysc_scene_tree_position(view.tree, &x, &y)
	width := int(view.toplevel.base.geometry.width)
	height := int(view.toplevel.base.geometry.height)
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	c.pointerGrab = &pointerGrab{
		mode: mode, cursorX: float64(c.cursor.x), cursorY: float64(c.cursor.y),
		x: int(x), y: int(y), width: width, height: height, edges: edges,
	}
	c.grabView = view
	c.grabButton = button
	return true
}

func (c *Compositor) clearPointerGrab() {
	c.pointerGrab = nil
	c.grabView = nil
	c.grabButton = 0
}

func (c *Compositor) handleToplevelMove(view *toplevelView, data unsafe.Pointer) {
	event := (*C.struct_wlr_xdg_toplevel_move_event)(data)
	button, pressed := c.pointerSerialButtons[uint32(event.serial)]
	if event.toplevel != view.toplevel || !bool(C.wlr_seat_validate_pointer_grab_serial(
		c.seat, view.toplevel.base.surface, event.serial)) || !pressed ||
		!c.pressedPointerButtons[button] {
		return
	}
	c.startPointerGrab(view, grabMove, 0, button)
}

func (c *Compositor) handleToplevelResize(view *toplevelView, data unsafe.Pointer) {
	event := (*C.struct_wlr_xdg_toplevel_resize_event)(data)
	button, pressed := c.pointerSerialButtons[uint32(event.serial)]
	if event.toplevel != view.toplevel || !bool(C.wlr_seat_validate_pointer_grab_serial(
		c.seat, view.toplevel.base.surface, event.serial)) || !pressed ||
		!c.pressedPointerButtons[button] {
		return
	}
	c.startPointerGrab(view, grabResize, uint32(event.edges), button)
}

func (c *Compositor) handleNewToplevel(data unsafe.Pointer) {
	toplevel := (*C.struct_wlr_xdg_toplevel)(data)
	tree := c.ensureSceneTree(toplevel.base)
	if tree == nil {
		return
	}
	c.nextViewID++
	view := &toplevelView{toplevel: toplevel, tree: tree, id: c.nextViewID}
	c.views[unsafe.Pointer(toplevel)] = view
	add := func(signal Signal, fn func(unsafe.Pointer)) {
		listener := AddListener(signal, fn)
		view.listeners = append(view.listeners, listener)
		c.keep(listener)
	}
	var initialCommit *Listener
	initialCommit = AddListener(unsafe.Pointer(&toplevel.base.surface.events.commit), func(unsafe.Pointer) {
		if !toplevel.base.initialized || !toplevel.base.initial_commit {
			return
		}
		c.drop(initialCommit)
		C.wlr_xdg_toplevel_set_size(toplevel, 0, 0)
	})
	view.listeners = append(view.listeners, initialCommit)
	c.keep(initialCommit)
	add(unsafe.Pointer(&toplevel.events.request_move), func(data unsafe.Pointer) {
		c.handleToplevelMove(view, data)
	})
	add(unsafe.Pointer(&toplevel.events.request_resize), func(data unsafe.Pointer) {
		c.handleToplevelResize(view, data)
	})
	add(unsafe.Pointer(&toplevel.events.destroy), func(unsafe.Pointer) {
		c.handleToplevelDestroy(view)
	})
	// The map signal fires inside the commit, before the scene attaches the
	// buffer, so a view is announced on its first commit after mapping: the
	// moment it can be hit-tested. The scene's own commit listener was added
	// first and runs first.
	add(unsafe.Pointer(&toplevel.base.surface.events.commit), func(unsafe.Pointer) {
		if view.announced || !bool(toplevel.base.surface.mapped) {
			return
		}
		view.announced = true
		if c.viewMapped != nil {
			c.viewMapped(view)
		}
	})
}

// handleRequestSetCursor lets the user seat's focused client set the drawn
// image. It is registered on the user seat only.
func (c *Compositor) handleRequestSetCursor(data unsafe.Pointer) {
	C.sysc_set_cursor_from_seat(c.cursor, c.seat,
		(*C.struct_wlr_seat_pointer_request_set_cursor_event)(data))
}

// setCursorListeners reports, for the user seat and an injected seat, whether
// anything listens for a client's request to change the drawn image.
func (c *Compositor) setCursorListeners(injected string) (user, other bool) {
	user = bool(C.sysc_seat_has_set_cursor_listener(c.seat))
	if s := c.injectedSeat(injected); s != nil {
		other = bool(C.sysc_seat_has_set_cursor_listener(s.seat))
	}
	return user, other
}

// userFocus is the user seat's keyboard focus and focused view, for tests.
func (c *Compositor) userFocus() (unsafe.Pointer, *toplevelView) {
	return unsafe.Pointer(c.seat.keyboard_state.focused_surface), c.focusedView
}

// injectedFocus is an injected seat's keyboard focus, for tests.
func (c *Compositor) injectedFocus(name string) unsafe.Pointer {
	if s := c.injectedSeat(name); s != nil {
		return unsafe.Pointer(s.seat.keyboard_state.focused_surface)
	}
	return nil
}

// treePosition is a view's scene-tree origin in layout coordinates.
func (v *toplevelView) treePosition() (int, int) {
	var x, y C.int
	C.sysc_scene_tree_position(v.tree, &x, &y)
	return int(x), int(y)
}

// viewCenter is a mapped view's centre in layout coordinates.
func (c *Compositor) viewCenter(view *toplevelView) (float64, float64) {
	var x, y C.int
	C.sysc_scene_tree_position(view.tree, &x, &y)
	g := view.toplevel.base.geometry
	return float64(x) + float64(g.x) + float64(g.width)/2,
		float64(y) + float64(g.y) + float64(g.height)/2
}

func (c *Compositor) handleToplevelDestroy(view *toplevelView) {
	if view == nil {
		return
	}
	if c.grabView == view {
		c.clearPointerGrab()
	}
	if c.focusedView == view {
		c.focusedView = nil
		C.wlr_seat_keyboard_notify_clear_focus(c.seat)
		C.wlr_seat_pointer_notify_clear_focus(c.seat)
	}
	delete(c.views, unsafe.Pointer(view.toplevel))
	for _, listener := range view.listeners {
		c.drop(listener)
	}
	view.listeners = nil
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
